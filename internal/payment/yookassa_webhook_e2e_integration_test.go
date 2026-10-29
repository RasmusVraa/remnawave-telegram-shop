//go:build integration

package payment

// Сквозной тест оплаты: уведомление ЮKassa → сверка платежа → продление в
// панели → запись в БД → реферальный бонус → сообщения в Telegram. ЮKassa,
// панель Remnawave и Bot API подменены httptest-серверами, база — настоящая.
// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/db?sslmode=disable \
//	  go test ./internal/payment/ -tags=integration -run E2E -count=1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/yookasa"
)

const (
	e2eDaysInMonth  = 30
	e2eReferralDays = 7
)

// fakePanel — минимальная панель Remnawave с состоянием: поиск по telegramId,
// сквады, PATCH пользователя, extend и сброс трафика.
type fakePanel struct {
	mu        sync.Mutex
	users     map[int64]*remnawave.User
	nextID    int64
	failPatch bool
	calls     map[string]int // "PATCH <tg>", "EXTEND <tg>", "RESET <tg>"
	unknown   []string
}

func newFakePanel() *fakePanel {
	return &fakePanel{users: map[int64]*remnawave.User{}, nextID: 100, calls: map[string]int{}}
}

func (p *fakePanel) addUser(telegramID int64, expireAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	tg := telegramID
	p.users[p.nextID] = &remnawave.User{
		ID:              p.nextID,
		Username:        fmt.Sprintf("1_%d", telegramID),
		SubscriptionUrl: fmt.Sprintf("https://sub.test/%d", telegramID),
		ExpireAt:        expireAt.UTC(),
		TelegramID:      &tg,
		Status:          "ACTIVE",
	}
}

func (p *fakePanel) user(telegramID int64) *remnawave.User {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, u := range p.users {
		if u.TelegramID != nil && *u.TelegramID == telegramID {
			c := *u
			return &c
		}
	}
	return nil
}

func (p *fakePanel) count(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[key]
}

func (p *fakePanel) setFailPatch(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failPatch = v
}

func (p *fakePanel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	writeResp := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"response": v})
	}
	tgOf := func(u *remnawave.User) string {
		if u.TelegramID == nil {
			return "-"
		}
		return strconv.FormatInt(*u.TelegramID, 10)
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/users/stream":
		want := r.URL.Query().Get("telegramId")
		out := []remnawave.User{}
		for _, u := range p.users {
			if want == "" || tgOf(u) == want {
				out = append(out, *u)
			}
		}
		writeResp(map[string]any{"users": out, "hasMore": false})
	case r.Method == http.MethodGet && r.URL.Path == "/api/internal-squads":
		writeResp(map[string]any{"internalSquads": []map[string]any{
			{"uuid": "11111111-1111-1111-1111-111111111111", "name": "default"},
		}})
	case r.Method == http.MethodPatch && r.URL.Path == "/api/users":
		var req remnawave.UpdateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == nil {
			http.Error(w, `{"message":"bad patch"}`, http.StatusBadRequest)
			return
		}
		u := p.users[*req.ID]
		if u == nil {
			http.Error(w, `{"message":"no user"}`, http.StatusNotFound)
			return
		}
		if p.failPatch && req.ExpireAt != nil {
			http.Error(w, `{"message":"panel is down"}`, http.StatusInternalServerError)
			return
		}
		if req.ExpireAt != nil {
			u.ExpireAt = req.ExpireAt.UTC()
			p.calls["PATCH "+tgOf(u)]++
		}
		if req.HwidDeviceLimit != nil {
			l := *req.HwidDeviceLimit
			u.HwidDeviceLimit = &l
		}
		writeResp(u)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/extend"):
		id, _ := strconv.ParseInt(strings.Split(r.URL.Path, "/")[3], 10, 64)
		u := p.users[id]
		var req struct {
			Days int `json:"days"`
		}
		if u == nil || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, `{"message":"bad extend"}`, http.StatusBadRequest)
			return
		}
		base := u.ExpireAt
		if base.Before(time.Now()) {
			base = time.Now().UTC()
		}
		u.ExpireAt = base.AddDate(0, 0, req.Days)
		p.calls["EXTEND "+tgOf(u)]++
		writeResp(u)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/reset-traffic"):
		id, _ := strconv.ParseInt(strings.Split(r.URL.Path, "/")[3], 10, 64)
		if u := p.users[id]; u != nil {
			p.calls["RESET "+tgOf(u)]++
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		p.unknown = append(p.unknown, r.Method+" "+r.URL.String())
		http.Error(w, `{"message":"unexpected request"}`, http.StatusNotImplemented)
	}
}

