//go:build integration

package linking

// Инфраструктура интеграционных тестов merge: живой PostgreSQL + фейковая
// панель Remnawave, отвечающая по контракту 3.3.2.
//
// Запуск из корня репозитория:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@host:port/db?sslmode=disable \
//	  go test ./internal/cabinet/linking/... -tags=integration -count=1
//
// БД должна быть ОДНОРАЗОВОЙ: тесты гоняют миграции и создают/удаляют строки.
// Каждый тест работает в своём диапазоне telegram_id и убирает за собой в Cleanup.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// ============================================================================
// Фейковая панель Remnawave (контракт 3.3.2)
// ============================================================================

// fakePanel — минимальная реализация тех эндпоинтов панели, которые дёргает
// merge: stream-листинг, DELETE и PATCH пользователя.
//
// Специально сделан на httptest, а не на Go-интерфейсе: merge ходит в панель
// настоящим *remnawave.Client, и тест обязан проверять реальные формы запроса
// и ответа 3.3.2 (users/stream вместо by-telegram-id, 204 без тела на DELETE,
// числовой id вместо uuid). Подмена интерфейсом это бы спрятала.
type fakePanel struct {
	mu      sync.Mutex
	server  *httptest.Server
	users   map[int64]*remnawave.User
	deleted []int64
	patched []remnawave.UpdateUserRequest
	nextID  int64

	// listErr — если true, /api/users/stream отвечает 500 (проверка «панель
	// недоступна не должна ломать уже закоммиченный merge»).
	listErr bool
}

func newFakePanel(t *testing.T) *fakePanel {
	t.Helper()
	p := &fakePanel{users: make(map[int64]*remnawave.User), nextID: 1000}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users/stream", p.handleStream)
	mux.HandleFunc("/api/users", p.handleUsersRoot)
	mux.HandleFunc("/api/users/", p.handleUserByID)
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakePanel) client() *remnawave.Client {
	return remnawave.NewClient(p.server.URL, "test-token", "")
}

// addUser заводит профиль в панели так, как его создаёт бот:
// username = "<customerID>_<telegramID>".
func (p *fakePanel) addUser(customerID, telegramID int64, subURL string, expireAt time.Time) *remnawave.User {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	id := p.nextID
	u := &remnawave.User{
		ID:              id,
		ShortUUID:       fmt.Sprintf("short%d", id),
		Username:        fmt.Sprintf("%d_%d", customerID, telegramID),
		SubscriptionUrl: subURL,
		ExpireAt:        expireAt,
		Status:          "ACTIVE",
	}
	// Синтетический telegram_id в панель не уходит — так делает и боевой код.
	if telegramID > 0 && !utils.IsSyntheticTelegramID(telegramID) {
		tg := telegramID
		u.TelegramID = &tg
	}
	p.users[id] = u
	return u
}

func (p *fakePanel) get(id int64) *remnawave.User {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, ok := p.users[id]
	if !ok {
		return nil
	}
	cp := *u
	return &cp
}

func (p *fakePanel) alive() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]int64, 0, len(p.users))
	for id := range p.users {
		out = append(out, id)
	}
	return out
}

func (p *fakePanel) deletedIDs() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int64(nil), p.deleted...)
}

func (p *fakePanel) writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (p *fakePanel) handleStream(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	failing := p.listErr
	p.mu.Unlock()
	if failing {
		http.Error(w, `{"message":"panel down"}`, http.StatusInternalServerError)
		return
	}

	filterTG := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("telegramId")); raw != "" {
		filterTG, _ = strconv.ParseInt(raw, 10, 64)
	}

	p.mu.Lock()
	out := make([]remnawave.User, 0, len(p.users))
	for _, u := range p.users {
		if filterTG != 0 && (u.TelegramID == nil || *u.TelegramID != filterTG) {
			continue
		}
		out = append(out, *u)
	}
	p.mu.Unlock()

	p.writeJSON(w, http.StatusOK, map[string]any{
		"response": map[string]any{
			"users":      out,
			"hasMore":    false,
			"nextCursor": nil,
		},
	})
}

