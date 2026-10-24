package linking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	tgverify "remnawave-tg-shop-bot/internal/cabinet/auth/telegram"
	"remnawave-tg-shop-bot/internal/cabinet/mail"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// ============================================================================
// Sentinel errors
// ============================================================================

var (
	// ErrNoClaimFound — /merge/preview|confirm без claim: нужен /link/telegram/confirm или привязка email с merge (пароль «чужого» аккаунта).
	ErrNoClaimFound = errors.New("linking: no merge claim; confirm Telegram or complete email link with merge")

	// ErrMergeAlreadyDone — Idempotency-Key уже был использован, merge выполнен ранее.
	ErrMergeAlreadyDone = errors.New("linking: merge already completed (idempotency key reuse)")

	// ErrTelegramDisabled — токен бота не прокинут в сервис.
	ErrTelegramDisabled = errors.New("linking: telegram token not configured")

	// ErrNonceInvalid — nonce не найден или просрочен.
	ErrNonceInvalid = errors.New("linking: nonce invalid or expired")

	// ErrTelegramAlreadyLinked — этот Telegram уже привязан к другому аккаунту кабинета
	// (cabinet_identity или customer↔link), повторная привязка запрещена.
	ErrTelegramAlreadyLinked = errors.New("linking: telegram already linked to another cabinet account")

	// ErrSubscriptionChoiceRequired — у обоих customer есть подписка (expire_at);
	// клиент должен передать keep_subscription web|tg в /link/merge/confirm.
	ErrSubscriptionChoiceRequired = errors.New("linking: subscription keep side required (web or tg)")
)

// ============================================================================
// DTOs
// ============================================================================

// CustomerSnapshot — короткое описание customer для preview.
type CustomerSnapshot struct {
	ID               int64
	ExpireAt         *time.Time
	LoyaltyXP        int64
	ExtraHwid        int
	IsWebOnly        bool
	TelegramID       int64
	SubscriptionLink *string
	CurrentTariffID  *int64
}

// MergePreview — результат dry-run, возвращается из Preview.
type MergePreview struct {
	CustomerWeb     *CustomerSnapshot // nil если нет web-customer
	CustomerTg      *CustomerSnapshot // Telegram customer
	MergedExpireAt  *time.Time        // результирующий expire_at
	MergedLoyaltyXP int64             // суммарный XP
	MergedExtraHwid int               // max(extra_hwid)
	PurchasesMoved  int               // кол-во переносимых purchase
	ReferralsMoved  int               // кол-во переnosимых referral
	IsNoop          bool              // customer_web == customer_tg
	// IsDangerous / DangerReason — устаревшие поля ответа: всегда false/"".
	// Оставлены только ради совместимости JSON-контракта с уже выкаченным
	// фронтендом; решение о слиянии принимает RequiresSubscriptionChoice.
	IsDangerous  bool
	DangerReason string
	// RequiresSubscriptionChoice — оба профиля имеют expire_at; нужен явный выбор подписки (web|tg).
	RequiresSubscriptionChoice bool
	// UISwapSides — подсказка UI: при merge с peer по email при привязанном Telegram
	// у текущего аккаунта в JSON customer_web — peer (email), customer_tg — текущий
	// кабинет; карточки «текущий / найденный» должны брать снимки наоборот от имён полей.
	UISwapSides bool
	// ClaimExpiresAt — срок действия подтверждённого Telegram claim (для таймера в UI).
	ClaimExpiresAt *time.Time
}

// MergeResult — итог реального merge.
type MergeResult struct {
	Result         string // "linked" | "merged" | "noop"
	CustomerID     int64  // итоговый customer_id
	PurchasesMoved int
	ReferralsMoved int
}

// ============================================================================
// MergeService
// ============================================================================

// MergeService реализует поток link/merge.
type MergeService struct {
	pool          *pgxpool.Pool
	nonces        *NonceStore
	claims        *ClaimStore
	customers     *database.CustomerRepository
	links         *repository.AccountCustomerLinkRepo
	auditRepo     *repository.MergeAuditRepo
	mailer        *mail.Mailer
	accounts      *repository.AccountRepo
	identities    *repository.IdentityRepo
	telegramToken string
	remnawave     *remnawave.Client // опционально; если nil — шаг RW пропускается
}

// Config — параметры конструктора.
type Config struct {
	TelegramToken string
}

// New — конструктор.
func New(
	pool *pgxpool.Pool,
	nonces *NonceStore,
	claims *ClaimStore,
	customers *database.CustomerRepository,
	links *repository.AccountCustomerLinkRepo,
	auditRepo *repository.MergeAuditRepo,
	accounts *repository.AccountRepo,
	identities *repository.IdentityRepo,
	mailer *mail.Mailer,
	telegramToken string,
	rw *remnawave.Client,
) *MergeService {
	return &MergeService{
		pool:          pool,
		nonces:        nonces,
		claims:        claims,
		customers:     customers,
		links:         links,
		auditRepo:     auditRepo,
		accounts:      accounts,
		identities:    identities,
		mailer:        mailer,
		telegramToken: telegramToken,
		remnawave:     rw,
	}
}

// ============================================================================
// POST /link/telegram/start
// ============================================================================

// Start генерирует nonce (TTL 10 мин) для использования в Telegram Login Widget.
func (s *MergeService) Start(ctx context.Context, accountID int64) (nonce string, err error) {
	nonce, genErr := generateRandHex(16)
	if genErr != nil {
		return "", fmt.Errorf("linking start: %w", genErr)
	}
	s.nonces.Save(accountID, nonce)
	return nonce, nil
}

// ============================================================================
// POST /link/telegram/confirm
// ============================================================================

// ConfirmInput — тело /link/telegram/confirm.
type ConfirmInput struct {
	Source    string // "widget" | "miniapp"
	Nonce     string // из /start
	UserAgent string
	IP        string

	// Widget-поля
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
	Hash      string

	// MiniApp-поля
	InitData string
}

// Confirm проверяет Telegram payload + nonce, ищет/сохраняет claim.
// После успешного Confirm можно вызывать Preview и Merge.
func (s *MergeService) Confirm(ctx context.Context, accountID int64, in ConfirmInput) (*TelegramClaim, error) {
	if s.telegramToken == "" {
		return nil, ErrTelegramDisabled
	}

	// Проверяем nonce.
	savedNonce, ok := s.nonces.Peek(accountID)
	if !ok {
		return nil, ErrNonceInvalid
	}
	if savedNonce != in.Nonce {
		return nil, ErrNonceInvalid
	}

	// Проверяем Telegram HMAC.
	var tgID int64
	var username string

	switch in.Source {
	case "widget":
		wd := tgverify.WidgetData{
			ID: in.ID, FirstName: in.FirstName, LastName: in.LastName,
			Username: in.Username, PhotoURL: in.PhotoURL,
			AuthDate: in.AuthDate, Hash: in.Hash,
		}
		if err := tgverify.VerifyWidget(wd, s.telegramToken); err != nil {
			return nil, mapTgErr(err)
		}
		tgID, username = in.ID, in.Username

	case "miniapp":
		data, err := tgverify.ParseAndVerifyMiniApp(in.InitData, s.telegramToken)
		if err != nil {
			return nil, mapTgErr(err)
		}
		tgID, username = data.UserID, data.Username

	default:
		return nil, fmt.Errorf("linking confirm: unknown source %q", in.Source)
	}

	if err := s.assertTelegramFreeForAccount(ctx, accountID, tgID); err != nil {
		return nil, err
	}

	// Ищем customer по telegram_id в БД бота.
	var customerTgID *int64
	cust, err := s.customers.FindByTelegramId(ctx, tgID)
	if err == nil && cust != nil {
		id := cust.ID
		customerTgID = &id
	}

	// Потребляем nonce — одноразовый.
	s.nonces.Consume(accountID)

	claim := TelegramClaim{
		TelegramID:       tgID,
		TelegramUsername: username,
		CustomerTgID:     customerTgID,
	}
	s.claims.Save(accountID, claim)
	slog.Info("claim_saved",
		"source", in.Source,
		"kind", "telegram",
		"account_id", accountID,
		"telegram_id", tgID,
		"has_customer_tg_id", customerTgID != nil,
	)
	return &claim, nil
}

