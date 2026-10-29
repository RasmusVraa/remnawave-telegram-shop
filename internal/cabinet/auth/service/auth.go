// Package service — бизнес-логика аутентификации кабинета.
//
// Сервис отдаёт «тонкие» DTO для HTTP-хендлеров и инкапсулирует всё, что связано
// с хешированием, токенами, почтой и anti-enumeration защитой.
//
// Важные инварианты:
//
//   - Ответы на register/login/forgot одинаковы по форме и близки по таймингу;
//     детали ошибок не утекают (мы никогда не говорим «email не существует»).
//   - При логине всегда выполняется аргон-2 операция (реальная или dummy),
//     чтобы таймингом нельзя было отличить существующий email от нет.
//   - Refresh-токены имеют ротацию и reuse-detection: использование старого
//     refresh отзывает всю family сессий пользователя.
//   - При ResetPassword все refresh-сессии аккаунта инвалидируются.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/cabinet/auth/csrf"
	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/auth/password"
	"remnawave-tg-shop-bot/internal/cabinet/auth/tokens"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/mail"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
)

// Config — параметры, которые сервису нужны во всех методах.
type Config struct {
	PublicURL         string        // базовый URL кабинета (из CABINET_PUBLIC_URL)
	CookieDomain      string        // refresh + csrf cookie domain
	RefreshCookiePath string        // обычно "/cabinet/api/auth"
	AccessTTL         time.Duration // из CABINET_ACCESS_TTL_MINUTES
	RefreshTTL        time.Duration // из CABINET_REFRESH_TTL_DAYS
	EmailVerifyTTL    time.Duration // 24h по ТЗ
	PasswordResetTTL  time.Duration // 30m по ТЗ
	DefaultLanguage   string        // "ru" / "en", для новых аккаунтов без указания языка
	AntiEnumLatency   time.Duration // целевая длительность login/register/forgot для равномерного тайминга
	PasswordParams    password.Params
	PasswordPolicy    password.Policy
	// EmailCodeKey — ключ HMAC для кодов подтверждения email (tokens.EmailVerifyOTPKey).
	EmailCodeKey []byte
}

// Service — фасад аутентификации.
type Service struct {
	cfg       Config
	accounts  *repository.AccountRepo
	ids       *repository.IdentityRepo
	sess      *repository.SessionRepo
	evs       *repository.EmailVerificationRepo
	prs       *repository.PasswordResetRepo
	jwt       *jwt.Issuer
	mailer    *mail.Mailer
	bootstrap *bootstrap.CustomerBootstrap
	emailMergeCodes *repository.EmailMergeVerificationRepo

	// Опциональные поля OAuth/Telegram. Устанавливаются через SetGoogle /
	// SetTelegramToken после вызова New(); nil означает «провайдер отключён».
	// Поля приватные — доступ только внутри пакета service (oauth.go).
	googleProvider *oauthGoogleProviderWrapper // объявлен в oauth.go
	yandexProvider *oauthYandexProviderWrapper
	vkProvider     *oauthVKProviderWrapper
	telegramOIDC   *oauthTelegramOIDCWrapper
	telegramToken  string
	telegramTokens []string
	linkStore      *pendingLinkStore

	// lookupCustomers / lookupLinks — для Telegram Login после link/merge:
	// customer уже с реальным telegram_id, но cabinet_identity(telegram) могла не создаваться.
	lookupCustomers *database.CustomerRepository
	lookupLinks     *repository.AccountCustomerLinkRepo

	// saveMergeEmailPeerClaim — опционально: сохранить claim для /link/merge после проверки пароля «чужого» email-аккаунта.
	saveMergeEmailPeerClaim func(ctx context.Context, currentAccountID, peerAccountID int64) error
	// saveMergeTelegramClaim — опционально: сохранить Telegram claim для /link/merge при OIDC-link конфликтах customer.
	saveMergeTelegramClaim func(ctx context.Context, currentAccountID, telegramID int64, telegramUsername string) error
}

// New собирает сервис. Все зависимости обязательны (mailer может быть в dry-run
// режиме, но объект должен быть). bootstrap может быть nil — тогда Register/Login
// не будут создавать customer-link (полезно для тестов), но на проде всегда
// передаём настоящий.
func New(
	cfg Config,
	accounts *repository.AccountRepo,
	ids *repository.IdentityRepo,
	sess *repository.SessionRepo,
	evs *repository.EmailVerificationRepo,
	prs *repository.PasswordResetRepo,
	jwtIssuer *jwt.Issuer,
	mailer *mail.Mailer,
	boot *bootstrap.CustomerBootstrap,
	emailMergeCodes *repository.EmailMergeVerificationRepo,
) *Service {
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	if cfg.EmailVerifyTTL == 0 {
		cfg.EmailVerifyTTL = 24 * time.Hour
	}
	if cfg.PasswordResetTTL == 0 {
		cfg.PasswordResetTTL = 30 * time.Minute
	}
	if cfg.DefaultLanguage == "" {
		cfg.DefaultLanguage = "ru"
	}
	if cfg.AntiEnumLatency == 0 {
		cfg.AntiEnumLatency = 300 * time.Millisecond
	}
	if cfg.RefreshCookiePath == "" {
		cfg.RefreshCookiePath = "/cabinet/api/auth"
	}
	return &Service{
		cfg:       cfg,
		accounts:  accounts,
		ids:       ids,
		sess:      sess,
		evs:       evs,
		prs:       prs,
		jwt:       jwtIssuer,
		mailer:    mailer,
		bootstrap:       boot,
		emailMergeCodes: emailMergeCodes,
	}
}

