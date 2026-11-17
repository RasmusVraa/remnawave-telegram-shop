package gate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/channelgate"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// --- поддельный Telegram ---------------------------------------------------

type fakeAPI struct {
	status models.ChatMemberType
	calls  atomic.Int32
}

func (f *fakeAPI) GetMe(context.Context) (*models.User, error) { return &models.User{ID: 1}, nil }
func (f *fakeAPI) GetChat(context.Context, *bot.GetChatParams) (*models.ChatFullInfo, error) {
	return &models.ChatFullInfo{Type: models.ChatTypeChannel, Username: "meows_news"}, nil
}
func (f *fakeAPI) GetChatMember(context.Context, *bot.GetChatMemberParams) (*models.ChatMember, error) {
	f.calls.Add(1)
	return &models.ChatMember{Type: f.status}, nil
}
func (f *fakeAPI) GetChatMemberCount(context.Context, *bot.GetChatMemberCountParams) (int, error) {
	return 1, nil
}

// --- поддельные хранилища --------------------------------------------------

type fakeIDs struct {
	byAccount map[int64][]repository.Identity
	calls     atomic.Int32
}

func (f *fakeIDs) ListLinkedByAccount(_ context.Context, id int64) ([]repository.Identity, error) {
	f.calls.Add(1)
	return f.byAccount[id], nil
}

type fakeLinks struct{ byAccount map[int64]int64 }

func (f fakeLinks) FindByAccountID(_ context.Context, id int64) (*repository.AccountCustomerLink, error) {
	if c, ok := f.byAccount[id]; ok {
		return &repository.AccountCustomerLink{AccountID: id, CustomerID: c}, nil
	}
	return nil, repository.ErrNotFound
}

type fakeCustomers struct {
	mu       sync.Mutex
	webOnly  map[int64]bool
	findErr  map[int64]error
	shown    map[int64]int
	attempts map[int64]int
	passed   map[int64]int
}

func newFakeCustomers() *fakeCustomers {
	return &fakeCustomers{webOnly: map[int64]bool{}, findErr: map[int64]error{}, shown: map[int64]int{}, attempts: map[int64]int{}, passed: map[int64]int{}}
}

func (f *fakeCustomers) FindById(_ context.Context, id int64) (*database.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.findErr[id]; err != nil {
		return nil, err
	}
	return &database.Customer{ID: id, IsWebOnly: f.webOnly[id]}, nil
}
func (f *fakeCustomers) MarkChannelGateShown(_ context.Context, id int64) error {
	f.mu.Lock()
	f.shown[id]++
	f.mu.Unlock()
	return nil
}
func (f *fakeCustomers) MarkChannelGateAttempt(_ context.Context, id int64) error {
	f.mu.Lock()
	f.attempts[id]++
	f.mu.Unlock()
	return nil
}
func (f *fakeCustomers) MarkChannelGatePassed(_ context.Context, id int64) error {
	f.mu.Lock()
	f.passed[id]++
	f.mu.Unlock()
	return nil
}

// Аккаунты теста.
const (
	accNormal  = 10 // Telegram 555, клиент 100
	accNoTG    = 11 // только почта
	accWebOnly = 12 // Telegram 556, но клиент web-only (101)
	accCustErr = 13 // Telegram 557, клиент не читается
	accAdmin   = 14 // Telegram 558 — админ бота
)

type env struct {
	g    *Gate
	api  *fakeAPI
	ids  *fakeIDs
	cust *fakeCustomers
}

func tgIdentity(account int64, tg string) []repository.Identity {
	return []repository.Identity{{AccountID: account, Provider: repository.ProviderTelegram, ProviderUserID: tg}}
}