// SaveEmailPeerClaim сохраняет merge-claim для текущей сессии после проверки пароля
// аккаунта peer (email уже занят у peer). Дальше клиент идёт на /link/merge как после Telegram.
func (s *MergeService) SaveEmailPeerClaim(ctx context.Context, currentAccountID, peerAccountID int64) error {
	if currentAccountID <= 0 || peerAccountID <= 0 || currentAccountID == peerAccountID {
		return fmt.Errorf("linking: invalid account ids for email peer claim")
	}
	linkPeer, err := s.links.FindByAccountID(ctx, peerAccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("linking: peer account has no customer link")
		}
		return fmt.Errorf("linking: peer link: %w", err)
	}
	peerCustomer, err := s.customers.FindById(ctx, linkPeer.CustomerID)
	if err != nil {
		return fmt.Errorf("linking: peer customer: %w", err)
	}
	if peerCustomer == nil {
		return fmt.Errorf("linking: peer customer not found")
	}

	var currentCustomer *database.Customer
	if linkCur, errCur := s.links.FindByAccountID(ctx, currentAccountID); errCur == nil && linkCur != nil {
		if linkCur.CustomerID == linkPeer.CustomerID {
			return fmt.Errorf("linking: peer customer same as current")
		}
		if c, cerr := s.customers.FindById(ctx, linkCur.CustomerID); cerr == nil {
			currentCustomer = c
		}
	}

	// Реальный Telegram для claim. Приоритет у Telegram ТЕКУЩЕГО аккаунта: бот
	// адресует клиента по telegram_id, и подменять «свой» Telegram чужим при
	// слиянии нельзя. Если настоящего Telegram нет ни у одной стороны, в claim
	// уходит 0: синтетический id web-only клиента Telegram'ом не является и
	// привязкой стать не должен.
	currentIdentityTG, _ := s.telegramIDFromIdentity(ctx, currentAccountID)
	peerIdentityTG, _ := s.telegramIDFromIdentity(ctx, peerAccountID)
	claimTelegramID := firstRealTelegramID(
		currentIdentityTG,
		telegramIDOf(currentCustomer),
		peerIdentityTG,
		telegramIDOf(peerCustomer),
	)

	tgUser := trimmedTelegramUsername(peerCustomer)
	if tgUser == "" {
		tgUser = trimmedTelegramUsername(currentCustomer)
	}

	cid := peerCustomer.ID
	s.claims.Save(currentAccountID, TelegramClaim{
		TelegramID:       claimTelegramID,
		TelegramUsername: tgUser,
		CustomerTgID:     &cid,
		PeerAccountID:    peerAccountID,
	})
	slog.Info("claim_saved",
		"source", "email_peer",
		"kind", "email_peer",
		"account_id", currentAccountID,
		"peer_account_id", peerAccountID,
		"has_real_telegram", claimTelegramID > 0,
	)
	return nil
}

func trimmedTelegramUsername(c *database.Customer) string {
	if c == nil || c.TelegramUsername == nil {
		return ""
	}
	return strings.TrimSpace(*c.TelegramUsername)
}

// telegramIDFromIdentity возвращает real Telegram ID из cabinet_identity(account, provider=telegram).
func (s *MergeService) telegramIDFromIdentity(ctx context.Context, accountID int64) (int64, bool) {
	if s.pool == nil || accountID <= 0 {
		return 0, false
	}
	var providerUID string
	err := s.pool.QueryRow(ctx, `
		SELECT provider_user_id
		  FROM cabinet_identity
		 WHERE account_id = $1 AND provider = $2
		 ORDER BY id DESC
		 LIMIT 1`,
		accountID, repository.ProviderTelegram,
	).Scan(&providerUID)
	if err != nil {
		return 0, false
	}
	parsed, perr := strconv.ParseInt(strings.TrimSpace(providerUID), 10, 64)
	if perr != nil {
		return 0, false
	}
	// Синтетический id — не Telegram, см. realTelegramID.
	if real := realTelegramID(parsed); real > 0 {
		return real, true
	}
	return 0, false
}

// SaveTelegramOIDCClaim сохраняет merge-claim после Telegram OIDC link/start callback.
// Используется auth/service для маршрута /auth/telegram/callback, чтобы /link/merge/preview
// имел валидный claim и не падал merge_claim_missing.
func (s *MergeService) SaveTelegramOIDCClaim(ctx context.Context, currentAccountID, telegramID int64, telegramUsername string) error {
	if currentAccountID <= 0 || telegramID <= 0 {
		return fmt.Errorf("linking: invalid account or telegram id for telegram claim")
	}
	var customerTgID *int64
	cust, err := s.customers.FindByTelegramId(ctx, telegramID)
	if err == nil && cust != nil {
		id := cust.ID
		customerTgID = &id
	}
	claim := TelegramClaim{
		TelegramID:       telegramID,
		TelegramUsername: strings.TrimSpace(telegramUsername),
		CustomerTgID:     customerTgID,
	}
	s.claims.Save(currentAccountID, claim)
	slog.Info("claim_saved",
		"source", "oidc",
		"kind", "telegram",
		"account_id", currentAccountID,
		"telegram_id", telegramID,
		"has_customer_tg_id", customerTgID != nil,
	)
	return nil
}

// ============================================================================
// POST /link/merge/preview (dry-run)
// ============================================================================

// Preview выполняет dry-run merge и возвращает ожидаемые изменения.
// Транзакция в конце делает ROLLBACK, БД не меняется.
func (s *MergeService) Preview(ctx context.Context, accountID int64) (*MergePreview, error) {
	claim, ok := s.claims.Get(accountID)
	if !ok {
		return nil, ErrNoClaimFound
	}
	preview, _, _, err := s.doMerge(ctx, accountID, claim, true, false, "", "preview")
	if preview != nil {
		exp := claim.ExpiresAt
		preview.ClaimExpiresAt = &exp
	}
	return preview, err
}

// ============================================================================
// POST /link/merge/confirm
// ============================================================================

// Merge выполняет реальный merge с Idempotency-Key.
// idempotencyKey — строка из заголовка Idempotency-Key.
// force=true снимает защиту от «опасного» merge.
func (s *MergeService) Merge(ctx context.Context, accountID int64, idempotencyKey string, force bool, keepSubscription string) (*MergeResult, error) {
	// Проверяем идемпотентность: уже выполнен?
	existing, err := s.auditRepo.FindByIdempotencyKey(ctx, accountID, idempotencyKey)
	if err == nil {
		// Нашли — merge уже выполнен ранее; возвращаем cached result.
		cid := int64(0)
		if existing.TargetCustomerID != nil {
			cid = *existing.TargetCustomerID
		}
		return &MergeResult{
			Result:     existing.Result,
			CustomerID: cid,
		}, ErrMergeAlreadyDone
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("linking merge: find audit: %w", err)
	}

	claim, ok := s.claims.Get(accountID)
	if !ok {
		return nil, ErrNoClaimFound
	}

	preview, mergeResult, _, err := s.doMerge(ctx, accountID, claim, false, force, keepSubscription, idempotencyKey)
	_ = preview // не нужен в ответе на confirm
	if err != nil {
		return nil, err
	}

	// Успех — удаляем claim.
	s.claims.Delete(accountID)

	return mergeResult, nil
}

// ============================================================================
// Ядро merge (используется и в dry-run, и в реальном merge)
// ============================================================================

// keepWeb / keepTg — значения keep_subscription.
const (
	keepWeb = "web"
	keepTg  = "tg"
)

// mergeSides — обе стороны слияния после разбора claim.
type mergeSides struct {
	web *database.Customer
	tg  *database.Customer
	// uiSwap — см. MergePreview.UISwapSides.
	uiSwap bool
	// finalTelegramID — реальный Telegram, который обязан оказаться у выжившего.
	// 0 означает «реального Telegram в слиянии нет» (два web-аккаунта).
	finalTelegramID int64
	// absorb — кабинет-аккаунты, которые обязаны исчезнуть в пользу текущего:
	// peer из claim и владельцы участвующих customer. Без этого после merge
	// на одном customer остались бы два аккаунта, либо второй аккаунт остался
	// бы жив, но без подписки.
	absorb []int64
}

