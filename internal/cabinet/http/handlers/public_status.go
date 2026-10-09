package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/statusprobe"
)

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
}

func NewPublicStatus(rw *remnawave.Client, probes *statusprobe.Runner, configured *statusprobe.KVTargets) *PublicStatus {
	return &PublicStatus{rw: rw, probes: probes, configured: configured}
}

type publicProbe struct {
	WorldPingMs *int                   `json:"world_ping_ms,omitempty"`
	WorldOK     int                    `json:"world_ok"`
	WorldTotal  int                    `json:"world_total"`
	RussiaOK    int                    `json:"russia_ok"`
	RussiaTotal int                    `json:"russia_total"`
	History     []*float64             `json:"history,omitempty"`
	MeasuredAt  string                 `json:"measured_at,omitempty"`
	Hits        []statusprobe.ProbeHit `json:"hits,omitempty"`
}

type publicStatusNode struct {
	Name      string       `json:"name"`
	Country   string       `json:"country,omitempty"`
	Note      string       `json:"note,omitempty"`
	Whitelist bool         `json:"whitelist,omitempty"`
	State     string       `json:"state"`
	Probe     *publicProbe `json:"probe,omitempty"`
}

type publicStatusBody struct {
	Available        bool               `json:"available"`
	UpdatedAt        string             `json:"updated_at,omitempty"`
	Online           int                `json:"online"`
	Total            int                `json:"total"`
	ShowMap          bool               `json:"show_map"`
	ProbesEnabled    bool               `json:"probes_enabled"`
	ProbeWorld       int                `json:"probe_world"`
	ProbeRussia      int                `json:"probe_russia"`
	ProbeIntervalMin int                `json:"probe_interval_min"`
	Title            string             `json:"title,omitempty"`
	Lead             string             `json:"lead,omitempty"`
	Nodes            []publicStatusNode `json:"nodes"`
}

func (h *PublicStatus) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	configured := h.savedTargets(r.Context())
	// Локации только из ручного списка. Панель нужна лишь чтобы подкрасить
	// уже добавленную страну, если её адрес совпал с узлом. Пустой список
	// не подставляет узлы Remnawave.
	var panel []remnawave.ProbeTarget
	if hasEnabledTargets(configured) {
		panel = h.panelCached()
	}
	if h.probes != nil {
		h.probes.EnsureLoaded(r.Context())
	}
	targets, kick := mergeStatusTargets(panel, configured)
	if h.probes != nil && len(kick) > 0 {
		h.probes.Kick(kick)
	}

	world, russia := statusprobe.ProbeLimits()
	title, lead := statusprobe.PageCopy()
	body := publicStatusBody{
		Nodes:            []publicStatusNode{},
		ShowMap:          statusprobe.ShowMap(),
		ProbesEnabled:    statusprobe.ProbesEnabled(),
		ProbeWorld:       world,
		ProbeRussia:      russia,
		ProbeIntervalMin: intervalMinutes(configured),
		Title:            title,
		Lead:             lead,
	}
	notes := map[string]string{}
	whitelist := map[string]bool{}
	for _, item := range configured {
		key := statusprobe.NodeKey(item.Country, item.Name)
		if note := strings.TrimSpace(item.Note); note != "" {
			notes[key] = note
		}
		if item.Whitelist {
			whitelist[key] = true
		}
	}
	if targets != nil {
		body.Available = true
		body.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		body.Nodes = make([]publicStatusNode, 0, len(targets))
		for _, n := range targets {
			if n.State == "up" {
				body.Online++
			}
			key := statusprobe.NodeKey(n.CountryCode, n.Name)
			node := publicStatusNode{
				Name:      n.Name,
				Country:   n.CountryCode,
				Note:      notes[key],
				Whitelist: whitelist[key],
				State:     n.State,
			}
			if h.probes != nil {
				sample, history, ok := h.probes.Snapshot(key)
				if ok && (sample.WorldTotal > 0 || sample.RussiaTotal > 0 || sample.WhitelistTotal > 0 || len(sample.Probes) > 0) {
					measured := ""
					if !sample.MeasuredAt.IsZero() {
						measured = sample.MeasuredAt.UTC().Format(time.RFC3339)
					}
					hits := append([]statusprobe.ProbeHit(nil), sample.Probes...)
					for i := range hits {
						if hits[i].Kind == "whitelist" {
							hits[i].PingMs = nil
						}
					}
					node.Probe = &publicProbe{
						WorldOK:     sample.WorldOK,
						WorldTotal:  sample.WorldTotal,
						RussiaOK:    sample.RussiaOK,
						RussiaTotal: sample.RussiaTotal,
						History:     history,
						MeasuredAt:  measured,
						Hits:        hits,
					}
				}
			}
			body.Nodes = append(body.Nodes, node)
		}
		body.Total = len(body.Nodes)
	}
	writeJSON(w, http.StatusOK, body)
}

// panelCached отдаёт уже известные узлы сразу и обновляет их в фоне.
func (h *PublicStatus) panelCached() []remnawave.ProbeTarget {
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
// Пустой список не подменяется узлами панели.
func mergeStatusTargets(panel []remnawave.ProbeTarget, configured []statusprobe.ConfiguredTarget) ([]remnawave.ProbeTarget, []statusprobe.Target) {
	enabled := make([]statusprobe.ConfiguredTarget, 0, len(configured))
	for _, item := range configured {
		if item.Enabled && strings.TrimSpace(item.Address) != "" {
			enabled = append(enabled, item)
		}
	}
	if len(enabled) == 0 {
		return []remnawave.ProbeTarget{}, nil
	}

	byAddr := map[string]string{}
	for _, n := range panel {
		byAddr[strings.TrimSpace(n.Address)] = n.State
	}
	out := make([]remnawave.ProbeTarget, 0, len(enabled))
	kick := make([]statusprobe.Target, 0, len(enabled))
	for _, item := range enabled {
		state := byAddr[strings.TrimSpace(item.Address)]
		if state == "" {
			state = "partial"
		}
		out = append(out, remnawave.ProbeTarget{
			Name:        item.Name,
			CountryCode: item.Country,
			State:       state,
			Address:     item.Address,
		})
		kick = append(kick, statusprobe.Target{
			Key:       statusprobe.NodeKey(item.Country, item.Name),
			Address:   item.Address,
			Interval:  time.Duration(item.IntervalMin) * time.Minute,
			World:     item.WorldProbes,
			Russia:    item.RussiaProbes,
			Whitelist: item.Whitelist,
		})
	}
	return out, kick
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
