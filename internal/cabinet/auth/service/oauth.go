package service

// oauth.go — методы Google OAuth и Telegram Login для *Service.
//
// Расширяет auth.go новыми методами; опциональные зависимости (Google, Telegram)
// подключаются через SetGoogle / SetTelegramToken после New().

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	googleoauth "remnawave-tg-shop-bot/internal/cabinet/auth/oauth"
	tgverify "remnawave-tg-shop-bot/internal/cabinet/auth/telegram"
	"remnawave-tg-shop-bot/internal/cabinet/auth/tokens"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/pgerr"
)

// oauthGoogleProviderWrapper — тонкая обёртка вокруг *googleoauth.GoogleProvider.
// Объявлена здесь, чтобы auth.go не тянул импорт пакета oauth напрямую.
type oauthGoogleProviderWrapper struct {
	p *googleoauth.GoogleProvider
}
type oauthYandexProviderWrapper struct {
	p *googleoauth.YandexProvider
}
type oauthVKProviderWrapper struct {
	p *googleoauth.VKProvider
}

type oauthTelegramOIDCWrapper struct {
	p *googleoauth.TelegramOIDCProvider
}

// ============================================================================
// Sentinel ошибки
// ============================================================================

// ErrGoogleDisabled — Google OAuth не настроен.
var ErrGoogleDisabled = errors.New("auth: google oauth disabled")
var ErrYandexDisabled = errors.New("auth: yandex oauth disabled")
var ErrVKDisabled = errors.New("auth: vk oauth disabled")

// ErrTelegramDisabled — Telegram-токен не прокинут.
var ErrTelegramDisabled = errors.New("auth: telegram login disabled")

// ErrGoogleLinkRequired — Google email совпадает с существующим аккаунтом.
// Требуется подтверждение через email перед привязкой.
var ErrGoogleLinkRequired = errors.New("auth: google link confirmation required")

// ErrGoogleLinkSessionMismatch — привязка Google: refresh-сессия не совпадает с account_id в OAuth state.
var ErrGoogleLinkSessionMismatch = errors.New("auth: google link session mismatch")

// ErrGoogleLinkedElsewhere — этот Google (sub) уже привязан к другому аккаунту кабинета.
var ErrGoogleLinkedElsewhere = errors.New("auth: google account already linked elsewhere")

// ErrGoogleMergeRequired — Google уже у другого аккаунта; создан merge-claim, нужно открыть merge-flow.
var ErrGoogleMergeRequired = errors.New("auth: google merge required")
var ErrYandexMergeRequired = errors.New("auth: yandex merge required")
var ErrVKMergeRequired = errors.New("auth: vk merge required")

// ErrGoogleLinkEmailConflict — email из Google уже принадлежит другому аккаунту кабинета.
var ErrGoogleLinkEmailConflict = errors.New("auth: google email belongs to another cabinet account")
var ErrYandexLinkEmailConflict = errors.New("auth: yandex email belongs to another cabinet account")
var ErrVKLinkEmailConflict = errors.New("auth: vk email belongs to another cabinet account")

var ErrTelegramOIDCDisabled = errors.New("auth: telegram oidc disabled")

// ============================================================================
// Pending link store
// ============================================================================

// pendingLink — запись ожидающего подтверждения Google-OAuth логина.
// pendingLink — вход через Google/Яндекс/VK, email которого совпал с уже
// существующим аккаунтом. Привязка ждёт подтверждения по ссылке из письма.
type pendingLink struct {
	accountID      int64
	provider       string
	providerUserID string
	providerEmail  string
	rawProfile     []byte
	expiresAt      time.Time
}

const pendingLinkTTL = 15 * time.Minute

type pendingLinkStore struct {
	mu    sync.Mutex
	store map[string]pendingLink
}

func newPendingLinkStore() *pendingLinkStore {
	return &pendingLinkStore{store: make(map[string]pendingLink)}
}

