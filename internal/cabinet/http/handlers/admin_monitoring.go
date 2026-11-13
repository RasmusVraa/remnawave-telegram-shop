package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/config"
)

// AdminMonitoringHandler — страница «Мониторинг»: лента событий, счётчики за
// период и «Требует внимания».
type AdminMonitoringHandler struct {
	pool  *pgxpool.Pool
	store *adminevents.Store

	mu      sync.Mutex
	summary map[string]cachedSummary
}

// cachedSummary — счётчики за период. Несколько открытых вкладок не должны
// каждые 15 секунд пересчитывать месяц событий.
type cachedSummary struct {
	at  time.Time
	sum *adminevents.Summary
}

const summaryTTL = 30 * time.Second

// supportWaitingMaxAge — обращения без ответа старше этого не считаются.
const supportWaitingMaxAge = 7 * 24 * time.Hour

// NewAdminMonitoring — конструктор.
func NewAdminMonitoring(pool *pgxpool.Pool) *AdminMonitoringHandler {
	return &AdminMonitoringHandler{pool: pool, store: adminevents.NewStore(pool), summary: map[string]cachedSummary{}}
}

type adminEventsResp struct {
	Items []adminevents.Row `json:"items"`
	// BotActions — пишутся ли нажатия в боте: без этого переключатель в фильтре прячется.
	BotActions bool `json:"bot_actions_enabled"`
	// Total — всего событий клиента; только для первой страницы его истории.
	Total *int64 `json:"total,omitempty"`
}

