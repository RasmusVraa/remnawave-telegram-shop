package remnawave

import (
	"context"
	"net/http"
	"strings"
)

// Состояние ноды для страницы «Ноды» в админке.
const (
	NodeStateOnline     = "online"
	NodeStateOffline    = "offline"
	NodeStateConnecting = "connecting"
	NodeStateDisabled   = "disabled"
)

// NodeOverview — нода панели со всем, что показывает страница «Ноды»: люди, нагрузка,
// скорость, трафик, uptime, версии, провайдер, плагин.
//
// Поля опциональны там, где панель отдаёт null (нода не в сети): nil-указатель значит «данных нет»,
// а не «ноль» — иначе на экране у отключённой ноды появились бы 0% и 0 Mb/s.
type NodeOverview struct {
	UUID        string
	Name        string
	CountryCode string
	Address     string
	State       string

	UsersOnline int

	Provider *NodeProvider
	// PluginName — имя плагина, назначенного ноде; пусто, если плагина нет.
	PluginName string

	// MemoryUsedPct — занято памяти, 0–100. nil — панель не прислала system.
	MemoryUsedPct *float64
	// LoadAvg — load average за 1/5/15 минут.
	LoadAvg []float64
	// CPUs — число ядер ноды: load average осмыслен только относительно него (0 — неизвестно).
	CPUs int
	// RxBytesPerSec / TxBytesPerSec — текущая скорость интерфейса ноды в байтах в секунду.
	// Rx — в ноду (скачивание), Tx — из ноды к клиентам (отдача).
	RxBytesPerSec *float64
	TxBytesPerSec *float64

	TrafficUsedBytes  int64
	TrafficLimitBytes int64 // 0 — без лимита
	// TrafficResetDay — день месяца, когда счётчик сбрасывается; 0, если ноде не включено отслеживание трафика.
	TrafficResetDay int

	XrayUptimeSec int64
	XrayVersion   string
	NodeVersion   string
}

// NodeProvider — провайдер, привязанный к ноде в панели.
type NodeProvider struct {
	Name        string
	FaviconLink string
}

type nodeOverviewWire struct {
	UUID                    string `json:"uuid"`
	Name                    string `json:"name"`
	Address                 string `json:"address"`
	CountryCode             string `json:"countryCode"`
	IsConnected             bool   `json:"isConnected"`
	IsConnecting            bool   `json:"isConnecting"`
	IsDisabled              bool   `json:"isDisabled"`
	IsTrafficTrackingActive bool   `json:"isTrafficTrackingActive"`
	TrafficResetDay         *int   `json:"trafficResetDay"`
	TrafficLimitBytes       *int64 `json:"trafficLimitBytes"`
	TrafficUsedBytes        *int64 `json:"trafficUsedBytes"`
	XrayUptime              *int64 `json:"xrayUptime"`
	UsersOnline             *int   `json:"usersOnline"`
	ActivePluginUUID        string `json:"activePluginUuid"`
	Provider                *struct {
		Name        string  `json:"name"`
		FaviconLink *string `json:"faviconLink"`
	} `json:"provider"`
	Versions *struct {
		Xray string `json:"xray"`
		Node string `json:"node"`
	} `json:"versions"`
	System *struct {
		Info struct {
			MemoryTotal float64 `json:"memoryTotal"`
			CPUs        int     `json:"cpus"`
		} `json:"info"`
		Stats struct {
			MemoryFree float64   `json:"memoryFree"`
			MemoryUsed float64   `json:"memoryUsed"`
			LoadAvg    []float64 `json:"loadAvg"`
			Interface  *struct {
				RxBytesPerSec float64 `json:"rxBytesPerSec"`
				TxBytesPerSec float64 `json:"txBytesPerSec"`
			} `json:"interface"`
		} `json:"stats"`
	} `json:"system"`
}

