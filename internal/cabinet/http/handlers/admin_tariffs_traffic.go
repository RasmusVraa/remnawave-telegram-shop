package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/trafficquota"
)

// Границы, чтобы опечатка в админке не создала пакет на петабайты или с отрицательной ценой.
const (
	maxTrafficPacksPerTariff = 20
	maxTrafficPackBytes      = int64(10) << 40 // 10 ТиБ
	maxTrafficPackRub        = 1_000_000
)

// SetTrafficRepo подключает настройки раздельного трафика к редактору тарифа.
func (h *AdminTariffsHandler) SetTrafficRepo(r *database.TrafficSplitRepository, rw *remnawave.Client) {
	h.traffic = r
	h.rw = rw
}

type adminTrafficPackDTO struct {
	Kind        string `json:"kind"`
	Bytes       int64  `json:"bytes"`
	AmountRub   int    `json:"amount_rub"`
	AmountStars *int   `json:"amount_stars,omitempty"`
	IsActive    bool   `json:"is_active"`
}

type adminTariffTrafficDTO struct {
	// BypassLimitBytes — 0: у тарифа нет отдельного лимита обхода.
	BypassLimitBytes int64                 `json:"bypass_limit_bytes"`
	BypassSquadUUIDs string                `json:"bypass_squad_uuids"`
	BypassNodeUUIDs  string                `json:"bypass_node_uuids"`
	Packs            []adminTrafficPackDTO `json:"packs"`
	// SplitEnabled — общий выключатель TRAFFIC_SPLIT_ENABLED (для подсказки в UI).
	SplitEnabled bool   `json:"split_enabled"`
	BypassLabel  string `json:"bypass_label"`
	// MainLimitBytes — действующий лимит обычного трафика тарифа (свой или общий TRAFFIC_LIMIT); 0 — безлимит.
	// Отдельный лимит обхода допустим только при безлимитных обычных серверах.
	MainLimitBytes int64 `json:"main_limit_bytes"`
	// MainLimitGlobal — лимит обычного трафика взят не из тарифа, а из общей настройки TRAFFIC_LIMIT.
	MainLimitGlobal bool `json:"main_limit_global"`
}