// handleUsersRoot — PATCH /api/users (в 3.x правка идёт в корень с id в теле).
func (p *fakePanel) handleUsersRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req remnawave.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message":"bad body"}`, http.StatusBadRequest)
		return
	}
	if req.ID == nil {
		http.Error(w, `{"message":"id required"}`, http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	u, ok := p.users[*req.ID]
	if !ok {
		p.mu.Unlock()
		http.Error(w, `{"message":"user not found"}`, http.StatusNotFound)
		return
	}
	p.patched = append(p.patched, req)
	if req.TelegramID != nil {
		// Фейк СТРОЖЕ живой панели: 3.3.2 дубль telegramId принимает
		// (см. TestContractMergeTelegramIDUniqueness). Запрет оставлен намеренно —
		// он ловит ошибки порядка «сначала удалить проигравшего».
		for id, other := range p.users {
			if id == u.ID {
				continue
			}
			if other.TelegramID != nil && *other.TelegramID == *req.TelegramID {
				p.mu.Unlock()
				http.Error(w, `{"message":"telegramId already taken"}`, http.StatusBadRequest)
				return
			}
		}
		tg := *req.TelegramID
		u.TelegramID = &tg
	}
	// username живая панель 3.3.2 в PATCH игнорирует — повторяем это,
	// чтобы merge не начал полагаться на переименование профиля.
	if req.Description != nil {
		d := *req.Description
		u.Description = &d
	}
	if req.ExpireAt != nil {
		u.ExpireAt = *req.ExpireAt
	}
	cp := *u
	p.mu.Unlock()

	p.writeJSON(w, http.StatusOK, map[string]any{"response": cp})
}

// handleUserByID — GET /api/users/{id} и DELETE /api/users/{id}.
func (p *fakePanel) handleUserByID(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if raw == "" || strings.Contains(raw, "/") {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		http.Error(w, `{"message":"bad id"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p.mu.Lock()
		u, ok := p.users[id]
		var cp remnawave.User
		if ok {
			cp = *u
		}
		p.mu.Unlock()
		if !ok {
			http.Error(w, `{"message":"user not found"}`, http.StatusNotFound)
			return
		}
		p.writeJSON(w, http.StatusOK, map[string]any{"response": cp})
	case http.MethodDelete:
		p.mu.Lock()
		_, ok := p.users[id]
		if ok {
			delete(p.users, id)
			p.deleted = append(p.deleted, id)
		}
		p.mu.Unlock()
		if !ok {
			http.Error(w, `{"message":"user not found"}`, http.StatusNotFound)
			return
		}
		// 3.0.0: 204 без тела.
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// Окружение теста
// ============================================================================

// mergeEnv — всё, что нужно одному тесту merge.
type mergeEnv struct {
	t     *testing.T
	ctx   context.Context
	pool  *pgxpool.Pool
	svc   *MergeService
	panel *fakePanel

	accounts   *repository.AccountRepo
	links      *repository.AccountCustomerLinkRepo
	identities *repository.IdentityRepo
	customers  *database.CustomerRepository

	// tgBase — начало диапазона «реальных» telegram_id этого теста.
	tgBase int64
	seq    int64

	createdAccounts  []int64
	createdCustomers []int64
	createdPromos    []int64
	createdTariffs   []int64
}

var (
	sharedPool     *pgxpool.Pool
	sharedPoolOnce sync.Once
	tgCounter      int64
)

func migrationsDirLinking(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	// .../internal/cabinet/linking/<file> → корень репозитория (3× ..)
	return filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", "..", "db", "migrations"))
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CABINET_INTEGRATION_PG")
	if dsn == "" {
		t.Skip("set CABINET_INTEGRATION_PG to run merge integration tests")
	}
	sharedPoolOnce.Do(func() {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		if err := database.RunMigrations(ctx, &database.MigrationConfig{
			Direction: "up", MigrationsPath: migrationsDirLinking(t),
		}, pool); err != nil {
			t.Fatalf("migrations: %v", err)
		}
		sharedPool = pool
	})
	if sharedPool == nil {
		t.Fatal("pool not initialised")
	}
	return sharedPool
}

func newMergeEnv(t *testing.T) *mergeEnv {
	t.Helper()
	pool := integrationPool(t)
	ctx := context.Background()
	panel := newFakePanel(t)

	e := &mergeEnv{
		t:          t,
		ctx:        ctx,
		pool:       pool,
		panel:      panel,
		accounts:   repository.NewAccountRepo(pool),
		links:      repository.NewAccountCustomerLinkRepo(pool),
		identities: repository.NewIdentityRepo(pool),
		customers:  database.NewCustomerRepository(pool),
		// Диапазон реальных tg_id: заведомо ниже synthetic base и вне значений
		// живых пользователей тестового стенда.
		tgBase: 500_000_000_000 + atomic.AddInt64(&tgCounter, 1_000_000) + int64(rand.Intn(1000))*1000,
	}

	e.svc = New(
		pool,
		NewNonceStore(),
		NewClaimStore(),
		e.customers,
		e.links,
		repository.NewMergeAuditRepo(pool),
		e.accounts,
		e.identities,
		nil, // mailer в merge не участвует
		"test-bot-token",
		panel.client(),
	)

	t.Cleanup(e.cleanup)
	return e
}

func (e *mergeEnv) cleanup() {
	ctx := context.Background()
	for _, id := range e.createdAccounts {
		_, _ = e.pool.Exec(ctx, `DELETE FROM cabinet_account WHERE id = $1`, id)
	}
	for _, id := range e.createdCustomers {
		_, _ = e.pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, id)
	}
	for _, id := range e.createdPromos {
		_, _ = e.pool.Exec(ctx, `DELETE FROM promo_code WHERE id = $1`, id)
	}
	for _, id := range e.createdTariffs {
		_, _ = e.pool.Exec(ctx, `DELETE FROM tariff WHERE id = $1`, id)
	}
}

// nextTelegramID выдаёт свежий «реальный» telegram_id внутри теста.
func (e *mergeEnv) nextTelegramID() int64 {
	e.seq++
	return e.tgBase + e.seq
}

// ============================================================================
// Сидеры
// ============================================================================

func (e *mergeEnv) newAccount(emailLocal string) *repository.Account {
	e.t.Helper()
	email := ""
	if emailLocal != "" {
		email = fmt.Sprintf("%s-%d-%d@merge.test", emailLocal, e.tgBase, e.seq)
	}
	acc, err := e.accounts.Create(e.ctx, email, "", "ru")
	if err != nil {
		e.t.Fatalf("create account: %v", err)
	}
	e.createdAccounts = append(e.createdAccounts, acc.ID)
	return acc
}

// newAccountWithPassword — аккаунт с парольным логином (нужен для email-peer merge).
func (e *mergeEnv) newAccountWithPassword(emailLocal, hash string) *repository.Account {
	e.t.Helper()
	acc := e.newAccount(emailLocal)
	if _, err := e.pool.Exec(e.ctx,
		`UPDATE cabinet_account SET password_hash = $2, email_verified_at = NOW() WHERE id = $1`,
		acc.ID, hash); err != nil {
		e.t.Fatalf("set password: %v", err)
	}
	fresh, err := e.accounts.FindByID(e.ctx, acc.ID)
	if err != nil {
		e.t.Fatalf("reload account: %v", err)
	}
	return fresh
}

// customerSpec — описание сидируемого customer.
type customerSpec struct {
	TelegramID  int64
	ExpireAt    *time.Time
	SubLink     string
	LoyaltyXP   int64
	ExtraHwid   int
	IsWebOnly   bool
	TariffID    *int64
	PanelUserID *int64
}

func (e *mergeEnv) newCustomer(spec customerSpec) *database.Customer {
	e.t.Helper()
	var subLink any
	if spec.SubLink != "" {
		subLink = spec.SubLink
	}
	var id int64
	err := e.pool.QueryRow(e.ctx, `
		INSERT INTO customer (telegram_id, expire_at, subscription_link, language,
		                      loyalty_xp, extra_hwid, is_web_only, current_tariff_id,
		                      remnawave_user_id)
		VALUES ($1, $2, $3, 'ru', $4, $5, $6, $7, $8)
		RETURNING id`,
		spec.TelegramID, spec.ExpireAt, subLink, spec.LoyaltyXP, spec.ExtraHwid,
		spec.IsWebOnly, spec.TariffID, spec.PanelUserID,
	).Scan(&id)
	if err != nil {
		e.t.Fatalf("insert customer: %v", err)
	}
	e.createdCustomers = append(e.createdCustomers, id)
	c, err := e.customers.FindById(e.ctx, id)
	if err != nil || c == nil {
		e.t.Fatalf("reload customer %d: %v", id, err)
	}
	return c
}

// webCustomerFor — web-only customer с synthetic telegram_id, как его создаёт bootstrap.
func (e *mergeEnv) webCustomerFor(acc *repository.Account, expire *time.Time, subLink string) *database.Customer {
	e.t.Helper()
	return e.newCustomer(customerSpec{
		TelegramID: utils.SyntheticTelegramID(acc.ID),
		ExpireAt:   expire,
		SubLink:    subLink,
		IsWebOnly:  true,
	})
}

func (e *mergeEnv) link(acc *repository.Account, c *database.Customer) {
	e.t.Helper()
	if _, err := e.links.Create(e.ctx, acc.ID, c.ID, repository.LinkStatusLinked); err != nil {
		e.t.Fatalf("create link acc=%d cust=%d: %v", acc.ID, c.ID, err)
	}
}

func (e *mergeEnv) addIdentity(acc *repository.Account, provider, providerUserID, email string) {
	e.t.Helper()
	if _, err := e.identities.Create(e.ctx, acc.ID, provider, providerUserID, email, nil); err != nil {
		e.t.Fatalf("create identity %s/%s: %v", provider, providerUserID, err)
	}
}

func (e *mergeEnv) addTelegramIdentity(acc *repository.Account, tgID int64) {
	e.t.Helper()
	e.addIdentity(acc, repository.ProviderTelegram, strconv.FormatInt(tgID, 10), "")
}

func (e *mergeEnv) addPurchase(c *database.Customer, amount int) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(e.ctx, `
		INSERT INTO purchase (customer_id, status, amount, currency, month)
		VALUES ($1, 'PAID', $2, 'RUB', 1)
		RETURNING id`, c.ID, amount).Scan(&id); err != nil {
		e.t.Fatalf("insert purchase: %v", err)
	}
	return id
}

func (e *mergeEnv) addReferral(referrerTG, refereeTG int64) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO referral (referrer_id, referee_id) VALUES ($1, $2)`,
		referrerTG, refereeTG); err != nil {
		e.t.Fatalf("insert referral: %v", err)
	}
}

func (e *mergeEnv) newTariff(slug string) int64 {
	e.t.Helper()
	var id int64
	uniq := fmt.Sprintf("%s-%d-%d", slug, e.tgBase, e.seq)
	if err := e.pool.QueryRow(e.ctx, `
		INSERT INTO tariff (slug, name) VALUES ($1, $2) RETURNING id`,
		uniq, slug).Scan(&id); err != nil {
		e.t.Fatalf("insert tariff: %v", err)
	}
	e.createdTariffs = append(e.createdTariffs, id)
	return id
}

func (e *mergeEnv) newPromoCode(code string) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(e.ctx, `
		INSERT INTO promo_code (code, type, discount_percent, active, max_uses)
		VALUES ($1, 'discount', 10, TRUE, -1)
		RETURNING id`, strings.ToUpper(code)).Scan(&id); err != nil {
		e.t.Fatalf("insert promo_code: %v", err)
	}
	e.createdPromos = append(e.createdPromos, id)
	return id
}

func (e *mergeEnv) addPromoRedemption(promoID int64, c *database.Customer) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO promo_redemption (promo_code_id, customer_id) VALUES ($1, $2)`,
		promoID, c.ID); err != nil {
		e.t.Fatalf("insert promo_redemption: %v", err)
	}
}

func (e *mergeEnv) addFortuneSpin(c *database.Customer) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO fortune_spins (customer_id, reward_type, reward_value) VALUES ($1, 'discount', 5)`,
		c.ID); err != nil {
		e.t.Fatalf("insert fortune_spin: %v", err)
	}
}