// mergePlan — форма конкретного слияния.
type mergePlan struct {
	result          string // repository.MergeResult*
	reason          string
	finalCustomerID int64
	purchasesMoved  int
	referralsMoved  int
	isNoop          bool

	// rwWinner/rwLoser — стороны с точки зрения ПОДПИСКИ, а не строки customer.
	// Профиль панели проигравшей стороны удаляется, профиль выигравшей
	// переезжает на выжившего клиента вместе с remnawave_user_id.
	rwWinner *database.Customer
	rwLoser  *database.Customer
}

// resolveSides блокирует и раскладывает участников merge по сторонам.
func (s *MergeService) resolveSides(ctx context.Context, tx pgx.Tx, accountID int64, claim *TelegramClaim) (mergeSides, error) {
	var sides mergeSides

	// customer текущего аккаунта.
	curCustomerID, hasLink, err := lockLinkedCustomerID(ctx, tx, accountID)
	if err != nil {
		return sides, fmt.Errorf("linking: read current link: %w", err)
	}
	if hasLink {
		sides.web, err = lockCustomerByID(ctx, tx, curCustomerID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return sides, fmt.Errorf("linking: lock customer_web: %w", err)
			}
			sides.web = nil
		}
	}
	currentOwnCustomer := sides.web

	// customer со стороны Telegram.
	if claim.CustomerTgID != nil {
		sides.tg, err = lockCustomerByID(ctx, tx, *claim.CustomerTgID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return sides, fmt.Errorf("linking: lock customer_tg: %w", err)
			}
			// Протухший in-memory claim: id мог исчезнуть после admin/sync.
			sides.tg = nil
		}
	}
	if sides.tg == nil && realTelegramID(claim.TelegramID) > 0 {
		sides.tg, err = lockCustomerByTelegramID(ctx, tx, claim.TelegramID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return sides, fmt.Errorf("linking: lock customer_tg by telegram_id: %w", err)
			}
			sides.tg = nil
		}
	}

	// Инвариант Telegram-first: если у аккаунта уже есть telegram identity,
	// каноническая tg-сторона — customer именно с этим реальным telegram_id.
	currentTelegramID, currentHasTelegram := s.telegramIDFromIdentityTx(ctx, tx, accountID)
	if currentHasTelegram {
		canonical, terr := lockCustomerByTelegramID(ctx, tx, currentTelegramID)
		if terr != nil && !errors.Is(terr, repository.ErrNotFound) {
			return sides, fmt.Errorf("linking: lock current telegram customer: %w", terr)
		}
		if terr == nil && canonical != nil {
			sides.tg = canonical
			claim.TelegramID = currentTelegramID
		}
	}

	// Merge по email/OAuth: вторая сторона — customer peer-аккаунта.
	if claim.PeerAccountID > 0 && claim.PeerAccountID != accountID {
		peerCustomerID, peerHasLink, perr := lockLinkedCustomerID(ctx, tx, claim.PeerAccountID)
		if perr != nil {
			return sides, fmt.Errorf("linking: peer link lookup: %w", perr)
		}
		if peerHasLink {
			peerCustomer, lockErr := lockCustomerByID(ctx, tx, peerCustomerID)
			if lockErr != nil && !errors.Is(lockErr, repository.ErrNotFound) {
				return sides, fmt.Errorf("linking: lock peer customer: %w", lockErr)
			}
			if lockErr == nil && peerCustomer != nil {
				switch {
				case sides.tg != nil && sides.tg.ID != peerCustomer.ID:
					// tg-сторона занята каноническим Telegram-клиентом:
					// peer встаёт web-стороной, карточки в UI меняются местами.
					sides.web = peerCustomer
					sides.uiSwap = currentOwnCustomer == nil || currentOwnCustomer.ID != peerCustomer.ID
				case sides.tg == nil:
					sides.tg = peerCustomer
				}
			}
		}
		sides.absorb = appendUnique(sides.absorb, claim.PeerAccountID)
	}

	// Собственный customer текущего аккаунта может не попасть ни в одну из
	// сторон: так бывает, если у аккаунта есть telegram identity, но link ведёт
	// на третьего клиента. Слить троих за один проход мы не умеем — фиксируем
	// это в логе, иначе строка молча осталась бы без владельца.
	if currentOwnCustomer != nil &&
		(sides.web == nil || sides.web.ID != currentOwnCustomer.ID) &&
		(sides.tg == nil || sides.tg.ID != currentOwnCustomer.ID) {
		slog.Warn("linking: current account customer is not part of the merge; it will be left unlinked",
			"account_id", accountID, "customer_id", currentOwnCustomer.ID)
	}

	// Реальный Telegram выжившего. Приоритет у Telegram ТЕКУЩЕГО аккаунта:
	// бот адресует клиента по telegram_id, и подменять «свой» Telegram чужим
	// при слиянии нельзя.
	sides.finalTelegramID = firstRealTelegramID(
		currentTelegramID,
		telegramIDOf(currentOwnCustomer),
		claim.TelegramID,
		telegramIDOf(sides.tg),
		telegramIDOf(sides.web),
	)

	// Любой второй аккаунт, висящий на участвующих customer, поглощается.
	for _, c := range []*database.Customer{sides.web, sides.tg} {
		if c == nil {
			continue
		}
		owners, oerr := accountsLinkedToCustomer(ctx, tx, c.ID)
		if oerr != nil {
			return sides, fmt.Errorf("linking: owners of customer %d: %w", c.ID, oerr)
		}
		for _, owner := range owners {
			if owner != accountID {
				sides.absorb = appendUnique(sides.absorb, owner)
			}
		}
	}

	return sides, nil
}

