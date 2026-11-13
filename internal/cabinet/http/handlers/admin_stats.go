package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// AdminStatsHandler — эндпоинты GET /cabinet/api/admin/stats и связанные.
type AdminStatsHandler struct {
	stats     *database.StatsRepository
	loyalty   *database.LoyaltyTierRepository
	customers *database.CustomerRepository
	promos    *database.PromoRepository
	partners  *database.PartnerRepository
}

// NewAdminStats — конструктор.
func NewAdminStats(
	stats *database.StatsRepository,
	loyalty *database.LoyaltyTierRepository,
	customers *database.CustomerRepository,
	promos *database.PromoRepository,
	partners *database.PartnerRepository,
) *AdminStatsHandler {
	return &AdminStatsHandler{
		stats:     stats,
		loyalty:   loyalty,
		customers: customers,
		promos:    promos,
		partners:  partners,
	}
}

type adminTopReferrerDTO struct {
	ReferrerID       int64   `json:"referrer_id"`
	CustomerID       int64   `json:"customer_id"`
	TelegramUsername *string `json:"telegram_username"`
	Nickname         *string `json:"nickname"`
	Referees         int64   `json:"referees"`
	PaidReferees     int64   `json:"paid_referees"`
	RevenueRub       float64 `json:"revenue_rub"`
	BonusDays        int64   `json:"bonus_days"`
}

type adminTariffStatDTO struct {
	TariffID         int64   `json:"tariff_id"`
	DisplayName      string  `json:"display_name"`
	SalesToday       int64   `json:"sales_today"`
	SalesWeek        int64   `json:"sales_week"`
	SalesMonth       int64   `json:"sales_month"`
	SalesHalfYear    int64   `json:"sales_half_year"`
	SalesYear        int64   `json:"sales_year"`
	SubsRevenueMonth float64 `json:"subs_revenue_month"`
	RevenueToday     float64 `json:"revenue_today"`
	RevenueWeek      float64 `json:"revenue_week"`
	RevenueHalfYear  float64 `json:"revenue_half_year"`
	RevenueYear      float64 `json:"revenue_year"`
	RevenueAll       float64 `json:"revenue_all"`
	ActivePaidUsers  int64   `json:"active_paid_users"`
}

type adminStatsResp struct {
	CapturedAt           string                `json:"captured_at"`
	TotalCustomers       int64                 `json:"total_customers"`
	ActiveSubscriptions  int64                 `json:"active_subscriptions"`
	NewToday             int64                 `json:"new_today"`
	NewWeek              int64                 `json:"new_week"`
	NewMonth             int64                 `json:"new_month"`
	NewPrevMonth         int64                 `json:"new_prev_month"`
	NewHalfYear          int64                 `json:"new_half_year"`
	NewYear              int64                 `json:"new_year"`
	TrialActive          int64                 `json:"trial_active"`
	PaidActive           int64                 `json:"paid_active"`
	Inactive             int64                 `json:"inactive"`
	InactivePaid         int64                 `json:"inactive_paid"`
	InactiveUnpaid       int64                 `json:"inactive_unpaid"`
	SalesSubToday        int64                 `json:"sales_sub_today"`
	SalesSubWeek         int64                 `json:"sales_sub_week"`
	SalesSubMonth        int64                 `json:"sales_sub_month"`
	SalesSubPrevMonth    int64                 `json:"sales_sub_prev_month"`
	SalesSubHalfYear     int64                 `json:"sales_sub_half_year"`
	SalesSubYear         int64                 `json:"sales_sub_year"`
	RevenueMonthRub      float64               `json:"revenue_month_rub"`
	RevenueTodayRub      float64               `json:"revenue_today_rub"`
	RevenueWeekRub       float64               `json:"revenue_week_rub"`
	RevenueHalfYearRub   float64               `json:"revenue_half_year_rub"`
	RevenueYearRub       float64               `json:"revenue_year_rub"`
	RevenueAllTimeRub    float64               `json:"revenue_all_time_rub"`
	RevenueSubsMonthRub  float64               `json:"revenue_subs_month_rub"`
	TransactionsToday    int64                 `json:"transactions_today"`
	TransactionsWeek     int64                 `json:"transactions_week"`
	TransactionsMonth    int64                 `json:"transactions_month"`
	TransactionsHalfYear int64                 `json:"transactions_half_year"`
	TransactionsYear     int64                 `json:"transactions_year"`
	UniquePayersDay      int64                 `json:"unique_payers_day"`
	UniquePayersWeek     int64                 `json:"unique_payers_week"`
	UniquePayersMonth    int64                 `json:"unique_payers_month"`
	UniquePayersHalfYear int64                 `json:"unique_payers_half_year"`
	UniquePayersYear     int64                 `json:"unique_payers_year"`
	PaymentRubByInvoice  map[string]float64    `json:"payment_rub_by_invoice"`
	DistinctReferrers    int64                 `json:"distinct_referrers"`
	ActiveReferrers      int64                 `json:"active_referrers"`
	RefBonusDaysAll      int64                 `json:"ref_bonus_days_all"`
	RefBonusDaysToday    int64                 `json:"ref_bonus_days_today"`
	RefBonusDaysWeek     int64                 `json:"ref_bonus_days_week"`
	RefBonusDaysMonth    int64                 `json:"ref_bonus_days_month"`
	RefBonusDaysHalfYear int64                 `json:"ref_bonus_days_half_year"`
	RefBonusDaysYear     int64                 `json:"ref_bonus_days_year"`
	TopReferrers         []adminTopReferrerDTO `json:"top_referrers"`
	TariffBreakdown      []adminTariffStatDTO  `json:"tariff_breakdown"`
}