type nodePluginsWire struct {
	NodePlugins []struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"nodePlugins"`
}

func overviewState(n nodeOverviewWire) string {
	switch {
	case n.IsDisabled:
		return NodeStateDisabled
	case n.IsConnecting:
		return NodeStateConnecting
	case n.IsConnected:
		return NodeStateOnline
	default:
		return NodeStateOffline
	}
}

func overviewFromWire(n nodeOverviewWire, plugins map[string]string) NodeOverview {
	out := NodeOverview{
		UUID:        strings.ToLower(strings.TrimSpace(n.UUID)),
		Name:        strings.TrimSpace(n.Name),
		CountryCode: strings.ToUpper(strings.TrimSpace(n.CountryCode)),
		Address:     strings.TrimSpace(n.Address),
		State:       overviewState(n),
	}
	if n.UsersOnline != nil {
		out.UsersOnline = *n.UsersOnline
	}
	if n.TrafficUsedBytes != nil {
		out.TrafficUsedBytes = *n.TrafficUsedBytes
	}
	if n.TrafficLimitBytes != nil {
		out.TrafficLimitBytes = *n.TrafficLimitBytes
	}
	// Без отслеживания трафика день сброса ничего не значит: панель его тоже не показывает.
	if n.IsTrafficTrackingActive && n.TrafficResetDay != nil {
		out.TrafficResetDay = *n.TrafficResetDay
	}
	if n.XrayUptime != nil {
		out.XrayUptimeSec = *n.XrayUptime
	}
	if n.Provider != nil && strings.TrimSpace(n.Provider.Name) != "" {
		p := &NodeProvider{Name: strings.TrimSpace(n.Provider.Name)}
		if n.Provider.FaviconLink != nil {
			p.FaviconLink = strings.TrimSpace(*n.Provider.FaviconLink)
		}
		out.Provider = p
	}
	// Плагин показываем, только если он назначен и нашёлся по uuid: голый uuid на экране бесполезен.
	if id := strings.ToLower(strings.TrimSpace(n.ActivePluginUUID)); id != "" {
		out.PluginName = plugins[id]
	}
	if n.Versions != nil {
		out.XrayVersion = strings.TrimSpace(n.Versions.Xray)
		out.NodeVersion = strings.TrimSpace(n.Versions.Node)
	}
	if n.System != nil {
		st := n.System.Stats
		if total := n.System.Info.MemoryTotal; total > 0 {
			pct := st.MemoryUsed / total * 100
			out.MemoryUsedPct = &pct
		} else if sum := st.MemoryUsed + st.MemoryFree; sum > 0 {
			pct := st.MemoryUsed / sum * 100
			out.MemoryUsedPct = &pct
		}
		out.LoadAvg = st.LoadAvg
		out.CPUs = n.System.Info.CPUs
		if st.Interface != nil {
			rx, tx := st.Interface.RxBytesPerSec, st.Interface.TxBytesPerSec
			out.RxBytesPerSec, out.TxBytesPerSec = &rx, &tx
		}
	}
	return out
}

// ListNodesOverview — GET /api/nodes плюс имена плагинов из GET /api/node-plugins.
// Плагины необязательны: старая панель без этой ручки или сбой запроса не должны ронять список нод.
func (r *Client) ListNodesOverview(ctx context.Context) ([]NodeOverview, error) {
	var resp apiResponse[[]nodeOverviewWire]
	if err := r.doJSON(ctx, http.MethodGet, "/api/nodes", nil, &resp); err != nil {
		return nil, err
	}

	plugins := map[string]string{}
	for _, n := range resp.Response {
		if strings.TrimSpace(n.ActivePluginUUID) != "" {
			var pr apiResponse[nodePluginsWire]
			if err := r.doJSON(ctx, http.MethodGet, "/api/node-plugins", nil, &pr); err == nil {
				for _, p := range pr.Response.NodePlugins {
					plugins[strings.ToLower(strings.TrimSpace(p.UUID))] = strings.TrimSpace(p.Name)
				}
			}
			break
		}
	}

	out := make([]NodeOverview, 0, len(resp.Response))
	for _, n := range resp.Response {
		out = append(out, overviewFromWire(n, plugins))
	}
	return out, nil
}