func (s *pendingLinkStore) save(tok string, link pendingLink) {
	link.expiresAt = time.Now().Add(pendingLinkTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[tok] = link
}

func (s *pendingLinkStore) pop(tok string) (pendingLink, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	link, ok := s.store[tok]
	if !ok {
		return pendingLink{}, false
	}
	delete(s.store, tok)
	if time.Now().After(link.expiresAt) {
		return pendingLink{}, false
	}
	return link, true
}

func (s *pendingLinkStore) runGC(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := time.Now()
				s.mu.Lock()
				for k, v := range s.store {
					if now.After(v.expiresAt) {
						delete(s.store, k)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

// ============================================================================
// Setter-методы (вызываются из router.go один раз при инициализации)
// ============================================================================

// SetGoogle подключает Google OAuth provider. ctx нужен для GC горутины.
func (s *Service) SetGoogle(ctx context.Context, p *googleoauth.GoogleProvider) {
	s.googleProvider = &oauthGoogleProviderWrapper{p: p}
	s.ensureLinkStore(ctx)
}

func (s *Service) SetYandex(ctx context.Context, p *googleoauth.YandexProvider) {
	s.yandexProvider = &oauthYandexProviderWrapper{p: p}
	s.ensureLinkStore(ctx)
}

func (s *Service) SetVK(ctx context.Context, p *googleoauth.VKProvider) {
	s.vkProvider = &oauthVKProviderWrapper{p: p}
	s.ensureLinkStore(ctx)
}

// ensureLinkStore — хранилище привязок, ждущих подтверждения по письму, общее для
// всех провайдеров; создаётся с первым из них.
func (s *Service) ensureLinkStore(ctx context.Context) {
	if s.linkStore != nil {
		return
	}
	s.linkStore = newPendingLinkStore()
	s.linkStore.runGC(ctx)
}

// requestOAuthEmailLink — вход через провайдера с email уже существующего
// аккаунта. Сразу привязывать нельзя: аккаунт с этим email мог заранее
// зарегистрировать кто угодно, не подтвердив почту, а провайдер не всегда
// подтверждает email сам. Тогда вход жертвы через провайдера отдал бы её
// аккаунту атакующего. Поэтому на email аккаунта уходит ссылка, и привязка
// происходит только по ней — то есть только у владельца почты.
func (s *Service) requestOAuthEmailLink(ctx context.Context, acc *repository.Account, provider, providerUserID, providerEmail string, rawProfile []byte) (GoogleCallbackResult, error) {
	var empty GoogleCallbackResult
	if acc.Status != repository.AccountStatusActive {
		return empty, ErrInvalidCredentials
	}
	if s.linkStore == nil || acc.Email == nil {
		return empty, ErrGoogleLinkEmailConflict
	}
	linkToken, _, err := tokens.Generate(0)
	if err != nil {
		return empty, fmt.Errorf("%s callback: gen link token: %w", provider, err)
	}
	s.linkStore.save(linkToken, pendingLink{
		accountID:      acc.ID,
		provider:       provider,
		providerUserID: providerUserID,
		providerEmail:  providerEmail,
		rawProfile:     rawProfile,
	})
	to := normalizeEmail(*acc.Email)
	confirmURL := cabinetAppURL(s.cfg.PublicURL, "/cabinet/api/auth/oauth/confirm?token="+url.QueryEscape(linkToken))
	if sendErr := s.mailer.SendOAuthLinkConfirm(ctx, to, acc.Language, provider, confirmURL); sendErr != nil {
		slog.Warn("oauth callback: send link confirm email", "provider", provider, "error", sendErr)
	}
	return GoogleCallbackResult{
		LinkCtx: &GoogleLinkContext{MaskedEmail: maskEmail(to)},
	}, ErrGoogleLinkRequired
}

func (s *Service) SetTelegramOIDC(ctx context.Context, p *googleoauth.TelegramOIDCProvider) {
	if p == nil {
		s.telegramOIDC = nil
		return
	}
	s.telegramOIDC = &oauthTelegramOIDCWrapper{p: p}
	p.RunGC(ctx)
}

// SetTelegramToken задаёт токен бота для Telegram HMAC.
func (s *Service) SetTelegramToken(token string) {
	t := strings.TrimSpace(token)
	s.telegramToken = t
	if t == "" {
		s.telegramTokens = nil
		return
	}
	s.telegramTokens = []string{t}
}

// SetTelegramTokens задаёт несколько токенов ботов для валидации Telegram HMAC.
// Это нужно, когда Mini App открывается из одного бота, а Login Widget настроен на другой.
func (s *Service) SetTelegramTokens(tokens ...string) {
	uniq := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, raw := range tokens {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		uniq = append(uniq, t)
	}
	s.telegramTokens = uniq
	if len(uniq) > 0 {
		s.telegramToken = uniq[0]
	} else {
		s.telegramToken = ""
	}
}

func (s *Service) effectiveTelegramTokens() []string {
	if len(s.telegramTokens) > 0 {
		return s.telegramTokens
	}
	if strings.TrimSpace(s.telegramToken) != "" {
		return []string{strings.TrimSpace(s.telegramToken)}
	}
	return nil
}

// ============================================================================
// Google OAuth
// ============================================================================

// GoogleStart возвращает URL редиректа на Google.
// referralCode — опционально тот же ref, что у /register?ref= (сохраняется в OAuth state).
func (s *Service) GoogleStart(referralCode string) (string, error) {
	if s.googleProvider == nil {
		return "", ErrGoogleDisabled
	}
	res, err := s.googleProvider.p.Start(referralCode, 0)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

// GoogleLinkStart — URL редиректа на Google для привязки к уже существующему account_id
// (account_id кладётся в server-side OAuth state; в callback проверяется refresh-cookie).
func (s *Service) GoogleLinkStart(accountID int64) (string, error) {
	if accountID <= 0 {
		return "", fmt.Errorf("%w: bad account id", ErrInvalidInput)
	}
	if s.googleProvider == nil {
		return "", ErrGoogleDisabled
	}
	res, err := s.googleProvider.p.Start("", accountID)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

// GoogleLinkContext — доп. данные при ErrGoogleLinkRequired.
type GoogleLinkContext struct {
	MaskedEmail string // «u***@example.com» — чтобы пользователь понял, куда ждать письмо
}

// GoogleCallbackResult — возвращается из GoogleCallback.
type GoogleCallbackResult struct {
	Pair    *TokenPair         // !=nil при успехе (новый/существующий аккаунт)
	LinkCtx *GoogleLinkContext // !=nil при ErrGoogleLinkRequired
	// SuccessRedirect — при успешной привязке Google к текущему аккаунту: браузерный редирект (вместо JSON).
	SuccessRedirect string
	// WasLinkAttempt — true, если OAuth state был «link»; при ошибке хендлер может редиректить в SPA с query.
	WasLinkAttempt bool
}

type ResolveIdentityStatus string

const (
	IdentityLinkedToCurrent ResolveIdentityStatus = "already_linked"
	IdentityLinkedToOther   ResolveIdentityStatus = "occupied_by_other_user"
	IdentityFree            ResolveIdentityStatus = "identity_new"
)

type ResolveIdentityResult struct {
	Status    ResolveIdentityStatus
	AccountID int64
}

// ResolveIdentity — единая проверка identity по (provider, provider_user_id)
// для Google/Telegram: уже у текущего, у другого, или свободна.
func (s *Service) ResolveIdentity(ctx context.Context, currentAccountID int64, provider, providerUserID string) (*ResolveIdentityResult, error) {
	ident, err := s.ids.FindByProvider(ctx, provider, providerUserID)
	if err == nil {
		if currentAccountID > 0 && ident.AccountID == currentAccountID {
			return &ResolveIdentityResult{Status: IdentityLinkedToCurrent, AccountID: ident.AccountID}, nil
		}
		if currentAccountID > 0 && ident.AccountID != currentAccountID {
			return &ResolveIdentityResult{Status: IdentityLinkedToOther, AccountID: ident.AccountID}, nil
		}
		return &ResolveIdentityResult{Status: IdentityLinkedToCurrent, AccountID: ident.AccountID}, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return &ResolveIdentityResult{Status: IdentityFree}, nil
	}
	return nil, fmt.Errorf("resolve identity: %w", err)
}

// GoogleCallback обрабатывает code + state от Google и выдаёт сессию.
//
// Три ветки:
//  1. Identity(google, sub) найдена → login.
//  2. Email совпадает с существующим аккаунтом → pending link + ErrGoogleLinkRequired.
//  3. Нет совпадений → новый аккаунт + identity → login.
func (s *Service) GoogleCallback(
	ctx context.Context,
	state, code, userAgent, ip, refreshFromCookie string,
) (GoogleCallbackResult, error) {
	var empty GoogleCallbackResult
	if s.googleProvider == nil {
		return empty, ErrGoogleDisabled
	}

	info, referralRaw, linkAccountID, err := s.googleProvider.p.Callback(ctx, state, code)
	if err != nil {
		if errors.Is(err, googleoauth.ErrStateInvalid) {
			return empty, ErrInvalidToken
		}
		return empty, fmt.Errorf("google callback: %w", err)
	}

	if linkAccountID > 0 {
		return s.googleOAuthLinkFlow(ctx, info, referralRaw, linkAccountID, refreshFromCookie, userAgent, ip)
	}

	// 1. Identity уже существует → логиним.
	resolved, err := s.ResolveIdentity(ctx, 0, repository.ProviderGoogle, info.Sub)
	if err != nil {
		return empty, fmt.Errorf("google callback: resolve identity: %w", err)
	}
	if resolved.Status == IdentityLinkedToCurrent && resolved.AccountID > 0 {
		acc, err := s.accounts.FindByID(ctx, resolved.AccountID)
		if err != nil {
			return empty, fmt.Errorf("google callback: load account: %w", err)
		}
		if acc.Status != repository.AccountStatusActive {
			return empty, ErrInvalidCredentials
		}
		_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
		s.ensureCustomer(ctx, acc.ID, acc.Language)
		pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
		if err != nil {
			return empty, err
		}
		s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderGoogle, info.Sub)
		return GoogleCallbackResult{Pair: pair}, nil
	}

	// 2. Identity не найдена. Проверяем email.
	normEmail := normalizeEmail(info.Email)
	if normEmail != "" {
		existingAcc, err := s.accounts.FindByEmail(ctx, normEmail)
		if err == nil {
			// Email занят. Ссылку на привязку шлём, только если Google сам
			// подтвердил этот адрес: иначе Google-аккаунт с чужим неподтверждённым
			// адресом получал бы ссылку в ящик владельца, и её мог открыть
			// почтовый сканер ссылок.
			if !info.EmailVerified {
				return empty, ErrGoogleLinkEmailConflict
			}
			rawProfile, _ := json.Marshal(info)
			return s.requestOAuthEmailLink(ctx, existingAcc, repository.ProviderGoogle, info.Sub, info.Email, rawProfile)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return empty, fmt.Errorf("google callback: find by email: %w", err)
		}
	}

	// 3. Новый пользователь → создаём аккаунт.
	acc, err := s.accounts.Create(ctx, normEmail, "", s.cfg.DefaultLanguage)
	if err != nil {
		return empty, fmt.Errorf("google callback: create account: %w", err)
	}
	// Google сам верифицирует email — сразу помечаем.
	if info.EmailVerified && acc.Email != nil {
		if err := s.accounts.MarkEmailVerified(ctx, acc.ID); err != nil {
			slog.Warn("google callback: mark email verified", "error", err)
		}
		if fresh, ferr := s.accounts.FindByID(ctx, acc.ID); ferr == nil {
			acc = fresh
		}
	}
	rawProfile, _ := json.Marshal(info)
	if _, err := s.ids.Create(ctx, acc.ID, repository.ProviderGoogle, info.Sub, info.Email, rawProfile); err != nil {
		return empty, fmt.Errorf("google callback: create identity: %w", err)
	}
	s.ensureCustomer(ctx, acc.ID, acc.Language)
	s.attachReferralBestEffort(ctx, acc.ID, acc.Language, referralRaw)
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return empty, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderGoogle, info.Sub)
	return GoogleCallbackResult{Pair: pair}, nil
}

// googleOAuthLinkFlow — привязка Google к аккаунту linkAccount_id (после OAuth).
func (s *Service) googleOAuthLinkFlow(
	ctx context.Context,
	info *googleoauth.GoogleUserInfo,
	referralRaw string,
	linkAccountID int64,
	refreshFromCookie, userAgent, ip string,
) (GoogleCallbackResult, error) {
	_ = referralRaw
	linkMeta := GoogleCallbackResult{WasLinkAttempt: true}

	sessAccID, err := s.accountIDFromValidRefresh(ctx, refreshFromCookie)
	if err != nil {
		return linkMeta, err
	}
	if sessAccID != linkAccountID {
		return linkMeta, ErrGoogleLinkSessionMismatch
	}

	resolved, err := s.ResolveIdentity(ctx, linkAccountID, repository.ProviderGoogle, info.Sub)
	if err != nil {
		return linkMeta, fmt.Errorf("google link: resolve identity: %w", err)
	}
	if resolved.Status == IdentityLinkedToCurrent {
		acc, err := s.accounts.FindByID(ctx, linkAccountID)
		if err != nil {
			return linkMeta, fmt.Errorf("google link: load account: %w", err)
		}
		if acc.Status != repository.AccountStatusActive {
			return linkMeta, ErrInvalidCredentials
		}
		_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
		s.ensureCustomer(ctx, acc.ID, acc.Language)
		pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
		if err != nil {
			return linkMeta, err
		}
		s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderGoogle, info.Sub)
		linkMeta.Pair = pair
		linkMeta.SuccessRedirect = "/cabinet/accounts?google_linked=1"
		return linkMeta, nil
	}
	if resolved.Status == IdentityLinkedToOther {
		if s.saveMergeEmailPeerClaim == nil {
			return linkMeta, ErrGoogleLinkedElsewhere
		}
		if err := s.saveMergeEmailPeerClaim(ctx, linkAccountID, resolved.AccountID); err != nil {
			return linkMeta, fmt.Errorf("google link: save merge claim: %w", err)
		}
		return linkMeta, ErrGoogleMergeRequired
	}

	normEmail := normalizeEmail(info.Email)
	if normEmail != "" {
		existingAcc, err := s.accounts.FindByEmail(ctx, normEmail)
		if err == nil && existingAcc.ID != linkAccountID {
			return linkMeta, ErrGoogleLinkEmailConflict
		}
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return linkMeta, fmt.Errorf("google link: find by email: %w", err)
		}
	}

	acc, err := s.accounts.FindByID(ctx, linkAccountID)
	if err != nil {
		return linkMeta, fmt.Errorf("google link: load account: %w", err)
	}
	if acc.Status != repository.AccountStatusActive {
		return linkMeta, ErrInvalidCredentials
	}

	rawProfile, _ := json.Marshal(info)
	if _, err := s.ids.Create(ctx, acc.ID, repository.ProviderGoogle, info.Sub, info.Email, rawProfile); err != nil {
		if pgerr.IsUniqueViolation(err) {
			ident2, e2 := s.ids.FindByProvider(ctx, repository.ProviderGoogle, info.Sub)
			if e2 == nil && ident2.AccountID == acc.ID {
				_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
				s.ensureCustomer(ctx, acc.ID, acc.Language)
				pair, ierr := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
				if ierr != nil {
					return linkMeta, ierr
				}
				s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderGoogle, info.Sub)
				linkMeta.Pair = pair
				linkMeta.SuccessRedirect = "/cabinet/accounts?google_linked=1"
				return linkMeta, nil
			}
			if s.saveMergeEmailPeerClaim == nil {
				return linkMeta, ErrGoogleLinkedElsewhere
			}
			if err := s.saveMergeEmailPeerClaim(ctx, linkAccountID, ident2.AccountID); err != nil {
				return linkMeta, fmt.Errorf("google link: save merge claim after unique: %w", err)
			}
			return linkMeta, ErrGoogleMergeRequired
		}
		return linkMeta, fmt.Errorf("google link: create identity: %w", err)
	}

	if info.EmailVerified && normEmail != "" && acc.Email != nil && normalizeEmail(*acc.Email) == normEmail && !acc.EmailVerified() {
		if err := s.accounts.MarkEmailVerified(ctx, acc.ID); err != nil {
			slog.Warn("google link: mark email verified", "error", err)
		}
		if fresh, ferr := s.accounts.FindByID(ctx, acc.ID); ferr == nil {
			acc = fresh
		}
	}

	_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
	s.ensureCustomer(ctx, acc.ID, acc.Language)
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return linkMeta, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderGoogle, info.Sub)
	linkMeta.Pair = pair
	linkMeta.SuccessRedirect = "/cabinet/accounts?google_linked=1"
	return linkMeta, nil
}