type adminFortunePeriodDTO struct {
	DistinctUsers     int64            `json:"distinct_users"`
	TotalSpins        int64            `json:"total_spins"`
	FreeSpins         int64            `json:"free_spins"`
	PaidSpins         int64            `json:"paid_spins"`
	PaidCostDaysSum   int64            `json:"paid_cost_days_sum"`
	WonSubsDaysSum    int64            `json:"won_subs_days_sum"`
	WonLoyaltyXPSum   int64            `json:"won_loyalty_xp_sum"`
	WonDiscountPctSum int64            `json:"won_discount_pct_sum"`
	ByReward          map[string]int64 `json:"by_reward"`
}

type adminFortuneStatsResp struct {
	CapturedAt string                `json:"captured_at"`
	Month      adminFortunePeriodDTO `json:"month"`
	Today      adminFortunePeriodDTO `json:"today"`
	AllTime    adminFortunePeriodDTO `json:"all_time"`
}

type adminStatsTimeSeriesPointDTO struct {
	Date         string  `json:"date"`
	RevenueRub   float64 `json:"revenue_rub"`
	Sales        int64   `json:"sales"`
	NewUsers     int64   `json:"new_users"`
	Transactions int64   `json:"transactions"`
}

type adminTariffTimeSeriesPointDTO struct {
	Date       string  `json:"date"`
	Sales      int64   `json:"sales"`
	RevenueRub float64 `json:"revenue_rub"`
}

type adminTariffTimeSeriesDTO struct {
	TariffID    int64                           `json:"tariff_id"`
	DisplayName string                          `json:"display_name"`
	Points      []adminTariffTimeSeriesPointDTO `json:"points"`
}

type adminStatsTimeSeriesResp struct {
	CapturedAt   string                         `json:"captured_at"`
	Period       string                         `json:"period"`
	Granularity  string                         `json:"granularity"`
	From         string                         `json:"from"`
	To           string                         `json:"to"`
	Points       []adminStatsTimeSeriesPointDTO `json:"points"`
	TariffSeries []adminTariffTimeSeriesDTO     `json:"tariff_series"`
}

