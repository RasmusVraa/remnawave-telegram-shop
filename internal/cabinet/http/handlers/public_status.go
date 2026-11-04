package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/statusprobe"
)

// publicStatusTTL — сколько собранный ответ живёт в памяти. Страница обновляется
// раз в 30 секунд, замер идёт в фоне, так что чаще пересобирать ответ незачем.
const publicStatusTTL = 10 * time.Second

// PublicStatus — GET /cabinet/api/public/status.
// Имена и доступность узлов отдаются сразу. Пинг зондов подмешивается из
// последнего замера и не блокирует ответ. Адрес узла в JSON не входит.
type PublicStatus struct {
	rw         *remnawave.Client
	probes     *statusprobe.Runner
	configured *statusprobe.KVTargets

	mu         sync.Mutex
	cachedAt   time.Time
	cached     []remnawave.ProbeTarget
	hasCache   bool
	refreshing bool

	cfgMu     sync.Mutex
	cfgAt     time.Time
	cfgCached []statusprobe.ConfiguredTarget
	cfgOK     bool

	bodyMu sync.Mutex
	bodyAt time.Time
	body   []byte
}

func NewPublicStatus(rw *remnawave.Client, probes *statusprobe.Runner, configured *statusprobe.KVTargets) *PublicStatus {
	return &PublicStatus{rw: rw, probes: probes, configured: configured}
}

// HasServers — в списке есть хотя бы один включённый сервер. Список берётся из кэша на 15 секунд.
func (h *PublicStatus) HasServers(ctx context.Context) bool {
	return hasEnabledTargets(h.savedTargets(ctx))
}

// Invalidate сбрасывает кэши: после правки списка админ сразу видит результат.
func (h *PublicStatus) Invalidate() {
	h.cfgMu.Lock()
	h.cfgOK = false
	h.cfgMu.Unlock()
	h.bodyMu.Lock()
	h.body = nil
	h.bodyMu.Unlock()
}

type publicProbe struct {
	WorldPingMs *int                   `json:"world_ping_ms,omitempty"`
	WorldOK     int                    `json:"world_ok"`
	WorldTotal  int                    `json:"world_total"`
	RussiaOK    int                    `json:"russia_ok"`
	RussiaTotal int                    `json:"russia_total"`
	History     []*float64             `json:"history,omitempty"`
	MeasuredAt  string                 `json:"measured_at,omitempty"`
	NextMin     int                    `json:"next_min,omitempty"`
	Hits        []statusprobe.ProbeHit `json:"hits,omitempty"`
}

type publicStatusNode struct {
	ID        string       `json:"id,omitempty"`
	Name      string       `json:"name"`
	Country   string       `json:"country,omitempty"`
	Note      string       `json:"note,omitempty"`
	Kind      string       `json:"kind,omitempty"`
	Whitelist bool         `json:"whitelist,omitempty"`
	State     string       `json:"state"`
	Probe     *publicProbe `json:"probe,omitempty"`
	// NoProbe — строку не меряют внешние зонды (адрес не отдаётся наружу), статус только из панели.
	NoProbe bool `json:"no_probe,omitempty"`

	// exit — состояние зарубежного выхода каскада; наружу не отдаётся.
	exit string
}

type publicStatusBody struct {
	Available        bool               `json:"available"`
	UpdatedAt        string             `json:"updated_at,omitempty"`
	Online           int                `json:"online"`
	Total            int                `json:"total"`
	ShowMap          bool               `json:"show_map"`
	ProbesEnabled    bool               `json:"probes_enabled"`
	// BypassProbes — операторы с белыми списками проверяются (задан ключ Latency Lab).
	BypassProbes     bool               `json:"bypass_probes"`
	ProbeWorld       int                `json:"probe_world"`
	ProbeRussia      int                `json:"probe_russia"`
	ProbeIntervalMin int                `json:"probe_interval_min"`
	Title            string             `json:"title,omitempty"`
	Lead             string             `json:"lead,omitempty"`
	Nodes            []publicStatusNode `json:"nodes"`
}

func (h *PublicStatus) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.cachedBody())
}

