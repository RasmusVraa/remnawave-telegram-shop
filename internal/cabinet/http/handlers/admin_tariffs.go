package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/tariffsquads"
)

// extractTariffPath разбирает /cabinet/api/admin/tariffs/{id}[/{sub}].
// sub — пустая строка для операций над самим тарифом.
func extractTariffPath(path string) (int64, string, bool) {
	s := strings.TrimPrefix(path, "/cabinet/api/admin/tariffs/")
	s = strings.Trim(s, "/")
	if s == "" {
		return 0, "", false
	}
	head, sub, _ := strings.Cut(s, "/")
	id, err := strconv.ParseInt(head, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, sub, true
}

func extractTariffID(path string) (int64, bool) {
	id, sub, ok := extractTariffPath(path)
	if !ok || sub != "" {
		return 0, false
	}
	return id, true
}

type AdminTariffsHandler struct {
	tariffs *database.TariffRepository
	// squads — применение состава сквадов к действующим подписчикам.
	// nil, если панель Remnawave не сконфигурирована: тарифы при этом
	// продолжают редактироваться, недоступно только массовое применение.
	squads *tariffsquads.Service
}

func NewAdminTariffs(tariffs *database.TariffRepository, squads *tariffsquads.Service) *AdminTariffsHandler {
	return &AdminTariffsHandler{tariffs: tariffs, squads: squads}
}

// validateSquadList проверяет строку active_internal_squad_uuids перед записью.
//
// Формат проверяется всегда: битый UUID раньше молча превращался в «нет
// ограничений» (payment.BuildRemnawaveTariffProfile глотал ошибку разбора,
// а пустой список означает «все сквады панели»). Существование сквада
// сверяется с панелью, но только если панель ответила — иначе её недоступность
// заблокировала бы правку любых полей тарифа.
//
// Сверяются ТОЛЬКО добавляемые сквады (те, которых в тарифе ещё не было).
// Уже записанный UUID пропускается, даже если панель его не отдаёт: тариф мог
// быть заведён из SQUAD_UUIDS мимо панели (см. tariff_admin.go), а сквад —
// пересоздан в панели с новым UUID. Раньше такой «унаследованный» UUID делал
// тариф нередактируемым: любая правка падала с «squad not found in panel», а
// убрать его через список галочек нельзя — он в нём не отображается.
func (h *AdminTariffsHandler) validateSquadList(r *http.Request, raw string, existing []uuid.UUID) error {
	list, err := database.ParseSquadUUIDList(raw)
	if err != nil {
		return err
	}
	if len(list) == 0 || h.squads == nil {
		return nil
	}
	had := make(map[uuid.UUID]struct{}, len(existing))
	for _, u := range existing {
		had[u] = struct{}{}
	}
	added := make([]uuid.UUID, 0, len(list))
	for _, u := range list {
		if _, ok := had[u]; !ok {
			added = append(added, u)
		}
	}
	if len(added) == 0 {
		return nil
	}
	return h.squads.ValidateSquadsExist(r.Context(), added)
}

type tariffPriceDTO struct {
	TariffID    int64 `json:"tariff_id"`
	Months      int   `json:"months"`
	AmountRub   int   `json:"amount_rub"`
	AmountStars *int  `json:"amount_stars"`
}

type tariffDTO struct {
	ID                        int64                     `json:"id"`
	Slug                      string                    `json:"slug"`
	Name                      *string                   `json:"name"`
	SortOrder                 int                       `json:"sort_order"`
	IsActive                  bool                      `json:"is_active"`
	DeviceLimit               int                       `json:"device_limit"`
	TrafficLimitBytes         int64                     `json:"traffic_limit_bytes"`
	TrafficLimitResetStrategy string                    `json:"traffic_limit_reset_strategy"`
	ActiveInternalSquadUUIDs  string                    `json:"active_internal_squad_uuids"`
	ExternalSquadUUID         *string                   `json:"external_squad_uuid"`
	RemnawaveTag              *string                   `json:"remnawave_tag"`
	TierLevel                 *int                      `json:"tier_level"`
	Description               *string                   `json:"description"`
	DescriptionDetail         *string                   `json:"description_detail"`
	Prices                    []tariffPriceDTO          `json:"prices"`
	Locations                 []database.TariffLocation `json:"locations"`
}

func tariffToDTO(t *database.Tariff, prices []database.TariffPrice, locs []database.TariffLocation) tariffDTO {
	var extUUID *string
	if t.ExternalSquadUUID != nil {
		s := t.ExternalSquadUUID.String()
		extUUID = &s
	}
	if locs == nil {
		locs = []database.TariffLocation{}
	}
	priceDTOs := make([]tariffPriceDTO, 0, len(prices))
	for _, p := range prices {
		priceDTOs = append(priceDTOs, tariffPriceDTO{
			TariffID: p.TariffID, Months: p.Months,
			AmountRub: p.AmountRub, AmountStars: p.AmountStars,
		})
	}
	return tariffDTO{
		ID: t.ID, Slug: t.Slug, Name: t.Name, SortOrder: t.SortOrder,
		IsActive: t.IsActive, DeviceLimit: t.DeviceLimit,
		TrafficLimitBytes:         t.TrafficLimitBytes,
		TrafficLimitResetStrategy: t.TrafficLimitResetStrategy,
		ActiveInternalSquadUUIDs:  t.ActiveInternalSquadUUIDs,
		ExternalSquadUUID:         extUUID, RemnawaveTag: t.RemnawaveTag,
		TierLevel: t.TierLevel, Description: t.Description, DescriptionDetail: t.DescriptionDetail,
		Prices:    priceDTOs,
		Locations: locs,
	}
}

// List — GET /cabinet/api/admin/tariffs
func (h *AdminTariffsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tariffs, err := h.tariffs.ListAll(r.Context())
	if err != nil {
		slog.Error("admin tariffs list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := make([]tariffDTO, 0, len(tariffs))
	for i := range tariffs {
		prices, perr := h.tariffs.ListPricesForTariff(r.Context(), tariffs[i].ID)
		if perr != nil {
			slog.Error("admin tariffs list prices", "tariff_id", tariffs[i].ID, "error", perr.Error())
			prices = []database.TariffPrice{}
		}
		if prices == nil {
			prices = []database.TariffPrice{}
		}
		locs, lerr := h.tariffs.ListLocations(r.Context(), tariffs[i].ID)
		if lerr != nil {
			slog.Error("admin tariffs list locations", "tariff_id", tariffs[i].ID, "error", lerr.Error())
			locs = []database.TariffLocation{}
		}
		result = append(result, tariffToDTO(&tariffs[i], prices, locs))
	}
	writeJSON(w, http.StatusOK, result)
}

type createTariffReq struct {
	Slug                      string                    `json:"slug"`
	Name                      *string                   `json:"name"`
	SortOrder                 int                       `json:"sort_order"`
	IsActive                  bool                      `json:"is_active"`
	DeviceLimit               int                       `json:"device_limit"`
	TrafficLimitBytes         int64                     `json:"traffic_limit_bytes"`
	TrafficLimitResetStrategy string                    `json:"traffic_limit_reset_strategy"`
	ActiveInternalSquadUUIDs  string                    `json:"active_internal_squad_uuids"`
	RemnawaveTag              *string                   `json:"remnawave_tag"`
	TierLevel                 *int                      `json:"tier_level"`
	Description               *string                   `json:"description"`
	DescriptionDetail         *string                   `json:"description_detail"`
	Rub                       [4]int                    `json:"rub"`
	Stars                     [4]*int                   `json:"stars"`
	Locations                 []database.TariffLocation `json:"locations"`
}

// Create — POST /cabinet/api/admin/tariffs
func (h *AdminTariffsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createTariffReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Slug) == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}
	if err := h.validateSquadList(r, req.ActiveInternalSquadUUIDs, nil); err != nil {
		http.Error(w, "invalid squads: "+err.Error(), http.StatusBadRequest)
		return
	}

	t := database.Tariff{
		Slug:                      req.Slug,
		Name:                      req.Name,
		SortOrder:                 req.SortOrder,
		IsActive:                  req.IsActive,
		DeviceLimit:               req.DeviceLimit,
		TrafficLimitBytes:         req.TrafficLimitBytes,
		TrafficLimitResetStrategy: req.TrafficLimitResetStrategy,
		ActiveInternalSquadUUIDs:  req.ActiveInternalSquadUUIDs,
		TierLevel:                 req.TierLevel,
		Description:               req.Description,
		DescriptionDetail:         req.DescriptionDetail,
	}
	if tag := config.RemnawaveTag(); tag != "" {
		t.RemnawaveTag = &tag
	}

	id, err := h.tariffs.CreateWithPrices(r.Context(), &t, req.Rub, req.Stars)
	if err != nil {
		slog.Error("admin tariffs create", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	t.ID = id
	prices, _ := h.tariffs.ListPricesForTariff(r.Context(), id)
	if prices == nil {
		prices = []database.TariffPrice{}
	}
	locs := []database.TariffLocation{}
	if len(req.Locations) > 0 {
		if err := h.tariffs.ReplaceLocations(r.Context(), id, req.Locations); err != nil {
			writeLocationErr(w, err)
			return
		}
		locs, _ = h.tariffs.ListLocations(r.Context(), id)
	}
	writeJSON(w, http.StatusCreated, tariffToDTO(&t, prices, locs))
}

// Get — GET /cabinet/api/admin/tariffs/{id}
func (h *AdminTariffsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractTariffID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	t, err := h.tariffs.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin tariffs get", "error", err.Error())
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	prices, err := h.tariffs.ListPricesForTariff(r.Context(), id)
	if err != nil {
		slog.Error("admin tariffs list prices", "error", err.Error())
		prices = []database.TariffPrice{}
	}
	if prices == nil {
		prices = []database.TariffPrice{}
	}
	locs, lerr := h.tariffs.ListLocations(r.Context(), id)
	if lerr != nil {
		slog.Error("admin tariffs list locations", "error", lerr.Error())
		locs = []database.TariffLocation{}
	}

	writeJSON(w, http.StatusOK, tariffToDTO(t, prices, locs))
}

// Update — PATCH /cabinet/api/admin/tariffs/{id}
func (h *AdminTariffsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractTariffID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var raw map[string]json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}

	tariffFields := map[string]bool{
		"slug": true, "name": true, "sort_order": true, "is_active": true,
		"device_limit": true, "traffic_limit_bytes": true,
		"traffic_limit_reset_strategy": true, "active_internal_squad_uuids": true,
		"tier_level": true, "description": true, "description_detail": true,
	}

	fields := make(map[string]interface{})
	for k, v := range raw {
		if !tariffFields[k] {
			continue
		}
		var val interface{}
		if err := json.Unmarshal(v, &val); err != nil {
			http.Error(w, "invalid field: "+k, http.StatusBadRequest)
			return
		}
		fields[k] = val
	}

	if v, ok := fields["active_internal_squad_uuids"]; ok {
		raw, isStr := v.(string)
		if !isStr {
			http.Error(w, "invalid field: active_internal_squad_uuids", http.StatusBadRequest)
			return
		}
		var stored []uuid.UUID
		if cur, curErr := h.tariffs.GetByID(r.Context(), id); curErr == nil && cur != nil {
			stored, _ = database.ParseSquadUUIDList(cur.ActiveInternalSquadUUIDs)
		}
		if err := h.validateSquadList(r, raw, stored); err != nil {
			http.Error(w, "invalid squads: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if len(fields) > 0 {
		if err := h.tariffs.UpdateTariff(r.Context(), id, fields); err != nil {
			slog.Error("admin tariffs update", "error", err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	if rawRub, hasRub := raw["rub"]; hasRub {
		var rub [4]int
		if err := json.Unmarshal(rawRub, &rub); err != nil {
			http.Error(w, "invalid rub", http.StatusBadRequest)
			return
		}
		var stars [4]*int
		if rawStars, hasStars := raw["stars"]; hasStars {
			if err := json.Unmarshal(rawStars, &stars); err != nil {
				http.Error(w, "invalid stars", http.StatusBadRequest)
				return
			}
		}
		if err := h.tariffs.ReplaceAllPrices(r.Context(), id, rub, stars); err != nil {
			slog.Error("admin tariffs replace prices", "error", err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	if rawLocs, hasLocs := raw["locations"]; hasLocs {
		var locs []database.TariffLocation
		if err := json.Unmarshal(rawLocs, &locs); err != nil {
			http.Error(w, "invalid locations", http.StatusBadRequest)
			return
		}
		if err := h.tariffs.ReplaceLocations(r.Context(), id, locs); err != nil {
			writeLocationErr(w, err)
			return
		}
	}

	t, err := h.tariffs.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin tariffs get after update", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	prices, _ := h.tariffs.ListPricesForTariff(r.Context(), id)
	if prices == nil {
		prices = []database.TariffPrice{}
	}
	locs, _ := h.tariffs.ListLocations(r.Context(), id)
	writeJSON(w, http.StatusOK, tariffToDTO(t, prices, locs))
}

func writeLocationErr(w http.ResponseWriter, err error) {
	if errors.Is(err, database.ErrDuplicateLocationName) {
		http.Error(w, "duplicate location name", http.StatusBadRequest)
		return
	}
	if strings.Contains(err.Error(), "too long") {
		http.Error(w, "location name too long", http.StatusBadRequest)
		return
	}
	slog.Error("admin tariffs locations", "error", err.Error())
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// Delete — DELETE /cabinet/api/admin/tariffs/{id}
func (h *AdminTariffsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractTariffID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.tariffs.DeleteTariff(r.Context(), id); err != nil {
		slog.Error("admin tariffs delete", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Handle dispatches /cabinet/api/admin/tariffs (no trailing path).
func (h *AdminTariffsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.List(w, r)
	case http.MethodPost:
		h.Create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleByID dispatches /cabinet/api/admin/tariffs/{id}[/squads[/apply]].
func (h *AdminTariffsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id, sub, ok := extractTariffPath(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch sub {
	case "":
		switch r.Method {
		case http.MethodGet:
			h.Get(w, r)
		case http.MethodPatch:
			h.Update(w, r)
		case http.MethodDelete:
			h.Delete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "squads":
		h.SquadsPreview(w, r, id)
	case "squads/apply":
		h.SquadsApply(w, r, id)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}