// Events — GET /cabinet/api/admin/monitoring/events
// ?after_id=&before_id=&categories=money,users&bot_actions=1&customer_id=&limit=50
func (h *AdminMonitoringHandler) Events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	f := adminevents.ListFilter{
		AfterID:    parseInt64Param(q.Get("after_id")),
		BeforeID:   parseInt64Param(q.Get("before_id")),
		CustomerID: parseInt64Param(q.Get("customer_id")),
		BotActions: q.Get("bot_actions") == "1",
		Limit:      int(parseInt64Param(q.Get("limit"))),
	}
	if sec := parseInt64Param(q.Get("since")); sec > 0 {
		f.Since = time.Unix(sec, 0)
	}
	f.HideExpired = f.CustomerID == 0
	for _, c := range strings.Split(q.Get("categories"), ",") {
		c = strings.TrimSpace(c)
		for _, known := range adminevents.Categories {
			if c == known {
				f.Categories = append(f.Categories, c)
			}
		}
	}
	items, err := h.store.List(r.Context(), f)
	if err != nil {
		slog.Error("admin monitoring: list events", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := adminEventsResp{Items: items, BotActions: config.MonitoringBotActions()}
	if f.CustomerID > 0 && f.AfterID == 0 && f.BeforeID == 0 {
		if n, err := h.store.CountByCustomer(r.Context(), f.CustomerID); err == nil {
			resp.Total = &n
		} else {
			slog.Warn("admin monitoring: count customer events", "error", err)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

type adminMonitoringSummaryResp struct {
	Period string `json:"period"`
	*adminevents.Summary
	NodesOnline int  `json:"nodes_online"`
	NodesTotal  int  `json:"nodes_total"`
	NodesKnown  bool `json:"nodes_known"`
}

var monitoringPeriods = map[string]time.Duration{
	"hour":  time.Hour,
	"day":   24 * time.Hour,
	"week":  7 * 24 * time.Hour,
	"month": 30 * 24 * time.Hour,
}

// Summary — GET /cabinet/api/admin/monitoring/summary?period=hour|day|week|month
func (h *AdminMonitoringHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := r.URL.Query().Get("period")
	d, ok := monitoringPeriods[period]
	if !ok {
		period, d = "hour", time.Hour
	}
	h.mu.Lock()
	cached, ok := h.summary[period]
	h.mu.Unlock()
	sum := cached.sum
	if !ok || time.Since(cached.at) > summaryTTL {
		fresh, err := h.store.Summary(r.Context(), time.Now().Add(-d))
		if err != nil {
			slog.Error("admin monitoring: summary", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sum = fresh
		h.mu.Lock()
		h.summary[period] = cachedSummary{at: time.Now(), sum: fresh}
		h.mu.Unlock()
	}
	resp := adminMonitoringSummaryResp{Period: period, Summary: sum}
	if snap := adminevents.Nodes().Snapshot(); snap.Ready {
		resp.NodesKnown, resp.NodesOnline, resp.NodesTotal = true, snap.Online, snap.Total
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// AttentionItem — пункт «Требует внимания». Kind=incident — горит сейчас
// (минуты), task — дело на дни (оплата серверов, заявки партнёров).
type AttentionItem struct {
	Key      string         `json:"key"`
	Kind     string         `json:"kind"`
	Severity string         `json:"severity"`
	Count    int            `json:"count"`
	Since    *time.Time     `json:"since,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

// Attention — GET /cabinet/api/admin/attention: инциденты для «Мониторинга» и бейджа в меню.
func (h *AdminMonitoringHandler) Attention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": h.Incidents(r.Context())})
}

// Incidents — то, что горит прямо сейчас. Считается из текущего состояния при
// каждом запросе и в базе не хранится: пункт пропадает сам, когда проблема ушла.
func (h *AdminMonitoringHandler) Incidents(ctx context.Context) []AttentionItem {
	items := []AttentionItem{}
	snap := adminevents.Nodes().Snapshot()
	if snap.PanelDown {
		since := snap.PanelSince
		items = append(items, AttentionItem{Key: "panel_down", Kind: "incident", Severity: "crit", Count: 1, Since: &since})
	}
	if snap.Ready && !snap.PanelDown {
		for _, n := range snap.Down {
			since := n.Since
			items = append(items, AttentionItem{
				Key: "node_down", Kind: "incident", Severity: "crit", Count: 1, Since: &since,
				Params: map[string]any{"name": n.Name},
			})
		}
	}
	if h.pool == nil {
		return items
	}
	var err error
	// 0 в настройке — «ждёт ответа» не показывать. Обращения старше недели
	// тоже не считаем: на них уже не ответят, а плашка висела бы вечно.
	if waitMin := config.MonitoringSupportWaitMinutes(); waitMin > 0 {
		var (
			waiting int
			oldest  *time.Time
		)
		err = h.pool.QueryRow(ctx, `
			SELECT count(*), min(m.created_at)
			FROM cabinet_support_ticket t
			JOIN LATERAL (
				SELECT direction, created_at FROM cabinet_support_message
				WHERE ticket_id = t.id ORDER BY created_at DESC, id DESC LIMIT 1
			) m ON TRUE
			WHERE t.status = 'open' AND m.direction = 'in' AND m.created_at < $1 AND m.created_at > $2`,
			time.Now().Add(-time.Duration(waitMin)*time.Minute), time.Now().Add(-supportWaitingMaxAge)).Scan(&waiting, &oldest)
		if err != nil {
			slog.Warn("admin monitoring: support waiting", "error", err)
		} else if waiting > 0 {
			items = append(items, AttentionItem{Key: "support_waiting", Kind: "incident", Severity: "warn", Count: waiting, Since: oldest})
		}
	}
	var (
		failed      int
		failedSince *time.Time
	)
	err = h.pool.QueryRow(ctx, `
		SELECT count(*), min(m.created_at)
		FROM cabinet_support_message m
		JOIN cabinet_support_ticket t ON t.id = m.ticket_id
		WHERE t.status = 'open' AND m.delivery_status = 'failed'
		  AND m.created_at > now() - interval '24 hours'`).Scan(&failed, &failedSince)
	if err != nil {
		slog.Warn("admin monitoring: support delivery", "error", err)
	} else if failed > 0 {
		items = append(items, AttentionItem{Key: "support_undelivered", Kind: "incident", Severity: "crit", Count: failed, Since: failedSince})
	}
	return items
}

func parseInt64Param(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
