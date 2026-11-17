// Package gate — обязательная подписка на канал в кабинете.
//
// Бот останавливает неподписанного на экране подписки; здесь то же самое для
// кабинета, но только для аккаунтов с привязанным Telegram: без него проверить
// подписку нечем, и такие аккаунты не блокируются.
//
// Защита стоит на сервере: закрытые маршруты отвечают 403
// channel_subscription_required, а модальное окно во фронте — лишь удобство.
package gate

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/channelgate"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// ErrorCode — значение поля error в ответе 403.
const ErrorCode = "channel_subscription_required"

// accountCacheTTL — как долго помним, какой Telegram привязан к аккаунту: иначе
// каждая закрытая ручка стоила бы тремя запросами в БД. Минута — чтобы привязка,
// отвязка или слияние аккаунтов подхватывались быстро без отдельных сбросов.
const accountCacheTTL = time.Minute

// Что нужно Gate от хранилищ. *repository.IdentityRepo, *repository.AccountCustomerLinkRepo
// и *database.CustomerRepository им отвечают; интерфейсы — чтобы тестировать без БД.
type identityLister interface {
	ListLinkedByAccount(ctx context.Context, accountID int64) ([]repository.Identity, error)
}

type linkFinder interface {
	FindByAccountID(ctx context.Context, accountID int64) (*repository.AccountCustomerLink, error)
}

type customerStore interface {
	FindById(ctx context.Context, id int64) (*database.Customer, error)
	MarkChannelGateShown(ctx context.Context, customerID int64) error
	MarkChannelGateAttempt(ctx context.Context, customerID int64) error
	MarkChannelGatePassed(ctx context.Context, customerID int64) error
}

type accountInfo struct {
	telegramID int64 // 0 — проверять нечем (нет Telegram или клиент без Telegram)
	customerID int64
	until      time.Time
}

// Gate решает, показывать ли аккаунту экран подписки.
type Gate struct {
	svc       *channelgate.Service
	ids       identityLister
	links     linkFinder
	customers customerStore

	sf    singleflight.Group
	mu    sync.Mutex
	cache map[int64]accountInfo
	shown sync.Map // customerID → struct{}: экран уже записан в статистику
}

// New собирает Gate. svc == nil даёт «выключено»: все методы пропускают.
// Админов отсеивает сам svc (config.IsAdminTelegramID по Telegram-id аккаунта).
func New(svc *channelgate.Service, ids *repository.IdentityRepo, links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository) *Gate {
	return &Gate{svc: svc, ids: ids, links: links, customers: customers, cache: make(map[int64]accountInfo)}
}

// Info — что отдаётся фронту в /me.
type Info struct {
	Required   bool   `json:"required"`
	ChannelURL string `json:"channel_url,omitempty"`
}

func (g *Gate) active() bool { return g != nil && g.svc.Active() }

func (g *Gate) lookup(ctx context.Context, accountID int64) accountInfo {
	now := time.Now()
	g.mu.Lock()
	if e, ok := g.cache[accountID]; ok && now.Before(e.until) {
		g.mu.Unlock()
		return e
	}
	g.mu.Unlock()

	// Страница кабинета шлёт около десятка закрытых запросов разом: холодный кэш
	// не должен превращаться в десять походов в БД на каждый.
	v, _, _ := g.sf.Do(strconv.FormatInt(accountID, 10), func() (any, error) {
		return g.loadAccount(context.WithoutCancel(ctx), accountID), nil
	})
	info, _ := v.(accountInfo)
	return info
}

