package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
)

var adminPanelStrategies = []string{"DAY", "WEEK", "MONTH", "MONTH_ROLLING", "NO_RESET"}
var adminTrafficPresetsGB = []int64{5, 10, 50, 100, 500}

type adminSquadDTO struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type adminRWPanelDTO struct {
	ID                   int64           `json:"id"`
	Username             string          `json:"username"`
	Status               string          `json:"status"`
	SubscriptionURL      string          `json:"subscription_url"`
	ExpireAt             *string         `json:"expire_at"`
	TrafficUsedBytes     float64         `json:"traffic_used_bytes"`
	TrafficLimitBytes    int64           `json:"traffic_limit_bytes"`
	TrafficLimitStrategy string          `json:"traffic_limit_strategy"`
	HwidDeviceLimit      *int            `json:"hwid_device_limit"`
	Description          *string         `json:"description"`
	Tag                  *string         `json:"tag"`
	LastTrafficResetAt   *string         `json:"last_traffic_reset_at"`
	OnlineAt             *string         `json:"online_at"`
	ActiveSquads         []adminSquadDTO `json:"active_squads"`
}

type adminTariffBriefDTO struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type adminUserPanelResp struct {
	Customer         adminCustomerDTO      `json:"customer"`
	HasRWUser        bool                  `json:"has_rw_user"`
	RW               *adminRWPanelDTO      `json:"rw"`
	AvailableSquads  []adminSquadDTO       `json:"available_squads"`
	TrafficPresetsGB []int64               `json:"traffic_presets_gb"`
	Strategies       []string              `json:"strategies"`
	Tariffs          []adminTariffBriefDTO `json:"tariffs,omitempty"`
	// HwidUnpinnedLimit — лимит устройств без закрепления (тариф + докупленные
	// слоты): окно устройств сравнивает с ним закреплённое число.
	HwidUnpinnedLimit int `json:"hwid_unpinned_limit"`
}

func mapRWToPanelDTO(rw *remnawave.User) adminRWPanelDTO {
	dto := adminRWPanelDTO{
		ID:                   rw.ID,
		Username:             rw.Username,
		Status:               rw.Status,
		SubscriptionURL:      rw.SubscriptionUrl,
		TrafficUsedBytes:     rw.UserTraffic.UsedTrafficBytes,
		TrafficLimitBytes:    rw.TrafficLimitBytes,
		TrafficLimitStrategy: rw.TrafficLimitStrategy,
		HwidDeviceLimit:      rw.HwidDeviceLimit,
		Description:          rw.Description,
		Tag:                  rw.Tag,
	}
	if !rw.ExpireAt.IsZero() {
		s := rw.ExpireAt.Format(time.RFC3339)
		dto.ExpireAt = &s
	}
	if rw.LastTrafficResetAt != nil && !rw.LastTrafficResetAt.IsZero() {
		s := rw.LastTrafficResetAt.Format(time.RFC3339)
		dto.LastTrafficResetAt = &s
	}
	if rw.UserTraffic.OnlineAt != nil && !rw.UserTraffic.OnlineAt.IsZero() {
		s := rw.UserTraffic.OnlineAt.Format(time.RFC3339)
		dto.OnlineAt = &s
	}
	for _, sq := range rw.ActiveInternalSquads {
		dto.ActiveSquads = append(dto.ActiveSquads, adminSquadDTO{
			UUID: sq.UUID.String(),
			Name: sq.Name,
		})
	}
	return dto
}