// ensureCustomer вызывает bootstrap best-effort: если что-то пошло не так, мы
// логируем warning и продолжаем. На следующем login / первой покупке вызов
// повторится (EnsureForAccount идемпотентен).
func (s *Service) ensureCustomer(ctx context.Context, accountID int64, language string) {
	if s.bootstrap == nil {
		return
	}
	if _, err := s.bootstrap.EnsureForAccount(ctx, accountID, language); err != nil {
		slog.Warn("cabinet bootstrap failed", "account_id", accountID, "error", err)
	}
}

// attachReferralBestEffort — после создания аккаунта по ref (email / OAuth / Telegram).
//
// Через это поле приходит и партнёрский код (?p=<code>), и реферальный
// telegram_id: у регистрации один параметр «откуда пришёл». Партнёрская ссылка
// проверяется первой, и при успехе реферальная привязка не делается — клиент
// принадлежит одной программе, иначе одна оплата оплачивается дважды.
func (s *Service) attachReferralBestEffort(ctx context.Context, accountID int64, language, referralRaw string) {
	if s.bootstrap == nil {
		return
	}
	if code := bootstrap.ParsePartnerCode(referralRaw); code != "" {
		attached, err := s.bootstrap.AttachPartnerAfterWebRegister(ctx, accountID, language, code)
		if err != nil {
			slog.Warn("cabinet partner attach failed", "account_id", accountID, "error", err)
		}
		if attached {
			return
		}
	}
	if refTG := bootstrap.ParseReferralTelegramID(referralRaw); refTG != 0 {
		if err := s.bootstrap.AttachReferralAfterWebRegister(ctx, accountID, language, refTG); err != nil {
			slog.Warn("cabinet referral attach failed", "account_id", accountID, "error", err)
		}
	}
}

// ensureCustomerTelegram — bootstrap с учётом реального telegram_id: привязка
// к существующему customer из бота или relink с synthetic. Ошибки (кроме
// отсутствия bootstrap) пробрасываются в Telegram login.
// telegramUsername — из Login Widget / Mini App / OIDC; сохраняется в customer.telegram_username
// для подписи в Remnawave (Description) при триале и оплатах.
func (s *Service) ensureCustomerTelegram(ctx context.Context, accountID int64, language string, telegramID int64, telegramUsername string) error {
	if s.bootstrap == nil {
		return nil
	}
	if _, err := s.bootstrap.EnsureForAccountTelegram(ctx, accountID, telegramID, language); err != nil {
		return err
	}
	s.persistTelegramUsernameForAccount(ctx, accountID, telegramUsername)
	return nil
}

func (s *Service) persistTelegramUsernameForAccount(ctx context.Context, accountID int64, raw string) {
	u := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if u == "" || s.lookupLinks == nil || s.lookupCustomers == nil {
		return
	}
	link, err := s.lookupLinks.FindByAccountID(ctx, accountID)
	if err != nil || link == nil {
		return
	}
	if err := s.lookupCustomers.UpdateFields(ctx, link.CustomerID, map[string]interface{}{"telegram_username": u}); err != nil {
		slog.Warn("auth: persist telegram_username", "account_id", accountID, "customer_id", link.CustomerID, "error", err)
	}
}

// SetTelegramCustomerLookup подключает поиск аккаунта по customer.telegram_id
// и cabinet_account_customer_link при Telegram Login. Нужен после link/merge:
// у customer уже реальный telegram_id, а cabinet_identity(telegram) могла не
// создаваться. На проде вызывается из cabinethttp.Mount один раз после New().
func (s *Service) SetTelegramCustomerLookup(customers *database.CustomerRepository, links *repository.AccountCustomerLinkRepo) {
	s.lookupCustomers = customers
	s.lookupLinks = links
}

// SetMergeEmailPeerClaimSaver подключает callback для сценария «email занят другим аккаунтом,
// пароль верный → merge». Вызывается из cabinethttp.Mount после создания MergeService.
func (s *Service) SetMergeEmailPeerClaimSaver(fn func(ctx context.Context, currentAccountID, peerAccountID int64) error) {
	s.saveMergeEmailPeerClaim = fn
}

// SetMergeTelegramClaimSaver подключает callback сохранения Telegram claim для /link/merge
// в OIDC-link сценариях, когда найден второй customer по тому же telegram_id.
func (s *Service) SetMergeTelegramClaimSaver(fn func(ctx context.Context, currentAccountID, telegramID int64, telegramUsername string) error) {
	s.saveMergeTelegramClaim = fn
}

// Ошибки, которые хендлеры мапят в HTTP-коды. Намеренно скудные: сервис не
// хочет рассказывать клиенту детали (anti-enumeration).
var (
	// ErrInvalidInput — валидация провалилась (например, email не парсится,
	// пароль не прошёл policy). Обычно это «400 Bad Request».
	ErrInvalidInput = errors.New("auth: invalid input")

	// ErrInvalidCredentials — пара email/пароль не совпала ИЛИ аккаунт
	// заблокирован. Всегда возвращается одинаково, без разделения.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")

	// ErrInvalidToken — verify/reset/refresh токен отсутствует, истёк или
	// повреждён. HTTP 400/401 на усмотрение хендлера.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrReused — refresh уже был ротирован. Параллельно с возвратом этой
	// ошибки сервис гасит всю family — пользователь будет вынужден войти заново
	// на всех устройствах.
	ErrReused = errors.New("auth: refresh reused")

	// ErrEmailNotVerified — попытка войти/обновить до подтверждения email.
	// Используется точечно (например, в RequireVerifiedEmail middleware).
	ErrEmailNotVerified = errors.New("auth: email not verified")

)

