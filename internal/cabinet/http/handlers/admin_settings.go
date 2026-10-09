package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// AdminSettingsHandler — GET/PATCH /cabinet/api/admin/settings (runtime bot config).
type AdminSettingsHandler struct {
	repo *database.RuntimeSettingsRepository
}

// NewAdminSettings — конструктор.
func NewAdminSettings(repo *database.RuntimeSettingsRepository) *AdminSettingsHandler {
	return &AdminSettingsHandler{repo: repo}
}

type adminSettingFieldDTO struct {
	Key        string   `json:"key"`
	Type       string   `json:"type"`
	Value      string   `json:"value"`
	Source     string   `json:"source"`
	Instant    bool     `json:"instant"`
	EnumValues []string `json:"enum_values,omitempty"`
	MinInt     *int     `json:"min_int,omitempty"`
	MaxInt     *int     `json:"max_int,omitempty"`
}

type adminSettingGroupDTO struct {
	ID     string                 `json:"id"`
	Fields []adminSettingFieldDTO `json:"fields"`
}

type adminSettingsGetResp struct {
	Groups []adminSettingGroupDTO `json:"groups"`
}

type adminSettingsPatchReq struct {
	Settings map[string]string `json:"settings"`
}

type adminSettingsPatchResp struct {
	OK      bool     `json:"ok"`
	Changed []string `json:"changed"`
}

// Get — GET /cabinet/api/admin/settings
func (h *AdminSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, buildAdminSettingsResponse())
}

// Patch — PATCH /cabinet/api/admin/settings
func (h *AdminSettingsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adminSettingsPatchReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Settings) == 0 {
		http.Error(w, "settings required", http.StatusBadRequest)
		return
	}

	normalized := make(map[string]string, len(req.Settings))
	for k, v := range req.Settings {
		key := strings.ToUpper(strings.TrimSpace(k))
		if key == "" {
			http.Error(w, "invalid key", http.StatusBadRequest)
			return
		}
		normalized[key] = v
	}

	beforeValues := make(map[string]string, len(normalized))
	for key := range normalized {
		for _, f := range config.RuntimeSettingsRegistry() {
			if f.Key != key {
				continue
			}
			var prev string
			config.ReadConfRL(func() { prev = f.Current() })
			beforeValues[key] = prev
			break
		}
	}

	changed, err := config.ApplyRuntimePatch(normalized)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var updatedBy *int64
	if claims := middleware.AuthClaims(r); claims != nil {
		id := claims.AccountID
		updatedBy = &id
	}

	toSave := make(map[string]string, len(changed))
	for _, key := range changed {
		toSave[key] = normalized[key]
	}
	if err := h.repo.UpsertBatch(r.Context(), toSave, updatedBy); err != nil {
		slog.Error("admin settings: upsert", "error", err)
		rollback := make(map[string]string, len(changed))
		for _, key := range changed {
			rollback[key] = beforeValues[key]
		}
		if _, rbErr := config.ApplyRuntimePatch(rollback); rbErr != nil {
			slog.Error("admin settings: rollback after upsert failure", "error", rbErr)
		}
		if needsFortuneReload(changed) {
			cabcfg.ReloadFortuneWheel()
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if needsFortuneReload(changed) {
		cabcfg.ReloadFortuneWheel()
	}

	accountID := int64(0)
	if claims := middleware.AuthClaims(r); claims != nil {
		accountID = claims.AccountID
	}
	slog.Info("admin bot settings updated", "admin_account_id", accountID, "changed_keys", changed)

	writeJSON(w, http.StatusOK, adminSettingsPatchResp{OK: true, Changed: changed})
}

func buildAdminSettingsResponse() adminSettingsGetResp {
	// Порядок групп: сверху важнее для ежедневной работы.
	//
	// Это ещё и белый список: группа, которой здесь нет, в ответ не попадёт и
	// в админке просто не появится — молча. Добавляя группу в
	// RuntimeSettingsRegistry, добавьте её и сюда (ниже есть проверка,
	// которая ругнётся в лог, если про это забыли).
	order := []string{
		"cabinet", "landing", "status", "cabinet_connect", "tariffs",
		"trial", "hwid", "referral", "partner", "stars", "loyalty",
		"payments", "payments_notify", "moynalog", "access", "links", "tags",
		"lifecycle", "fortune",
	}
	groupFields := make(map[string][]adminSettingFieldDTO)
	for _, f := range config.RuntimeSettingsRegistry() {
		var value string
		config.ReadConfRL(func() {
			value = f.Current()
		})
		dto := adminSettingFieldDTO{
			Key:        f.Key,
			Type:       string(f.Type),
			Value:      value,
			Source:     config.SettingSource(f.Key),
			Instant:    f.Instant,
			EnumValues: f.EnumValues,
			MinInt:     f.MinInt,
			MaxInt:     f.MaxInt,
		}
		groupFields[f.Group] = append(groupFields[f.Group], dto)
	}
	groups := make([]adminSettingGroupDTO, 0, len(order))
	known := make(map[string]bool, len(order))
	for _, id := range order {
		known[id] = true
		fields, ok := groupFields[id]
		if !ok || len(fields) == 0 {
			continue
		}
		groups = append(groups, adminSettingGroupDTO{ID: id, Fields: fields})
	}
	// Группа есть в реестре, но забыта в order — её настройки недоступны из
	// админки, и без этого сигнала понять это можно только по пустому экрану.
	for id := range groupFields {
		if !known[id] {
			slog.Error("admin settings: group is missing from the display order and will not be shown", "group", id)
		}
	}
	return adminSettingsGetResp{Groups: groups}
}

func needsFortuneReload(changed []string) bool {
	for _, k := range changed {
		if strings.HasPrefix(k, "FORTUNE_") {
			return true
		}
	}
	return false
}

// Handle dispatches GET/PATCH on /cabinet/api/admin/settings.
func (h *AdminSettingsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.Get(w, r)
	case http.MethodPatch:
		h.Patch(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