// GoogleLinkConfirm — пользователь кликнул ссылку из письма; привязываем
// Google identity к существующему аккаунту и выдаём сессию.
func (s *Service) GoogleLinkConfirm(ctx context.Context, token, userAgent, ip string) (*TokenPair, error) {
	if s.linkStore == nil {
		// Ни один провайдер не подключён — ожидающих привязок быть не может.
		return nil, ErrInvalidToken
	}
	link, ok := s.linkStore.pop(token)
	if !ok {
		return nil, ErrInvalidToken
	}
	acc, err := s.accounts.FindByID(ctx, link.accountID)
	if err != nil {
		return nil, fmt.Errorf("google link confirm: load account: %w", err)
	}
	if acc.Status != repository.AccountStatusActive {
		return nil, ErrInvalidCredentials
	}
	// Создаём identity. Ошибка UNIQUE-нарушения — допустима (повторный confirm).
	if _, err := s.ids.Create(ctx, acc.ID, link.provider, link.providerUserID, link.providerEmail, link.rawProfile); err != nil {
		slog.Debug("oauth link confirm: create identity (may be duplicate)", "provider", link.provider, "error", err)
	}
	_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
	s.ensureCustomer(ctx, acc.ID, acc.Language)
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return nil, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, link.provider, link.providerUserID)
	return pair, nil
}