// Исход привязки email (POST /me/email/link).
const (
	LinkEmailOutcomeVerifySent     = "verify_sent"
	LinkEmailOutcomeMergeRequired = "merge_required"
	LinkEmailOutcomeMergeVerificationRequired = "merge_verification_required"
)

const mergeEmailCodeTTL = 15 * time.Minute
const mergeEmailCodeAttempts = 5

// RegisterInput — вход регистрации.
type RegisterInput struct {
	Email    string
	Password string
	Language string // "ru" / "en" или ""
	// ReferralCode — опционально: ref_<telegram_id> реферера (как deep-link бота) или число.
	ReferralCode string
	UserAgent    string
	IP           string
}

// RegisterResult — что вернуть клиенту. Сознательно без account.id и прочего:
// всегда один и тот же ответ независимо от того, был ли email занят.
type RegisterResult struct {
	Message string // текстовая строка для UI; "check your email" + подсказка
}

// Register создаёт аккаунт и отправляет письмо подтверждения.
//
// Если email уже занят — отправляем письмо «кто-то пытался зарегаться» и
// возвращаем ТАКОЙ ЖЕ ответ клиенту. Это стандартная защита от перечисления
// email-адресов.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*RegisterResult, error) {
	start := time.Now()
	defer s.equalizeLatency(start)

	email := normalizeEmail(in.Email)
	if !isLikelyEmail(email) {
		return nil, fmt.Errorf("%w: email", ErrInvalidInput)
	}

	normPwd := password.Normalize(in.Password)
	if err := password.Validate(normPwd, email, "", s.cfg.PasswordPolicy); err != nil {
		// Возвращаем конкретную причину — это не утечка (email мы не раскрываем).
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	lang := normalizeLang(in.Language, s.cfg.DefaultLanguage)

	// Если аккаунт уже существует — молча отправляем «попытка регистрации».
	existing, err := s.accounts.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("find by email: %w", err)
	}
	if existing != nil {
		if err := s.mailer.SendDuplicateRegister(ctx, email, existing.Language, mail.DuplicateRegisterData{
			LoginURL: cabinetAppURL(s.cfg.PublicURL, "/cabinet/login"),
		}); err != nil {
			slog.Warn("failed to send duplicate_register", "error", err)
		}
		return registerOKMessage(lang), nil
	}

	hash, err := password.HashPassword(normPwd, s.cfg.PasswordParams)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	acc, err := s.accounts.Create(ctx, email, hash, lang)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	// Identity для email-провайдера: provider_user_id = account_id в строковом виде.
	// Это гарантирует уникальность и упрощает ListByAccount.
	if _, err := s.ids.Create(ctx, acc.ID, repository.ProviderEmail, strconv.FormatInt(acc.ID, 10), email, nil); err != nil {
		return nil, fmt.Errorf("create email identity: %w", err)
	}

	// Bootstrap customer: web-only клиент + link. Best-effort — если провалится,
	// Login его повторит. Регистрацию из-за этого не фейлим (письмо уже уходит).
	s.ensureCustomer(ctx, acc.ID, acc.Language)

	s.attachReferralBestEffort(ctx, acc.ID, acc.Language, in.ReferralCode)

	if err := s.sendVerifyEmail(ctx, acc); err != nil {
		slog.Warn("failed to send verify email", "account_id", acc.ID, "error", err)
		// Регистрация создана — не отменяем её из-за проблем со SMTP; пользователь
		// сможет нажать «отправить повторно» из /me/email/verify/resend.
	}

	return registerOKMessage(lang), nil
}

// sendVerifyEmail выпускает новый одноразовый код подтверждения и шлёт письмо.
// Предыдущие (если были) инвалидируются.
func (s *Service) sendVerifyEmail(ctx context.Context, acc *repository.Account) error {
	if acc.Email == nil || *acc.Email == "" {
		return fmt.Errorf("account has no email")
	}
	if err := s.evs.InvalidateForAccount(ctx, acc.ID); err != nil {
		return err
	}
	code, hash, err := tokens.GenerateEmailVerifyOTP(s.cfg.EmailCodeKey)
	if err != nil {
		return err
	}
	if _, err := s.evs.Create(ctx, acc.ID, hash, time.Now().Add(s.cfg.EmailVerifyTTL)); err != nil {
		return err
	}
	return s.mailer.SendVerifyEmail(ctx, *acc.Email, acc.Language, mail.VerifyEmailData{
		Code:     code,
		TTLHuman: humanDuration(s.cfg.EmailVerifyTTL, acc.Language),
	})
}

// LoginInput — вход логина.
type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

// TokenPair — то, что сервис отдаёт хендлеру после успешного login/refresh.
// Refresh лежит в cookie, access — в теле ответа (Bearer в Authorization header).
type TokenPair struct {
	AccessToken  string
	AccessExp    time.Time
	RefreshToken string
	RefreshExp   time.Time
	SessionID    int64
	AccountID    int64
	CSRFToken    string
}