func newEnv(t *testing.T, status models.ChatMemberType, active bool) *env {
	t.Helper()
	patch := map[string]string{"CHANNEL_GATE_CHANNEL": "@meows_news", "CHANNEL_GATE_ENABLED": "false"}
	if active {
		patch["CHANNEL_GATE_ENABLED"] = "true"
	}
	if _, err := config.ApplyRuntimePatch(patch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	})

	api := &fakeAPI{status: status}
	svc := channelgate.New(api, nil)
	svc.SetExemptFunc(func(id int64) bool { return id == 558 })
	ids := &fakeIDs{byAccount: map[int64][]repository.Identity{
		accNormal:  tgIdentity(accNormal, "555"),
		accNoTG:    {{AccountID: accNoTG, Provider: repository.ProviderEmail, ProviderUserID: "a@b.c"}},
		accWebOnly: tgIdentity(accWebOnly, "556"),
		accCustErr: tgIdentity(accCustErr, "557"),
		accAdmin:   tgIdentity(accAdmin, "558"),
	}}
	cust := newFakeCustomers()
	cust.webOnly[101] = true
	cust.findErr[103] = errors.New("db down")
	gate := &Gate{
		svc: svc, ids: ids, customers: cust, cache: map[int64]accountInfo{},
		links: fakeLinks{byAccount: map[int64]int64{accNormal: 100, accWebOnly: 101, accCustErr: 103, accAdmin: 104}},
	}
	return &env{g: gate, api: api, ids: ids, cust: cust}
}

func request(accountID int64, method string) *http.Request {
	r := httptest.NewRequest(method, "/cabinet/api/me/subscription", nil)
	if accountID != 0 {
		r = r.WithContext(middleware.SetClaims(r.Context(), &jwt.Claims{AccountID: accountID}))
	}
	return r
}

func serve(g *Gate, r *http.Request) (*httptest.ResponseRecorder, bool) {
	called := false
	rec := httptest.NewRecorder()
	g.Require()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)
	return rec, called
}

func TestRequire_Disabled_PassesThrough(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, false)
	if _, called := serve(e.g, request(accNormal, http.MethodGet)); !called {
		t.Fatal("выключенная подписка не должна блокировать кабинет")
	}
	if e.api.calls.Load() != 0 || e.ids.calls.Load() != 0 {
		t.Fatal("при выключенной функции ни в Telegram, ни в БД ходить не нужно")
	}
}

func TestRequire_NotSubscribed_Returns403(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	rec, called := serve(e.g, request(accNormal, http.MethodGet))
	if called {
		t.Fatal("неподписанный не должен доходить до обработчика")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("код %d, ожидали 403", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != ErrorCode || body["channel_url"] != "https://t.me/meows_news" {
		t.Fatalf("тело ответа: %v", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("ответ 403 нельзя кэшировать")
	}
}

func TestRequire_Subscribed_PassesThrough(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeMember, true)
	if _, called := serve(e.g, request(accNormal, http.MethodGet)); !called {
		t.Fatal("подписанный должен проходить")
	}
}

func TestRequire_NotBlocked(t *testing.T) {
	cases := []struct {
		name    string
		account int64
	}{
		{"аккаунт без Telegram", accNoTG},
		{"клиент без Telegram (web-only) при привязанной identity", accWebOnly},
		{"админ бота", accAdmin},
		{"клиента не удалось прочитать — не гадаем", accCustErr},
		{"неизвестный аккаунт", 999},
	}
	for _, c := range cases {
		e := newEnv(t, models.ChatMemberTypeLeft, true)
		if _, called := serve(e.g, request(c.account, http.MethodGet)); !called {
			t.Errorf("%s: блокировать нельзя", c.name)
		}
		if c.account != 999 && e.api.calls.Load() != 0 {
			t.Errorf("%s: в Telegram ходить не должны (web-only/админ/нет Telegram)", c.name)
		}
	}
}

func TestRequire_CustomerLookupErrorIsNotCached(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	serve(e.g, request(accCustErr, http.MethodGet))
	serve(e.g, request(accCustErr, http.MethodGet))
	if e.ids.calls.Load() != 2 {
		t.Fatalf("ошибку чтения клиента нельзя запоминать на минуту: запросов к identity %d, ожидали 2", e.ids.calls.Load())
	}
}

func TestRequire_NoClaims_PassesToAuthLayer(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	if _, called := serve(e.g, request(0, http.MethodGet)); !called {
		t.Fatal("без claims решает RequireAuth, а не подписка")
	}
}

func TestRequire_NilGate_PassesThrough(t *testing.T) {
	var g *Gate
	if _, called := serve(g, request(accNormal, http.MethodGet)); !called {
		t.Fatal("nil-Gate = функция не подключена, запросы идут как раньше")
	}
}

func TestRequire_ColdCacheIsCoalesced(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(e.g, request(accNormal, http.MethodGet))
		}()
	}
	wg.Wait()
	if n := e.ids.calls.Load(); n > 2 {
		t.Fatalf("10 одновременных запросов страницы дали %d чтений identity, ожидали 1–2", n)
	}
}

