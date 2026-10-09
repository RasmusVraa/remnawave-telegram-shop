package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/statusprobe"
)

// AdminStatusTargets — список стран, которые пингует страница статуса.
// Адрес остаётся в ответе админки и не уходит на публичную страницу.
type AdminStatusTargets struct {
	store  *statusprobe.KVTargets
	probes *statusprobe.Runner
}

func NewAdminStatusTargets(store *statusprobe.KVTargets, probes *statusprobe.Runner) *AdminStatusTargets {
	return &AdminStatusTargets{store: store, probes: probes}
}

type statusTargetsBody struct {
	Targets []statusprobe.ConfiguredTarget `json:"targets"`
}

func (h *AdminStatusTargets) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.Load(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, statusTargetsBody{Targets: list})
	case http.MethodPut:
		var req statusTargetsBody
		if !decodeJSON(w, r, &req) {
			return
		}
		clean, err := statusprobe.NormalizeTargets(req.Targets)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.store.Save(r.Context(), clean); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, statusTargetsBody{Targets: clean})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Probe запускает замер сохранённого списка сразу, не дожидаясь интервала.
func (h *AdminStatusTargets) Probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.store.Load(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.probes.Force(probeTargets(list)); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, statusprobe.ErrBusy) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func probeTargets(list []statusprobe.ConfiguredTarget) []statusprobe.Target {
	out := make([]statusprobe.Target, 0, len(list))
	for _, item := range list {
		if !item.Enabled || strings.TrimSpace(item.Address) == "" {
			continue
		}
		out = append(out, statusprobe.Target{
			Key:       statusprobe.NodeKey(item.Country, item.Name),
			Address:   item.Address,
			Interval:  time.Duration(item.IntervalMin) * time.Minute,
			World:     item.WorldProbes,
			Russia:    item.RussiaProbes,
			Whitelist: item.Whitelist,
		})
	}
	return out
}