func (h *PublicStatus) cachedBody() []byte {
	h.bodyMu.Lock()
	defer h.bodyMu.Unlock()
	if h.body != nil && time.Since(h.bodyAt) < publicStatusTTL {
		return h.body
	}
	// Свой контекст: ответ кэшируется для всех, отмена первого запроса не должна его портить.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(h.build(ctx))
	if err != nil {
		return []byte(`{"available":false,"nodes":[]}`)
	}
	h.body = raw
	h.bodyAt = time.Now()
	return raw
}

func (h *PublicStatus) build(ctx context.Context) publicStatusBody {
	configured := h.savedTargets(ctx)
	// Локации только из ручного списка. Панель нужна, чтобы подкрасить
	// добавленную строку по узлу (UUID или адрес). Пустой список
	// не подставляет узлы Remnawave.
	var panel []remnawave.ProbeTarget
	if hasEnabledTargets(configured) {
		panel = h.panelCached()
	}
	if h.probes != nil {
		h.probes.EnsureLoaded(ctx)
	}
	// Замер запускает только планировщик. Открытие страницы читает уже готовый снимок.
	targets, kick := mergeStatusTargets(panel, configured)

	world, russia := statusprobe.ProbeLimits()
	title, lead := statusprobe.PageCopy()
	body := publicStatusBody{
		Nodes:            []publicStatusNode{},
		ShowMap:          statusprobe.ShowMap(),
		ProbesEnabled:    statusprobe.ProbesEnabled(),
		BypassProbes:     statusprobe.LatencyLabConfigured(),
		ProbeWorld:       world,
		ProbeRussia:      russia,
		ProbeIntervalMin: intervalMinutes(configured),
		Title:            title,
		Lead:             lead,
	}
	if targets == nil {
		return body
	}
	body.Available = true
	body.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	body.Nodes = make([]publicStatusNode, 0, len(targets))
	for i, n := range targets {
		key := ""
		minutes := 0
		external := true
		if i < len(kick) {
			key = kick[i].Key
			minutes = int(kick[i].Interval / time.Minute)
			external = !kick[i].NoExternal
		}
		node := n
		node.NoProbe = !external
		var measured *statusprobe.Sample
		// Строка без внешних зондов замеров не показывает: старый снимок остался бы навсегда.
		if h.probes != nil && external {
			sample, history, ok := h.probes.Snapshot(key)
			if ok && (sample.WorldTotal > 0 || sample.RussiaTotal > 0 || sample.WhitelistTotal > 0 || len(sample.Probes) > 0) {
				if usableForState(sample) {
					measured = &sample
				}
				stamp := ""
				if !sample.MeasuredAt.IsZero() {
					stamp = sample.MeasuredAt.UTC().Format(time.RFC3339)
				}
				hits := make([]statusprobe.ProbeHit, 0, len(sample.Probes))
				for _, hit := range sample.Probes {
					if hit.Kind == "whitelist" && (!n.Whitelist || strings.TrimSpace(hit.City) != "") {
						continue
					}
					hits = append(hits, hit)
				}
				node.Probe = &publicProbe{
					WorldOK:     sample.WorldOK,
					WorldTotal:  sample.WorldTotal,
					RussiaOK:    sample.RussiaOK,
					RussiaTotal: sample.RussiaTotal,
					History:     history,
					MeasuredAt:  stamp,
					NextMin:     statusprobe.NextPingMinutes(minutes, sample),
					Hits:        hits,
				}
			}
		}
		node.State = statusprobe.EffectiveState(n.State, measured, n.exit)
		if node.State == statusprobe.StateUp {
			body.Online++
		}
		body.Nodes = append(body.Nodes, node)
	}
	body.Total = len(body.Nodes)
	return body
}

// staleSample — замер старше этого срока не влияет на статус: зонды могли встать
// (лимит сервиса, выключенная проверка), и старый провал не должен висеть вечно.
const staleSample = 6 * time.Hour

func usableForState(sample statusprobe.Sample) bool {
	if !statusprobe.ProbesEnabled() {
		return false
	}
	return !sample.MeasuredAt.IsZero() && time.Since(sample.MeasuredAt) < staleSample
}