// Panel — GET /cabinet/api/admin/users/{id}/panel.
func (h *AdminUsersHandler) Panel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if !h.requireRW(w) {
		return
	}

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: panel — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := cabsvc.CleanupExpiredExtraHwid(ctx, h.rw, h.customers, cust); err != nil {
		slog.Warn("admin users: panel extra cleanup", "error", err.Error())
	}
	if cust, err = h.customers.FindById(ctx, id); err != nil || cust == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := adminUserPanelResp{
		Customer:          h.customerDTOWithStatus(ctx, cust),
		TrafficPresetsGB:  adminTrafficPresetsGB,
		Strategies:        adminPanelStrategies,
		HwidUnpinnedLimit: cabsvc.UnpinnedDeviceLimit(ctx, h.tariffs, cust),
	}

	if squads, err := h.rw.ListInternalSquads(ctx); err == nil {
		for _, sq := range squads {
			resp.AvailableSquads = append(resp.AvailableSquads, adminSquadDTO{
				UUID: sq.UUID.String(),
				Name: sq.Name,
			})
		}
	}

	if rw, err := h.findRWUser(ctx, cust); err == nil && rw != nil {
		resp.HasRWUser = true
		dto := mapRWToPanelDTO(rw)
		resp.RW = &dto
		applyRWStatusToDTO(&resp.Customer, rw)
	}

	if config.SalesMode() == "tariffs" && h.tariffs != nil {
		tariffList, err := h.tariffs.ListActive(ctx)
		if err == nil {
			for _, t := range tariffList {
				name := t.Slug
				if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
					name = strings.TrimSpace(*t.Name)
				}
				resp.Tariffs = append(resp.Tariffs, adminTariffBriefDTO{
					ID: t.ID, Slug: t.Slug, Name: name,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// SetSquads — PATCH /cabinet/api/admin/users/{id}/squads.
func (h *AdminUsersHandler) SetSquads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		SquadUUIDs []string `json:"squad_uuids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cust, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	var uuids []uuid.UUID
	for _, s := range body.SquadUUIDs {
		u, parseErr := uuid.Parse(strings.TrimSpace(s))
		if parseErr != nil {
			http.Error(w, "invalid squad uuid: "+s, http.StatusBadRequest)
			return
		}
		uuids = append(uuids, u)
	}

	st := strings.TrimSpace(rw.Status)
	if st == "" {
		st = "ACTIVE"
	}
	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:                   &rw.ID,
		Status:               st,
		ActiveInternalSquads: &uuids,
	})
	if err != nil {
		slog.Error("admin users: set squads failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}
	_ = h.syncCustomerAfterPatch(ctx, cust.ID, out)
	slog.Info("admin: set squads", "customer_id", id, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetTraffic — PATCH /cabinet/api/admin/users/{id}/traffic.
func (h *AdminUsersHandler) SetTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		LimitBytes int64 `json:"limit_bytes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.LimitBytes < 0 {
		http.Error(w, "limit_bytes must be >= 0", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cust, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	st := strings.TrimSpace(rw.Status)
	if st == "" {
		st = "ACTIVE"
	}
	limit := body.LimitBytes
	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:                &rw.ID,
		Status:            st,
		TrafficLimitBytes: &limit,
	})
	if err != nil {
		slog.Error("admin users: set traffic failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}
	_ = h.syncCustomerAfterPatch(ctx, cust.ID, out)
	slog.Info("admin: set traffic", "customer_id", id, "limit_bytes", body.LimitBytes, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetStrategy — PATCH /cabinet/api/admin/users/{id}/strategy.
func (h *AdminUsersHandler) SetStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		Strategy string `json:"strategy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	strategy := strings.TrimSpace(strings.ToUpper(body.Strategy))
	valid := false
	for _, s := range adminPanelStrategies {
		if s == strategy {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "invalid strategy", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cust, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	st := strings.TrimSpace(rw.Status)
	if st == "" {
		st = "ACTIVE"
	}
	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:                   &rw.ID,
		Status:               st,
		TrafficLimitStrategy: strategy,
	})
	if err != nil {
		slog.Error("admin users: set strategy failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}
	_ = h.syncCustomerAfterPatch(ctx, cust.ID, out)
	slog.Info("admin: set strategy", "customer_id", id, "strategy", strategy, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetDescription — PATCH /cabinet/api/admin/users/{id}/description.
func (h *AdminUsersHandler) SetDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cust, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	st := strings.TrimSpace(rw.Status)
	if st == "" {
		st = "ACTIVE"
	}
	desc := ""
	if body.Description != nil {
		desc = *body.Description
	}
	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:          &rw.ID,
		Status:      st,
		Description: &desc,
	})
	if err != nil {
		slog.Error("admin users: set description failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}
	_ = h.syncCustomerAfterPatch(ctx, cust.ID, out)
	slog.Info("admin: set description", "customer_id", id, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetTariff — PATCH /cabinet/api/admin/users/{id}/tariff.
func (h *AdminUsersHandler) SetTariff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if config.SalesMode() != "tariffs" || h.tariffs == nil {
		http.Error(w, "tariffs mode not enabled", http.StatusBadRequest)
		return
	}
	if !h.requireRW(w) {
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		TariffID int64 `json:"tariff_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.TariffID <= 0 {
		http.Error(w, "invalid tariff_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cust, err := h.customers.FindById(ctx, id)
	if err != nil || cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	tariff, err := h.tariffs.GetByID(ctx, body.TariffID)
	if err != nil || tariff == nil {
		http.Error(w, "tariff not found", http.StatusNotFound)
		return
	}

	profile := payment.BuildRemnawaveTariffProfile(tariff)
	out, err := h.rw.CreateOrUpdateUserWithTariffProfile(ctx, cust.ID, cust.TelegramID, 0, profile)
	if err != nil {
		slog.Error("admin users: set tariff failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}
	if syncErr := h.syncCustomerAfterPatch(ctx, cust.ID, out); syncErr != nil {
		slog.Warn("admin users: set tariff sync failed", "error", syncErr.Error())
	}
	if err := h.customers.UpdateFields(ctx, cust.ID, map[string]interface{}{"current_tariff_id": tariff.ID}); err != nil {
		slog.Error("admin users: set tariff db failed", "error", err.Error())
		http.Error(w, "failed to save tariff assignment", http.StatusInternalServerError)
		return
	}

	cust, _ = h.customers.FindById(ctx, id)
	if err := h.adminRelimitExtraAfterTariff(ctx, cust, tariff); err != nil {
		slog.Warn("admin users: set tariff extra relimit", "error", err.Error())
	}

	slog.Info("admin: set tariff", "customer_id", id, "tariff_id", body.TariffID, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type adminDeviceDTO struct {
	Hwid        string  `json:"hwid"`
	Platform    *string `json:"platform"`
	OsVersion   *string `json:"os_version"`
	DeviceModel *string `json:"device_model"`
	CreatedAt   string  `json:"created_at"`
}

// Devices — GET /cabinet/api/admin/users/{id}/devices.
func (h *AdminUsersHandler) Devices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	_, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	devices, err := h.rw.GetUserDevices(ctx, rw.ID)
	if err != nil {
		slog.Error("admin users: devices failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	dtos := make([]adminDeviceDTO, 0, len(devices))
	for _, d := range devices {
		dtos = append(dtos, adminDeviceDTO{
			Hwid:        d.Hwid,
			Platform:    d.Platform,
			OsVersion:   d.OsVersion,
			DeviceModel: d.DeviceModel,
			CreatedAt:   d.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": dtos})
}

// DeleteDevice — DELETE /cabinet/api/admin/users/{id}/devices/{hwid}.
func (h *AdminUsersHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.requireRW(w) {
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	hwid := adminUsersExtractDeviceHwid(r.URL.Path)
	if hwid == "" {
		http.Error(w, "invalid hwid", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	_, rw, err := h.requireRWUser(ctx, id)
	if err != nil {
		writeRWLookupError(w, err)
		return
	}

	if err := h.rw.DeleteUserDevice(ctx, rw.ID, hwid); err != nil {
		slog.Error("admin users: delete device failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: delete device", "customer_id", id, "hwid", hwid, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ExtraHwid — POST /cabinet/api/admin/users/{id}/extra-hwid.
func (h *AdminUsersHandler) ExtraHwid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var body struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	// Раньше принималось только ±1 — под кнопки-степперы. Но модалка устройств
	// копит правки и сохраняет их одним запросом, отправляя суммарную разницу:
	// выставить 7 доп. слотов означало delta=7, и сохранение падало с 400.
	//
	// Ограничение было артефактом валидации, а не логики: adminApplyExtraHwidDelta
	// складывает дельту с текущим значением и сама зажимает результат по нулю снизу
	// и по HWID_MAX_DEVICES сверху. Поэтому принимаем любую ненулевую дельту —
	// так правка остаётся одним атомарным запросом вместо серии из семи.
	const maxExtraHwidDelta = 100
	if body.Delta == 0 {
		http.Error(w, "delta must not be zero", http.StatusBadRequest)
		return
	}
	if body.Delta > maxExtraHwidDelta || body.Delta < -maxExtraHwidDelta {
		http.Error(w, "delta out of range", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if !h.requireRW(w) {
		return
	}

	cust, err := h.customers.FindById(ctx, id)
	if err != nil || cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	updated, applyErr := h.adminApplyExtraHwidDelta(ctx, cust, body.Delta)
	if applyErr != nil {
		if errors.Is(applyErr, errExtraHwidMax) {
			http.Error(w, "extra_hwid limit reached", http.StatusBadRequest)
			return
		}
		slog.Error("admin users: extra hwid failed", "error", applyErr.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: extra hwid", "customer_id", id, "delta", body.Delta, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, mapCustomerToDTO(updated))
}

var errExtraHwidMax = errors.New("extra_hwid limit reached")

func (h *AdminUsersHandler) adminApplyExtraHwidDelta(ctx context.Context, cust *database.Customer, delta int) (*database.Customer, error) {
	if cust == nil {
		return nil, errors.New("nil customer")
	}
	if err := cabsvc.CleanupExpiredExtraHwid(ctx, h.rw, h.customers, cust); err != nil {
		return nil, err
	}
	fresh, err := h.customers.FindById(ctx, cust.ID)
	if err != nil || fresh == nil {
		return nil, errors.New("customer not found")
	}
	cust = fresh

	newExtra := cust.ExtraHwid + delta
	if newExtra < 0 {
		newExtra = 0
	}

	rw, err := h.findRWUser(ctx, cust)
	if err != nil {
		return nil, err
	}
	if rw != nil {
		curLim := cabsvc.CurrentDeviceLimitFromRW(rw)
		base := curLim - cabsvc.ActiveExtraSlots(cust)
		if base < 1 {
			base = 1
		}
		maxTotal := config.HwidMaxDevices()
		if maxTotal > 0 && base+newExtra > maxTotal {
			newExtra = maxTotal - base
			if newExtra < 0 {
				newExtra = 0
			}
		}
		if delta > 0 && newExtra <= cust.ExtraHwid {
			return nil, errExtraHwidMax
		}
		newLim := base + newExtra
		if newLim < 1 {
			newLim = 1
		}
		if _, err := h.rw.UpdateUserDeviceLimitByCustomer(ctx, cust.ID, cust.TelegramID, newLim); err != nil {
			return nil, err
		}
	}

	updates := map[string]interface{}{"extra_hwid": newExtra, "extra_hwid_expires_at": nil}
	if newExtra > 0 && cust.ExpireAt != nil && cust.ExpireAt.After(time.Now()) {
		updates["extra_hwid_expires_at"] = cust.ExpireAt
	}
	if err := h.customers.UpdateFields(ctx, cust.ID, updates); err != nil {
		return nil, err
	}
	return h.customers.FindById(ctx, cust.ID)
}

func (h *AdminUsersHandler) adminRelimitExtraAfterTariff(ctx context.Context, cust *database.Customer, tariff *database.Tariff) error {
	if cust == nil || tariff == nil {
		return nil
	}
	if err := cabsvc.CleanupExpiredExtraHwid(ctx, h.rw, h.customers, cust); err != nil {
		return err
	}
	fresh, err := h.customers.FindById(ctx, cust.ID)
	if err != nil || fresh == nil {
		return err
	}
	cust = fresh

	extra := cabsvc.ActiveExtraSlots(cust)
	if extra <= 0 {
		return nil
	}
	base := tariff.DeviceLimit
	if base < 1 {
		base = config.GetHwidFallbackDeviceLimit()
	}
	newLim := base + extra
	if maxL := config.HwidMaxDevices(); maxL > 0 && newLim > maxL {
		newLim = maxL
	}
	_, err = h.rw.UpdateUserDeviceLimitByCustomer(ctx, cust.ID, cust.TelegramID, newLim)
	return err
}

// SetHwidPin — PUT /cabinet/api/admin/users/{id}/hwid-pin.
//
// {"limit": N} закрепляет лимит устройств: дальше любая запись лимита в панель
// (оплата, докупка, промокод, смена тарифа) подставляет N. {"limit": null}
// снимает закрепление и возвращает расчётный лимит — базу тарифа плюс
// действующие докупленные слоты; без пересчёта осталось бы закреплённое число,
// только уже без защиты.
func (h *AdminUsersHandler) SetHwidPin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Limit *int `json:"limit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Limit != nil && (*body.Limit < 1 || *body.Limit > 100) {
		http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
		return
	}
	if !h.requireRW(w) {
		return
	}

	ctx := r.Context()
	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: hwid pin — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := h.customers.SetHwidLimitOverride(ctx, cust.ID, body.Limit); err != nil {
		slog.Error("admin users: hwid pin — db failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	limit := 0
	if body.Limit != nil {
		limit = *body.Limit
	} else {
		if err := cabsvc.CleanupExpiredExtraHwid(ctx, h.rw, h.customers, cust); err != nil {
			slog.Warn("admin users: hwid unpin extra cleanup", "error", err.Error())
		}
		limit = cabsvc.UnpinnedDeviceLimit(ctx, h.tariffs, cust)
	}
	// Профиля в панели может ещё не быть (клиент не покупал) — закрепление
	// сохранено и применится, когда профиль создадут.
	if _, err := h.rw.UpdateUserDeviceLimitByCustomer(ctx, cust.ID, cust.TelegramID, limit); err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Error("admin users: hwid pin — rw failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: hwid pin", "customer_id", id, "pinned", body.Limit != nil, "limit", limit, "admin_account_id", adminAccountID(r))
	updated, err := h.customers.FindById(ctx, cust.ID)
	if err != nil || updated == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, mapCustomerToDTO(updated))
}

func (h *AdminUsersHandler) requireRW(w http.ResponseWriter) bool {
	if h.rw == nil {
		http.Error(w, "panel not configured", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (h *AdminUsersHandler) requireRWUser(ctx context.Context, customerID int64) (*database.Customer, *remnawave.User, error) {
	cust, err := h.customers.FindById(ctx, customerID)
	if err != nil {
		return nil, nil, err
	}
	if cust == nil {
		return nil, nil, errors.New("not found")
	}
	rw, err := h.findRWUser(ctx, cust)
	if err != nil {
		return cust, nil, err
	}
	if rw == nil {
		return cust, nil, remnawave.ErrUserNotFound
	}
	return cust, rw, nil
}

func writeRWLookupError(w http.ResponseWriter, err error) {
	if err != nil && err.Error() == "not found" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil && strings.Contains(err.Error(), "not configured") {
		http.Error(w, "panel not configured", http.StatusServiceUnavailable)
		return
	}
	if errors.Is(err, remnawave.ErrUserNotFound) {
		http.Error(w, "remnawave user not found", http.StatusNotFound)
		return
	}
	slog.Error("admin users: rw lookup", "error", err.Error())
	http.Error(w, "internal error", http.StatusInternalServerError)
}
