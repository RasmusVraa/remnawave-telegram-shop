package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
	// Checks — значки «мир / РФ» по id строки; только в ответе на GET.
	Checks map[string]statusprobe.RowChecks `json:"checks,omitempty"`
}

// checksFor берёт последний замер каждой строки. Порядок совпадает с TargetsFrom:
// строки без адреса там пропускаются.
func (h *AdminStatusTargets) checksFor(list []statusprobe.ConfiguredTarget) map[string]statusprobe.RowChecks {
	if h.probes == nil {
		return nil
	}
	targets := statusprobe.TargetsFrom(list)
	out := make(map[string]statusprobe.RowChecks, len(list))
	next := 0
	for _, item := range list {
		if next >= len(targets) {
			break
		}
		if strings.TrimSpace(item.Address) == "" {
			continue
		}
		key := targets[next].Key
		next++
		var sample *statusprobe.Sample
		if s, _, ok := h.probes.Snapshot(key); ok {
			sample = &s
		}
		out[item.ID] = statusprobe.ChecksOf(sample, item.IsBypass())
	}
	return out
}

func (h *AdminStatusTargets) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := h.store.Load(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if h.probes != nil {
			h.probes.EnsureLoaded(r.Context())
		}
		writeJSON(w, http.StatusOK, statusTargetsBody{Targets: list, Checks: h.checksFor(list)})
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

// registryQuota — данные карточки «Реестр РКН»: свежесть списков сервиса и итог по серверам.
// Сервис не ответил — карточка просто без времени обновления.
func (h *AdminStatusTargets) registryQuota(ctx context.Context, list []statusprobe.ConfiguredTarget) *statusprobe.RegistryQuota {
	out := &statusprobe.RegistryQuota{}
	if info, err := statusprobe.FetchRegistryInfo(ctx, nil); err == nil {
		out.Info = info
	}
	if h.probes != nil {
		if until := h.probes.RegistryPause(); until.After(time.Now()) {
			out.PausedUntil = &until
		}
		checks := h.checksFor(list)
		for _, item := range list {
			check, ok := checks[item.ID]
			if !ok || !item.Enabled || item.NoExternal || item.IsBypass() || check.Registry.State == statusprobe.CheckNone {
				continue
			}
			out.Checked++
			if check.Registry.State == statusprobe.CheckBad {
				out.Listed++
			}
		}
	}
	return out
}

// cheburcheckQuota — данные карточки Cheburcheck и переключателя источника «России».
func (h *AdminStatusTargets) cheburcheckQuota(ctx context.Context, list []statusprobe.ConfiguredTarget) *statusprobe.CheburcheckQuota {
	out := &statusprobe.CheburcheckQuota{
		Configured: statusprobe.CheburcheckConfigured(),
		Mode:       statusprobe.RussiaSourceMode(),
		Active:     statusprobe.SourceGlobalping,
	}
	if h.probes != nil {
		out.Active = h.probes.ActiveRussiaSource()
		if until, note := h.probes.CheburcheckPause(); until.After(time.Now()) {
			out.PausedUntil = &until
			out.PauseNote = note
		}
		for _, target := range statusprobe.TargetsFrom(list) {
			sample, _, ok := h.probes.Snapshot(target.Key)
			if !ok || sample.ChebAt.IsZero() {
				continue
			}
			if out.LastAt == nil || sample.ChebAt.After(*out.LastAt) {
				at := sample.ChebAt
				out.LastAt, out.Pass, out.Total = &at, sample.ChebPass, sample.ChebTotal
			}
		}
	}
	if out.Configured {
		// Ноль не показываем: по замеру на стенде проверки идут и при нулевом балансе.
		if credits, err := statusprobe.FetchCheburcheckCredits(ctx); err == nil && *credits > 0 {
			out.Credits = credits
		}
	}
	return out
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
		pingNote, whiteNote := h.probes.PauseNotes()
		if ping.After(now) {
			est.GlobalpingPausedUntil = &ping
			est.GlobalpingPauseNote = pingNote
		}
		if white.After(now) {
			est.LatencyLabPausedUntil = &white
			est.LatencyLabPauseNote = whiteNote
		}
	}
	limCtx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	if lim, err := statusprobe.FetchGlobalpingLimits(limCtx, nil); err == nil {
		est.Real = lim
	}
	est.Registry = h.registryQuota(limCtx, list)
	chebCtx, chebCancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer chebCancel()
	est.Cheburcheck = h.cheburcheckQuota(chebCtx, list)
	writeJSON(w, http.StatusOK, est)
}
