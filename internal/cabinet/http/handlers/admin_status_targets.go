package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/statusprobe"
)

// AdminStatusTargets — список стран, которые пингует страница статуса.
// Адрес остаётся в ответе админки и не уходит на публичную страницу.
type AdminStatusTargets struct {
	store     *statusprobe.KVTargets
	probes    *statusprobe.Runner
	rw        *remnawave.Client
	onChanged func()
}

// onChanged вызывается после сохранения списка — сбросить кэш публичной страницы.
func NewAdminStatusTargets(store *statusprobe.KVTargets, probes *statusprobe.Runner, rw *remnawave.Client, onChanged func()) *AdminStatusTargets {
	return &AdminStatusTargets{store: store, probes: probes, rw: rw, onChanged: onChanged}
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
		if h.probes != nil {
			h.probes.EnsureLoaded(r.Context())
			h.probes.Retain(statusprobe.TargetsFrom(clean))
		}
		if h.onChanged != nil {
			h.onChanged()
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
	if err := h.probes.Force(statusprobe.EnabledTargets(list)); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, statusprobe.ErrBusy) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

type statusPanelNode struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Country string `json:"country,omitempty"`
	Address string `json:"address"`
	State   string `json:"state"`
}

// Nodes — узлы панели для кнопки «Добавить из панели». Адрес виден только админу.
func (h *AdminStatusTargets) Nodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out := []statusPanelNode{}
	if h.rw != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		nodes, err := h.rw.ListProbeTargets(ctx)
		if err != nil {
			http.Error(w, "panel unavailable", http.StatusBadGateway)
			return
		}
		for _, n := range nodes {
			out = append(out, statusPanelNode{UUID: n.UUID, Name: n.Name, Country: n.CountryCode, Address: n.Address, State: n.State})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}

// Quota — оценка расхода Globalping для сохранённого списка.
func (h *AdminStatusTargets) Quota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := h.store.Load(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	est := statusprobe.EstimateQuota(list)
	if h.probes != nil {
		now := time.Now()
		ping, white := h.probes.Pauses()
		if ping.After(now) {
			est.GlobalpingPausedUntil = &ping
		}
		if white.After(now) {
			est.LatencyLabPausedUntil = &white
		}
	}
	writeJSON(w, http.StatusOK, est)
}
