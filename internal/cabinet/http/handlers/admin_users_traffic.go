package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/trafficquota"
)

// SetTrafficSplit подключает раздельный трафик к карточке пользователя в админке.
// quota нужен, чтобы выданный пакет сразу применился в панели, а не через минуту воркером.
func (h *AdminUsersHandler) SetTrafficSplit(repo *database.TrafficSplitRepository, quota *trafficquota.Service) {
	h.traffic = repo
	h.quota = quota
}

type adminUserTrafficNodeDTO struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Bypass bool   `json:"bypass"`
}

type adminUserTrafficExtraDTO struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Bytes      int64     `json:"bytes"`
	ValidUntil time.Time `json:"valid_until"`
	// Paid — пакет куплен клиентом (есть платёж); false — выдан админом.
	Paid bool `json:"paid"`
}

type adminUserTrafficSplitResp struct {
	// Enabled — у клиента есть отдельный лимит обхода (иначе показывать нечего, кроме выдачи обычного пакета).
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
	// Bypass — nil, если у клиента нет отдельного лимита обхода.
	Bypass *struct {
		LimitBytes   int64      `json:"limit_bytes"`
		BaseBytes    int64      `json:"base_bytes"`
		ExtraBytes   int64      `json:"extra_bytes"`
		UsedBytes    int64      `json:"used_bytes"`
		SquadRemoved bool       `json:"squad_removed"`
		PeriodStart  time.Time  `json:"period_start"`
		CheckedAt    *time.Time `json:"checked_at,omitempty"`
	} `json:"bypass,omitempty"`
	MainExtraBytes int64                      `json:"main_extra_bytes"`
	Extras         []adminUserTrafficExtraDTO `json:"extras"`
	// Nodes — расход за месяц по нодам панели (то, из чего считается обход).
	Nodes []adminUserTrafficNodeDTO `json:"nodes"`
}