// Login аутентифицирует пользователя.
func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	start := time.Now()
	defer s.equalizeLatency(start)

	email := normalizeEmail(in.Email)
	if !isLikelyEmail(email) {
		// Всё равно съедаем время на argon, чтобы тайминг не отличался.
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, ErrInvalidCredentials
	}
	normPwd := password.Normalize(in.Password)

	acc, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			password.DummyCompare(s.cfg.PasswordParams)
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find account: %w", err)
	}
	if acc.Status != repository.AccountStatusActive || acc.PasswordHash == nil {
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, ErrInvalidCredentials
	}

	ok, _, err := password.ComparePasswordAndHash(normPwd, *acc.PasswordHash, s.cfg.PasswordParams)
	if err != nil {
		return nil, fmt.Errorf("compare password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	// last_login_at обновляем «best-effort», не валим логин из-за ошибки апдейта.
	if err := s.accounts.UpdateLastLogin(ctx, acc.ID); err != nil {
		slog.Warn("update last_login failed", "account_id", acc.ID, "error", err)
	}

	// Defensive bootstrap: если при регистрации что-то упало (или аккаунт
	// был создан до Этапа 3), гарантируем наличие customer-link до выдачи
	// сессии. Идемпотентно, дёшево.
	s.ensureCustomer(ctx, acc.ID, acc.Language)

	if s.ids != nil {
		pid := strconv.FormatInt(acc.ID, 10)
		if err := s.ids.ClearUnlinkedAtForSubject(ctx, acc.ID, repository.ProviderEmail, pid); err != nil {
			slog.Warn("login: clear email identity unlinked_at", "account_id", acc.ID, "error", err.Error())
		}
	}

	return s.issueSession(ctx, acc, uuid.New(), in.UserAgent, in.IP)
}

// issueSession создаёт новую refresh-сессию + подписывает JWT + генерит CSRF.
// family может быть новой (при логине) или наследоваться от старой (при ротации).
func (s *Service) issueSession(ctx context.Context, acc *repository.Account, family uuid.UUID, userAgent, ip string) (*TokenPair, error) {
	refreshToken, refreshHash, err := tokens.Generate(tokens.DefaultRefreshBytes)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}
	refreshExp := time.Now().Add(s.cfg.RefreshTTL)

	sess, err := s.sess.Create(ctx, repository.CreateInput{
		AccountID: acc.ID,
		TokenHash: refreshHash,
		FamilyID:  family,
		UserAgent: truncate(userAgent, 512),
		IP:        ip,
		ExpiresAt: refreshExp,
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	access, accessExp, err := s.jwt.Issue(acc.ID, emailOr(acc.Email), acc.EmailVerified(), acc.Language)
	if err != nil {
		return nil, fmt.Errorf("issue access: %w", err)
	}

	csrfToken, err := csrf.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate csrf: %w", err)
	}

	return &TokenPair{
		AccessToken:  access,
		AccessExp:    accessExp,
		RefreshToken: refreshToken,
		RefreshExp:   refreshExp,
		SessionID:    sess.ID,
		AccountID:    acc.ID,
		CSRFToken:    csrfToken,
	}, nil
}

// Refresh ротирует refresh-токен. Возвращает пару с новыми access/refresh/csrf.
func (s *Service) Refresh(ctx context.Context, refresh, userAgent, ip string) (*TokenPair, error) {
	refresh = strings.TrimSpace(refresh)
	if refresh == "" {
		return nil, ErrInvalidToken
	}
	hash, err := tokens.HashString(refresh)
	if err != nil {
		return nil, ErrInvalidToken
	}

	sess, err := s.sess.FindByRefreshHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find session: %w", err)
	}

	// Если сессия уже revoked — это reuse старого (после ротации) токена.
	// Реакция: отзываем всю family и отвечаем ErrReused.
	if sess.RevokedAt != nil {
		if _, err := s.sess.RevokeFamily(ctx, sess.RefreshTokenFamilyID); err != nil {
			slog.Warn("revoke family on reuse failed", "family", sess.RefreshTokenFamilyID, "error", err)
		}
		return nil, ErrReused
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrInvalidToken
	}

	acc, err := s.accounts.FindByID(ctx, sess.AccountID)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}
	if acc.Status != repository.AccountStatusActive {
		return nil, ErrInvalidCredentials
	}

	newRefresh, newHash, err := tokens.Generate(tokens.DefaultRefreshBytes)
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}
	newExp := time.Now().Add(s.cfg.RefreshTTL)

	newSess, err := s.sess.Rotate(ctx, sess.ID, repository.CreateInput{
		AccountID: sess.AccountID,
		TokenHash: newHash,
		UserAgent: truncate(userAgent, 512),
		IP:        ip,
		ExpiresAt: newExp,
	})
	if err != nil {
		if errors.Is(err, repository.ErrReused) {
			if _, err := s.sess.RevokeFamily(ctx, sess.RefreshTokenFamilyID); err != nil {
				slog.Warn("revoke family on reuse failed", "family", sess.RefreshTokenFamilyID, "error", err)
			}
			return nil, ErrReused
		}
		return nil, fmt.Errorf("rotate session: %w", err)
	}

	access, accessExp, err := s.jwt.Issue(acc.ID, emailOr(acc.Email), acc.EmailVerified(), acc.Language)
	if err != nil {
		return nil, fmt.Errorf("issue access: %w", err)
	}

	csrfToken, err := csrf.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate csrf: %w", err)
	}

	return &TokenPair{
		AccessToken:  access,
		AccessExp:    accessExp,
		RefreshToken: newRefresh,
		RefreshExp:   newExp,
		SessionID:    newSess.ID,
		AccountID:    acc.ID,
		CSRFToken:    csrfToken,
	}, nil
}