// Stats — GET /cabinet/api/admin/stats (RequireAdmin).
func (h *AdminStatsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap, err := h.stats.FetchAdminStatsSnapshot(r.Context())
	if err != nil {
		slog.Error("admin stats: fetch snapshot failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	topRef := make([]adminTopReferrerDTO, 0, len(snap.TopReferrers))
	for _, tr := range snap.TopReferrers {
		topRef = append(topRef, adminTopReferrerDTO{
			ReferrerID:       tr.ReferrerID,
			CustomerID:       tr.CustomerID,
			TelegramUsername: tr.TelegramUsername,
			Nickname:         tr.Nickname,
			Referees:         tr.Referees,
			PaidReferees:     tr.PaidReferees,
			RevenueRub:       tr.RevenueRub,
			BonusDays:        tr.BonusDays,
		})
	}

	tariffs := make([]adminTariffStatDTO, 0, len(snap.TariffBreakdown))
	for _, ts := range snap.TariffBreakdown {
		tariffs = append(tariffs, adminTariffStatDTO{
			TariffID:         ts.TariffID,
			DisplayName:      ts.DisplayName,
			SalesToday:       ts.SalesToday,
			SalesWeek:        ts.SalesWeek,
			SalesMonth:       ts.SalesMonth,
			SalesHalfYear:    ts.SalesHalfYear,
			SalesYear:        ts.SalesYear,
			SubsRevenueMonth: ts.SubsRevenueMonth,
			RevenueToday:     ts.RevenueToday,
			RevenueWeek:      ts.RevenueWeek,
			RevenueHalfYear:  ts.RevenueHalfYear,
			RevenueYear:      ts.RevenueYear,
			RevenueAll:       ts.RevenueAll,
			ActivePaidUsers:  ts.ActivePaidUsers,
		})
	}

	resp := adminStatsResp{
		CapturedAt:           snap.CapturedAt.Format(time.RFC3339),
		TotalCustomers:       snap.TotalCustomers,
		ActiveSubscriptions:  snap.ActiveSubscriptions,
		NewToday:             snap.NewToday,
		NewWeek:              snap.NewWeek,
		NewMonth:             snap.NewMonth,
		NewPrevMonth:         snap.NewPrevMonth,
		NewHalfYear:          snap.NewHalfYear,
		NewYear:              snap.NewYear,
		TrialActive:          snap.TrialActive,
		PaidActive:           snap.PaidActive,
		Inactive:             snap.Inactive,
		InactivePaid:         snap.InactivePaid,
		InactiveUnpaid:       snap.InactiveUnpaid,
		SalesSubToday:        snap.SalesSubToday,
		SalesSubWeek:         snap.SalesSubWeek,
		SalesSubMonth:        snap.SalesSubMonth,
		SalesSubPrevMonth:    snap.SalesSubPrevMonth,
		SalesSubHalfYear:     snap.SalesSubHalfYear,
		SalesSubYear:         snap.SalesSubYear,
		RevenueMonthRub:      snap.RevenueMonthRub,
		RevenueTodayRub:      snap.RevenueTodayRub,
		RevenueWeekRub:       snap.RevenueWeekRub,
		RevenueHalfYearRub:   snap.RevenueHalfYearRub,
		RevenueYearRub:       snap.RevenueYearRub,
		RevenueAllTimeRub:    snap.RevenueAllTimeRub,
		RevenueSubsMonthRub:  snap.RevenueSubsMonthRub,
		TransactionsToday:    snap.TransactionsToday,
		TransactionsWeek:     snap.TransactionsWeek,
		TransactionsMonth:    snap.TransactionsMonth,
		TransactionsHalfYear: snap.TransactionsHalfYear,
		TransactionsYear:     snap.TransactionsYear,
		UniquePayersDay:      snap.UniquePayersDay,
		UniquePayersWeek:     snap.UniquePayersWeek,
		UniquePayersMonth:    snap.UniquePayersMonth,
		UniquePayersHalfYear: snap.UniquePayersHalfYear,
		UniquePayersYear:     snap.UniquePayersYear,
		PaymentRubByInvoice:  snap.PaymentRubByInvoice,
		DistinctReferrers:    snap.DistinctReferrers,
		ActiveReferrers:      snap.ActiveReferrers,
		RefBonusDaysAll:      snap.RefBonusDaysAll,
		RefBonusDaysToday:    snap.RefBonusDaysToday,
		RefBonusDaysWeek:     snap.RefBonusDaysWeek,
		RefBonusDaysMonth:    snap.RefBonusDaysMonth,
		RefBonusDaysHalfYear: snap.RefBonusDaysHalfYear,
		RefBonusDaysYear:     snap.RefBonusDaysYear,
		TopReferrers:         topRef,
		TariffBreakdown:      tariffs,
	}

	writeJSON(w, http.StatusOK, resp)
}

// TimeSeries — GET /cabinet/api/admin/stats/timeseries?period=month
// или ?from=YYYY-MM-DD&to=YYYY-MM-DD (RequireAdmin).
func (h *AdminStatsHandler) TimeSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	fromStr := q.Get("from")
	toStr := q.Get("to")

	var (
		series *database.AdminStatsTimeSeries
		err    error
	)

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
		series, err = h.stats.FetchAdminStatsTimeSeriesRange(r.Context(), fromDate, toDate)
		if err != nil {
			if errors.Is(err, database.ErrInvalidStatsTimeSeriesRange) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			slog.Error("admin stats: fetch timeseries range failed", "error", err.Error(), "from", fromStr, "to", toStr)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		period := q.Get("period")
		if period == "" {
			period = "month"
		}
		switch period {
		case "day", "week", "month", "half_year", "year", "all_time":
		default:
			http.Error(w, "invalid period", http.StatusBadRequest)
			return
		}

		series, err = h.stats.FetchAdminStatsTimeSeries(r.Context(), period)
		if err != nil {
			slog.Error("admin stats: fetch timeseries failed", "error", err.Error(), "period", period)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	points := make([]adminStatsTimeSeriesPointDTO, 0, len(series.Points))
	for _, p := range series.Points {
		points = append(points, adminStatsTimeSeriesPointDTO{
			Date:         p.Date,
			RevenueRub:   p.RevenueRub,
			Sales:        p.Sales,
			NewUsers:     p.NewUsers,
			Transactions: p.Transactions,
		})
	}

	tariffSeries := make([]adminTariffTimeSeriesDTO, 0, len(series.TariffSeries))
	for _, ts := range series.TariffSeries {
		pts := make([]adminTariffTimeSeriesPointDTO, 0, len(ts.Points))
		for _, p := range ts.Points {
			pts = append(pts, adminTariffTimeSeriesPointDTO{
				Date:       p.Date,
				Sales:      p.Sales,
				RevenueRub: p.RevenueRub,
			})
		}
		tariffSeries = append(tariffSeries, adminTariffTimeSeriesDTO{
			TariffID:    ts.TariffID,
			DisplayName: ts.DisplayName,
			Points:      pts,
		})
	}

	resp := adminStatsTimeSeriesResp{
		CapturedAt:   series.CapturedAt.Format(time.RFC3339),
		Period:       series.Period,
		Granularity:  series.Granularity,
		From:         series.From,
		To:           series.To,
		Points:       points,
		TariffSeries: tariffSeries,
	}

	writeJSON(w, http.StatusOK, resp)
}

// FortuneStats — GET /cabinet/api/admin/stats/fortune (RequireAdmin).
func (h *AdminStatsHandler) FortuneStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap, err := h.stats.FetchAdminFortuneStats(r.Context())
	if err != nil {
		slog.Error("admin stats: fetch fortune stats failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := adminFortuneStatsResp{
		CapturedAt: snap.CapturedAt.Format(time.RFC3339),
		Month:      mapFortunePeriod(snap.Month),
		Today:      mapFortunePeriod(snap.Today),
		AllTime:    mapFortunePeriod(snap.AllTime),
	}

	writeJSON(w, http.StatusOK, resp)
}

type adminLoyaltyTierStatDTO struct {
	SortOrder       int     `json:"sort_order"`
	XpMin           int64   `json:"xp_min"`
	DiscountPercent int     `json:"discount_percent"`
	DisplayName     *string `json:"display_name,omitempty"`
	UserCount       int64   `json:"user_count"`
}

type adminLoyaltyStatsResp struct {
	CapturedAt string                    `json:"captured_at"`
	Enabled    bool                      `json:"enabled"`
	Tiers      []adminLoyaltyTierStatDTO `json:"tiers"`
}

// LoyaltyStats — GET /cabinet/api/admin/stats/loyalty (RequireAdmin).
func (h *AdminStatsHandler) LoyaltyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := adminLoyaltyStatsResp{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Enabled:    config.LoyaltyEnabled(),
		Tiers:      []adminLoyaltyTierStatDTO{},
	}
	if !config.LoyaltyEnabled() || h.loyalty == nil || h.customers == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	tiers, err := h.loyalty.ListAllOrderedByXpMinAsc(r.Context())
	if err != nil {
		slog.Error("admin stats: loyalty tiers failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]adminLoyaltyTierStatDTO, 0, len(tiers))
	for i, t := range tiers {
		var maxEx *int64
		if i+1 < len(tiers) {
			nx := tiers[i+1].XpMin
			maxEx = &nx
		}
		n, cntErr := h.customers.CountCustomersLoyaltyXPHalfOpen(r.Context(), t.XpMin, maxEx)
		if cntErr != nil {
			slog.Error("admin stats: loyalty tier count failed", "error", cntErr, "tier", t.SortOrder)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out = append(out, adminLoyaltyTierStatDTO{
			SortOrder:       t.SortOrder,
			XpMin:           t.XpMin,
			DiscountPercent: t.DiscountPercent,
			DisplayName:     t.DisplayName,
			UserCount:       n,
		})
	}
	resp.Tiers = out
	writeJSON(w, http.StatusOK, resp)
}

type adminPromoStatsTopDTO struct {
	ID               int64  `json:"id"`
	Code             string `json:"code"`
	Active           bool   `json:"active"`
	UsesCount        int    `json:"uses_count"`
	Redemptions      int    `json:"redemptions"`
	Type             string `json:"type"`
	SubscriptionDays *int   `json:"subscription_days"`
	TrialDays        *int   `json:"trial_days"`
	ExtraHwidDelta   *int   `json:"extra_hwid_delta"`
	DiscountPercent  *int   `json:"discount_percent"`
}

type adminPromoStatsResp struct {
	CapturedAt       string                  `json:"captured_at"`
	Total            int                     `json:"total"`
	Active           int                     `json:"active"`
	Inactive         int                     `json:"inactive"`
	TotalRedemptions int                     `json:"total_redemptions"`
	RedemptionsToday int                     `json:"redemptions_today"`
	TopByRedemptions []adminPromoStatsTopDTO `json:"top_by_redemptions"`
}

// PromoStats — GET /cabinet/api/admin/stats/promos (RequireAdmin).
func (h *AdminStatsHandler) PromoStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promos == nil {
		http.Error(w, "promo repository not configured", http.StatusServiceUnavailable)
		return
	}

	snap, err := h.promos.AdminStatsSnapshot(r.Context())
	if err != nil {
		slog.Error("admin stats: promo snapshot failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	top := make([]adminPromoStatsTopDTO, 0, len(snap.TopByRedemptions))
	for _, row := range snap.TopByRedemptions {
		top = append(top, adminPromoStatsTopDTO{
			ID:               row.ID,
			Code:             row.Code,
			Active:           row.Active,
			UsesCount:        row.UsesCount,
			Redemptions:      row.Redemptions,
			Type:             row.Type,
			SubscriptionDays: row.SubscriptionDays,
			TrialDays:        row.TrialDays,
			ExtraHwidDelta:   row.ExtraHwidDelta,
			DiscountPercent:  row.DiscountPercent,
		})
	}

	writeJSON(w, http.StatusOK, adminPromoStatsResp{
		CapturedAt:       time.Now().UTC().Format(time.RFC3339),
		Total:            snap.Total,
		Active:           snap.Active,
		Inactive:         snap.Inactive,
		TotalRedemptions: snap.TotalRedemptions,
		RedemptionsToday: snap.RedemptionsToday,
		TopByRedemptions: top,
	})
}

func mapFortunePeriod(p database.AdminFortunePeriodAgg) adminFortunePeriodDTO {
	byReward := p.ByReward
	if byReward == nil {
		byReward = make(map[string]int64)
	}
	return adminFortunePeriodDTO{
		DistinctUsers:     p.DistinctUsers,
		TotalSpins:        p.TotalSpins,
		FreeSpins:         p.FreeSpins,
		PaidSpins:         p.PaidSpins,
		PaidCostDaysSum:   p.PaidCostDaysSum,
		WonSubsDaysSum:    p.WonSubsDaysSum,
		WonLoyaltyXPSum:   p.WonLoyaltyXPSum,
		WonDiscountPctSum: p.WonDiscountPctSum,
		ByReward:          byReward,
	}
}
