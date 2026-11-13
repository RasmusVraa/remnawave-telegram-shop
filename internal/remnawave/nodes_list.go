package remnawave

import (
	"context"
	"net/http"
	"strings"
)

// NodeInfo — нода панели в объёме, нужном админке тарифа (выбор нод обхода).
type NodeInfo struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	CountryCode string `json:"countryCode"`
	IsConnected bool   `json:"isConnected"`
	IsDisabled  bool   `json:"isDisabled"`
	// IsTrafficTrackingActive — «Отслеживание трафика» в карточке ноды. Без него байты ноды
	// не попадают в статистику пользователей, и расход обхода будет занижен.
	IsTrafficTrackingActive bool `json:"isTrafficTrackingActive"`
	// ConfigProfile — профиль конфигурации ноды: какие inbound'ы она раздаёт.
	ConfigProfile struct {
		ActiveInbounds []struct {
			UUID string `json:"uuid"`
		} `json:"activeInbounds"`
	} `json:"configProfile"`
}

// ListNodes — GET /api/nodes.
func (r *Client) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	var resp apiResponse[[]NodeInfo]
	if err := r.doJSON(ctx, http.MethodGet, "/api/nodes", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Response, nil
}

// squadInbounds — internal squads панели с их inbound'ами (только то, что нужно для связи с нодами).
type squadInbounds struct {
	UUID     string `json:"uuid"`
	Inbounds []struct {
		UUID string `json:"uuid"`
	} `json:"inbounds"`
}

// NodesForSquads возвращает ноды, которые раздают хотя бы один inbound из указанных сквадов:
// сквад → inbound'ы, нода → активные inbound'ы её профиля конфигурации.
//
// filtered=false — панель не отдала нужных связей (старая версия или сквад без inbound'ов), и вернулись все ноды:
// фильтровать нечем, а показывать пустой список хуже.
func (r *Client) NodesForSquads(ctx context.Context, squadUUIDs []string) (nodes []NodeInfo, filtered bool, err error) {
	all, err := r.ListNodes(ctx)
	if err != nil {
		return nil, false, err
	}
	var resp apiResponse[struct {
		InternalSquads []squadInbounds `json:"internalSquads"`
	}]
	if err := r.doJSON(ctx, http.MethodGet, "/api/internal-squads", nil, &resp); err != nil {
		return all, false, nil
	}
	want := map[string]bool{}
	for _, u := range squadUUIDs {
		want[strings.ToLower(strings.TrimSpace(u))] = true
	}
	inbounds := map[string]bool{}
	for _, s := range resp.Response.InternalSquads {
		if !want[strings.ToLower(s.UUID)] {
			continue
		}
		for _, in := range s.Inbounds {
			inbounds[strings.ToLower(in.UUID)] = true
		}
	}
	if len(inbounds) == 0 {
		return all, false, nil
	}
	known := false
	for _, n := range all {
		if len(n.ConfigProfile.ActiveInbounds) > 0 {
			known = true
			break
		}
	}
	if !known {
		return all, false, nil
	}
	out := make([]NodeInfo, 0, len(all))
	for _, n := range all {
		for _, in := range n.ConfigProfile.ActiveInbounds {
			if inbounds[strings.ToLower(in.UUID)] {
				out = append(out, n)
				break
			}
		}
	}
	return out, true, nil
}
