package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
)

// TrafficHandler — раздельный трафик в кабинете: расход, лимиты, пакеты докупки.
type TrafficHandler struct {
	bootstrap *bootstrap.CustomerBootstrap
	customers *database.CustomerRepository
	traffic   *database.TrafficSplitRepository
	rw        *remnawave.Client
}

func NewTraffic(boot *bootstrap.CustomerBootstrap, customers *database.CustomerRepository, traffic *database.TrafficSplitRepository, rw *remnawave.Client) *TrafficHandler {
	return &TrafficHandler{bootstrap: boot, customers: customers, traffic: traffic, rw: rw}
}

type trafficKindDTO struct {
	// LimitBytes — действующий лимит (тариф + пакеты); 0 — безлимит.
	LimitBytes int64 `json:"limit_bytes"`
	BaseBytes  int64 `json:"base_bytes"`
	ExtraBytes int64 `json:"extra_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

type trafficExtraDTO struct {
	Kind       string    `json:"kind"`
	Bytes      int64     `json:"bytes"`
	ValidUntil time.Time `json:"valid_until"`
}

type trafficPackDTO struct {
	ID          int64  `json:"id"`
	Bytes       int64  `json:"bytes"`
	AmountRub   int    `json:"amount_rub"`
	AmountStars *int   `json:"amount_stars,omitempty"`
	Kind        string `json:"kind"`
}

type trafficOverviewResp struct {
	// Enabled — у клиента есть что показывать (лимит обхода или пакеты).
	Enabled bool `json:"enabled"`
	// CanBuy — можно ли сейчас докупить (продажа включена, подписка активна, у тарифа есть пакеты).
	CanBuy bool `json:"can_buy"`
	// Label — как называть серверы обхода (настраивает админ).
	Label   string         `json:"label"`
	ResetAt time.Time      `json:"reset_at"`
	Main    trafficKindDTO `json:"main"`
	// Bypass — nil, если у клиента нет отдельного лимита обхода.
	Bypass        *trafficKindDTO   `json:"bypass,omitempty"`
	BypassBlocked bool              `json:"bypass_blocked"`
	Extras        []trafficExtraDTO `json:"extras"`
	Packs         []trafficPackDTO  `json:"packs"`
}

// customerFor — клиент аккаунта. status != 0 — что отвечать, если клиента нет.
func (h *TrafficHandler) customerFor(r *http.Request) (c *database.Customer, status int) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		return nil, http.StatusUnauthorized
	}
	link, err := h.bootstrap.EnsureForAccount(r.Context(), claims.AccountID, "")
	if err != nil || link == nil {
		return nil, http.StatusInternalServerError
	}
	c, err = h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil || c == nil {
		return nil, http.StatusInternalServerError
	}
	return c, 0
}

// Get — GET /cabinet/api/me/traffic.
func (h *TrafficHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, status := h.customerFor(r)
	if c == nil {
		if status == http.StatusInternalServerError {
			// График и плитка — украшение главной: при сбое отдаём «пусто», а не ошибку.
			writeJSON(w, http.StatusOK, trafficOverviewResp{Label: config.TrafficBypassLabel(), Extras: []trafficExtraDTO{}, Packs: []trafficPackDTO{}})
			return
		}
		http.Error(w, "unauthorized", status)
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	resp := trafficOverviewResp{
		Label:   config.TrafficBypassLabel(),
		ResetAt: database.TrafficPeriodEnd(now),
		Extras:  []trafficExtraDTO{},
		Packs:   []trafficPackDTO{},
	}
	if !config.TrafficSplitEnabled() && !config.TrafficPacksEnabled() {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	extraMain, err := h.traffic.ActiveExtraBytes(ctx, c.ID, database.TrafficKindMain, now)
	if err != nil {
		slog.Warn("me traffic: extra main", "customer_id", c.ID, "error", err)
	}
	if h.rw != nil {
		if u, err := cabsvc.ResolveRemnawaveCustomerUser(ctx, h.rw, h.customers, c); err == nil && u != nil {
			resp.Main.UsedBytes = int64(u.UserTraffic.UsedTrafficBytes)
			resp.Main.LimitBytes = u.TrafficLimitBytes
			resp.Main.ExtraBytes = extraMain
			if u.TrafficLimitBytes > 0 {
				resp.Main.BaseBytes = u.TrafficLimitBytes - extraMain
				if resp.Main.BaseBytes < 0 {
					resp.Main.BaseBytes = 0
				}
			}
		}
	}

	if config.TrafficSplitEnabled() {
		if st, err := h.traffic.GetState(ctx, c.ID); err == nil && st != nil && st.LimitBytes > 0 {
			extra, _ := h.traffic.ActiveExtraBytes(ctx, c.ID, database.TrafficKindBypass, now)
			used := st.UsedBytes
			if !st.PeriodStart.UTC().Equal(database.TrafficPeriodStart(now)) {
				used = 0 // месяц сменился, воркер ещё не пересчитал
			}
			resp.Bypass = &trafficKindDTO{LimitBytes: st.LimitBytes + extra, BaseBytes: st.LimitBytes, ExtraBytes: extra, UsedBytes: used}
			resp.BypassBlocked = st.SquadRemoved && used >= resp.Bypass.LimitBytes
			// Панель считает один общий счётчик: обычные серверы — это всё, кроме обхода.
			if resp.Main.UsedBytes -= used; resp.Main.UsedBytes < 0 {
				resp.Main.UsedBytes = 0
			}
		}
	}

	if extras, err := h.traffic.ListActiveExtras(ctx, c.ID, now); err == nil {
		for _, e := range extras {
			resp.Extras = append(resp.Extras, trafficExtraDTO{Kind: e.Kind, Bytes: e.Bytes, ValidUntil: e.ValidUntil})
		}
	}

	active := c.ExpireAt != nil && c.ExpireAt.After(now)
	if config.TrafficPacksEnabled() && active && c.CurrentTariffID != nil {
		if packs, err := h.traffic.ListPacks(ctx, *c.CurrentTariffID, "", true); err == nil {
			for _, p := range packs {
				// Показываем только пакеты того вида, у которого есть лимит: докупать безлимит нечего.
				if p.Kind == database.TrafficKindMain && resp.Main.LimitBytes <= 0 {
					continue
				}
				if p.Kind == database.TrafficKindBypass && resp.Bypass == nil {
					continue
				}
				var stars *int
				if s := payment.TrafficPackStars(p.AmountRub, p.AmountStars); s > 0 {
					stars = &s
				}
				resp.Packs = append(resp.Packs, trafficPackDTO{ID: p.ID, Bytes: p.Bytes, AmountRub: p.AmountRub, AmountStars: stars, Kind: p.Kind})
			}
		}
	}
	resp.CanBuy = len(resp.Packs) > 0
	resp.Enabled = resp.Bypass != nil || resp.CanBuy
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}