type YandexCallbackResult = GoogleCallbackResult
type VKCallbackResult = GoogleCallbackResult

func (s *Service) YandexStart(referralCode string) (string, error) {
	if s.yandexProvider == nil {
		return "", ErrYandexDisabled
	}
	res, err := s.yandexProvider.p.Start(referralCode, 0)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

func (s *Service) VKStart(referralCode string) (string, error) {
	if s.vkProvider == nil {
		return "", ErrVKDisabled
	}
	res, err := s.vkProvider.p.Start(referralCode, 0)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

func (s *Service) YandexLinkStart(accountID int64) (string, error) {
	if accountID <= 0 {
		return "", fmt.Errorf("%w: bad account id", ErrInvalidInput)
	}
	if s.yandexProvider == nil {
		return "", ErrYandexDisabled
	}
	res, err := s.yandexProvider.p.Start("", accountID)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

func (s *Service) VKLinkStart(accountID int64) (string, error) {
	if accountID <= 0 {
		return "", fmt.Errorf("%w: bad account id", ErrInvalidInput)
	}
	if s.vkProvider == nil {
		return "", ErrVKDisabled
	}
	res, err := s.vkProvider.p.Start("", accountID)
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

func (s *Service) YandexCallback(ctx context.Context, state, code, userAgent, ip, refreshFromCookie string) (YandexCallbackResult, error) {
	var empty YandexCallbackResult
	if s.yandexProvider == nil {
		return empty, ErrYandexDisabled
	}
	info, referralRaw, linkAccountID, err := s.yandexProvider.p.Callback(ctx, state, code)
	if err != nil {
		if errors.Is(err, googleoauth.ErrStateInvalid) {
			return empty, ErrInvalidToken
		}
		return empty, fmt.Errorf("yandex callback: %w", err)
	}
	pid := strings.TrimSpace(info.ID)
	email := normalizeEmail(strings.TrimSpace(info.DefaultEmail))
	if email == "" {
		email = normalizeEmail(strings.TrimSpace(info.Email))
	}
	rawProfile, _ := json.Marshal(info)

	if linkAccountID > 0 {
		return s.oauthLinkFlowGeneric(ctx, linkAccountID, refreshFromCookie, repository.ProviderYandex, pid, email, rawProfile, userAgent, ip, "yandex")
	}
	return s.oauthLoginFlowGeneric(ctx, repository.ProviderYandex, pid, email, rawProfile, referralRaw, userAgent, ip, "yandex")
}

func (s *Service) VKCallback(ctx context.Context, state, code, deviceID, userAgent, ip, refreshFromCookie string) (VKCallbackResult, error) {
	var empty VKCallbackResult
	if s.vkProvider == nil {
		return empty, ErrVKDisabled
	}
	info, referralRaw, linkAccountID, err := s.vkProvider.p.Callback(ctx, state, code, deviceID)
	if err != nil {
		if errors.Is(err, googleoauth.ErrStateInvalid) {
			return empty, ErrInvalidToken
		}
		return empty, fmt.Errorf("vk callback: %w", err)
	}
	pid := strconv.FormatInt(info.ID, 10)
	email := normalizeEmail(strings.TrimSpace(info.Email))
	rawProfile, _ := json.Marshal(info)
	if linkAccountID > 0 {
		return s.oauthLinkFlowGeneric(ctx, linkAccountID, refreshFromCookie, repository.ProviderVK, pid, email, rawProfile, userAgent, ip, "vk")
	}
	return s.oauthLoginFlowGeneric(ctx, repository.ProviderVK, pid, email, rawProfile, referralRaw, userAgent, ip, "vk")
}

func (s *Service) oauthLoginFlowGeneric(
	ctx context.Context,
	provider, providerUserID, providerEmail string,
	rawProfile []byte,
	referralRaw, userAgent, ip, providerLog string,
) (GoogleCallbackResult, error) {
	var empty GoogleCallbackResult
	resolved, err := s.ResolveIdentity(ctx, 0, provider, providerUserID)
	if err != nil {
		return empty, fmt.Errorf("%s callback: resolve identity: %w", providerLog, err)
	}
	if resolved.Status == IdentityLinkedToCurrent && resolved.AccountID > 0 {
		acc, err := s.accounts.FindByID(ctx, resolved.AccountID)
		if err != nil {
			return empty, fmt.Errorf("%s callback: load account: %w", providerLog, err)
		}
		if acc.Status != repository.AccountStatusActive {
			return empty, ErrInvalidCredentials
		}
		_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
		s.ensureCustomer(ctx, acc.ID, acc.Language)
		pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
		if err != nil {
			return empty, err
		}
		s.reactivateIdentityAfterOAuth(ctx, acc.ID, provider, providerUserID)
		return GoogleCallbackResult{Pair: pair}, nil
	}

	if providerEmail != "" {
		existingAcc, err := s.accounts.FindByEmail(ctx, providerEmail)
		if err == nil {
			// Раньше аккаунт привязывался сразу — по одному совпадению email,
			// даже если почта у аккаунта не подтверждена.
			return s.requestOAuthEmailLink(ctx, existingAcc, provider, providerUserID, providerEmail, rawProfile)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return empty, fmt.Errorf("%s callback: find by email: %w", providerLog, err)
		}
	}

	acc, err := s.accounts.Create(ctx, providerEmail, "", s.cfg.DefaultLanguage)
	if err != nil {
		return empty, fmt.Errorf("%s callback: create account: %w", providerLog, err)
	}
	if _, err := s.ids.Create(ctx, acc.ID, provider, providerUserID, providerEmail, rawProfile); err != nil {
		return empty, fmt.Errorf("%s callback: create identity: %w", providerLog, err)
	}
	if providerEmail != "" && acc.Email != nil && normalizeEmail(*acc.Email) == providerEmail && !acc.EmailVerified() {
		_ = s.accounts.MarkEmailVerified(ctx, acc.ID)
		if fresh, ferr := s.accounts.FindByID(ctx, acc.ID); ferr == nil {
			acc = fresh
		}
	}
	s.ensureCustomer(ctx, acc.ID, acc.Language)
	s.attachReferralBestEffort(ctx, acc.ID, acc.Language, referralRaw)
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return empty, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, provider, providerUserID)
	return GoogleCallbackResult{Pair: pair}, nil
}

func (s *Service) oauthLinkFlowGeneric(
	ctx context.Context,
	linkAccountID int64,
	refreshFromCookie, provider, providerUserID, providerEmail string,
	rawProfile []byte,
	userAgent, ip, providerLog string,
) (GoogleCallbackResult, error) {
	linkMeta := GoogleCallbackResult{WasLinkAttempt: true}
	sessAccID, err := s.accountIDFromValidRefresh(ctx, refreshFromCookie)
	if err != nil {
		return linkMeta, err
	}
	if sessAccID != linkAccountID {
		return linkMeta, ErrGoogleLinkSessionMismatch
	}
	resolved, err := s.ResolveIdentity(ctx, linkAccountID, provider, providerUserID)
	if err != nil {
		return linkMeta, fmt.Errorf("%s link: resolve identity: %w", providerLog, err)
	}
	if resolved.Status == IdentityLinkedToCurrent {
		acc, err := s.accounts.FindByID(ctx, linkAccountID)
		if err != nil {
			return linkMeta, fmt.Errorf("%s link: load account: %w", providerLog, err)
		}
		pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
		if err != nil {
			return linkMeta, err
		}
		s.reactivateIdentityAfterOAuth(ctx, acc.ID, provider, providerUserID)
		linkMeta.Pair = pair
		linkMeta.SuccessRedirect = "/cabinet/accounts?status=linked&provider=" + provider
		return linkMeta, nil
	}
	if resolved.Status == IdentityLinkedToOther {
		if s.saveMergeEmailPeerClaim != nil && resolved.AccountID > 0 {
			if err := s.saveMergeEmailPeerClaim(ctx, linkAccountID, resolved.AccountID); err != nil {
				return linkMeta, fmt.Errorf("%s link: save merge claim: %w", providerLog, err)
			}
			if provider == repository.ProviderYandex {
				return linkMeta, ErrYandexMergeRequired
			}
			return linkMeta, ErrVKMergeRequired
		}
		return linkMeta, ErrGoogleLinkedElsewhere
	}
	if providerEmail != "" {
		existingAcc, err := s.accounts.FindByEmail(ctx, providerEmail)
		if err == nil && existingAcc.ID != linkAccountID {
			// Как у Google: совпадение email не повод сливать аккаунты. Заявка на
			// слияние без доказательства, что вы владеете вторым аккаунтом,
			// позволила бы поглотить чужой аккаунт через провайдера с чужим
			// неподтверждённым адресом. Объединить свои аккаунты можно через
			// привязку почты — там владение подтверждается паролем или кодом.
			if provider == repository.ProviderYandex {
				return linkMeta, ErrYandexLinkEmailConflict
			}
			return linkMeta, ErrVKLinkEmailConflict
		}
	}
	acc, err := s.accounts.FindByID(ctx, linkAccountID)
	if err != nil {
		return linkMeta, fmt.Errorf("%s link: load account: %w", providerLog, err)
	}
	if _, err := s.ids.Create(ctx, acc.ID, provider, providerUserID, providerEmail, rawProfile); err != nil {
		return linkMeta, fmt.Errorf("%s link: create identity: %w", providerLog, err)
	}
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return linkMeta, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, provider, providerUserID)
	linkMeta.Pair = pair
	linkMeta.SuccessRedirect = "/cabinet/accounts?status=linked&provider=" + provider
	return linkMeta, nil
}

// ============================================================================
// Telegram Login
// ============================================================================

// TelegramWidgetInput — данные от Telegram Login Widget.
type TelegramWidgetInput struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
	Hash      string
	// ReferralCode — опционально ref_<tg> реферера (регистрация по ссылке кабинета).
	ReferralCode string
	UserAgent    string
	IP           string
}

// TelegramMiniAppInput — данные от Telegram Mini App.
type TelegramMiniAppInput struct {
	InitData     string
	ReferralCode string // из start_param WebApp или query; опционально
	UserAgent    string
	IP           string
}

// TelegramLoginWidget аутентифицирует через Telegram Login Widget.
func (s *Service) TelegramLoginWidget(ctx context.Context, in TelegramWidgetInput) (*TokenPair, error) {
	tokens := s.effectiveTelegramTokens()
	if len(tokens) == 0 {
		return nil, ErrTelegramDisabled
	}
	wd := tgverify.WidgetData{
		ID:        in.ID,
		FirstName: in.FirstName,
		LastName:  in.LastName,
		Username:  in.Username,
		PhotoURL:  in.PhotoURL,
		AuthDate:  in.AuthDate,
		Hash:      in.Hash,
	}
	var lastErr error
	verified := false
	for _, token := range tokens {
		if err := tgverify.VerifyWidget(wd, token); err == nil {
			verified = true
			break
		} else {
			lastErr = err
		}
	}
	if !verified {
		return nil, mapTgErr(lastErr)
	}
	return s.telegramFindOrCreate(ctx, in.ID, in.Username, in.ReferralCode, in.UserAgent, in.IP)
}

// TelegramLoginMiniApp аутентифицирует через initData из Telegram Mini App.
func (s *Service) TelegramLoginMiniApp(ctx context.Context, in TelegramMiniAppInput) (*TokenPair, error) {
	tokens := s.effectiveTelegramTokens()
	if len(tokens) == 0 {
		return nil, ErrTelegramDisabled
	}
	var (
		data    *tgverify.MiniAppData
		lastErr error
	)
	for _, token := range tokens {
		parsed, err := tgverify.ParseAndVerifyMiniApp(in.InitData, token)
		if err == nil {
			data = parsed
			break
		}
		lastErr = err
	}
	if data == nil {
		return nil, mapTgErr(lastErr)
	}
	return s.telegramFindOrCreate(ctx, data.UserID, data.Username, in.ReferralCode, in.UserAgent, in.IP)
}

type TelegramOIDCStartInput struct {
	Mode        googleoauth.TelegramOIDCMode
	ReferralRaw string
	AccountID   int64
}

type TelegramOIDCCallbackResult struct {
	Mode              googleoauth.TelegramOIDCMode
	Pair              *TokenPair
	HasMergeCandidate bool
}

func (s *Service) TelegramOIDCStart(in TelegramOIDCStartInput) (string, error) {
	if s.telegramOIDC == nil || s.telegramOIDC.p == nil {
		return "", ErrTelegramOIDCDisabled
	}
	res, err := s.telegramOIDC.p.Start(googleoauth.TelegramOIDCStartInput{
		Mode:        in.Mode,
		ReferralRaw: in.ReferralRaw,
		AccountID:   in.AccountID,
	})
	if err != nil {
		return "", err
	}
	return res.RedirectURL, nil
}

func (s *Service) TelegramOIDCCallback(ctx context.Context, state, code, userAgent, ip, refreshFromCookie string) (*TelegramOIDCCallbackResult, error) {
	if s.telegramOIDC == nil || s.telegramOIDC.p == nil {
		return nil, ErrTelegramOIDCDisabled
	}
	cb, err := s.telegramOIDC.p.Callback(ctx, state, code)
	if err != nil {
		return nil, err
	}
	switch cb.Mode {
	case googleoauth.TelegramOIDCModeLink:
		// Как у Google/Yandex/VK: привязываем только в браузере владельца аккаунта
		// из state. Иначе атакующий отдаёт жертве свою ссылку на oauth.telegram.org,
		// и её Telegram (вплоть до ADMIN_TELEGRAM_ID) привязывается к его аккаунту.
		linkRes := &TelegramOIDCCallbackResult{Mode: cb.Mode}
		sessAccID, err := s.accountIDFromValidRefresh(ctx, refreshFromCookie)
		if err != nil {
			return linkRes, err
		}
		if sessAccID != cb.AccountID {
			return linkRes, ErrGoogleLinkSessionMismatch
		}
		hasMerge, err := s.linkTelegramIdentity(ctx, cb.AccountID, cb.TelegramID, cb.Username)
		if err != nil {
			return nil, err
		}
		return &TelegramOIDCCallbackResult{Mode: cb.Mode, HasMergeCandidate: hasMerge}, nil
	default:
		pair, err := s.telegramFindOrCreate(ctx, cb.TelegramID, cb.Username, cb.ReferralRaw, userAgent, ip)
		if err != nil {
			return nil, err
		}
		return &TelegramOIDCCallbackResult{Mode: googleoauth.TelegramOIDCModeLogin, Pair: pair}, nil
	}
}

func (s *Service) linkTelegramIdentity(ctx context.Context, accountID, tgID int64, username string) (hasMergeCandidate bool, err error) {
	if accountID <= 0 || tgID <= 0 {
		return false, fmt.Errorf("%w: bad telegram link input", ErrInvalidInput)
	}
	ensureCurrentLink := func() *repository.AccountCustomerLink {
		if s.lookupLinks == nil {
			return nil
		}
		link, lerr := s.lookupLinks.FindByAccountID(ctx, accountID)
		if lerr == nil && link != nil {
			return link
		}
		// Self-heal broken/missing link state before merge detection.
		// Without this, telegram with active subscription can be silently linked
		// as identity and skip merge screen.
		acc, aerr := s.accounts.FindByID(ctx, accountID)
		if aerr == nil && acc != nil {
			s.ensureCustomer(ctx, accountID, acc.Language)
			link2, lerr2 := s.lookupLinks.FindByAccountID(ctx, accountID)
			if lerr2 == nil && link2 != nil {
				return link2
			}
		}
		return nil
	}
	// Порядок проверок здесь принципиален.
	//
	// Сначала выясняем, не принадлежит ли этот Telegram ДРУГОМУ кабинет-аккаунту:
	// такой аккаунт нужно поглотить, а для этого merge должен получить claim с
	// PeerAccountID. Раньше первой стояла ветка «просто telegram-claim», она
	// перехватывала почти все случаи, peer в claim не попадал — и после merge
	// второй аккаунт оставался жить рядом с тем же customer (или терял link
	// вместе с подпиской), а его telegram-привязку забирал текущий аккаунт.
	linkedAcc := s.findAccountLinkedToTelegramCustomer(ctx, tgID)
	if linkedAcc == nil && s.lookupLinks != nil {
		pid := strconv.FormatInt(tgID, 10)
		if resolved, rerr := s.ResolveIdentity(ctx, accountID, repository.ProviderTelegram, pid); rerr == nil &&
			resolved.Status == IdentityLinkedToOther && resolved.AccountID > 0 {
			// Peer годится в merge, только если у него есть свой customer:
			// иначе поглощать нечего, а claim без link merge не примет.
			if _, lerr := s.lookupLinks.FindByAccountID(ctx, resolved.AccountID); lerr == nil {
				if acc, aerr := s.accounts.FindByID(ctx, resolved.AccountID); aerr == nil && acc != nil {
					linkedAcc = acc
				}
			}
		}
	}

	// Telegram свободен, но у текущего аккаунта уже другой customer —
	// сливаем клиентов без поглощения аккаунта (второго аккаунта просто нет).
	if linkedAcc == nil && s.lookupCustomers != nil && s.lookupLinks != nil && s.saveMergeTelegramClaim != nil {
		curLink := ensureCurrentLink()
		if curLink != nil {
			tgCust, cerr := s.lookupCustomers.FindByTelegramId(ctx, tgID)
			if cerr == nil && tgCust != nil && tgCust.ID != curLink.CustomerID {
				if err := s.saveMergeTelegramClaim(ctx, accountID, tgID, username); err != nil {
					return false, fmt.Errorf("telegram link: save telegram merge claim: %w", err)
				}
				slog.Info("claim_saved",
					"source", "oidc",
					"kind", "telegram",
					"account_id", accountID,
					"telegram_id", tgID,
				)
				return true, nil
			}
		}
	}
	if linkedAcc != nil && linkedAcc.ID != accountID {
		if s.saveMergeEmailPeerClaim != nil {
			if err := s.saveMergeEmailPeerClaim(ctx, accountID, linkedAcc.ID); err != nil {
				return false, fmt.Errorf("telegram link: save merge claim: %w", err)
			}
			slog.Info("claim_saved",
				"source", "oidc",
				"kind", "email_peer",
				"account_id", accountID,
				"peer_account_id", linkedAcc.ID,
				"telegram_id", tgID,
			)
			return true, nil
		}
		return false, bootstrap.ErrTelegramCustomerLinkedElsewhere
	}

	pid := strconv.FormatInt(tgID, 10)
	resolved, err := s.ResolveIdentity(ctx, accountID, repository.ProviderTelegram, pid)
	if err != nil {
		return false, err
	}
	if resolved.Status == IdentityLinkedToOther {
		if s.saveMergeEmailPeerClaim != nil && resolved.AccountID > 0 {
			if err := s.saveMergeEmailPeerClaim(ctx, accountID, resolved.AccountID); err != nil {
				return false, fmt.Errorf("telegram link: save merge claim by identity: %w", err)
			}
			slog.Info("claim_saved",
				"source", "oidc",
				"kind", "email_peer",
				"account_id", accountID,
				"peer_account_id", resolved.AccountID,
				"telegram_id", tgID,
			)
			return true, nil
		}
		return false, bootstrap.ErrTelegramCustomerLinkedElsewhere
	}
	if resolved.Status == IdentityLinkedToCurrent {
		acc, aerr := s.accounts.FindByID(ctx, accountID)
		if aerr == nil && acc != nil {
			_ = s.ensureCustomerTelegram(ctx, accountID, acc.Language, tgID, username)
		}
		// Already linked to current account; no merge flow required.
		return false, nil
	}
	rawProfile, _ := json.Marshal(map[string]any{"id": tgID, "username": username})
	if _, err := s.ids.Create(ctx, accountID, repository.ProviderTelegram, pid, "", rawProfile); err != nil {
		return false, err
	}
	acc, aerr := s.accounts.FindByID(ctx, accountID)
	if aerr == nil && acc != nil {
		if err := s.ensureCustomerTelegram(ctx, accountID, acc.Language, tgID, username); err != nil {
			return false, err
		}
	}
	// Fresh successful link to current account; no merge flow required.
	return false, nil
}

// findAccountLinkedToTelegramCustomer — аккаунт кабинета, у которого в боте
// уже есть customer с этим telegram_id (после link/merge или promote web).
func (s *Service) findAccountLinkedToTelegramCustomer(ctx context.Context, tgID int64) *repository.Account {
	if s.lookupCustomers == nil || s.lookupLinks == nil {
		return nil
	}
	cust, err := s.lookupCustomers.FindByTelegramId(ctx, tgID)
	if err != nil || cust == nil {
		return nil
	}
	link, err := s.lookupLinks.FindByCustomerID(ctx, cust.ID)
	if err != nil || link == nil {
		return nil
	}
	acc, err := s.accounts.FindByID(ctx, link.AccountID)
	if err != nil || acc == nil || acc.Status != repository.AccountStatusActive {
		return nil
	}
	return acc
}

// preferReassignTelegramIdentity — «осиротевший» tg-only аккаунт (без email)
// уступает аккаунту с почтой/Google, к которому уже привязан этот Telegram в боте.
func preferReassignTelegramIdentity(linked, orphan *repository.Account) bool {
	if linked == nil || orphan == nil || linked.ID == orphan.ID {
		return false
	}
	if linked.Email == nil || strings.TrimSpace(*linked.Email) == "" {
		return false
	}
	return orphan.Email == nil || strings.TrimSpace(*orphan.Email) == ""
}

// telegramFindOrCreate — ищет cabinet_identity(telegram) → login; иначе
// аккаунт по customer.telegram_id + link (после merge); иначе создаёт новый.
func (s *Service) telegramFindOrCreate(ctx context.Context, tgID int64, username, referralCode, userAgent, ip string) (*TokenPair, error) {
	pid := strconv.FormatInt(tgID, 10)
	linkedAcc := s.findAccountLinkedToTelegramCustomer(ctx, tgID)
	resolved, err := s.ResolveIdentity(ctx, 0, repository.ProviderTelegram, pid)
	if err != nil {
		return nil, fmt.Errorf("telegram login: resolve identity: %w", err)
	}
	if resolved.Status == IdentityLinkedToCurrent && resolved.AccountID > 0 {
		acc, err := s.accounts.FindByID(ctx, resolved.AccountID)
		if err != nil {
			return nil, fmt.Errorf("telegram login: load account: %w", err)
		}
		if acc == nil || acc.Status != repository.AccountStatusActive {
			return nil, ErrInvalidCredentials
		}

		if linkedAcc != nil && acc.ID != linkedAcc.ID && preferReassignTelegramIdentity(linkedAcc, acc) {
			if err := s.ids.UpdateTelegramIdentityAccountID(ctx, pid, linkedAcc.ID); err != nil {
				return nil, fmt.Errorf("telegram login: reassign identity: %w", err)
			}
			if _, err := s.sess.RevokeAllForAccount(ctx, acc.ID); err != nil {
				slog.Warn("telegram login: revoke orphan sessions after identity reassign",
					"orphan_account_id", acc.ID, "error", err)
			}
			acc = linkedAcc
		}

		_ = s.accounts.UpdateLastLogin(ctx, acc.ID)
		if err := s.ensureCustomerTelegram(ctx, acc.ID, acc.Language, tgID, username); err != nil {
			return nil, err
		}
		pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
		if err != nil {
			return nil, err
		}
		s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderTelegram, pid)
		return pair, nil
	}

	if linkedAcc != nil {
		rawProfile, _ := json.Marshal(map[string]any{"id": tgID, "username": username})
		if _, err := s.ids.Create(ctx, linkedAcc.ID, repository.ProviderTelegram, pid, "", rawProfile); err != nil {
			return nil, fmt.Errorf("telegram login: create identity on linked account: %w", err)
		}
		_ = s.accounts.UpdateLastLogin(ctx, linkedAcc.ID)
		if err := s.ensureCustomerTelegram(ctx, linkedAcc.ID, linkedAcc.Language, tgID, username); err != nil {
			return nil, err
		}
		pair, err := s.issueSession(ctx, linkedAcc, uuid.New(), userAgent, ip)
		if err != nil {
			return nil, err
		}
		s.reactivateIdentityAfterOAuth(ctx, linkedAcc.ID, repository.ProviderTelegram, pid)
		return pair, nil
	}

	// Новый аккаунт без email/пароля.
	acc, err := s.accounts.Create(ctx, "", "", s.cfg.DefaultLanguage)
	if err != nil {
		return nil, fmt.Errorf("telegram login: create account: %w", err)
	}
	rawProfile, _ := json.Marshal(map[string]any{"id": tgID, "username": username})
	if _, err := s.ids.Create(ctx, acc.ID, repository.ProviderTelegram, pid, "", rawProfile); err != nil {
		return nil, fmt.Errorf("telegram login: create identity: %w", err)
	}
	if err := s.ensureCustomerTelegram(ctx, acc.ID, acc.Language, tgID, username); err != nil {
		return nil, err
	}
	s.attachReferralBestEffort(ctx, acc.ID, acc.Language, referralCode)
	pair, err := s.issueSession(ctx, acc, uuid.New(), userAgent, ip)
	if err != nil {
		return nil, err
	}
	s.reactivateIdentityAfterOAuth(ctx, acc.ID, repository.ProviderTelegram, pid)
	return pair, nil
}

// ============================================================================
// Helpers
// ============================================================================

func (s *Service) reactivateIdentityAfterOAuth(ctx context.Context, accountID int64, provider, providerUserID string) {
	if s.ids == nil || accountID <= 0 || provider == "" || providerUserID == "" {
		return
	}
	if err := s.ids.ClearUnlinkedAtForSubject(ctx, accountID, provider, providerUserID); err != nil {
		slog.Warn("auth: reactivate identity after oauth", "account_id", accountID, "provider", provider, "error", err.Error())
	}
}

func mapTgErr(err error) error {
	switch {
	case errors.Is(err, tgverify.ErrInvalidHash):
		return ErrInvalidCredentials
	case errors.Is(err, tgverify.ErrAuthDateExpired):
		return fmt.Errorf("%w: auth_date expired", ErrInvalidToken)
	case errors.Is(err, tgverify.ErrMissingFields):
		return fmt.Errorf("%w: missing telegram fields", ErrInvalidInput)
	default:
		return err
	}
}

func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at < 1 {
		return email
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 2 {
		return local[:1] + "***" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + domain
}