// panelCached отдаёт уже известные узлы сразу и обновляет их в фоне.
func (h *PublicStatus) panelCached() []remnawave.ProbeTarget {
	h.mu.Lock()
	cold := !h.hasCache && h.rw != nil && !h.refreshing
	if cold {
		h.refreshing = true
	}
	h.mu.Unlock()
	if cold {
		// Первый ответ после запуска ждёт панель: без неё все строки выглядели бы «узел не найден».
		h.refreshPanel()
	}
	h.mu.Lock()
	out := append([]remnawave.ProbeTarget(nil), h.cached...)
	fresh := h.hasCache && time.Since(h.cachedAt) < 20*time.Second
	start := !fresh && !h.refreshing && h.rw != nil
	if start {
		h.refreshing = true
	}
	h.mu.Unlock()
	if start {
		go h.refreshPanel()
	}
	return out
}

func (h *PublicStatus) refreshPanel() {
	defer func() {
		h.mu.Lock()
		h.refreshing = false
		h.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	nodes, err := h.rw.ListProbeTargets(ctx)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.cached = nodes
	h.cachedAt = time.Now()
	h.hasCache = true
	h.mu.Unlock()
}

func hasEnabledTargets(configured []statusprobe.ConfiguredTarget) bool {
	for _, item := range configured {
		if item.Enabled && strings.TrimSpace(item.Address) != "" {
			return true
		}
	}
	return false
}

func (h *PublicStatus) savedTargets(ctx context.Context) []statusprobe.ConfiguredTarget {
	if h.configured == nil {
		return nil
	}
	h.cfgMu.Lock()
	if h.cfgOK && time.Since(h.cfgAt) < 15*time.Second {
		out := append([]statusprobe.ConfiguredTarget(nil), h.cfgCached...)
		h.cfgMu.Unlock()
		return out
	}
	h.cfgMu.Unlock()

	list, err := h.configured.Load(ctx)
	if err != nil {
		return nil
	}
	h.cfgMu.Lock()
	h.cfgCached = list
	h.cfgAt = time.Now()
	h.cfgOK = true
	h.cfgMu.Unlock()
	return list
}

// mergeStatusTargets показывает только страны, которые админ включил вручную.
// Пустой список не подменяется узлами панели. Состояние входа берётся у узла
// панели по UUID, а если UUID не задан — по совпадению адреса.
func mergeStatusTargets(panel []remnawave.ProbeTarget, configured []statusprobe.ConfiguredTarget) ([]publicStatusNode, []statusprobe.Target) {
	enabled := make([]statusprobe.ConfiguredTarget, 0, len(configured))
	for _, item := range configured {
		if item.Enabled && strings.TrimSpace(item.Address) != "" {
			enabled = append(enabled, item)
		}
	}
	if len(enabled) == 0 {
		return []publicStatusNode{}, nil
	}

	byAddr := map[string]string{}
	byUUID := map[string]string{}
	for _, n := range panel {
		byAddr[strings.TrimSpace(n.Address)] = n.State
		if n.UUID != "" {
			byUUID[n.UUID] = n.State
		}
	}
	out := make([]publicStatusNode, 0, len(enabled))
	for _, item := range enabled {
		state := ""
		if item.NodeUUID != "" {
			state = byUUID[item.NodeUUID]
		} else {
			state = byAddr[strings.TrimSpace(item.Address)]
		}
		kind := item.Kind
		if kind == "" {
			kind = statusprobe.KindRegular
			if item.IsBypass() {
				kind = statusprobe.KindBypass
			}
		}
		out = append(out, publicStatusNode{
			ID:        item.ID,
			Name:      item.Name,
			Country:   item.Country,
			Note:      strings.TrimSpace(item.Note),
			Kind:      kind,
			Whitelist: item.IsBypass(),
			State:     state,
			exit:      byUUID[item.ExitNodeUUID],
		})
	}
	return out, statusprobe.TargetsFrom(enabled)
}

func intervalMinutes(configured []statusprobe.ConfiguredTarget) int {
	min := 0
	for _, item := range configured {
		if !item.Enabled {
			continue
		}
		n := item.IntervalMin
		if n <= 0 {
			n = statusprobe.ProbeIntervalMinutes()
		}
		if min == 0 || n < min {
			min = n
		}
	}
	if min == 0 {
		return statusprobe.ProbeIntervalMinutes()
	}
	return min
}
