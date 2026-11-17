package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// GET /cabinet/api/admin/stats/insights — метрики, которых не было в снимке:
// воронка, когда покупают, срок жизни подписки, продления против первых
// покупок, предыдущий период и сводка партнёрской программы.
//
// Отдельный эндпоинт, а не расширение /admin/stats, потому что запросы здесь
// заметно тяжелее (оконные агрегаты по purchase) и зависят от выбранного
// периода: класть их в снимок значило бы пересчитывать всё при каждом
// переключении фильтра.

type adminStatsFunnelDTO struct {
	Registered      int64 `json:"registered"`
	Invoiced        int64 `json:"invoiced"`
	Paid            int64 `json:"paid"`
	InvoicesCreated int64 `json:"invoices_created"`
	InvoicesPaid    int64 `json:"invoices_paid"`
}

type adminStatsHeatCellDTO struct {
	Weekday    int     `json:"weekday"`
	Hour       int     `json:"hour"`
	RevenueRub float64 `json:"revenue_rub"`
	Sales      int64   `json:"sales"`
}

type adminStatsLifetimeDTO struct {
	PayingCustomers int64   `json:"paying_customers"`
	AvgLifetimeDays float64 `json:"avg_lifetime_days"`
	AvgPaidMonths   float64 `json:"avg_paid_months"`
	AvgPurchases    float64 `json:"avg_purchases"`
}

type adminStatsRenewalsDTO struct {
	FirstCount     int64   `json:"first_count"`
	FirstRevenue   float64 `json:"first_revenue"`
	RenewalCount   int64   `json:"renewal_count"`
	RenewalRevenue float64 `json:"renewal_revenue"`
}

type adminStatsGatewayDTO struct {
	InvoiceType string  `json:"invoice_type"`
	RevenueRub  float64 `json:"revenue_rub"`
	Payments    int64   `json:"payments"`
}

type adminStatsWindowDTO struct {
	RevenueRub   float64 `json:"revenue_rub"`
	Sales        int64   `json:"sales"`
	NewUsers     int64   `json:"new_users"`
	Transactions int64   `json:"transactions"`
	UniquePayers int64   `json:"unique_payers"`
}

type adminPartnerTopDTO struct {
	PartnerID        int64   `json:"partner_id"`
	CustomerID       int64   `json:"customer_id"`
	TelegramID       int64   `json:"telegram_id"`
	TelegramUsername *string `json:"telegram_username"`
	Nickname         *string `json:"nickname"`
	Customers        int64   `json:"customers"`
	PayingCustomers  int64   `json:"paying_customers"`
	Earned           float64 `json:"earned"`
}

type adminPartnerProgramDTO struct {
	PartnersTotal     int64                `json:"partners_total"`
	PartnersActive    int64                `json:"partners_active"`
	PartnersPending   int64                `json:"partners_pending"`
	PartnersSuspended int64                `json:"partners_suspended"`
	Customers         int64                `json:"customers"`
	PayingCustomers   int64                `json:"paying_customers"`
	ActiveCustomers   int64                `json:"active_customers"`
	EarnedTotal       float64              `json:"earned_total"`
	EarnedPeriod      float64              `json:"earned_period"`
	EarnedFirst       float64              `json:"earned_first"`
	EarnedRenewal     float64              `json:"earned_renewal"`
	HoldBalance       float64              `json:"hold_balance"`
	AvailableBalance  float64              `json:"available_balance"`
	ReservedBalance   float64              `json:"reserved_balance"`
	PaidTotal         float64              `json:"paid_total"`
	OpenPayouts       int64                `json:"open_payouts"`
	OpenPayoutsAmount float64              `json:"open_payouts_amount"`
	Top               []adminPartnerTopDTO `json:"top"`
}

// adminChannelGateDTO — блок «Подписка на канал»: новые клиенты, увидевшие экран
// подписки в окне, и старые, которые его увидели и не подписались.
type adminChannelGateDTO struct {
	Enabled      bool   `json:"enabled"`
	Channel      string `json:"channel,omitempty"`
	Shown        int64  `json:"shown"`
	Passed       int64  `json:"passed"`
	NotPassed    int64  `json:"not_passed"`
	InProgress   int64  `json:"in_progress"`
	TriedFailed  int64  `json:"tried_failed"`
	OldNotPassed int64  `json:"old_not_passed"`
}

