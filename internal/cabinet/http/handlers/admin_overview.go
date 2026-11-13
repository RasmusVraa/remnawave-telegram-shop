package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
)

// GET /cabinet/api/admin/overview — дашборд админки.
//
// Собирает две несвязанные вещи: оперативные счётчики из нашей базы и сводку
// панели Remnawave. Панель — внешняя зависимость, и её недоступность не должна
// ронять страницу: числа магазина показываются всегда, а вместо панели
// приезжает признак «связи нет» с причиной.

// panelTTL — как долго живёт кэш ответа панели.
//
// Дашборд открывают часто и держат открытым; без кэша каждый заход и каждое
// обновление — два запроса к панели. Полминуты достаточно, чтобы «онлайн
// сейчас» оставался правдой, и хватает, чтобы панель не дёргали десятками
// запросов подряд.
const panelTTL = 30 * time.Second

// panelTimeout — панель отвечает за доли секунды; если не ответила за это
// время, дашборд лучше показать без неё, чем заставлять ждать.
const panelTimeout = 6 * time.Second

type panelSnapshot struct {
	stats     *remnawave.SystemStats
	bandwidth *remnawave.BandwidthStats
	billing   *remnawave.InfraBillingNodesBody
	err       error
	at        time.Time
}

// Пороги по оплате серверов. Неделя — чтобы успеть пополнить счёт у
// провайдера и чтобы строка не висела там постоянно; трое суток — когда
// напоминание превращается в срочное дело.
const (
	billingSoonWindow   = 7 * 24 * time.Hour
	billingUrgentWindow = 3 * 24 * time.Hour
)

// AdminOverviewHandler — обработчик дашборда.
type AdminOverviewHandler struct {
	stats    *database.StatsRepository
	partners *database.PartnerRepository
	rw       *remnawave.Client

	mu    sync.Mutex
	cache map[string]panelSnapshot

	// incidents — горящие пункты «Мониторинга» (нода лежит, поддержка ждёт):
	// «Обзор» показывает их в той же полосе «Требует внимания», что и дела.
	incidents func(ctx context.Context) []AttentionItem
}

// SetIncidents подключает общий список инцидентов «Мониторинга».
func (h *AdminOverviewHandler) SetIncidents(fn func(ctx context.Context) []AttentionItem) {
	h.incidents = fn
}

// NewAdminOverview — конструктор.
func NewAdminOverview(
	stats *database.StatsRepository,
	partners *database.PartnerRepository,
	rw *remnawave.Client,
) *AdminOverviewHandler {
	return &AdminOverviewHandler{
		stats:    stats,
		partners: partners,
		rw:       rw,
		cache:    make(map[string]panelSnapshot),
	}
}

type adminBandwidthDTO struct {
	Current    string `json:"current"`
	Previous   string `json:"previous"`
	Difference string `json:"difference"`
}