// accountIDFromValidRefresh возвращает account_id активной (не отозванной,
// не просроченной) сессии по refresh-токену. Не ротирует токен.
func (s *Service) accountIDFromValidRefresh(ctx context.Context, refresh string) (int64, error) {
	refresh = strings.TrimSpace(refresh)
	if refresh == "" {
		return 0, ErrInvalidToken
	}
	hash, err := tokens.HashString(refresh)
	if err != nil {
		return 0, ErrInvalidToken
	}
	sess, err := s.sess.FindByRefreshHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrInvalidToken
		}
		return 0, fmt.Errorf("find session: %w", err)
	}
	if sess.RevokedAt != nil {
		return 0, ErrInvalidToken
	}
	if time.Now().After(sess.ExpiresAt) {
		return 0, ErrInvalidToken
	}
	return sess.AccountID, nil
}

// Logout отзывает конкретную сессию (по refresh-токену из cookie).
// Если токен уже невалиден — не считается ошибкой (идемпотентный logout).
func (s *Service) Logout(ctx context.Context, refresh string) error {
	refresh = strings.TrimSpace(refresh)
	if refresh == "" {
		return nil
	}
	hash, err := tokens.HashString(refresh)
	if err != nil {
		return nil
	}
	sess, err := s.sess.FindByRefreshHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.sess.Revoke(ctx, sess.ID)
}

// ForgotPassword принимает email, создаёт reset-токен и шлёт письмо.
// Всегда возвращает nil: мы не разглашаем, зарегистрирован email или нет.
// (Ошибки SMTP логируются, но наружу не летят.)
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	start := time.Now()
	defer s.equalizeLatency(start)

	email = normalizeEmail(email)
	if !isLikelyEmail(email) {
		return nil
	}
	acc, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		slog.Warn("forgot: find account failed", "error", err)
		return nil
	}
	if err := s.prs.InvalidateForAccount(ctx, acc.ID); err != nil {
		slog.Warn("forgot: invalidate failed", "error", err)
	}
	token, hash, err := tokens.Generate(0)
	if err != nil {
		slog.Warn("forgot: generate token failed", "error", err)
		return nil
	}
	if _, err := s.prs.Create(ctx, acc.ID, hash, time.Now().Add(s.cfg.PasswordResetTTL)); err != nil {
		slog.Warn("forgot: create token failed", "error", err)
		return nil
	}
	resetURL := cabinetAppURL(s.cfg.PublicURL, "/cabinet/password/reset?token="+url.QueryEscape(token))
	if err := s.mailer.SendPasswordReset(ctx, email, acc.Language, mail.PasswordResetData{
		ResetURL: resetURL,
		TTLHuman: humanDuration(s.cfg.PasswordResetTTL, acc.Language),
	}); err != nil {
		slog.Warn("forgot: send email failed", "error", err)
	}
	return nil
}

// ResetPassword применяет новый пароль по reset-токену и инвалидирует все
// refresh-сессии пользователя.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidToken
	}
	hash, err := tokens.HashString(token)
	if err != nil {
		return ErrInvalidToken
	}
	pr, err := s.prs.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("find reset: %w", err)
	}
	if !pr.IsUsable() {
		return ErrInvalidToken
	}
	acc, err := s.accounts.FindByID(ctx, pr.AccountID)
	if err != nil {
		return fmt.Errorf("find account: %w", err)
	}

	normPwd := password.Normalize(newPassword)
	if err := password.Validate(normPwd, emailOr(acc.Email), "", s.cfg.PasswordPolicy); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	newHash, err := password.HashPassword(normPwd, s.cfg.PasswordParams)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	if err := s.accounts.UpdatePasswordHash(ctx, acc.ID, newHash); err != nil {
		return err
	}
	if _, err := s.sess.RevokeAllForAccount(ctx, acc.ID); err != nil {
		slog.Warn("reset: revoke sessions failed", "error", err)
	}
	if err := s.prs.MarkUsed(ctx, pr.ID); err != nil {
		slog.Warn("reset: mark used failed", "error", err)
	}
	return nil
}

// ChangePassword меняет пароль в активной сессии: проверяет текущий пароль,
// обновляет хеш, отзывает все refresh-сессии и выдаёт новую пару токенов
// для текущего устройства.
func (s *Service) ChangePassword(ctx context.Context, accountID int64, currentPassword, newPassword, userAgent, ip string) (*TokenPair, error) {
	start := time.Now()
	defer s.equalizeLatency(start)

	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc.Status != repository.AccountStatusActive || acc.PasswordHash == nil {
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, ErrInvalidCredentials
	}

	normCurrent := password.Normalize(currentPassword)
	ok, _, err := password.ComparePasswordAndHash(normCurrent, *acc.PasswordHash, s.cfg.PasswordParams)
	if err != nil {
		return nil, fmt.Errorf("compare password: %w", err)
	}
	if !ok {
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, ErrInvalidCredentials
	}

	normNew := password.Normalize(newPassword)
	if err := password.Validate(normNew, emailOr(acc.Email), "", s.cfg.PasswordPolicy); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	newHash, err := password.HashPassword(normNew, s.cfg.PasswordParams)
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}
	if err := s.accounts.UpdatePasswordHash(ctx, acc.ID, newHash); err != nil {
		return nil, err
	}
	if _, err := s.sess.RevokeAllForAccount(ctx, acc.ID); err != nil {
		slog.Warn("change password: revoke sessions failed", "error", err)
	}

	return s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
}