// doMerge выполняет merge-транзакцию.
// dryRun=true → ROLLBACK в конце, preview заполнен, mergeResult=nil.
// dryRun=false → COMMIT, mergeResult заполнен.
//
// Устройство: сначала раскладываем стороны и считаем preview, затем одна
// ветка мутаций по форме merge, затем ОБЩИЙ хвост (link, поглощение вторых
// аккаунтов, identity, аудит, commit). Общий хвост принципиален: раньше
// ветки noop/linked выходили до него, и привязки email/соцсетей второго
// аккаунта молча терялись, а merge при этом рапортовал успех.
func (s *MergeService) doMerge(
	ctx context.Context,
	accountID int64,
	claim TelegramClaim,
	dryRun bool,
	force bool,
	keepSubscription string,
	idempotencyKey string,
) (preview *MergePreview, result *MergeResult, auditIn repository.MergeAuditCreateInput, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, nil, auditIn, fmt.Errorf("linking: begin tx: %w", err)
	}

	// При любом выходе — rollback если не закоммичено.
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	sides, err := s.resolveSides(ctx, tx, accountID, &claim)
	if err != nil {
		return nil, nil, auditIn, err
	}
	custWeb, custTg := sides.web, sides.tg
	finalTelegramID := sides.finalTelegramID

	preview = &MergePreview{UISwapSides: sides.uiSwap}
	if custWeb != nil {
		preview.CustomerWeb = snapshotCustomer(custWeb)
	}
	if custTg != nil {
		preview.CustomerTg = snapshotCustomer(custTg)
	}

	fullMerge := custWeb != nil && custTg != nil && custWeb.ID != custTg.ID
	preview.IsNoop = (custWeb == nil && custTg == nil) ||
		(custWeb != nil && custTg != nil && custWeb.ID == custTg.ID)

	// ─────────────────────────────────────────────────────────────
	// Preview: подписка, накопления, необходимость выбора
	// ─────────────────────────────────────────────────────────────
	keep := strings.TrimSpace(strings.ToLower(keepSubscription))
	mergedXP := int64(0)
	mergedExtraHwid := 0

	if fullMerge {
		preview.PurchasesMoved, err = countPurchases(ctx, tx, custWeb.ID)
		if err != nil {
			return nil, nil, auditIn, fmt.Errorf("linking: count purchases: %w", err)
		}
		preview.ReferralsMoved, err = countReferrals(ctx, tx, custWeb.TelegramID)
		if err != nil {
			return nil, nil, auditIn, fmt.Errorf("linking: count referrals: %w", err)
		}

		mergedXP = custWeb.LoyaltyXP + custTg.LoyaltyXP
		mergedExtraHwid = maxInt(custWeb.ExtraHwid, custTg.ExtraHwid)
		preview.MergedLoyaltyXP = mergedXP
		preview.MergedExtraHwid = mergedExtraHwid

		reqChoice := subscriptionChoiceRequired(custWeb, custTg)
		preview.RequiresSubscriptionChoice = reqChoice

		if keep != keepWeb && keep != keepTg {
			switch {
			case !reqChoice:
				keep = defaultKeepSide(custWeb, custTg)
			case force:
				// Легаси-обходной путь: без явного выбора берём Telegram-сторону.
				keep = keepTg
			case dryRun:
				preview.MergedExpireAt = nil
				auditIn = auditInput(accountID, &custWeb.ID, &custTg.ID, repository.MergeResultDryRun, "dry_run", idempotencyKey, true)
				_, _ = s.auditRepo.Create(ctx, tx, auditIn)
				return preview, nil, auditIn, nil
			default:
				return preview, nil, auditIn, ErrSubscriptionChoiceRequired
			}
		}
		if keep == keepWeb {
			preview.MergedExpireAt = custWeb.ExpireAt
		} else {
			preview.MergedExpireAt = custTg.ExpireAt
		}
	} else if alive := firstNonNilCustomer(custTg, custWeb); alive != nil {
		preview.MergedExpireAt = alive.ExpireAt
		preview.MergedLoyaltyXP = alive.LoyaltyXP
		preview.MergedExtraHwid = alive.ExtraHwid
	}

	if dryRun {
		auditIn = auditInput(accountID, optionalCustomerID(custWeb), optionalCustomerID(custTg),
			repository.MergeResultDryRun, "dry_run", idempotencyKey, true)
		_, _ = s.auditRepo.Create(ctx, tx, auditIn)
		return preview, nil, auditIn, nil
	}

	// ─────────────────────────────────────────────────────────────
	// Мутации по форме merge
	// ─────────────────────────────────────────────────────────────
	var plan mergePlan
	switch {
	case custWeb == nil && custTg == nil:
		plan = mergePlan{result: repository.MergeResultLinked, reason: "no customers to merge", isNoop: true}

	case custTg == nil:
		// Telegram не имеет клиента в боте: поднимаем web-клиента на реальный id.
		if finalTelegramID > 0 {
			if err := promoteWebCustomer(ctx, tx, custWeb.ID, custWeb.TelegramID, finalTelegramID); err != nil {
				return nil, nil, auditIn, fmt.Errorf("linking: promote web customer: %w", err)
			}
		}
		plan = mergePlan{
			result: repository.MergeResultLinked, reason: "telegram linked to web customer",
			finalCustomerID: custWeb.ID, rwWinner: custWeb,
		}

	case custWeb == nil:
		plan = mergePlan{
			result: repository.MergeResultLinked, reason: "link to existing tg customer",
			finalCustomerID: custTg.ID, rwWinner: custTg,
		}

	case custWeb.ID == custTg.ID:
		plan = mergePlan{
			result: repository.MergeResultLinked, reason: "noop: same customer",
			finalCustomerID: custTg.ID, rwWinner: custTg, isNoop: true,
		}

	default:
		plan, err = s.executeFullMerge(ctx, tx, fullMergeInput{
			custWeb:         custWeb,
			custTg:          custTg,
			keep:            keep,
			finalTelegramID: finalTelegramID,
			mergedXP:        mergedXP,
			mergedExtraHwid: mergedExtraHwid,
			purchasesMoved:  preview.PurchasesMoved,
			referralsMoved:  preview.ReferralsMoved,
		})
		if err != nil {
			return nil, nil, auditIn, err
		}
	}

	// ─────────────────────────────────────────────────────────────
	// Общий хвост: одинаков для всех форм merge
	// ─────────────────────────────────────────────────────────────
	if plan.finalCustomerID > 0 {
		if err := upsertAccountLink(ctx, tx, accountID, plan.finalCustomerID); err != nil {
			return nil, nil, auditIn, fmt.Errorf("linking: upsert link: %w", err)
		}
	}

	for _, peerID := range sides.absorb {
		if err := s.absorbAccountTx(ctx, tx, accountID, peerID); err != nil {
			return nil, nil, auditIn, err
		}
	}
	if len(sides.absorb) > 0 {
		if err := s.ensureCabinetEmailIdentityTx(ctx, tx, accountID); err != nil {
			return nil, nil, auditIn, err
		}
	}
	if finalTelegramID > 0 {
		if err := s.ensureCabinetTelegramIdentityTx(ctx, tx, accountID, finalTelegramID); err != nil {
			return nil, nil, auditIn, err
		}
	}

	auditIn = auditInput(accountID, optionalCustomerID(custWeb), optionalCustomerID(custTg),
		plan.result, plan.reason, idempotencyKey, false)
	if _, err := s.auditRepo.Create(ctx, tx, auditIn); err != nil {
		if !errors.Is(err, repository.ErrMergeAuditConflict) {
			return nil, nil, auditIn, fmt.Errorf("linking: write audit: %w", err)
		}
	}

	slog.Info("merge_decision",
		"stage", "pre_commit",
		"account_id", accountID,
		"keep_subscription", keep,
		"requires_choice", preview.RequiresSubscriptionChoice,
		"final_customer_id", plan.finalCustomerID,
		"final_telegram_id", finalTelegramID,
		"absorbed_accounts", len(sides.absorb),
	)

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, auditIn, fmt.Errorf("linking: commit merge: %w", err)
	}
	committed = true

	// Панель трогаем только после успешного коммита: иначе упавшая транзакция
	// оставила бы клиента без профиля в Remnawave.
	s.remnawaveAfterMerge(ctx, plan.rwLoser, plan.rwWinner, plan.finalCustomerID, finalTelegramID, claim.TelegramUsername)

	resultKind := "merged"
	switch {
	case plan.isNoop:
		resultKind = "noop"
	case plan.result != repository.MergeResultMerged:
		resultKind = "linked"
	}
	slog.Info("merge_decision",
		"stage", "post_commit",
		"account_id", accountID,
		"result", resultKind,
		"final_customer_id", plan.finalCustomerID,
	)
	return preview, &MergeResult{
		Result:         resultKind,
		CustomerID:     plan.finalCustomerID,
		PurchasesMoved: plan.purchasesMoved,
		ReferralsMoved: plan.referralsMoved,
	}, auditIn, nil
}

// fullMergeInput — параметры слияния двух разных customer.
type fullMergeInput struct {
	custWeb         *database.Customer
	custTg          *database.Customer
	keep            string
	finalTelegramID int64
	mergedXP        int64
	mergedExtraHwid int
	purchasesMoved  int
	referralsMoved  int
}