func (g *Gate) loadAccount(ctx context.Context, accountID int64) accountInfo {
	info := accountInfo{until: time.Now().Add(accountCacheTTL)}
	ids, err := g.ids.ListLinkedByAccount(ctx, accountID)
	if err != nil {
		slog.Warn("channel gate: list identities", "account_id", accountID, "error", err)
		return accountInfo{}
	}
	for _, id := range ids {
		if id.Provider != repository.ProviderTelegram {
			continue
		}
		if v, perr := strconv.ParseInt(strings.TrimSpace(id.ProviderUserID), 10, 64); perr == nil && v > 0 {
			info.telegramID = v
			break
		}
	}
	if info.telegramID != 0 {
		link, err := g.links.FindByAccountID(ctx, accountID)
		switch {
		case err == nil:
			info.customerID = link.CustomerID
			// Клиент без Telegram (web-only): к Bot API по нему не ходим. Не смогли
			// прочитать клиента — не гадаем и не кэшируем, пропускаем.
			cust, cerr := g.customers.FindById(ctx, link.CustomerID)
			if cerr != nil {
				slog.Warn("channel gate: find customer", "account_id", accountID, "error", cerr)
				return accountInfo{}
			}
			if cust == nil || cust.IsWebOnly {
				info.telegramID = 0
			}
		case errors.Is(err, repository.ErrNotFound):
			info.telegramID = 0
		default:
			slog.Warn("channel gate: find link", "account_id", accountID, "error", err)
			return accountInfo{}
		}
	}
	g.mu.Lock()
	if len(g.cache) > 20000 {
		g.cache = make(map[int64]accountInfo)
	}
	g.cache[accountID] = info
	g.mu.Unlock()
	return info
}

// evaluate: нужно ли остановить этот аккаунт на экране подписки.
func (g *Gate) evaluate(ctx context.Context, accountID int64) (blocked bool, info accountInfo) {
	if !g.active() || accountID == 0 {
		return false, accountInfo{}
	}
	info = g.lookup(ctx, accountID)
	if info.telegramID == 0 {
		return false, info
	}
	if !g.svc.Blocks(ctx, info.telegramID) {
		return false, info
	}
	g.markShown(ctx, info.customerID)
	return true, info
}

func (g *Gate) markShown(ctx context.Context, customerID int64) {
	if customerID == 0 {
		return
	}
	if _, done := g.shown.LoadOrStore(customerID, struct{}{}); done {
		return
	}
	if err := g.customers.MarkChannelGateShown(ctx, customerID); err != nil {
		g.shown.Delete(customerID)
		slog.Error("channel gate: mark shown", "error", err)
	}
}

func accountID(r *http.Request) (int64, bool) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		return 0, false
	}
	return claims.AccountID, true
}

// StateFor — поле channel_gate для GET /me.
func (g *Gate) StateFor(r *http.Request) Info {
	if !g.active() {
		return Info{}
	}
	id, ok := accountID(r)
	if !ok {
		return Info{}
	}
	if blocked, _ := g.evaluate(r.Context(), id); blocked {
		return Info{Required: true, ChannelURL: config.ChannelGateURL()}
	}
	return Info{}
}

// Require — middleware после RequireAuth: неподписанному отвечает 403.
func (g *Gate) Require() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.active() {
				next.ServeHTTP(w, r)
				return
			}
			id, ok := accountID(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if blocked, _ := g.evaluate(r.Context(), id); blocked {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":       ErrorCode,
					"channel_url": config.ChannelGateURL(),
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type checkResp struct {
	Subscribed bool `json:"subscribed"`
	// TooFast — кнопку нажали чаще раза в 3 секунды; фронт просто ждёт.
	TooFast bool `json:"too_fast,omitempty"`
}

// Check — POST /me/channel-gate/check: кнопка «Да, я подписался».
func (g *Gate) Check(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	respond := func(v checkResp) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(v)
	}
	if !g.active() {
		respond(checkResp{Subscribed: true})
		return
	}
	id, ok := accountID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	info := g.lookup(r.Context(), id)
	if info.telegramID == 0 || g.svc.Exempt(info.telegramID) {
		respond(checkResp{Subscribed: true})
		return
	}

	state, err := g.svc.CheckNow(r.Context(), info.telegramID)
	if errors.Is(err, channelgate.ErrTooFast) {
		respond(checkResp{TooFast: true})
		return
	}
	if info.customerID != 0 {
		if err := g.customers.MarkChannelGateAttempt(r.Context(), info.customerID); err != nil {
			slog.Error("channel gate: mark attempt", "error", err)
		}
	}
	if state == channelgate.NotSubscribed {
		respond(checkResp{Subscribed: false})
		return
	}
	if state == channelgate.Subscribed && info.customerID != 0 {
		if err := g.customers.MarkChannelGatePassed(r.Context(), info.customerID); err != nil {
			slog.Error("channel gate: mark passed", "error", err)
		}
	}
	// Подписан, а при сбое проверки пускаем, как и везде.
	respond(checkResp{Subscribed: true})
}