type adminOverviewPanelDTO struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`

	Traffic struct {
		Today         adminBandwidthDTO `json:"today"`
		LastSevenDays adminBandwidthDTO `json:"last_seven_days"`
		LastThirty    adminBandwidthDTO `json:"last_thirty_days"`
		CalendarMonth adminBandwidthDTO `json:"calendar_month"`
		CurrentYear   adminBandwidthDTO `json:"current_year"`
	} `json:"traffic"`

	Online struct {
		Now         int64 `json:"now"`
		Today       int64 `json:"today"`
		Week        int64 `json:"week"`
		NeverOnline int64 `json:"never_online"`
	} `json:"online"`

	System struct {
		NodesOnline        int64   `json:"nodes_online"`
		TotalBytesLifetime string  `json:"total_bytes_lifetime"`
		MemoryUsed         float64 `json:"memory_used"`
		MemoryTotal        float64 `json:"memory_total"`
		CPUCores           int     `json:"cpu_cores"`
		UptimeSeconds      float64 `json:"uptime_seconds"`
	} `json:"system"`

	PanelUsers struct {
		Total        int64            `json:"total"`
		StatusCounts map[string]int64 `json:"status_counts"`
	} `json:"panel_users"`
}

type adminOverviewResp struct {
	CapturedAt string `json:"captured_at"`

	Shop struct {
		TotalCustomers      int64   `json:"total_customers"`
		ActiveSubscriptions int64   `json:"active_subscriptions"`
		RevenueTodayRub     float64 `json:"revenue_today_rub"`
		RevenueMonthRub     float64 `json:"revenue_month_rub"`
		SalesToday          int64   `json:"sales_today"`
		PayersToday         int64   `json:"payers_today"`
	} `json:"shop"`

	// Что ждёт действия администратора. Ради этого блока дашборд и открывают
	// каждый день — цифры смотрят реже.
	Attention struct {
		PartnerApplications int   `json:"partner_applications"`
		PartnerPayouts      int   `json:"partner_payouts"`
		OpenInvoices        int64 `json:"open_invoices"`
		// Оплата серверов: просрочена, горит (меньше трёх суток) и подходит
		// в ближайшую неделю.
		BillingOverdue   int `json:"billing_overdue"`
		BillingDueUrgent int `json:"billing_due_urgent"`
		BillingDueSoon   int `json:"billing_due_soon"`
		// Колесо ушло в минус: за месяц роздано днями подписки больше, чем
		// собрано за платные крутки. Ноль — колесо выключено или в плюсе.
		FortuneNetLossDays int64 `json:"fortune_net_loss_days"`
	} `json:"attention"`

	// Incidents — то же, что «Требует внимания» в «Мониторинге».
	Incidents []AttentionItem `json:"incidents"`

	Panel adminOverviewPanelDTO `json:"panel"`
}

// panelData возвращает сводку панели, переиспользуя недавний ответ.
func (h *AdminOverviewHandler) panelData(ctx context.Context, tz string) panelSnapshot {
	h.mu.Lock()
	if cached, ok := h.cache[tz]; ok && time.Since(cached.at) < panelTTL {
		h.mu.Unlock()
		return cached
	}
	h.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, panelTimeout)
	defer cancel()

	snap := panelSnapshot{at: time.Now()}
	stats, err := h.rw.GetSystemStats(callCtx, tz)
	if err != nil {
		snap.err = err
	} else {
		snap.stats = stats
		bw, bwErr := h.rw.GetBandwidthStats(callCtx, tz)
		if bwErr != nil {
			snap.err = bwErr
		} else {
			snap.bandwidth = bw
		}
		// Биллинг узлов необязателен: он может быть не заведён, и это не
		// повод считать всю панель недоступной.
		if billing, bErr := h.rw.GetInfraBillingNodes(callCtx); bErr != nil {
			slog.Warn("admin overview: infra billing unavailable", "error", bErr.Error())
		} else {
			snap.billing = billing
		}
	}

	h.mu.Lock()
	h.cache[tz] = snap
	h.mu.Unlock()
	return snap
}

// Overview — GET /cabinet/api/admin/overview?tz=Europe/Moscow (RequireAdmin).
func (h *AdminOverviewHandler) Overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tz := r.URL.Query().Get("tz")
	if _, err := time.LoadLocation(tz); tz != "" && err != nil {
		// Неизвестная зона — не повод падать: панель посчитает в своей.
		tz = ""
	}

	resp := adminOverviewResp{CapturedAt: time.Now().UTC().Format(time.RFC3339)}

	counters, err := h.stats.FetchAdminOverviewCounters(r.Context())
	if err != nil {
		slog.Error("admin overview: counters failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp.Shop.TotalCustomers = counters.TotalCustomers
	resp.Shop.ActiveSubscriptions = counters.ActiveSubscriptions
	resp.Shop.RevenueTodayRub = counters.RevenueTodayRub
	resp.Shop.RevenueMonthRub = counters.RevenueMonthRub
	resp.Shop.SalesToday = counters.SalesToday
	resp.Shop.PayersToday = counters.PayersToday
	resp.Attention.OpenInvoices = counters.OpenInvoices
	resp.Incidents = []AttentionItem{}
	if h.incidents != nil {
		resp.Incidents = h.incidents(r.Context())
	}

	// Колесо считаем только когда оно включено: у выключенного цифры за месяц
	// остаются от прошлой жизни и висели бы вечным напоминанием.
	if cabcfg.GetFortuneWheel().Enabled {
		if net := counters.FortuneWonDays - counters.FortunePaidDays; net > 0 {
			resp.Attention.FortuneNetLossDays = net
		}
	}

	// Партнёрская программа может быть не собрана — тогда просто нет счётчиков.
	if h.partners != nil {
		if work, wErr := h.partners.PendingWork(r.Context()); wErr != nil {
			slog.Error("admin overview: partner pending work failed", "error", wErr.Error())
		} else {
			resp.Attention.PartnerApplications = work.Applications
			resp.Attention.PartnerPayouts = work.Payouts
		}
	}

	if h.rw == nil {
		resp.Panel.Reason = "not_configured"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	snap := h.panelData(r.Context(), tz)
	if snap.err != nil || snap.stats == nil || snap.bandwidth == nil {
		if snap.err != nil {
			slog.Warn("admin overview: panel unavailable", "error", snap.err.Error())
		}
		resp.Panel.Reason = "unreachable"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Panel.Available = true
	resp.Panel.Traffic.Today = mapBandwidth(snap.bandwidth.LastTwoDays)
	resp.Panel.Traffic.LastSevenDays = mapBandwidth(snap.bandwidth.LastSevenDays)
	resp.Panel.Traffic.LastThirty = mapBandwidth(snap.bandwidth.Last30Days)
	resp.Panel.Traffic.CalendarMonth = mapBandwidth(snap.bandwidth.CalendarMonth)
	resp.Panel.Traffic.CurrentYear = mapBandwidth(snap.bandwidth.CurrentYear)

	resp.Panel.Online.Now = snap.stats.OnlineStats.OnlineNow
	resp.Panel.Online.Today = snap.stats.OnlineStats.LastDay
	resp.Panel.Online.Week = snap.stats.OnlineStats.LastWeek
	resp.Panel.Online.NeverOnline = snap.stats.OnlineStats.NeverOnline

	resp.Panel.System.NodesOnline = snap.stats.Nodes.TotalOnline
	resp.Panel.System.TotalBytesLifetime = snap.stats.Nodes.TotalBytesLifetime
	resp.Panel.System.MemoryUsed = snap.stats.Memory.Used
	resp.Panel.System.MemoryTotal = snap.stats.Memory.Total
	resp.Panel.System.CPUCores = snap.stats.CPU.Cores
	resp.Panel.System.UptimeSeconds = snap.stats.Uptime

	if snap.billing != nil {
		now := time.Now()
		urgent := now.Add(billingUrgentWindow)
		soon := now.Add(billingSoonWindow)
		for _, node := range snap.billing.BillingNodes {
			switch {
			case node.NextBillingAt.Before(now):
				resp.Attention.BillingOverdue++
			case node.NextBillingAt.Before(urgent):
				resp.Attention.BillingDueUrgent++
			case node.NextBillingAt.Before(soon):
				resp.Attention.BillingDueSoon++
			}
		}
	}

	resp.Panel.PanelUsers.Total = snap.stats.Users.TotalUsers
	resp.Panel.PanelUsers.StatusCounts = snap.stats.Users.StatusCounts
	if resp.Panel.PanelUsers.StatusCounts == nil {
		resp.Panel.PanelUsers.StatusCounts = map[string]int64{}
	}

	writeJSON(w, http.StatusOK, resp)
}

func mapBandwidth(p remnawave.BandwidthPeriod) adminBandwidthDTO {
	return adminBandwidthDTO{
		Current:    p.Current,
		Previous:   p.Previous,
		Difference: p.Difference,
	}
}