// executeFullMerge сливает две разные строки customer в одну.
//
// Здесь две НЕЗАВИСИМЫЕ оси, которые раньше путались между собой:
//   - какая СТРОКА customer выживает — см. survivingCustomerRow;
//   - чья ПОДПИСКА выживает — определяется keep.
//
// При keep=web с привязанным Telegram выживает tg-строка, но забирает поля
// подписки И профиль панели с web-стороны. Раньше профиль панели всегда брали
// с tg-стороны, а web-профиль удаляли — то есть выбранная пользователем
// подписка оставалась в БД ссылкой на удалённый профиль.
func (s *MergeService) executeFullMerge(ctx context.Context, tx pgx.Tx, in fullMergeInput) (mergePlan, error) {
	var plan mergePlan
	custWeb, custTg := in.custWeb, in.custTg

	fieldWinner, fieldLoser := custTg, custWeb
	if in.keep == keepWeb {
		fieldWinner, fieldLoser = custWeb, custTg
	}
	survivor, doomed := survivingCustomerRow(custWeb, custTg, in.finalTelegramID)

	// Всё, что привязано к обречённой СТРОКЕ, переезжает на выжившую.
	if err := movePurchases(ctx, tx, doomed.ID, survivor.ID); err != nil {
		return plan, fmt.Errorf("linking: move purchases: %w", err)
	}
	if err := movePerCustomerRecords(ctx, tx, doomed.ID, survivor.ID); err != nil {
		return plan, fmt.Errorf("linking: move per-customer records: %w", err)
	}

	// UNIQUE(customer.telegram_id): снимаем id с обречённой строки до того,
	// как выживший его займёт.
	if in.finalTelegramID > 0 && doomed.TelegramID == in.finalTelegramID {
		if _, err := tx.Exec(ctx, `UPDATE customer SET telegram_id = $2 WHERE id = $1`,
			doomed.ID, mergeTempTelegramID(doomed.ID)); err != nil {
			return plan, fmt.Errorf("linking: detach doomed telegram_id: %w", err)
		}
	}
	// Частичный UNIQUE(remnawave_user_id): освобождаем привязку панели, чтобы
	// выживший мог забрать профиль победителя подписки.
	if _, err := tx.Exec(ctx,
		`UPDATE customer SET remnawave_user_id = NULL, remnawave_short_uuid = NULL WHERE id = $1`,
		doomed.ID); err != nil {
		return plan, fmt.Errorf("linking: detach doomed panel id: %w", err)
	}

	targetTelegramID := in.finalTelegramID
	if targetTelegramID == 0 {
		// Реального Telegram в слиянии нет — выживший остаётся на своём
		// синтетическом id и остаётся web-only.
		targetTelegramID = survivor.TelegramID
	}
	if err := rebindCustomerTelegram(ctx, tx, survivor.ID, survivor.TelegramID, targetTelegramID, in.finalTelegramID > 0); err != nil {
		return plan, fmt.Errorf("linking: rebind survivor telegram_id: %w", err)
	}
	if err := moveReferrals(ctx, tx, doomed.TelegramID, targetTelegramID); err != nil {
		return plan, fmt.Errorf("linking: move referrals: %w", err)
	}

	if err := applyWinnerMergedFields(ctx, tx, survivor.ID, fieldWinner, fieldLoser,
		in.mergedXP, in.mergedExtraHwid, in.finalTelegramID > 0); err != nil {
		return plan, fmt.Errorf("linking: apply merge fields: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM customer WHERE id = $1`, doomed.ID); err != nil {
		return plan, fmt.Errorf("linking: delete merged customer: %w", err)
	}

	return mergePlan{
		result:          repository.MergeResultMerged,
		reason:          "merged_keep_" + in.keep,
		finalCustomerID: survivor.ID,
		purchasesMoved:  in.purchasesMoved,
		referralsMoved:  in.referralsMoved,
		rwWinner:        fieldWinner,
		rwLoser:         fieldLoser,
	}, nil
}

// ============================================================================
// SQL helpers (all use pgx.Tx)
// ============================================================================

func lockCustomerByID(ctx context.Context, tx pgx.Tx, id int64) (*database.Customer, error) {
	const q = `SELECT id, telegram_id, expire_at, created_at, subscription_link, language,
		extra_hwid, extra_hwid_expires_at, current_tariff_id, subscription_period_start,
		subscription_period_months, loyalty_xp, telegram_username, is_web_only,
		remnawave_user_id, remnawave_short_uuid, broadcast_opt_out
		FROM customer WHERE id = $1 FOR UPDATE`
	row := tx.QueryRow(ctx, q, id)
	var c database.Customer
	err := row.Scan(
		&c.ID, &c.TelegramID, &c.ExpireAt, &c.CreatedAt, &c.SubscriptionLink, &c.Language,
		&c.ExtraHwid, &c.ExtraHwidExpiresAt, &c.CurrentTariffID, &c.SubscriptionPeriodStart,
		&c.SubscriptionPeriodMonths, &c.LoyaltyXP, &c.TelegramUsername, &c.IsWebOnly,
		&c.RemnawaveUserID, &c.RemnawaveShortUUID, &c.BroadcastOptOut,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("lock customer %d: %w", id, err)
	}
	return &c, nil
}

func lockCustomerByTelegramID(ctx context.Context, tx pgx.Tx, telegramID int64) (*database.Customer, error) {
	const q = `SELECT id, telegram_id, expire_at, created_at, subscription_link, language,
		extra_hwid, extra_hwid_expires_at, current_tariff_id, subscription_period_start,
		subscription_period_months, loyalty_xp, telegram_username, is_web_only,
		remnawave_user_id, remnawave_short_uuid, broadcast_opt_out
		FROM customer WHERE telegram_id = $1 FOR UPDATE`
	row := tx.QueryRow(ctx, q, telegramID)
	var c database.Customer
	err := row.Scan(
		&c.ID, &c.TelegramID, &c.ExpireAt, &c.CreatedAt, &c.SubscriptionLink, &c.Language,
		&c.ExtraHwid, &c.ExtraHwidExpiresAt, &c.CurrentTariffID, &c.SubscriptionPeriodStart,
		&c.SubscriptionPeriodMonths, &c.LoyaltyXP, &c.TelegramUsername, &c.IsWebOnly,
		&c.RemnawaveUserID, &c.RemnawaveShortUUID, &c.BroadcastOptOut,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("lock customer by telegram_id %d: %w", telegramID, err)
	}
	return &c, nil
}

func countPurchases(ctx context.Context, tx pgx.Tx, customerID int64) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM purchase WHERE customer_id = $1`, customerID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func countReferrals(ctx context.Context, tx pgx.Tx, telegramID int64) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM referral WHERE referrer_id = $1 OR referee_id = $1`, telegramID).Scan(&n)
	return n, err
}

func movePurchases(ctx context.Context, tx pgx.Tx, fromID, toID int64) error {
	_, err := tx.Exec(ctx, `UPDATE purchase SET customer_id = $2 WHERE customer_id = $1`, fromID, toID)
	return err
}

func moveReferrals(ctx context.Context, tx pgx.Tx, fromTgID, toTgID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE referral SET referrer_id = $2 WHERE referrer_id = $1`, fromTgID, toTgID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE referral SET referee_id = $2 WHERE referee_id = $1`, fromTgID, toTgID); err != nil {
		return err
	}
	// После merge возможен self-referral (referrer_id == referee_id), его нужно удалить.
	_, err := tx.Exec(ctx, `DELETE FROM referral WHERE referrer_id = referee_id`)
	return err
}

// applyWinnerMergedFields переносит на выжившую строку поля выбранной стороны.
//
// hasRealTelegram определяет is_web_only: клиент web-only ровно тогда, когда у
// него нет настоящего Telegram. Раньше флаг снимался безусловно, и после
// слияния двух web-аккаунтов клиент с синтетическим id переставал считаться
// web-only.
//
// remnawave_user_id/short_uuid тоже переезжают с победителя: иначе выживший
// ссылался бы на профиль панели проигравшей стороны, который merge удаляет.
func applyWinnerMergedFields(ctx context.Context, tx pgx.Tx, targetID int64, winner, loser *database.Customer, mergedXP int64, mergedExtra int, hasRealTelegram bool) error {
	extraExp := winner.ExtraHwidExpiresAt
	if loser.ExtraHwidExpiresAt != nil {
		if extraExp == nil || loser.ExtraHwidExpiresAt.After(*extraExp) {
			extraExp = loser.ExtraHwidExpiresAt
		}
	}
	incomingUsername := pickTelegramUsername(winner, loser)
	var unameArg any
	if incomingUsername != nil {
		unameArg = *incomingUsername
	}
	_, err := tx.Exec(ctx, `
		UPDATE customer SET
			expire_at                     = $2,
			subscription_link             = $3,
			loyalty_xp                    = $4,
			extra_hwid                    = $5,
			extra_hwid_expires_at         = $6,
			current_tariff_id             = $7,
			subscription_period_start     = $8,
			subscription_period_months    = $9,
			is_web_only                   = $11,
			remnawave_user_id             = $12,
			remnawave_short_uuid          = $13,
			telegram_username             = COALESCE(telegram_username, $10),
			broadcast_opt_out             = $14
		WHERE id = $1`,
		targetID,
		winner.ExpireAt,
		winner.SubscriptionLink,
		mergedXP,
		mergedExtra,
		extraExp,
		winner.CurrentTariffID,
		winner.SubscriptionPeriodStart,
		winner.SubscriptionPeriodMonths,
		unameArg,
		!hasRealTelegram,
		winner.RemnawaveUserID,
		winner.RemnawaveShortUUID,
		winner.BroadcastOptOut || loser.BroadcastOptOut,
	)
	return err
}

