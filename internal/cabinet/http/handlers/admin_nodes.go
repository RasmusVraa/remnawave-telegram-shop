package handlers

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/remnawave"
)

// nodesCacheTTL — сколько держим ответ панели. Страница опрашивает раз в 10 с; с кэшем несколько
// открытых вкладок и несколько админов дают панели один запрос за срок, а не по запросу на каждого.
const nodesCacheTTL = 5 * time.Second

type AdminNodesHandler struct {
	rw *remnawave.Client

	mu      sync.Mutex
	cached  []remnawave.NodeOverview
	cacheAt time.Time
}

func NewAdminNodes(rw *remnawave.Client) *AdminNodesHandler {
	return &AdminNodesHandler{rw: rw}
}

type adminNodeProviderDTO struct {
	Name        string `json:"name"`
	FaviconLink string `json:"favicon_link,omitempty"`
}

type adminNodeDTO struct {
	UUID        string                `json:"uuid"`
	Name        string                `json:"name"`
	CountryCode string                `json:"country_code"`
	Address     string                `json:"address"`
	State       string                `json:"state"`
	UsersOnline int                   `json:"users_online"`
	Provider    *adminNodeProviderDTO `json:"provider,omitempty"`
	PluginName  string                `json:"plugin_name,omitempty"`

	// Живые метрики. null — панель их не прислала (нода не в сети).
	MemoryUsedPct *float64  `json:"memory_used_pct"`
	LoadAvg       []float64 `json:"load_avg"`
	CPUs          int       `json:"cpus"`
	RxBytesPerSec *float64  `json:"rx_bytes_per_sec"`
	TxBytesPerSec *float64  `json:"tx_bytes_per_sec"`

	TrafficUsedBytes  int64 `json:"traffic_used_bytes"`
	TrafficLimitBytes int64 `json:"traffic_limit_bytes"`
	TrafficResetDay   int   `json:"traffic_reset_day"`

	XrayUptimeSec int64  `json:"xray_uptime_sec"`
	XrayVersion   string `json:"xray_version,omitempty"`
	NodeVersion   string `json:"node_version,omitempty"`
}

type adminNodesDTO struct {
	Nodes []adminNodeDTO `json:"nodes"`
}

func nodeToDTO(n remnawave.NodeOverview) adminNodeDTO {
	d := adminNodeDTO{
		UUID:              n.UUID,
		Name:              n.Name,
		CountryCode:       n.CountryCode,
		Address:           n.Address,
		State:             n.State,
		UsersOnline:       n.UsersOnline,
		PluginName:        n.PluginName,
		MemoryUsedPct:     n.MemoryUsedPct,
		LoadAvg:           n.LoadAvg,
		CPUs:              n.CPUs,
		RxBytesPerSec:     n.RxBytesPerSec,
		TxBytesPerSec:     n.TxBytesPerSec,
		TrafficUsedBytes:  n.TrafficUsedBytes,
		TrafficLimitBytes: n.TrafficLimitBytes,
		TrafficResetDay:   n.TrafficResetDay,
		XrayUptimeSec:     n.XrayUptimeSec,
		XrayVersion:       n.XrayVersion,
		NodeVersion:       n.NodeVersion,
	}
	if n.Provider != nil {
		d.Provider = &adminNodeProviderDTO{Name: n.Provider.Name, FaviconLink: n.Provider.FaviconLink}
	}
	return d
}

func (h *AdminNodesHandler) load(r *http.Request) ([]remnawave.NodeOverview, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cached != nil && time.Since(h.cacheAt) < nodesCacheTTL {
		return h.cached, nil
	}
	nodes, err := h.rw.ListNodesOverview(r.Context())
	if err != nil {
		return nil, err
	}
	h.cached, h.cacheAt = nodes, time.Now()
	return nodes, nil
}

// List — GET /cabinet/api/admin/nodes: ноды панели с людьми, нагрузкой, скоростью, трафиком и версиями.
func (h *AdminNodesHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.rw == nil {
		http.Error(w, "panel not configured", http.StatusServiceUnavailable)
		return
	}
	nodes, err := h.load(r)
	if err != nil {
		slog.Error("admin nodes: list", "error", err)
		http.Error(w, "failed to fetch nodes", http.StatusBadGateway)
		return
	}
	out := adminNodesDTO{Nodes: make([]adminNodeDTO, 0, len(nodes))}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, nodeToDTO(n))
	}
	writeJSON(w, http.StatusOK, out)
}
