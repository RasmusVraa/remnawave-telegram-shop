package remnawave

import (
	"context"
	"net/http"
	"strings"
)

// NodePresence — публично безопасное состояние узла.
// Адрес и порт сюда не входят: их нельзя показывать на открытой странице статуса.
type NodePresence struct {
	Name        string
	CountryCode string
	// State: up | partial | down.
	State string
}

type nodeWire struct {
	Name         string `json:"name"`
	Address      string `json:"address"`
	CountryCode  string `json:"countryCode"`
	IsConnected  bool   `json:"isConnected"`
	IsDisabled   bool   `json:"isDisabled"`
	IsConnecting bool   `json:"isConnecting"`
}

// ProbeTarget — узел для замера. Address остаётся внутри процесса и не уходит в публичный JSON.
type ProbeTarget struct {
	Name        string
	CountryCode string
	State       string
	Address     string
}

func nodeState(n nodeWire) string {
	switch {
	case n.IsDisabled:
		return "down"
	case n.IsConnecting:
		return "partial"
	case n.IsConnected:
		return "up"
	default:
		return "down"
	}
}

// ListNodePresence читает GET /api/nodes и оставляет только имя, страну и доступность.
func (r *Client) ListNodePresence(ctx context.Context) ([]NodePresence, error) {
	targets, err := r.ListProbeTargets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]NodePresence, 0, len(targets))
	for _, t := range targets {
		out = append(out, NodePresence{
			Name:        t.Name,
			CountryCode: t.CountryCode,
			State:       t.State,
		})
	}
	return out, nil
}

// ListProbeTargets читает узлы панели вместе с адресом для внутреннего пинга.
func (r *Client) ListProbeTargets(ctx context.Context) ([]ProbeTarget, error) {
	var resp apiResponse[[]nodeWire]
	if err := r.doJSON(ctx, http.MethodGet, "/api/nodes", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]ProbeTarget, 0, len(resp.Response))
	for _, n := range resp.Response {
		name := strings.TrimSpace(n.Name)
		if name == "" {
			continue
		}
		out = append(out, ProbeTarget{
			Name:        name,
			CountryCode: strings.ToUpper(strings.TrimSpace(n.CountryCode)),
			State:       nodeState(n),
			Address:     strings.TrimSpace(n.Address),
		})
	}
	return out, nil
}