// TrafficSplit — GET /cabinet/api/admin/users/{id}/traffic-split.
func (h *AdminUsersHandler) TrafficSplit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cust, ok := h.trafficCustomer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	resp := adminUserTrafficSplitResp{Label: config.TrafficBypassLabel(), Extras: []adminUserTrafficExtraDTO{}, Nodes: []adminUserTrafficNodeDTO{}}

	extras, err := h.traffic.ListActiveExtras(ctx, cust.ID, now)
	if err != nil {
		slog.Error("admin users: traffic split extras", "customer_id", cust.ID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var bypassExtra int64
	for _, e := range extras {
		resp.Extras = append(resp.Extras, adminUserTrafficExtraDTO{ID: e.ID, Kind: e.Kind, Bytes: e.Bytes, ValidUntil: e.ValidUntil, Paid: e.PurchaseID != nil})
		switch e.Kind {
		case database.TrafficKindBypass:
			bypassExtra += e.Bytes
		case database.TrafficKindMain:
			resp.MainExtraBytes += e.Bytes
		}
	}

	nodeSet := map[string]struct{}{}
	if st, err := h.traffic.GetState(ctx, cust.ID); err == nil && st != nil && st.LimitBytes > 0 {
		used := st.UsedBytes
		if !st.PeriodStart.UTC().Equal(database.TrafficPeriodStart(now)) {
			used = 0
		}
		resp.Enabled = true
		resp.Bypass = &struct {
			LimitBytes   int64      `json:"limit_bytes"`
			BaseBytes    int64      `json:"base_bytes"`
			ExtraBytes   int64      `json:"extra_bytes"`
			UsedBytes    int64      `json:"used_bytes"`
			SquadRemoved bool       `json:"squad_removed"`
			PeriodStart  time.Time  `json:"period_start"`
			CheckedAt    *time.Time `json:"checked_at,omitempty"`
		}{
			LimitBytes: st.LimitBytes + bypassExtra, BaseBytes: st.LimitBytes, ExtraBytes: bypassExtra, UsedBytes: used,
			SquadRemoved: st.SquadRemoved, PeriodStart: st.PeriodStart, CheckedAt: st.CheckedAt,
		}
		for _, n := range strings.Split(st.NodeUUIDs, ",") {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				nodeSet[n] = struct{}{}
			}
		}
	}

	if h.rw != nil {
		if rwUser, err := h.findRWUser(ctx, cust); err == nil && rwUser != nil {
			if series, err := h.rw.GetUserUsageByRange(ctx, rwUser.ID, database.TrafficPeriodStart(now), now); err == nil {
				for _, n := range series.Series {
					if n.Total <= 0 {
						continue
					}
					_, byp := nodeSet[strings.ToLower(n.UUID)]
					resp.Nodes = append(resp.Nodes, adminUserTrafficNodeDTO{Name: n.Name, Bytes: int64(n.Total), Bypass: byp})
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type adminGrantPackReq struct {
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
}

// TrafficSplitGrant — POST /cabinet/api/admin/users/{id}/traffic-split/grant: выдать пакет трафика без оплаты.
// Действует до 1-го числа следующего месяца, как купленный.
func (h *AdminUsersHandler) TrafficSplitGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cust, ok := h.trafficCustomer(w, r)
	if !ok {
		return
	}
	var req adminGrantPackReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if (req.Kind != database.TrafficKindMain && req.Kind != database.TrafficKindBypass) || req.Bytes <= 0 || req.Bytes > 10<<40 {
		http.Error(w, "bad kind or size", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if _, err := h.traffic.AddExtra(ctx, database.TrafficExtra{
		CustomerID: cust.ID, Kind: req.Kind, Bytes: req.Bytes, ValidUntil: database.TrafficPeriodEnd(time.Now()),
	}); err != nil {
		slog.Error("admin users: grant traffic pack", "customer_id", cust.ID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.applyTrafficChange(r, cust.ID, req.Kind)
	slog.Info("admin: granted traffic pack", "customer_id", cust.ID, "kind", req.Kind, "bytes", req.Bytes, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// TrafficSplitRevoke — DELETE /cabinet/api/admin/users/{id}/traffic-split/extras/{extraID}: убрать пакет.
func (h *AdminUsersHandler) TrafficSplitRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cust, ok := h.trafficCustomer(w, r)
	if !ok {
		return
	}
	idx := strings.LastIndex(r.URL.Path, "/extras/")
	if idx < 0 {
		http.Error(w, "invalid extra id", http.StatusBadRequest)
		return
	}
	extraID, err := strconv.ParseInt(strings.Trim(r.URL.Path[idx+len("/extras/"):], "/"), 10, 64)
	if err != nil {
		http.Error(w, "invalid extra id", http.StatusBadRequest)
		return
	}
	kind, err := h.traffic.DeleteExtra(r.Context(), cust.ID, extraID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.applyTrafficChange(r, cust.ID, kind)
	slog.Info("admin: revoked traffic pack", "customer_id", cust.ID, "extra_id", extraID, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// applyTrafficChange применяет выданный или снятый пакет в панели сразу; ошибка не фатальна —
// воркер подхватит обход на ближайшем проходе, а лимит обычного трафика останется как был.
func (h *AdminUsersHandler) applyTrafficChange(r *http.Request, customerID int64, kind string) {
	if h.quota == nil {
		return
	}
	err := h.quota.OnPackGranted(r.Context(), customerID, kind)
	if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Warn("admin users: apply traffic change", "customer_id", customerID, "kind", kind, "error", err.Error())
	}
}

// snapshotBypassAfterTariff — назначение тарифа админом та же выдача, что и оплата: настройки обхода
// нового тарифа действуют сразу, а не со следующей покупки. Без этого клиент, купивший до появления
// функции, оставался без снимка, и воркер его не видел. Ошибки не фатальны: тариф уже назначен.
func (h *AdminUsersHandler) snapshotBypassAfterTariff(r *http.Request, cust *database.Customer, tariffID int64, rwUser *remnawave.User) {
	if h.traffic == nil || cust == nil {
		return
	}
	ctx := r.Context()
	// Воркер ищет клиента по id профиля в панели — запоминаем его сразу, как при оплате.
	if rwUser != nil && rwUser.ID > 0 {
		if err := h.customers.SetRemnawaveIdentity(ctx, cust.ID, rwUser.ID, rwUser.ShortUUID); err != nil {
			slog.Warn("admin users: remember panel identity", "customer_id", cust.ID, "error", err.Error())
		}
	}
	b, err := h.traffic.GetTariffBypass(ctx, tariffID)
	if err != nil {
		slog.Warn("admin users: read tariff bypass", "tariff_id", tariffID, "error", err.Error())
		return
	}
	if err := h.traffic.SaveSnapshot(ctx, cust.ID, &tariffID, b, time.Now()); err != nil {
		slog.Warn("admin users: save bypass snapshot", "customer_id", cust.ID, "error", err.Error())
		return
	}
	if h.quota == nil || !config.TrafficSplitEnabled() || !b.Enabled() {
		return
	}
	// Расход за месяц мог уже превысить лимит нового тарифа — проверяем сразу, не дожидаясь воркера.
	if err := h.quota.ReevaluateBypass(ctx, cust.ID); err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Warn("admin users: reevaluate bypass after tariff", "customer_id", cust.ID, "error", err.Error())
	}
}

func (h *AdminUsersHandler) trafficCustomer(w http.ResponseWriter, r *http.Request) (*database.Customer, bool) {
	if h.traffic == nil {
		http.Error(w, "traffic split is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return nil, false
	}
	cust, err := h.customers.FindById(r.Context(), id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, false
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	return cust, true
}