func pickTelegramUsername(a, b *database.Customer) *string {
	if a.TelegramUsername != nil && strings.TrimSpace(*a.TelegramUsername) != "" {
		return a.TelegramUsername
	}
	if b.TelegramUsername != nil && strings.TrimSpace(*b.TelegramUsername) != "" {
		return b.TelegramUsername
	}
	return nil
}

// mergeTempTelegramID — временный telegram_id при merge (keep web), чтобы
// освободить реальный id под customer_web (UNIQUE customer.telegram_id).
func mergeTempTelegramID(customerID int64) int64 {
	return -(100_000_000_000_000 + customerID)
}

// promoteWebCustomer переводит web-only customer на реальный telegram_id и переносит
// строки referral с synthetic id (требуются DEFERRABLE FK на referral).
func promoteWebCustomer(ctx context.Context, tx pgx.Tx, customerID, oldTelegramID, newTelegramID int64) error {
	if _, err := tx.Exec(ctx,
		`UPDATE customer SET is_web_only = FALSE, telegram_id = $2 WHERE id = $1`,
		customerID, newTelegramID); err != nil {
		return err
	}
	return moveReferrals(ctx, tx, oldTelegramID, newTelegramID)
}

// rebindCustomerTelegram ensures the winner customer is attached to a real telegram_id
// and moves referral rows from old telegram_id to the new one.
func rebindCustomerTelegram(ctx context.Context, tx pgx.Tx, customerID, oldTelegramID, newTelegramID int64, hasRealTelegram bool) error {
	if oldTelegramID == newTelegramID {
		_, err := tx.Exec(ctx, `UPDATE customer SET is_web_only = $2 WHERE id = $1`, customerID, !hasRealTelegram)
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customer SET is_web_only = $3, telegram_id = $2 WHERE id = $1`,
		customerID, newTelegramID, !hasRealTelegram); err != nil {
		return err
	}
	return moveReferrals(ctx, tx, oldTelegramID, newTelegramID)
}

// ============================================================================
// Merge helpers (правила 10.2)
// ============================================================================

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func valOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func snapshotCustomer(c *database.Customer) *CustomerSnapshot {
	return &CustomerSnapshot{
		ID:               c.ID,
		ExpireAt:         c.ExpireAt,
		LoyaltyXP:        c.LoyaltyXP,
		ExtraHwid:        c.ExtraHwid,
		IsWebOnly:        c.IsWebOnly,
		TelegramID:       c.TelegramID,
		SubscriptionLink: c.SubscriptionLink,
		CurrentTariffID:  c.CurrentTariffID,
	}
}

func auditInput(accountID int64, srcID, dstID *int64, result, reason, ikey string, dryRun bool) repository.MergeAuditCreateInput {
	return repository.MergeAuditCreateInput{
		AccountID:        accountID,
		SourceCustomerID: srcID,
		TargetCustomerID: dstID,
		Actor:            repository.MergeActorUser,
		Result:           result,
		Reason:           reason,
		DryRun:           dryRun,
		IdempotencyKey:   ikey,
	}
}

// ============================================================================
// Misc helpers
// ============================================================================

func (s *MergeService) ensureCabinetTelegramIdentityTx(ctx context.Context, tx pgx.Tx, accountID, telegramID int64) error {
	if accountID <= 0 || telegramID <= 0 {
		return fmt.Errorf("linking: ensure telegram identity tx: invalid ids")
	}
	// Синтетический id web-only клиента — не Telegram. Такая привязка показала
	// бы в кабинете «Telegram подключён» с несуществующим аккаунтом и навсегда
	// закрыла бы возможность привязать настоящий.
	if utils.IsSyntheticTelegramID(telegramID) {
		return fmt.Errorf("linking: refusing to store synthetic telegram id as identity")
	}
	pid := strconv.FormatInt(telegramID, 10)
	if _, err := tx.Exec(ctx, `
		INSERT INTO cabinet_identity (account_id, provider, provider_user_id, provider_email, raw_profile_json)
		VALUES ($1, $2, $3, NULL, NULL)
		ON CONFLICT (provider, provider_user_id)
		DO UPDATE SET account_id = EXCLUDED.account_id`,
		accountID, repository.ProviderTelegram, pid); err != nil {
		return fmt.Errorf("linking: ensure telegram identity tx: upsert identity: %w", err)
	}
	return nil
}

// assertTelegramFreeForAccount — один Telegram user id не может быть привязан
// к двум разным cabinet_account (ни через identity, ни через customer↔link).
func (s *MergeService) assertTelegramFreeForAccount(ctx context.Context, accountID, tgID int64) error {
	pid := strconv.FormatInt(tgID, 10)
	if s.identities != nil {
		ident, err := s.identities.FindByProvider(ctx, repository.ProviderTelegram, pid)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("linking: check telegram identity: %w", err)
		}
		if ident != nil && ident.AccountID != accountID {
			return ErrTelegramAlreadyLinked
		}
	}
	cust, err := s.customers.FindByTelegramId(ctx, tgID)
	if err != nil {
		return fmt.Errorf("linking: find customer by telegram: %w", err)
	}
	if cust == nil {
		return nil
	}
	link, err := s.links.FindByCustomerID(ctx, cust.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("linking: find link by customer: %w", err)
	}
	if link.AccountID != accountID {
		return ErrTelegramAlreadyLinked
	}
	return nil
}

func mapTgErr(err error) error {
	switch {
	case errors.Is(err, tgverify.ErrInvalidHash):
		return fmt.Errorf("telegram: invalid signature")
	case errors.Is(err, tgverify.ErrAuthDateExpired):
		return fmt.Errorf("telegram: auth_date expired")
	case errors.Is(err, tgverify.ErrMissingFields):
		return fmt.Errorf("telegram: missing required fields")
	default:
		return err
	}
}

func generateRandHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// ============================================================================
// Хелперы разбора сторон
// ============================================================================

// realTelegramID возвращает id, только если это НАСТОЯЩИЙ Telegram.
// Синтетические id web-only клиентов — внутренний суррогат, а не Telegram:
// превращать их в telegram-привязку нельзя (иначе в кабинете появится
// «Telegram привязан» с несуществующим id, и настоящий уже не привяжешь).
func realTelegramID(id int64) int64 {
	if id <= 0 || utils.IsSyntheticTelegramID(id) {
		return 0
	}
	return id
}

func telegramIDOf(c *database.Customer) int64 {
	if c == nil {
		return 0
	}
	return c.TelegramID
}

// firstRealTelegramID — первый настоящий Telegram из списка кандидатов.
func firstRealTelegramID(candidates ...int64) int64 {
	for _, c := range candidates {
		if real := realTelegramID(c); real > 0 {
			return real
		}
	}
	return 0
}

func firstNonNilCustomer(cs ...*database.Customer) *database.Customer {
	for _, c := range cs {
		if c != nil {
			return c
		}
	}
	return nil
}

func optionalCustomerID(c *database.Customer) *int64 {
	if c == nil {
		return nil
	}
	id := c.ID
	return &id
}

func appendUnique(dst []int64, v int64) []int64 {
	if v <= 0 {
		return dst
	}
	for _, existing := range dst {
		if existing == v {
			return dst
		}
	}
	return append(dst, v)
}

// survivingCustomerRow — какая СТРОКА customer переживает слияние.
//
// Правило одно и намеренно простое: если в слиянии участвует реальный Telegram,
// выживает строка, которая им уже владеет — её знает бот, на неё ссылается
// история. Иначе выживает строка текущего кабинет-аккаунта (web-сторона).
//
// Обратите внимание: это НЕ то же самое, что выбор подписки. Выжившая строка
// может забрать поля подписки у противоположной стороны (keep).
func survivingCustomerRow(web, tg *database.Customer, finalTelegramID int64) (survivor, doomed *database.Customer) {
	if finalTelegramID > 0 {
		if tg != nil && tg.TelegramID == finalTelegramID {
			return tg, web
		}
		if web != nil && web.TelegramID == finalTelegramID {
			return web, tg
		}
	}
	return web, tg
}

// isActiveAt — подписка живая на момент now.
func isActiveAt(expireAt *time.Time, now time.Time) bool {
	return expireAt != nil && expireAt.After(now)
}

// subscriptionChoiceRequired — спрашивать пользователя нужно, только когда
// обе подписки СЕЙЧАС живые: выбор между живой и давно истёкшей — не выбор,
// а лишний шанс случайно потерять активную подписку.
func subscriptionChoiceRequired(web, tg *database.Customer) bool {
	if web == nil || tg == nil {
		return false
	}
	now := time.Now()
	return isActiveAt(web.ExpireAt, now) && isActiveAt(tg.ExpireAt, now)
}

// defaultKeepSide — сторона, побеждающая без явного выбора пользователя:
// живая подписка важнее истёкшей, среди равных — та, что заканчивается позже.
func defaultKeepSide(web, tg *database.Customer) string {
	now := time.Now()
	webActive := isActiveAt(web.ExpireAt, now)
	tgActive := isActiveAt(tg.ExpireAt, now)
	switch {
	case webActive && !tgActive:
		return keepWeb
	case tgActive && !webActive:
		return keepTg
	}
	if web.ExpireAt != nil && (tg.ExpireAt == nil || web.ExpireAt.After(*tg.ExpireAt)) {
		return keepWeb
	}
	return keepTg
}

// ============================================================================
// SQL-хелперы связей и переноса данных
// ============================================================================

// lockLinkedCustomerID читает и блокирует link аккаунта ВНУТРИ транзакции.
// Через пул читать нельзя: merge меняет эту же строку, и чтение мимо tx
// давало бы решение по устаревшему снимку.
func lockLinkedCustomerID(ctx context.Context, tx pgx.Tx, accountID int64) (int64, bool, error) {
	var customerID int64
	err := tx.QueryRow(ctx,
		`SELECT customer_id FROM cabinet_account_customer_link WHERE account_id = $1 FOR UPDATE`,
		accountID).Scan(&customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return customerID, true, nil
}

// accountsLinkedToCustomer — все кабинет-аккаунты, висящие на этом customer.
func accountsLinkedToCustomer(ctx context.Context, tx pgx.Tx, customerID int64) ([]int64, error) {
	rows, err := tx.Query(ctx,
		`SELECT account_id FROM cabinet_account_customer_link WHERE customer_id = $1 FOR UPDATE`,
		customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// upsertAccountLink гарантирует, что аккаунт указывает на выжившего customer.
// Именно upsert, а не UPDATE: если bootstrap когда-то не доработал и строки
// link нет, UPDATE молча затронул бы 0 строк, а merge отрапортовал бы успех,
// оставив аккаунт вообще без клиента.
func upsertAccountLink(ctx context.Context, tx pgx.Tx, accountID, customerID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO cabinet_account_customer_link (account_id, customer_id, link_status)
		VALUES ($1, $2, 'linked')
		ON CONFLICT (account_id) DO UPDATE
		   SET customer_id = EXCLUDED.customer_id,
		       link_status = 'linked',
		       updated_at  = NOW()`,
		accountID, customerID)
	return err
}