type adminStatsInsightsResp struct {
	CapturedAt      string                  `json:"captured_at"`
	Period          string                  `json:"period"`
	From            string                  `json:"from"`
	To              string                  `json:"to"`
	TZOffsetMinutes int                     `json:"tz_offset_minutes"`
	Funnel          adminStatsFunnelDTO     `json:"funnel"`
	Heatmap         []adminStatsHeatCellDTO `json:"heatmap"`
	Lifetime        adminStatsLifetimeDTO   `json:"lifetime"`
	Renewals        adminStatsRenewalsDTO   `json:"renewals"`
	Gateways        []adminStatsGatewayDTO  `json:"gateways"`
	Current         adminStatsWindowDTO     `json:"current"`
	Previous        adminStatsWindowDTO     `json:"previous"`
	Partners        *adminPartnerProgramDTO `json:"partners"`
	ChannelGate     *adminChannelGateDTO    `json:"channel_gate"`
}

// Insights — GET /cabinet/api/admin/stats/insights?period=month
// либо ?from=YYYY-MM-DD&to=YYYY-MM-DD, плюс необязательный ?tz=<минуты от UTC>
// для тепловой карты (RequireAdmin).
func (h *AdminStatsHandler) Insights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	now := time.Now().UTC()

	tzOffset := 0
	if raw := q.Get("tz"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "invalid tz offset", http.StatusBadRequest)
			return
		}
		tzOffset = parsed
	}

	var (
		from, to time.Time
		period   string
	)

	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr != "" || toStr != "" {
		if fromStr == "" || toStr == "" {
			http.Error(w, "from and to are required together", http.StatusBadRequest)
			return
		}
		fromDate, errFrom := time.Parse("2006-01-02", fromStr)
		toDate, errTo := time.Parse("2006-01-02", toStr)
		if errFrom != nil || errTo != nil {
			http.Error(w, "invalid from or to date (YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
		var err error
		from, to, _, err = database.ResolveStatsTimeSeriesCustomWindow(fromDate, toDate, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		period = "custom"
	} else {
		period = q.Get("period")
		if period == "" {
			period = "month"
		}
		switch period {
		case "day", "week", "month", "half_year", "year", "all_time":
		default:
			http.Error(w, "invalid period", http.StatusBadRequest)
			return
		}
		from, to, _ = database.ResolveStatsTimeSeriesWindow(period, now)
	}

	insights, err := h.stats.FetchAdminStatsInsights(r.Context(), period, from, to, tzOffset)
	if err != nil {
		if errors.Is(err, database.ErrInvalidStatsTimeSeriesRange) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Error("admin stats: fetch insights failed", "error", err.Error(), "period", period)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	heat := make([]adminStatsHeatCellDTO, 0, len(insights.Heatmap))
	for _, c := range insights.Heatmap {
		heat = append(heat, adminStatsHeatCellDTO{
			Weekday:    c.Weekday,
			Hour:       c.Hour,
			RevenueRub: c.RevenueRub,
			Sales:      c.Sales,
		})
	}

	gateways := make([]adminStatsGatewayDTO, 0, len(insights.Gateways))
	for _, g := range insights.Gateways {
		gateways = append(gateways, adminStatsGatewayDTO{
			InvoiceType: g.InvoiceType,
			RevenueRub:  g.RevenueRub,
			Payments:    g.Payments,
		})
	}

	resp := adminStatsInsightsResp{
		CapturedAt:      insights.CapturedAt.Format(time.RFC3339),
		Period:          insights.Period,
		From:            insights.From,
		To:              insights.To,
		TZOffsetMinutes: insights.TZOffsetMinutes,
		Funnel: adminStatsFunnelDTO{
			Registered:      insights.Funnel.Registered,
			Invoiced:        insights.Funnel.Invoiced,
			Paid:            insights.Funnel.Paid,
			InvoicesCreated: insights.Funnel.InvoicesCreated,
			InvoicesPaid:    insights.Funnel.InvoicesPaid,
		},
		Heatmap: heat,
		Lifetime: adminStatsLifetimeDTO{
			PayingCustomers: insights.Lifetime.PayingCustomers,
			AvgLifetimeDays: insights.Lifetime.AvgLifetimeDays,
			AvgPaidMonths:   insights.Lifetime.AvgPaidMonths,
			AvgPurchases:    insights.Lifetime.AvgPurchases,
		},
		Renewals: adminStatsRenewalsDTO{
			FirstCount:     insights.Renewals.FirstCount,
			FirstRevenue:   insights.Renewals.FirstRevenue,
			RenewalCount:   insights.Renewals.RenewalCount,
			RenewalRevenue: insights.Renewals.RenewalRevenue,
		},
		Gateways: gateways,
		Current:  mapStatsWindow(insights.Current),
		Previous: mapStatsWindow(insights.Previous),
	}

	// Партнёрская программа необязательна: репозиторий может быть не собран, а
	// ошибка сводки не должна ронять всю страницу статистики.
	if h.partners != nil {
		prog, perr := h.partners.AdminProgramStats(r.Context(), from, to, 10)
		if perr != nil {
			slog.Error("admin stats: partner program stats failed", "error", perr.Error())
		} else {
			resp.Partners = mapPartnerProgram(prog)
		}
	}

	// Подписка на канал — тоже необязательный блок: сбой подсчёта не роняет страницу.
	if cg, cerr := h.stats.ChannelGateStats(r.Context(), from, to); cerr != nil {
		slog.Error("admin stats: channel gate stats failed", "error", cerr.Error())
	} else {
		resp.ChannelGate = &adminChannelGateDTO{
			Enabled:      config.ChannelGateActive(),
			Channel:      config.ChannelGateChannel(),
			Shown:        cg.Shown,
			Passed:       cg.Passed,
			NotPassed:    cg.NotPassed,
			InProgress:   cg.InProgress,
			TriedFailed:  cg.TriedFailed,
			OldNotPassed: cg.OldNotPassed,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func mapStatsWindow(w database.AdminStatsWindowTotals) adminStatsWindowDTO {
	return adminStatsWindowDTO{
		RevenueRub:   w.RevenueRub,
		Sales:        w.Sales,
		NewUsers:     w.NewUsers,
		Transactions: w.Transactions,
		UniquePayers: w.UniquePayers,
	}
}

func mapPartnerProgram(p *database.PartnerProgramStats) *adminPartnerProgramDTO {
	top := make([]adminPartnerTopDTO, 0, len(p.Top))
	for _, row := range p.Top {
		top = append(top, adminPartnerTopDTO{
			PartnerID:        row.PartnerID,
			CustomerID:       row.CustomerID,
			TelegramID:       row.TelegramID,
			TelegramUsername: row.TelegramUsername,
			Nickname:         row.Nickname,
			Customers:        row.Customers,
			PayingCustomers:  row.PayingCustomers,
			Earned:           row.Earned,
		})
	}
	return &adminPartnerProgramDTO{
		PartnersTotal:     p.PartnersTotal,
		PartnersActive:    p.PartnersActive,
		PartnersPending:   p.PartnersPending,
		PartnersSuspended: p.PartnersSuspended,
		Customers:         p.Customers,
		PayingCustomers:   p.PayingCustomers,
		ActiveCustomers:   p.ActiveCustomers,
		EarnedTotal:       p.EarnedTotal,
		EarnedPeriod:      p.EarnedPeriod,
		EarnedFirst:       p.EarnedFirst,
		EarnedRenewal:     p.EarnedRenewal,
		HoldBalance:       p.HoldBalance,
		AvailableBalance:  p.AvailableBalance,
		ReservedBalance:   p.ReservedBalance,
		PaidTotal:         p.PaidTotal,
		OpenPayouts:       p.OpenPayouts,
		OpenPayoutsAmount: p.OpenPayoutsAmount,
		Top:               top,
	}
}