// ============================================================================
// Ассерты
// ============================================================================

func (e *mergeEnv) customerByID(id int64) *database.Customer {
	e.t.Helper()
	c, err := e.customers.FindById(e.ctx, id)
	if err != nil {
		e.t.Fatalf("find customer %d: %v", id, err)
	}
	return c
}

func (e *mergeEnv) customerExists(id int64) bool {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM customer WHERE id = $1`, id).Scan(&n); err != nil {
		e.t.Fatalf("count customer: %v", err)
	}
	return n > 0
}

func (e *mergeEnv) accountExists(id int64) bool {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM cabinet_account WHERE id = $1`, id).Scan(&n); err != nil {
		e.t.Fatalf("count account: %v", err)
	}
	return n > 0
}

func (e *mergeEnv) linkedCustomerID(accountID int64) (int64, bool) {
	e.t.Helper()
	var cid int64
	err := e.pool.QueryRow(e.ctx,
		`SELECT customer_id FROM cabinet_account_customer_link WHERE account_id = $1`, accountID).Scan(&cid)
	if err != nil {
		return 0, false
	}
	return cid, true
}

func (e *mergeEnv) linksToCustomer(customerID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM cabinet_account_customer_link WHERE customer_id = $1`, customerID).Scan(&n); err != nil {
		e.t.Fatalf("count links: %v", err)
	}
	return n
}

func (e *mergeEnv) identityAccount(provider, providerUserID string) (int64, bool) {
	e.t.Helper()
	var accID int64
	err := e.pool.QueryRow(e.ctx,
		`SELECT account_id FROM cabinet_identity WHERE provider = $1 AND provider_user_id = $2`,
		provider, providerUserID).Scan(&accID)
	if err != nil {
		return 0, false
	}
	return accID, true
}

func (e *mergeEnv) purchaseCount(customerID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM purchase WHERE customer_id = $1`, customerID).Scan(&n); err != nil {
		e.t.Fatalf("count purchases: %v", err)
	}
	return n
}

func (e *mergeEnv) referralCount(tgID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM referral WHERE referrer_id = $1 OR referee_id = $1`, tgID).Scan(&n); err != nil {
		e.t.Fatalf("count referrals: %v", err)
	}
	return n
}

func (e *mergeEnv) promoRedemptionCount(customerID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM promo_redemption WHERE customer_id = $1`, customerID).Scan(&n); err != nil {
		e.t.Fatalf("count promo_redemption: %v", err)
	}
	return n
}

func (e *mergeEnv) fortuneSpinCount(customerID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM fortune_spins WHERE customer_id = $1`, customerID).Scan(&n); err != nil {
		e.t.Fatalf("count fortune_spins: %v", err)
	}
	return n
}

// ============================================================================
// Мелочи
// ============================================================================

func activeUntil(d time.Duration) *time.Time {
	t := time.Now().UTC().Add(d).Truncate(time.Second)
	return &t
}

func expiredSince(d time.Duration) *time.Time {
	t := time.Now().UTC().Add(-d).Truncate(time.Second)
	return &t
}

func idemKey(name string) string {
	return fmt.Sprintf("it-%s-%d", name, time.Now().UnixNano())
}

func strPtr(s string) *string { return &s }

func int64Ptr(v int64) *int64 { return &v }