// fakeYookassa отдаёт заранее заданные платежи на GET /payments/{id}.
type fakeYookassa struct {
	mu       sync.Mutex
	payments map[uuid.UUID]yookasa.Payment
}

func (f *fakeYookassa) set(p yookasa.Payment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payments[p.ID] = p
}

func (f *fakeYookassa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/payments/"))
	f.mu.Lock()
	p, ok := f.payments[id]
	f.mu.Unlock()
	if err != nil || !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(p)
}

// fakeTelegram принимает любой метод Bot API и считает sendMessage по чатам.
type fakeTelegram struct {
	mu   sync.Mutex
	sent map[string]int
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	w.Header().Set("Content-Type", "application/json")
	if method != "sendMessage" {
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		return
	}
	chat := r.FormValue("chat_id")
	f.mu.Lock()
	f.sent[chat]++
	f.mu.Unlock()
	_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":%s,"type":"private"}}}`, chat)
}

func (f *fakeTelegram) count(chat int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[strconv.FormatInt(chat, 10)]
}

type e2eEnv struct {
	pool    *pgxpool.Pool
	panel   *fakePanel
	kassa   *fakeYookassa
	tg      *fakeTelegram
	webhook http.Handler
}

func newE2EEnv(t *testing.T) *e2eEnv {
	t.Helper()
	pool := guardsTestPool(t)

	panel := newFakePanel()
	panelSrv := httptest.NewServer(panel)
	t.Cleanup(panelSrv.Close)
	kassa := &fakeYookassa{payments: map[uuid.UUID]yookasa.Payment{}}
	kassaSrv := httptest.NewServer(kassa)
	t.Cleanup(kassaSrv.Close)
	tg := &fakeTelegram{sent: map[string]int{}}
	tgSrv := httptest.NewServer(tg)
	t.Cleanup(tgSrv.Close)

	// Конфиг читается так же, как при запуске бота, — из окружения.
	for k, v := range map[string]string{
		"DISABLE_ENV_FILE":    "true",
		"ADMIN_TELEGRAM_ID":   "1",
		"TELEGRAM_TOKEN":      "1:test",
		"REMNAWAVE_URL":       panelSrv.URL,
		"REMNAWAVE_TOKEN":     "test",
		"DATABASE_URL":        "postgres://unused",
		"TRAFFIC_LIMIT":       "100",
		"TRIAL_TRAFFIC_LIMIT": "10",
		"TRIAL_DAYS":          "3",
		"PRICE_1":             "199",
		"PRICE_3":             "499",
		"PRICE_6":             "899",
		"PRICE_12":            "1599",
		"HWID_ADD_PRICE":      "50",
		"REFERRAL_DAYS":       strconv.Itoa(e2eReferralDays),
		"DAYS_IN_MONTH":       strconv.Itoa(e2eDaysInMonth),
		"SALES_MODE":          "classic",
		"REFERRAL_MODE":       "default",
		// Уведомление в группу о платеже тоже проходит через поддельный Bot API.
		"PAYMENTS_NOTIFY_ENABLED": "true",
		"PAYMENTS_NOTIFY_CHAT_ID": "-1001",
	} {
		t.Setenv(k, v)
	}
	config.InitConfig()

	_, f, _, _ := runtime.Caller(0)
	tr := translation.GetInstance()
	if err := tr.InitTranslations(filepath.Join(filepath.Dir(f), "..", "..", "translations"), "ru"); err != nil {
		t.Fatalf("translations: %v", err)
	}

	tgBot, err := bot.New("1:test", bot.WithServerURL(tgSrv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatalf("bot: %v", err)
	}

	rw := remnawave.NewClient(panelSrv.URL, "test", "remote")
	customers := database.NewCustomerRepository(pool)
	purchases := database.NewPurchaseRepository(pool)
	promoSvc := promo.NewService(database.NewPromoRepository(pool), customers, purchases, rw)
	kassaClient := yookasa.NewClient(kassaSrv.URL, "shop", "secret")
	svc := NewPaymentService(tr, purchases, database.NewTariffRepository(pool), rw, customers, tgBot,
		nil, kassaClient, nil, nil,
		database.NewReferralRepository(pool), database.NewReferralBonusLedgerRepository(pool),
		database.NewPartnerRepository(pool), cache.NewCache(time.Minute), nil, promoSvc,
		database.NewLoyaltyTierRepository(pool))

	t.Cleanup(func() {
		if len(panel.unknown) > 0 {
			t.Errorf("unexpected panel requests: %v", panel.unknown)
		}
	})
	return &e2eEnv{pool: pool, panel: panel, kassa: kassa, tg: tg,
		webhook: yookasa.NewWebhookHandler(kassaClient, svc, purchases)}
}

func (e *e2eEnv) customer(t *testing.T, tgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO customer (telegram_id, language) VALUES ($1, 'ru') RETURNING id`, tgID).Scan(&id); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = e.pool.Exec(ctx, `DELETE FROM referral_bonus_ledger WHERE referrer_telegram_id = $1 OR referee_telegram_id = $1`, tgID)
		_, _ = e.pool.Exec(ctx, `DELETE FROM referral WHERE referrer_id = $1 OR referee_id = $1`, tgID)
		_, _ = e.pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id = $1`, id)
		_, _ = e.pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, id)
	})
	return id
}