// maxEmailCodeAttempts — сколько догадок принимает один код из письма. Вместе
// с поиском в пределах аккаунта это шанс 5 из 10^6 подобрать код — дальше
// нужен новый, а его выдача ограничена по email.
const maxEmailCodeAttempts = 5

// ConfirmEmail применяет код из письма (6 цифр) или старый base64url-токен из
// ссылки: помечает email_verified_at и сразу выдаёт сессию (как login).
//
// Код из шести цифр проверяется только против живого кода одного аккаунта:
// по email из запроса, а без него — по refresh-сессии залогиненного
// пользователя. Раньше код искался по всей таблице, и перебор 000000…999999
// давал сессию в любом аккаунте с живым кодом. Сессию выдавать по-прежнему
// можно: верный код доказывает владение почтой, а владелец почты и так войдёт
// через сброс пароля.
func (s *Service) ConfirmEmail(ctx context.Context, token, email, refreshFromCookie, userAgent, ip string) (*TokenPair, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidToken
	}
	var ev *repository.EmailVerification
	if tokens.IsEmailVerifyOTP(token) {
		var err error
		ev, err = s.checkEmailCode(ctx, token, email, refreshFromCookie)
		if err != nil {
			return nil, err
		}
	} else {
		// Старые ссылки из писем: 256 бит, перебором не подобрать — ищем по хэшу.
		hash, err := tokens.HashString(token)
		if err != nil {
			return nil, ErrInvalidToken
		}
		ev, err = s.evs.FindByHash(ctx, hash)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrInvalidToken
			}
			return nil, fmt.Errorf("find ev: %w", err)
		}
		if !ev.IsUsable() {
			return nil, ErrInvalidToken
		}
	}
	consumed, err := s.evs.Consume(ctx, ev.ID)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, ErrInvalidToken
	}
	acc, err := s.accounts.FindByID(ctx, ev.AccountID)
	if err != nil {
		return nil, fmt.Errorf("confirm: find account: %w", err)
	}
	if acc.Status != repository.AccountStatusActive {
		return nil, ErrInvalidToken
	}
	if err := s.accounts.MarkEmailVerified(ctx, ev.AccountID); err != nil {
		return nil, err
	}
	// Перечитываем: токен выписывается по acc, а прочитан он до подтверждения.
	// Без этого в выданном access-токене было email_verified=false, и до
	// следующего refresh кабинет получал 403 на подписке и оплате.
	acc, err = s.accounts.FindByID(ctx, ev.AccountID)
	if err != nil {
		return nil, fmt.Errorf("confirm: reload account: %w", err)
	}
	// Defensive bootstrap: если линк customer отсутствует (редкий случай), подшьём.
	s.ensureCustomer(ctx, acc.ID, acc.Language)
	if s.ids != nil {
		pid := strconv.FormatInt(acc.ID, 10)
		if err := s.ids.ClearUnlinkedAtForSubject(ctx, acc.ID, repository.ProviderEmail, pid); err != nil {
			slog.Warn("confirm email: clear email identity unlinked_at", "account_id", acc.ID, "error", err.Error())
		}
	}
	return s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
}

// checkEmailCode находит живой код аккаунта и сверяет с ним догадку, засчитывая
// попытку до сравнения. Любой отказ — ErrInvalidToken, чтобы ответ не выдавал,
// существует ли email.
func (s *Service) checkEmailCode(ctx context.Context, code, email, refreshFromCookie string) (*repository.EmailVerification, error) {
	var accountID int64
	if email = normalizeEmail(email); email != "" {
		acc, err := s.accounts.FindByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrInvalidToken
			}
			return nil, fmt.Errorf("confirm: find account by email: %w", err)
		}
		accountID = acc.ID
	} else {
		id, err := s.accountIDFromValidRefresh(ctx, refreshFromCookie)
		if err != nil {
			// Ни email, ни сессии: искать код не в чем.
			return nil, ErrInvalidToken
		}
		accountID = id
	}

	ev, err := s.evs.FindActiveForAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find ev: %w", err)
	}
	reserved, err := s.evs.ReserveAttempt(ctx, ev.ID, maxEmailCodeAttempts)
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, ErrInvalidToken
	}
	keyed := tokens.HashEmailVerifyOTPKeyed(s.cfg.EmailCodeKey, code)
	// Коды, выданные до появления ключа, хэшированы голым sha256.
	legacy := tokens.HashEmailVerifyOTP(code)
	if subtle.ConstantTimeCompare(ev.TokenHash, keyed[:]) != 1 &&
		subtle.ConstantTimeCompare(ev.TokenHash, legacy[:]) != 1 {
		return nil, ErrInvalidToken
	}
	return ev, nil
}

type LinkEmailResult struct {
	Status      string
	ReasonCode  string
	MaskedEmail string
}