func TestShownIsRecordedOnce(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	for i := 0; i < 5; i++ {
		serve(e.g, request(accNormal, http.MethodGet))
	}
	if got := e.cust.shown[100]; got != 1 {
		t.Fatalf("показ экрана в статистике записан %d раз, ожидали 1", got)
	}
}

func TestStateFor(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	st := e.g.StateFor(request(accNormal, http.MethodGet))
	if !st.Required || st.ChannelURL != "https://t.me/meows_news" {
		t.Fatalf("StateFor неподписанного: %+v", st)
	}
	if e.g.StateFor(request(accNoTG, http.MethodGet)).Required {
		t.Fatal("аккаунт без Telegram не должен получать окно")
	}
	if e.g.StateFor(request(accAdmin, http.MethodGet)).Required {
		t.Fatal("админ не должен получать окно")
	}
	off := newEnv(t, models.ChatMemberTypeLeft, false)
	if off.g.StateFor(request(accNormal, http.MethodGet)).Required {
		t.Fatal("при выключенной функции окна нет")
	}
}

func check(g *Gate, accountID int64) checkResp {
	rec := httptest.NewRecorder()
	g.Check(rec, request(accountID, http.MethodPost))
	var out checkResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestCheck_NotSubscribedCountsAttemptThenTooFast(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	if got := check(e.g, accNormal); got.Subscribed || got.TooFast {
		t.Fatalf("неподписанный: %+v", got)
	}
	if e.cust.attempts[100] != 1 || e.cust.passed[100] != 0 {
		t.Fatalf("попытка должна записаться, прохождение — нет: attempts=%d passed=%d", e.cust.attempts[100], e.cust.passed[100])
	}
	// Повтор в пределах 3 секунд — «слишком часто», попытка не засчитывается.
	if got := check(e.g, accNormal); !got.TooFast {
		t.Fatalf("повторное нажатие: %+v", got)
	}
	if e.cust.attempts[100] != 1 {
		t.Fatal("слишком частое нажатие не должно раздувать счётчик попыток")
	}
}

func TestCheck_SubscribedMarksPassed(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeMember, true)
	if got := check(e.g, accNormal); !got.Subscribed {
		t.Fatalf("подписанный: %+v", got)
	}
	if e.cust.passed[100] != 1 || e.cust.attempts[100] != 1 {
		t.Fatalf("прохождение и попытка должны записаться: passed=%d attempts=%d", e.cust.passed[100], e.cust.attempts[100])
	}
}

func TestCheck_ExemptAndNoTelegramAreOK(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	for name, acc := range map[string]int64{"без Telegram": accNoTG, "админ": accAdmin, "web-only": accWebOnly} {
		if got := check(e.g, acc); !got.Subscribed {
			t.Errorf("%s: окно не показывают, проверка отвечает «подписан»: %+v", name, got)
		}
	}
	if e.api.calls.Load() != 0 {
		t.Fatal("для них Telegram дёргать не нужно")
	}
}

func TestCheck_RejectsNonPost(t *testing.T) {
	e := newEnv(t, models.ChatMemberTypeLeft, true)
	rec := httptest.NewRecorder()
	e.g.Check(rec, request(accNormal, http.MethodGet))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код %d, ожидали 405", rec.Code)
	}
}