// movePerCustomerRecords переносит на выжившего всё, что привязано к customer_id
// и влияет на будущие права клиента.
//
// Без этого DELETE FROM customer уносил такие строки каскадом: одноразовый
// промокод становился доступен заново, отложенная скидка и история колеса
// исчезали, а дедуп lifecycle-уведомлений сбрасывался (дубли писем).
// Конфликтующие строки не переносим — они и так уже есть у выжившего.
func movePerCustomerRecords(ctx context.Context, tx pgx.Tx, fromID, toID int64) error {
	stmts := []string{
		`UPDATE promo_redemption r SET customer_id = $2
		  WHERE r.customer_id = $1
		    AND NOT EXISTS (SELECT 1 FROM promo_redemption x
		                     WHERE x.customer_id = $2 AND x.promo_code_id = r.promo_code_id)`,
		`UPDATE fortune_spins SET customer_id = $2 WHERE customer_id = $1`,
		`UPDATE customer_lifecycle_notify_sent n SET customer_id = $2
		  WHERE n.customer_id = $1
		    AND NOT EXISTS (SELECT 1 FROM customer_lifecycle_notify_sent x
		                     WHERE x.customer_id = $2 AND x.kind = n.kind
		                       AND x.reference_key = n.reference_key)`,
		`UPDATE customer_pending_discount SET customer_id = $2
		  WHERE customer_id = $1
		    AND NOT EXISTS (SELECT 1 FROM customer_pending_discount x WHERE x.customer_id = $2)`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q, fromID, toID); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// Remnawave: приведение панели в соответствие результату merge
// ============================================================================

// remnawaveAfterMerge удаляет профиль проигравшей стороны и переносит профиль
// выигравшей на выжившего клиента. Best-effort: панель может быть недоступна,
// но merge в БД уже закоммичен и откатываться из-за этого не должен.
//
// Порядок важен: сначала резолвим ОБА профиля, затем удаляем проигравшего и
// только потом патчим победителя — иначе telegram_id остался бы занят
// удаляемым профилем и PATCH упал бы с «telegramId already taken».
func (s *MergeService) remnawaveAfterMerge(
	ctx context.Context,
	loser, winner *database.Customer,
	finalCustomerID, finalTelegramID int64,
	telegramUsername string,
) {
	if s.remnawave == nil || finalCustomerID <= 0 {
		return
	}
	_ = config.RemnawaveTag() // config используется и в других ветках merge

	users, listErr := s.remnawave.GetUsers(ctx)
	if listErr != nil {
		slog.Warn("linking: remnawave list users failed (non-fatal)",
			"final_customer_id", finalCustomerID, "error", listErr)
	}

	winnerProfile := s.resolvePanelProfile(ctx, users, winner)
	loserProfile := s.resolvePanelProfile(ctx, users, loser)

	if loserProfile != nil && (winnerProfile == nil || loserProfile.ID != winnerProfile.ID) {
		if err := s.remnawave.DeleteUser(ctx, loserProfile.ID); err != nil {
			slog.Warn("linking: remnawave delete loser user failed (non-fatal)",
				"user_id", loserProfile.ID, "error", err)
		} else {
			slog.Info("linking: remnawave loser user deleted",
				"user_id", loserProfile.ID, "username", loserProfile.Username)
		}
	}

	if winnerProfile == nil {
		slog.Info("linking: remnawave winner profile not found; skip winner sync",
			"final_customer_id", finalCustomerID)
		return
	}

	// Профиль остаётся под старым именем "<староеCustomerID>_<старыйTelegramID>":
	// Remnawave 3.3.2 молча игнорирует username в PATCH /api/users — запрос
	// проходит с 200, а значение не меняется (зафиксировано контрактным тестом
	// TestContractMergePanelHandover). Значит, поиск профиля по префиксу
	// "<customerID>_" после merge неверен, и единственная надёжная привязка —
	// remnawave_user_id, который мы фиксируем ниже.
	patchOK := true
	if finalTelegramID > 0 {
		req := &remnawave.UpdateUserRequest{ID: &winnerProfile.ID}
		tid := finalTelegramID
		req.TelegramID = &tid
		if name := strings.TrimSpace(telegramUsername); name != "" {
			req.Description = &name
		}
		if _, err := s.remnawave.PatchUser(ctx, req); err != nil {
			patchOK = false
			// Именно Error: клиент в БД уже слит, но профиль в панели остался на
			// прежнем telegram_id, и бот не найдёт его поиском по Telegram.
			// Лечится повторной синхронизацией клиента из админки.
			slog.Error("linking: remnawave winner rebind failed; panel is out of sync with merged customer",
				"final_customer_id", finalCustomerID, "user_id", winnerProfile.ID,
				"telegram_id", finalTelegramID, "error", err)
		} else {
			slog.Info("linking: remnawave winner synced",
				"final_customer_id", finalCustomerID, "user_id", winnerProfile.ID,
				"telegram_id", finalTelegramID)
		}
	}

	// Привязка панели к выжившему: следующий резолв пойдёт коротким путём по id,
	// а не по эвристикам вокруг устаревшего имени профиля.
	if s.customers != nil && patchOK {
		if err := s.customers.SetRemnawaveIdentity(ctx, finalCustomerID, winnerProfile.ID, winnerProfile.ShortUUID); err != nil {
			slog.Warn("linking: persist remnawave identity after merge (non-fatal)",
				"final_customer_id", finalCustomerID, "user_id", winnerProfile.ID, "error", err)
		}
	}
}

// resolvePanelProfile ищет профиль панели конкретного клиента.
// Порядок: сохранённый remnawave_user_id → ссылка подписки → имя
// "<customerID>_" → реальный telegram_id.
//
// Префикс имени стоит третьим не случайно: панель не даёт переименовывать
// профиль (username в PATCH игнорируется), поэтому у уже слитого клиента имя
// осталось от прежнего владельца. Надёжен только сохранённый id.
func (s *MergeService) resolvePanelProfile(ctx context.Context, users []remnawave.User, c *database.Customer) *remnawave.User {
	if c == nil {
		return nil
	}
	if u := pickPanelProfile(users, c); u != nil {
		return u
	}
	// Список не пришёл (панель недоступна или отдала ошибку) — пробуем точечно.
	if len(users) == 0 && c.RemnawaveUserID != nil && *c.RemnawaveUserID > 0 {
		if u, err := s.remnawave.GetUserByID(ctx, *c.RemnawaveUserID); err == nil && u != nil {
			return u
		}
	}
	if len(users) == 0 && realTelegramID(c.TelegramID) > 0 {
		if u, err := s.remnawave.GetUserTrafficInfo(ctx, c.TelegramID); err == nil && u != nil {
			return u
		}
	}
	return nil
}

func pickPanelProfile(users []remnawave.User, c *database.Customer) *remnawave.User {
	if c == nil || len(users) == 0 {
		return nil
	}
	if c.RemnawaveUserID != nil && *c.RemnawaveUserID > 0 {
		for i := range users {
			if users[i].ID == *c.RemnawaveUserID {
				return &users[i]
			}
		}
	}
	if sub := strings.TrimSpace(valOrEmpty(c.SubscriptionLink)); sub != "" {
		for i := range users {
			if strings.TrimSpace(users[i].SubscriptionUrl) == sub {
				return &users[i]
			}
		}
	}
	prefix := strconv.FormatInt(c.ID, 10) + "_"
	for i := range users {
		if strings.HasPrefix(strings.TrimSpace(users[i].Username), prefix) {
			return &users[i]
		}
	}
	if tg := realTelegramID(c.TelegramID); tg > 0 {
		for i := range users {
			if users[i].TelegramID != nil && *users[i].TelegramID == tg {
				return &users[i]
			}
		}
	}
	return nil
}

// telegramIDFromIdentityTx — как telegramIDFromIdentity, но внутри транзакции
// merge: решение о канонической tg-стороне нельзя принимать по снимку мимо tx.
func (s *MergeService) telegramIDFromIdentityTx(ctx context.Context, tx pgx.Tx, accountID int64) (int64, bool) {
	if accountID <= 0 {
		return 0, false
	}
	var providerUID string
	err := tx.QueryRow(ctx, `
		SELECT provider_user_id
		  FROM cabinet_identity
		 WHERE account_id = $1 AND provider = $2
		 ORDER BY id DESC
		 LIMIT 1`,
		accountID, repository.ProviderTelegram,
	).Scan(&providerUID)
	if err != nil {
		return 0, false
	}
	parsed, perr := strconv.ParseInt(strings.TrimSpace(providerUID), 10, 64)
	if perr != nil {
		return 0, false
	}
	if real := realTelegramID(parsed); real > 0 {
		return real, true
	}
	return 0, false
}

// absorbAccountTx поглощает второй кабинет-аккаунт в выживший.
//
// Вызывается для КАЖДОГО аккаунта, участвовавшего в слиянии: и для peer из
// claim, и для владельца customer с противоположной стороны. Иначе после merge
// на одного клиента оставалось бы два аккаунта (второй продолжал бы видеть
// чужую подписку), либо второй аккаунт оставался бы жив, но с удалённым link —
// то есть без подписки и покупок.
//
// Правило по email: выживший сохраняет СВОЙ парольный логин, если он есть.
// Забирать чужой email поверх своего нельзя — это молча отобрало бы у человека
// способ входа. Email-identity поглощаемого не переносим вовсе: её
// provider_user_id — это id удаляемого аккаунта, после удаления она мусор.
func (s *MergeService) absorbAccountTx(ctx context.Context, tx pgx.Tx, survivorID, peerID int64) error {
	if survivorID <= 0 || peerID <= 0 || survivorID == peerID {
		return fmt.Errorf("linking: absorb account: invalid ids")
	}

	var peerEmail, peerPwd *string
	var peerVerified *time.Time
	err := tx.QueryRow(ctx, `
		SELECT email, password_hash, email_verified_at
		  FROM cabinet_account WHERE id = $1 FOR UPDATE`, peerID).
		Scan(&peerEmail, &peerPwd, &peerVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // уже поглощён/удалён
		}
		return fmt.Errorf("linking: absorb account: read peer: %w", err)
	}

	var survivorPwd *string
	if err := tx.QueryRow(ctx,
		`SELECT password_hash FROM cabinet_account WHERE id = $1 FOR UPDATE`, survivorID).
		Scan(&survivorPwd); err != nil {
		return fmt.Errorf("linking: absorb account: read survivor: %w", err)
	}

	// Соц- и telegram-привязки переезжают; дубли (provider, provider_user_id)
	// пропускаем — такой способ входа у выжившего уже есть.
	if _, err := tx.Exec(ctx, `
		UPDATE cabinet_identity AS ci
		   SET account_id = $1
		 WHERE ci.account_id = $2
		   AND ci.provider <> $3
		   AND NOT EXISTS (
		     SELECT 1 FROM cabinet_identity s
		      WHERE s.account_id = $1
		        AND s.provider = ci.provider
		        AND s.provider_user_id = ci.provider_user_id
		   )`,
		survivorID, peerID, repository.ProviderEmail); err != nil {
		return fmt.Errorf("linking: absorb account: move identities: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cabinet_identity WHERE account_id = $1`, peerID); err != nil {
		return fmt.Errorf("linking: absorb account: cleanup peer identities: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, peerID); err != nil {
		return fmt.Errorf("linking: absorb account: delete peer: %w", err)
	}

	peerHasEmailLogin := peerEmail != nil && strings.TrimSpace(*peerEmail) != ""
	survivorHasEmailLogin := survivorPwd != nil && strings.TrimSpace(*survivorPwd) != ""
	switch {
	case peerHasEmailLogin && !survivorHasEmailLogin:
		if _, err := tx.Exec(ctx, `
			UPDATE cabinet_account
			   SET email = $2, password_hash = $3, email_verified_at = $4, updated_at = NOW()
			 WHERE id = $1`,
			survivorID, strings.TrimSpace(strings.ToLower(*peerEmail)), peerPwd, peerVerified); err != nil {
			return fmt.Errorf("linking: absorb account: move email login: %w", err)
		}
	case peerHasEmailLogin:
		slog.Info("linking: absorb keeps survivor own email login",
			"survivor_account_id", survivorID, "peer_account_id", peerID)
	}
	return nil
}

// ensureCabinetEmailIdentityTx создаёт identity для парольного логина выжившего.
func (s *MergeService) ensureCabinetEmailIdentityTx(ctx context.Context, tx pgx.Tx, accountID int64) error {
	var email *string
	if err := tx.QueryRow(ctx, `SELECT email FROM cabinet_account WHERE id = $1`, accountID).Scan(&email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("linking: ensure email identity tx: load account: %w", err)
	}
	if email == nil || strings.TrimSpace(*email) == "" {
		return nil
	}
	pid := strconv.FormatInt(accountID, 10)
	if _, err := tx.Exec(ctx, `
		INSERT INTO cabinet_identity (account_id, provider, provider_user_id, provider_email, raw_profile_json)
		VALUES ($1, $2, $3, $4, NULL)
		ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		accountID, repository.ProviderEmail, pid, strings.TrimSpace(*email)); err != nil {
		return fmt.Errorf("linking: ensure email identity tx: insert identity: %w", err)
	}
	return nil
}