// invoice создаёт открытый счёт ЮKassa и платёж к нему с суммой paid.
func (e *e2eEnv) invoice(t *testing.T, customerID int64, amount int, paid string) (int64, uuid.UUID) {
	t.Helper()
	paymentID := uuid.New()
	var purchaseID int64
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, yookasa_id)
		VALUES ($1, $2, 1, 'RUB', 'pending', 'yookasa', $3) RETURNING id`, amount, customerID, paymentID).Scan(&purchaseID); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}
	e.kassa.set(yookasa.Payment{
		ID:       paymentID,
		Status:   "succeeded",
		Paid:     true,
		Amount:   yookasa.Amount{Value: paid, Currency: "RUB"},
		Metadata: map[string]string{"purchaseId": strconv.FormatInt(purchaseID, 10)},
	})
	return purchaseID, paymentID
}

func (e *e2eEnv) notify(t *testing.T, paymentID uuid.UUID) int {
	t.Helper()
	body := fmt.Sprintf(`{"type":"notification","event":"payment.succeeded","object":{"id":%q,"status":"succeeded"}}`, paymentID)
	rec := httptest.NewRecorder()
	e.webhook.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/yookasa/webhook", strings.NewReader(body)))
	return rec.Code
}

func (e *e2eEnv) status(t *testing.T, purchaseID int64) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM purchase WHERE id = $1`, purchaseID).Scan(&s); err != nil {
		t.Fatalf("purchase status: %v", err)
	}
	return s
}

func (e *e2eEnv) expireAt(t *testing.T, customerID int64) (time.Time, string) {
	t.Helper()
	var exp *time.Time
	var link *string
	if err := e.pool.QueryRow(context.Background(), `SELECT expire_at, subscription_link FROM customer WHERE id = $1`, customerID).Scan(&exp, &link); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if exp == nil || link == nil {
		t.Fatalf("customer %d: expire_at=%v subscription_link=%v", customerID, exp, link)
	}
	return *exp, *link
}

func e2eTelegramIDs() (int64, int64) {
	base := time.Now().UnixNano() % 1_000_000_000_000
	return base, base + 1
}

func sameDay(a, b time.Time) bool { return a.Sub(b).Abs() < time.Minute }

