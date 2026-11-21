package handlers

import (
	"log/slog"
	"net/http"

	"remnawave-tg-shop-bot/internal/remnawave"
)

// AdminSquadsHandler — GET /cabinet/api/admin/squads.
type AdminSquadsHandler struct {
	rw *remnawave.Client
}

func NewAdminSquads(rw *remnawave.Client) *AdminSquadsHandler {
	return &AdminSquadsHandler{rw: rw}
}

// List — GET /cabinet/api/admin/squads.
func (h *AdminSquadsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.rw == nil {
		http.Error(w, "panel not configured", http.StatusServiceUnavailable)
		return
	}

	squads, err := h.rw.ListInternalSquads(r.Context())
	if err != nil {
		slog.Error("admin squads list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]adminSquadDTO, 0, len(squads))
	for _, sq := range squads {
		items = append(items, adminSquadDTO{UUID: sq.UUID.String(), Name: sq.Name})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// ListExternal — GET /cabinet/api/admin/squads/external.
func (h *AdminSquadsHandler) ListExternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.rw == nil {
		http.Error(w, "panel not configured", http.StatusServiceUnavailable)
		return
	}

	squads, err := h.rw.ListExternalSquads(r.Context())
	if err != nil {
		slog.Error("admin external squads list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]adminSquadDTO, 0, len(squads))
	for _, sq := range squads {
		items = append(items, adminSquadDTO{UUID: sq.UUID.String(), Name: sq.Name})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}