// LinkEmailToAccount — привязка email+пароля к уже существующему аккаунту (OAuth/Telegram без пароля).
// Если адрес свободен — сохраняет пароль, создаёт identity email при необходимости, шлёт письмо verify.
// Если адрес у другого аккаунта B и пароль совпадает с B — сохраняет claim для /link/merge.
// Если адрес у B, а пароль у B отсутствует (OAuth-only) — отправляет код подтверждения на email для авторизации merge.
func (s *Service) LinkEmailToAccount(ctx context.Context, accountID int64, email, pwd string) (*LinkEmailResult, error) {
	start := time.Now()
	defer s.equalizeLatency(start)

	email = normalizeEmail(email)
	if !isLikelyEmail(email) {
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, fmt.Errorf("%w: email", ErrInvalidInput)
	}
	normPwd := password.Normalize(pwd)
	if err := password.Validate(normPwd, email, "", s.cfg.PasswordPolicy); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc.Status != repository.AccountStatusActive {
		password.DummyCompare(s.cfg.PasswordParams)
		return nil, ErrInvalidCredentials
	}
	if acc.PasswordHash != nil {
		return nil, fmt.Errorf("%w: email login already enabled", ErrInvalidInput)
	}
	// If this email already comes from linked social providers in the same account,
	// don't create a duplicate "email login" over the same address.
	ids, err := s.ids.ListLinkedByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	var socialProvider string
	for _, id := range ids {
		if id.Provider != repository.ProviderGoogle && id.Provider != repository.ProviderYandex && id.Provider != repository.ProviderVK {
			continue
		}
		if id.ProviderEmail == nil {
			continue
		}
		if normalizeEmail(*id.ProviderEmail) == email {
			socialProvider = id.Provider
			break
		}
	}
	if socialProvider != "" {
		return nil, fmt.Errorf("%w: email is already used in %s sign-in", ErrInvalidInput, socialProvider)
	}

	other, err := s.accounts.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("find by email: %w", err)
	}
	if other != nil && other.ID != accountID {
		if other.PasswordHash == nil {
			if err := s.startMergeEmailVerification(ctx, accountID, other.ID, email); err != nil {
				return nil, fmt.Errorf("start merge email verification: %w", err)
			}
			return &LinkEmailResult{
				Status:      LinkEmailOutcomeMergeVerificationRequired,
				ReasonCode:  "peer_password_missing_email_code_required",
				MaskedEmail: maskEmail(email),
			}, nil
		}
		ok, _, cmpErr := password.ComparePasswordAndHash(normPwd, *other.PasswordHash, s.cfg.PasswordParams)
		if cmpErr != nil {
			return nil, fmt.Errorf("compare password: %w", cmpErr)
		}
		if !ok {
			password.DummyCompare(s.cfg.PasswordParams)
			return nil, ErrInvalidCredentials
		}
		if s.saveMergeEmailPeerClaim == nil {
			return nil, errors.New("auth: merge email peer claim saver not configured")
		}
		if err := s.saveMergeEmailPeerClaim(ctx, accountID, other.ID); err != nil {
			return nil, err
		}
		return &LinkEmailResult{
			Status:     LinkEmailOutcomeMergeRequired,
			ReasonCode: "peer_account_verified_by_password",
		}, nil
	}

	hash, err := password.HashPassword(normPwd, s.cfg.PasswordParams)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := s.accounts.UpdateEmailPasswordUnverified(ctx, accountID, email, hash); err != nil {
		return nil, err
	}

	pid := strconv.FormatInt(accountID, 10)
	_, err = s.ids.FindByProvider(ctx, repository.ProviderEmail, pid)
	if errors.Is(err, repository.ErrNotFound) {
		if _, err := s.ids.Create(ctx, accountID, repository.ProviderEmail, pid, email, nil); err != nil {
			return nil, fmt.Errorf("create email identity: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("find email identity: %w", err)
	} else if err := s.ids.ClearUnlinkedAtForSubject(ctx, accountID, repository.ProviderEmail, pid); err != nil {
		return nil, fmt.Errorf("reactivate email identity: %w", err)
	}

	acc2, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := s.sendVerifyEmail(ctx, acc2); err != nil {
		slog.Warn("link email: send verify email", "account_id", accountID, "error", err)
	}
	return &LinkEmailResult{
		Status:     LinkEmailOutcomeVerifySent,
		ReasonCode: "email_linked_verification_sent",
	}, nil
}

func (s *Service) ConfirmEmailMergeCode(ctx context.Context, currentAccountID int64, code string) (*LinkEmailResult, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, fmt.Errorf("%w: verification code", ErrInvalidInput)
	}
	if s.saveMergeEmailPeerClaim == nil {
		return nil, errors.New("auth: merge email peer claim saver not configured")
	}
	peerID, masked, err := s.consumeMergeEmailVerification(ctx, currentAccountID, code)
	if err != nil {
		return nil, err
	}
	if err := s.saveMergeEmailPeerClaim(ctx, currentAccountID, peerID); err != nil {
		return nil, err
	}
	return &LinkEmailResult{
		Status:      LinkEmailOutcomeMergeRequired,
		ReasonCode:  "peer_account_verified_by_email_code",
		MaskedEmail: masked,
	}, nil
}

// ResendVerifyEmailPublic повторно шлёт код на email без сессии (после регистрации).
// Ответ снаружи не зависит от наличия неподтверждённого аккаунта (anti-enumeration).
func (s *Service) ResendVerifyEmailPublic(ctx context.Context, email string) error {
	start := time.Now()
	defer s.equalizeLatency(start)

	email = normalizeEmail(email)
	if !isLikelyEmail(email) {
		return nil
	}
	acc, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		slog.Warn("resend verify public: find account failed", "error", err)
		return nil
	}
	if acc.EmailVerified() {
		return nil
	}
	if err := s.sendVerifyEmail(ctx, acc); err != nil {
		slog.Warn("resend verify public: send failed", "account_id", acc.ID, "error", err)
	}
	return nil
}

// ResendVerify отправляет новое письмо подтверждения для указанного аккаунта.
// Требует, чтобы вызывающий (middleware) уже проверил, что аккаунт аутентифицирован.
func (s *Service) ResendVerify(ctx context.Context, accountID int64) error {
	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acc.EmailVerified() {
		// Уже подтверждён — ничего не делаем, но возвращаем ok.
		return nil
	}
	return s.sendVerifyEmail(ctx, acc)
}