// Оплата приглашённого: подписка продлевается на месяц от текущего срока,
// реферер получает бонусные дни, повтор уведомления ничего не начисляет.
func TestYookassaWebhookE2E_PaidExtendsSubscriptionAndReferral(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()
	referrerTG, refereeTG := e2eTelegramIDs()
	referrerID := e.customer(t, referrerTG)
	refereeID := e.customer(t, refereeTG)
	if _, err := e.pool.Exec(ctx, `INSERT INTO referral (referrer_id, referee_id) VALUES ($1, $2)`, referrerTG, refereeTG); err != nil {
		t.Fatalf("insert referral: %v", err)
	}
	refereeExpire := time.Now().UTC().Add(5 * 24 * time.Hour).Truncate(time.Second)
	referrerExpire := time.Now().UTC().Add(10 * 24 * time.Hour).Truncate(time.Second)
	e.panel.addUser(refereeTG, refereeExpire)
	e.panel.addUser(referrerTG, referrerExpire)

	purchaseID, paymentID := e.invoice(t, refereeID, 199, "199.00")
	if code := e.notify(t, paymentID); code != http.StatusOK {
		t.Fatalf("webhook: status %d, want 200", code)
	}

	if s := e.status(t, purchaseID); s != string(database.PurchaseStatusPaid) {
		t.Fatalf("purchase status %q, want paid", s)
	}
	wantExpire := refereeExpire.AddDate(0, 0, e2eDaysInMonth)
	if got := e.panel.user(refereeTG).ExpireAt; !sameDay(got, wantExpire) {
		t.Fatalf("panel expire %v, want %v", got, wantExpire)
	}
	exp, link := e.expireAt(t, refereeID)
	if !sameDay(exp, wantExpire) || link != fmt.Sprintf("https://sub.test/%d", refereeTG) {
		t.Fatalf("customer expire_at=%v link=%q, want %v and panel link", exp, link, wantExpire)
	}
	if n := e.panel.count(fmt.Sprintf("RESET %d", refereeTG)); n != 1 {
		t.Fatalf("traffic reset calls %d, want 1", n)
	}

	var bonusGranted bool
	if err := e.pool.QueryRow(ctx, `SELECT bonus_granted FROM referral WHERE referee_id = $1`, refereeTG).Scan(&bonusGranted); err != nil || !bonusGranted {
		t.Fatalf("referral bonus_granted=%v err=%v, want true", bonusGranted, err)
	}
	wantReferrer := referrerExpire.AddDate(0, 0, e2eReferralDays)
	if exp, _ := e.expireAt(t, referrerID); !sameDay(exp, wantReferrer) {
		t.Fatalf("referrer expire_at %v, want %v", exp, wantReferrer)
	}
	var ledger int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM referral_bonus_ledger WHERE purchase_id = $1 AND recipient_telegram_id = $2 AND days = $3`,
		purchaseID, referrerTG, e2eReferralDays).Scan(&ledger); err != nil || ledger != 1 {
		t.Fatalf("referral ledger rows %d err=%v, want 1", ledger, err)
	}

	if e.tg.count(refereeTG) == 0 || e.tg.count(referrerTG) == 0 || e.tg.count(-1001) == 0 {
		t.Fatalf("telegram messages: referee=%d referrer=%d group=%d, want each > 0",
			e.tg.count(refereeTG), e.tg.count(referrerTG), e.tg.count(-1001))
	}

	// ЮKassa повторяет уведомления: второй раз ничего не продлевается.
	if code := e.notify(t, paymentID); code != http.StatusOK {
		t.Fatalf("repeated webhook: status %d, want 200", code)
	}
	if n := e.panel.count(fmt.Sprintf("PATCH %d", refereeTG)); n != 1 {
		t.Fatalf("panel extended referee %d times, want 1", n)
	}
	if n := e.panel.count(fmt.Sprintf("EXTEND %d", referrerTG)); n != 1 {
		t.Fatalf("panel extended referrer %d times, want 1", n)
	}
}

// Платёж на меньшую сумму не зачитывается покупке, панель не трогается.
func TestYookassaWebhookE2E_AmountMismatchNotCredited(t *testing.T) {
	e := newE2EEnv(t)
	tgID, _ := e2eTelegramIDs()
	customerID := e.customer(t, tgID)
	e.panel.addUser(tgID, time.Now().Add(24*time.Hour))

	purchaseID, paymentID := e.invoice(t, customerID, 199, "99.00")
	if code := e.notify(t, paymentID); code != http.StatusOK {
		t.Fatalf("webhook: status %d, want 200", code)
	}
	if s := e.status(t, purchaseID); s != string(database.PurchaseStatusPending) {
		t.Fatalf("purchase status %q, want pending", s)
	}
	if n := e.panel.count(fmt.Sprintf("PATCH %d", tgID)); n != 0 {
		t.Fatalf("panel extended %d times, want 0", n)
	}
}

// Панель недоступна: покупка остаётся неоплаченной, ЮKassa получает 500 и
// повторит уведомление; повтор после восстановления панели продлевает подписку.
func TestYookassaWebhookE2E_PanelDownThenRetry(t *testing.T) {
	e := newE2EEnv(t)
	tgID, _ := e2eTelegramIDs()
	customerID := e.customer(t, tgID)
	expire := time.Now().UTC().Add(3 * 24 * time.Hour).Truncate(time.Second)
	e.panel.addUser(tgID, expire)

	purchaseID, paymentID := e.invoice(t, customerID, 299, "299.00")
	e.panel.setFailPatch(true)
	if code := e.notify(t, paymentID); code != http.StatusInternalServerError {
		t.Fatalf("webhook with panel down: status %d, want 500", code)
	}
	if s := e.status(t, purchaseID); s != string(database.PurchaseStatusPending) {
		t.Fatalf("purchase status %q after panel error, want pending", s)
	}

	e.panel.setFailPatch(false)
	if code := e.notify(t, paymentID); code != http.StatusOK {
		t.Fatalf("retried webhook: status %d, want 200", code)
	}
	if s := e.status(t, purchaseID); s != string(database.PurchaseStatusPaid) {
		t.Fatalf("purchase status %q after retry, want paid", s)
	}
	want := expire.AddDate(0, 0, e2eDaysInMonth)
	if exp, _ := e.expireAt(t, customerID); !sameDay(exp, want) {
		t.Fatalf("customer expire_at %v, want %v", exp, want)
	}
}
