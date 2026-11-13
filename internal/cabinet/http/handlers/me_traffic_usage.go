package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
)

// Расход трафика по дням за расчётный период — для графика на главной кабинета.
//
// Период считается от последнего сброса счётчика трафика в панели
// (lastTrafficResetAt), а не от начала календарного месяца: только так сумма
// на графике сходится с лимитом тарифа, который показан рядом.
//
// Если панель сброса не сообщает (поле пустое — например, тариф без лимита
// трафика), берём последние 30 суток: показать динамику всё равно полезно,
// а сходиться там не с чем.
const usageFallbackDays = 30

// Верхняя граница окна: панель считает посуточно, слишком длинный диапазон
// и грузит её, и не влезает в спарклайн.
const usageMaxDays = 92

type meTrafficUsageResp struct {
	// Enabled=false — интеграции нет или пользователь не заведён в панели.
	Enabled bool `json:"enabled"`
	// PeriodStart/PeriodEnd — границы окна в формате YYYY-MM-DD.
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
	// Categories — подписи точек, по одной на значение Points.
	Categories []string `json:"categories"`
	// Points — расход по дням в байтах.
	Points []float64 `json:"points"`
	// TotalBytes — сумма за период.
	TotalBytes float64 `json:"total_bytes"`
	// BypassPoints — часть расхода по дням, ушедшая на серверы обхода (та же длина, что Points).
	// Есть только у клиентов с отдельным лимитом обхода; остальное — обычные серверы.
	BypassPoints []float64 `json:"bypass_points,omitempty"`
}

func emptyTrafficUsage(enabled bool) meTrafficUsageResp {
	return meTrafficUsageResp{Enabled: enabled, Categories: []string{}, Points: []float64{}}
}

// GetTrafficUsage — GET /cabinet/api/me/traffic-usage.
//
// Отдаёт пустой ряд вместо ошибки, когда панель недоступна или пользователь
// в ней не заведён: график — украшение главной, из-за него страница падать
// не должна.
func (h *MeHandler) GetTrafficUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.rw == nil || h.bootstrap == nil || h.customers == nil {
		writeJSON(w, http.StatusOK, emptyTrafficUsage(false))
		return
	}

	link, err := h.bootstrap.EnsureForAccount(r.Context(), claims.AccountID, "")
	if err != nil || link == nil {
		if handleAccountGone(w, err, "me.traffic_usage", claims.AccountID) {
			return
		}
		writeJSON(w, http.StatusOK, emptyTrafficUsage(false))
		return
	}
	c, err := h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil || c == nil {
		writeJSON(w, http.StatusOK, emptyTrafficUsage(false))
		return
	}
	rwUser, err := cabsvc.ResolveRemnawaveCustomerUser(r.Context(), h.rw, h.customers, c)
	if err != nil || rwUser == nil {
		writeJSON(w, http.StatusOK, emptyTrafficUsage(false))
		return
	}

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -usageFallbackDays)
	if rwUser.LastTrafficResetAt != nil && !rwUser.LastTrafficResetAt.IsZero() {
		if reset := rwUser.LastTrafficResetAt.UTC(); reset.Before(end) {
			start = reset
		}
	}
	if earliest := end.AddDate(0, 0, -usageMaxDays); start.Before(earliest) {
		start = earliest
	}
	// График показывает текущий календарный месяц: прошлый месяц он всё равно не рисует, а запрашивать его незачем.
	if month := database.TrafficPeriodStart(end); start.Before(month) {
		start = month
	}

	// Раздельный трафик: лимиты считаются по календарному месяцу, и график должен
	// начинаться с 1-го числа, а не с последнего сброса счётчика в панели.
	var splitState *database.BypassState
	if h.traffic != nil && config.TrafficSplitEnabled() {
		if st, err := h.traffic.GetState(r.Context(), c.ID); err == nil && st != nil && st.LimitBytes > 0 {
			splitState = st
			if month := database.TrafficPeriodStart(end); month.Before(start) {
				start = month
			}
		}
	}

	series, err := h.rw.GetUserUsageByRange(r.Context(), rwUser.ID, start, end)
	if err != nil {
		slog.Warn("traffic usage: panel request failed",
			"account_id", claims.AccountID, "error", err.Error())
		writeJSON(w, http.StatusOK, emptyTrafficUsage(true))
		return
	}

	resp := meTrafficUsageResp{
		Enabled:     true,
		PeriodStart: start.Format(time.DateOnly),
		PeriodEnd:   end.Format(time.DateOnly),
		Categories:  series.Categories,
		Points:      series.Sparkline,
		TotalBytes:  series.Total(),
	}
	if resp.Categories == nil {
		resp.Categories = []string{}
	}
	if resp.Points == nil {
		resp.Points = []float64{}
	}

	if splitState != nil {
		resp.BypassPoints = bypassDailyPoints(series, splitState.NodeUUIDs)
	}

	// Данные меняются раз в сутки, но кэш держим коротким: страница открывается
	// часто, а лишний поход в панель на каждое открытие ни к чему.
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeJSON(w, http.StatusOK, resp)
}

// bypassDailyPoints — расход по дням только на нодах обхода (uuid через запятую).
func bypassDailyPoints(series *remnawave.UserUsageSeries, nodeUUIDs string) []float64 {
	want := map[string]struct{}{}
	for _, p := range strings.Split(nodeUUIDs, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			want[p] = struct{}{}
		}
	}
	out := make([]float64, len(series.Categories))
	for _, n := range series.Series {
		if _, ok := want[strings.ToLower(n.UUID)]; !ok {
			continue
		}
		for i, v := range n.Data {
			if i < len(out) {
				out[i] += v
			}
		}
	}
	return out
}