// ============================================================================
// Хелперы
// ============================================================================

// equalizeLatency добивает общую длительность метода до AntiEnumLatency, если
// реальная обработка завершилась быстрее. Защита от тайминг-анализа.
func (s *Service) equalizeLatency(start time.Time) {
	if s.cfg.AntiEnumLatency <= 0 {
		return
	}
	elapsed := time.Since(start)
	if elapsed < s.cfg.AntiEnumLatency {
		time.Sleep(s.cfg.AntiEnumLatency - elapsed)
	}
}

func registerOKMessage(lang string) *RegisterResult {
	if lang == "en" {
		return &RegisterResult{Message: "Check your inbox — we've sent a verification email."}
	}
	return &RegisterResult{Message: "Проверьте почту — мы отправили письмо с подтверждением."}
}

// normalizeEmail приводит email к каноническому виду: lower-case, trim.
// Дополнительной Unicode-нормализации (IDN punycode) пока не делаем — MVP.
func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// isLikelyEmail — быстрая проверка без тяжёлого RFC-парсинга. Настоящая
// валидация — только sending the email: если MX нет, письмо не уйдёт.
func isLikelyEmail(s string) bool {
	if len(s) < 3 || len(s) > 320 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	return strings.ContainsRune(s[at+1:], '.')
}

func normalizeLang(lang, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "ru":
		return "ru"
	case "en":
		return "en"
	default:
		if fallback == "" {
			return "ru"
		}
		return fallback
	}
}

func emailOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// cabinetAppURL — ссылки из писем и OAuth: SPA и API живут под префиксом /cabinet/.
// publicURL — как в CABINET_PUBLIC_URL, без завершающего слэша.
func cabinetAppURL(publicURL, path string) string {
	base := strings.TrimSuffix(strings.TrimSpace(publicURL), "/")
	if path == "" {
		return base
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return base + path
}

// humanDuration превращает TTL в человекочитаемую строку для писем.
// Грубая эвристика — достаточно для 24h / 30m.
func humanDuration(d time.Duration, language string) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if language == "en" {
		if hours > 0 {
			if minutes == 0 {
				if hours == 1 {
					return "1 hour"
				}
				return fmt.Sprintf("%d hours", hours)
			}
			return fmt.Sprintf("%d hours %d minutes", hours, minutes)
		}
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if hours > 0 {
		if minutes == 0 {
			return fmt.Sprintf("%d ч", hours)
		}
		return fmt.Sprintf("%d ч %d мин", hours, minutes)
	}
	return fmt.Sprintf("%d мин", minutes)
}

func (s *Service) startMergeEmailVerification(ctx context.Context, currentAccountID, peerAccountID int64, email string) error {
	if s.emailMergeCodes == nil {
		return errors.New("auth: email merge verification repo not configured")
	}
	code, err := generateSixDigitCode()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(code))
	codeHash := hex.EncodeToString(sum[:])
	lang := s.cfg.DefaultLanguage
	if acc, aerr := s.accounts.FindByID(ctx, currentAccountID); aerr == nil && acc != nil {
		lang = acc.Language
	}
	if err := s.mailer.SendEmailMergeCode(ctx, email, lang, code, humanDuration(mergeEmailCodeTTL, lang)); err != nil {
		slog.Warn("merge email verification: send code failed", "account_id", currentAccountID, "error", err)
	}
	expiresAt := time.Now().Add(mergeEmailCodeTTL)
	return s.emailMergeCodes.Upsert(ctx, currentAccountID, peerAccountID, codeHash, maskEmail(email), mergeEmailCodeAttempts, expiresAt)
}

func (s *Service) consumeMergeEmailVerification(ctx context.Context, currentAccountID int64, code string) (int64, string, error) {
	if s.emailMergeCodes == nil {
		return 0, "", errors.New("auth: email merge verification repo not configured")
	}
	v, err := s.emailMergeCodes.Get(ctx, currentAccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, "", ErrInvalidToken
		}
		return 0, "", err
	}
	if time.Now().After(v.ExpiresAt) {
		_ = s.emailMergeCodes.Delete(ctx, currentAccountID)
		return 0, "", ErrInvalidToken
	}
	inSum := sha256.Sum256([]byte(code))
	inHash := hex.EncodeToString(inSum[:])
	if inHash != v.CodeHash {
		left, derr := s.emailMergeCodes.DecrementAttempts(ctx, currentAccountID)
		if derr == nil && left <= 0 {
			_ = s.emailMergeCodes.Delete(ctx, currentAccountID)
		}
		return 0, "", ErrInvalidCredentials
	}
	if err := s.emailMergeCodes.Delete(ctx, currentAccountID); err != nil {
		return 0, "", err
	}
	return v.PeerAccountID, v.MaskedEmail, nil
}

func generateSixDigitCode() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint64(b[:]) % 1000000
	return fmt.Sprintf("%06d", n), nil
}

// RefreshCookieFromRequest — утилита для handler'а: достаёт refresh-токен из
// cookie по имени. Вынесена в пакет сервиса, чтобы имя cookie жило рядом с
// кодом, который его выпускает (см. RefreshCookieName).
func RefreshCookieFromRequest(r *http.Request) string {
	c, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// RefreshCookieName — имя HttpOnly cookie с refresh-токеном. Используется и
// при SetCookie в login/refresh, и при Logout (Max-Age=0).
const RefreshCookieName = "cab_refresh"