// Traffic — GET/PUT /cabinet/api/admin/tariffs/{id}/traffic.
//
// Сохранение не трогает действующих клиентов: настройки обхода запоминаются у клиента при его
// следующей покупке или продлении этого тарифа.
func (h *AdminTariffsHandler) Traffic(w http.ResponseWriter, r *http.Request, tariffID int64) {
	if h.traffic == nil {
		http.Error(w, "traffic split is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.trafficGet(w, r, tariffID)
	case http.MethodPut:
		h.trafficPut(w, r, tariffID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AdminTariffsHandler) trafficGet(w http.ResponseWriter, r *http.Request, tariffID int64) {
	ctx := r.Context()
	b, err := h.traffic.GetTariffBypass(ctx, tariffID)
	if err != nil {
		slog.Error("admin tariff traffic: read", "tariff_id", tariffID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	packs, err := h.traffic.ListPacks(ctx, tariffID, "", false)
	if err != nil {
		slog.Error("admin tariff traffic: packs", "tariff_id", tariffID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := adminTariffTrafficDTO{
		BypassLimitBytes: b.LimitBytes, BypassSquadUUIDs: b.SquadUUIDs, BypassNodeUUIDs: b.NodeUUIDs,
		Packs: []adminTrafficPackDTO{}, SplitEnabled: config.TrafficSplitEnabled(), BypassLabel: config.TrafficBypassLabel(),
	}
	for _, p := range packs {
		out.Packs = append(out.Packs, adminTrafficPackDTO{Kind: p.Kind, Bytes: p.Bytes, AmountRub: p.AmountRub, AmountStars: p.AmountStars, IsActive: p.IsActive})
	}
	if t, err := h.tariffs.GetByID(ctx, tariffID); err == nil && t != nil {
		out.MainLimitBytes = trafficquota.MainLimitForTariff(t)
		out.MainLimitGlobal = t.TrafficLimitBytes <= 0 && out.MainLimitBytes > 0
	}
	writeJSON(w, http.StatusOK, out)
}

// trafficSummary — лимит обхода и число пакетов докупки для карточки тарифа в списке.
// Правила те же, что у клиента (me_traffic.go): обход — только при включённом раздельном
// трафике, пакет вида без лимита не считается: докупать безлимит нечего.
func (h *AdminTariffsHandler) trafficSummary(ctx context.Context, t *database.Tariff) (bypassBytes int64, packs int) {
	if h.traffic == nil {
		return 0, 0
	}
	if config.TrafficSplitEnabled() {
		b, err := h.traffic.GetTariffBypass(ctx, t.ID)
		if err != nil {
			slog.Warn("admin tariffs list: bypass", "tariff_id", t.ID, "error", err)
		} else {
			bypassBytes = b.LimitBytes
		}
	}
	if !config.TrafficPacksEnabled() {
		return bypassBytes, 0
	}
	list, err := h.traffic.ListPacks(ctx, t.ID, "", true)
	if err != nil {
		slog.Warn("admin tariffs list: packs", "tariff_id", t.ID, "error", err)
		return bypassBytes, 0
	}
	mainLimited := trafficquota.MainLimitForTariff(t) > 0
	for _, p := range list {
		if (p.Kind == database.TrafficKindMain && mainLimited) || (p.Kind == database.TrafficKindBypass && bypassBytes > 0) {
			packs++
		}
	}
	return bypassBytes, packs
}

func (h *AdminTariffsHandler) trafficPut(w http.ResponseWriter, r *http.Request, tariffID int64) {
	var req adminTariffTrafficDTO
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.BypassLimitBytes < 0 {
		http.Error(w, "bypass_limit_bytes must be >= 0", http.StatusBadRequest)
		return
	}
	if _, err := database.ParseSquadUUIDList(req.BypassSquadUUIDs); err != nil {
		http.Error(w, "bad bypass_squad_uuids: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := database.ParseSquadUUIDList(req.BypassNodeUUIDs); err != nil {
		http.Error(w, "bad bypass_node_uuids: "+err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if req.BypassLimitBytes > 0 {
		if strings.TrimSpace(req.BypassNodeUUIDs) == "" {
			http.Error(w, "укажите ноды обхода: по ним считается расход", http.StatusBadRequest)
			return
		}
		if sq, _ := database.ParseSquadUUIDList(req.BypassSquadUUIDs); len(sq) == 0 {
			http.Error(w, "укажите сквад обхода: без него лимит нечем отключать", http.StatusBadRequest)
			return
		}
		// Панель держит один общий лимит на клиента и не умеет отрезать только обычные серверы,
		// поэтому отдельный лимит обхода возможен только при безлимитных обычных серверах.
		t, err := h.tariffs.GetByID(ctx, tariffID)
		if err != nil || t == nil {
			http.Error(w, "tariff not found", http.StatusNotFound)
			return
		}
		if trafficquota.MainLimitForTariff(t) > 0 {
			http.Error(w, "у тарифа задан лимит обычного трафика: отдельный лимит обхода возможен только при безлимитных обычных серверах. Уберите лимит трафика в тарифе (0 — безлимит) и проверьте общий TRAFFIC_LIMIT", http.StatusBadRequest)
			return
		}
	}
	if len(req.Packs) > maxTrafficPacksPerTariff {
		http.Error(w, "слишком много пакетов", http.StatusBadRequest)
		return
	}
	packs := make([]database.TrafficPack, 0, len(req.Packs))
	seen := map[string]struct{}{}
	for _, p := range req.Packs {
		if p.Kind != database.TrafficKindMain && p.Kind != database.TrafficKindBypass {
			http.Error(w, "unknown pack kind", http.StatusBadRequest)
			return
		}
		if p.Kind == database.TrafficKindMain && req.BypassLimitBytes > 0 {
			http.Error(w, "у тарифа с отдельным обходом обычные серверы безлимитны: пакеты обычного трафика не нужны", http.StatusBadRequest)
			return
		}
		if p.Bytes <= 0 || p.Bytes > maxTrafficPackBytes {
			http.Error(w, "pack: размер должен быть больше 0 и не больше 10 ТБ", http.StatusBadRequest)
			return
		}
		if p.AmountRub <= 0 || p.AmountRub > maxTrafficPackRub {
			http.Error(w, "pack: цена в рублях должна быть больше 0", http.StatusBadRequest)
			return
		}
		if p.AmountStars != nil && (*p.AmountStars <= 0 || *p.AmountStars > maxTrafficPackRub) {
			http.Error(w, "pack: цена в звёздах должна быть больше 0 (или пусто — посчитается из рублей)", http.StatusBadRequest)
			return
		}
		key := p.Kind + "|" + strconv.FormatInt(p.Bytes, 10)
		if _, dup := seen[key]; dup {
			http.Error(w, "pack: два пакета одного размера в одном виде", http.StatusBadRequest)
			return
		}
		seen[key] = struct{}{}
		packs = append(packs, database.TrafficPack{TariffID: tariffID, Kind: p.Kind, Bytes: p.Bytes, AmountRub: p.AmountRub, AmountStars: p.AmountStars, IsActive: p.IsActive})
	}
	if err := h.traffic.SaveTariffTraffic(ctx, tariffID, database.TariffBypass{
		LimitBytes: req.BypassLimitBytes, SquadUUIDs: strings.TrimSpace(req.BypassSquadUUIDs), NodeUUIDs: strings.TrimSpace(req.BypassNodeUUIDs),
	}, packs); err != nil {
		slog.Error("admin tariff traffic: save", "tariff_id", tariffID, "error", err)
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	h.trafficGet(w, r, tariffID)
}

type adminTrafficNodeDTO struct {
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	CountryCode     string `json:"country_code"`
	Connected       bool   `json:"connected"`
	TrackingEnabled bool   `json:"tracking_enabled"`
}

// TrafficNodes — GET /cabinet/api/admin/tariffs/{id}/traffic/nodes?squads=uuid,uuid: ноды выбранных сквадов
// для выбора нод обхода. Без сквадов список пустой: сначала выбирается сквад.
func (h *AdminTariffsHandler) TrafficNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.rw == nil {
		writeJSON(w, http.StatusOK, []adminTrafficNodeDTO{})
		return
	}
	squads, _ := database.ParseSquadUUIDList(r.URL.Query().Get("squads"))
	if len(squads) == 0 {
		writeJSON(w, http.StatusOK, []adminTrafficNodeDTO{})
		return
	}
	ids := make([]string, 0, len(squads))
	for _, s := range squads {
		ids = append(ids, s.String())
	}
	nodes, _, err := h.rw.NodesForSquads(r.Context(), ids)
	if err != nil {
		slog.Warn("admin tariff traffic: list nodes", "error", err)
		http.Error(w, "panel unavailable", http.StatusBadGateway)
		return
	}
	out := make([]adminTrafficNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		if n.IsDisabled {
			continue
		}
		out = append(out, adminTrafficNodeDTO{UUID: n.UUID, Name: n.Name, CountryCode: n.CountryCode, Connected: n.IsConnected, TrackingEnabled: n.IsTrafficTrackingActive})
	}
	writeJSON(w, http.StatusOK, out)
}
