package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/avatartoken"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// AdminUsersHandler — эндпоинты /cabinet/api/admin/users/*.
type AdminUsersHandler struct {
	customers *database.CustomerRepository
	purchases *database.PurchaseRepository
	referrals *database.ReferralRepository
	tariffs   *database.TariffRepository
	loyalty   *database.LoyaltyTierRepository
	rw        *remnawave.Client
	// avatarSecret — секрет кабинета, которым подписываются ссылки на
	// аватарку. Пустой — карточка обойдётся инициалами.
	avatarSecret []byte
}

// adminAvatarTTL — срок жизни ссылки на аватарку в админской карточке.
// Сутки: карточка перезапрашивается на каждом заходе и обновляет ссылку, а
// вкладка, забытая открытой на ночь, доживёт до утра без битой картинки.
const adminAvatarTTL = 24 * time.Hour

// NewAdminUsers — конструктор.
func NewAdminUsers(
	customers *database.CustomerRepository,
	purchases *database.PurchaseRepository,
	referrals *database.ReferralRepository,
	tariffs *database.TariffRepository,
	loyalty *database.LoyaltyTierRepository,
	rw *remnawave.Client,
	avatarSecret []byte,
) *AdminUsersHandler {
	return &AdminUsersHandler{
		customers:    customers,
		purchases:    purchases,
		referrals:    referrals,
		tariffs:      tariffs,
		loyalty:      loyalty,
		rw:           rw,
		avatarSecret: avatarSecret,
	}
}

// --- DTOs -------------------------------------------------------------------

type adminCustomerDTO struct {
	ID                 int64   `json:"id"`
	TelegramID         int64   `json:"telegram_id,string"`
	TelegramUsername   *string `json:"telegram_username"`
	Language           string  `json:"language"`
	ExpireAt           *string `json:"expire_at"`
	CreatedAt          string  `json:"created_at"`
	SubscriptionLink   *string `json:"subscription_link"`
	ExtraHwid          int     `json:"extra_hwid"`
	ExtraHwidExpiresAt *string `json:"extra_hwid_expires_at"`
	// HwidLimitOverride — закреплённый админом лимит устройств; null — не закреплён.
	HwidLimitOverride        *int    `json:"hwid_limit_override"`
	CurrentTariffID          *int64  `json:"current_tariff_id"`
	SubscriptionPeriodStart  *string `json:"subscription_period_start"`
	SubscriptionPeriodMonths *int    `json:"subscription_period_months"`
	LoyaltyXP                int64   `json:"loyalty_xp"`
	LoyaltyLevel             *int    `json:"loyalty_level,omitempty"`
	LoyaltyDiscountPercent   *int    `json:"loyalty_discount_percent,omitempty"`
	IsWebOnly                bool    `json:"is_web_only"`
	Status                   string  `json:"status"`
	RwStatus                 *string `json:"rw_status,omitempty"`
	// PanelLogin — логин web-клиента в панели ("<id>_<local-part email>").
	// У клиентов с реальным Telegram пусто: там опознавание идёт по @username.
	// Заполняется отдельно от mapCustomerToDTO, потому что требует запроса
	// в cabinet_account; собирается пакетно на всю страницу, без N+1.
	PanelLogin *string `json:"panel_login,omitempty"`
	// AvatarURL — подписанная ссылка на аватарку из Telegram
	// (/cabinet/api/avatar). Заполняется только в карточке одного
	// пользователя: в списке это были бы десятки запросов в Telegram на
	// страницу. Пусто у web-клиентов; 404 на саму картинку — законный ответ
	// (аватарки нет или закрыта приватностью), UI рисует инициалы.
	AvatarURL string `json:"avatar_url,omitempty"`
}

// avatarURL — ссылка на аватарку пользователя бота.
//
// Существование картинки здесь не проверяется намеренно: это стоило бы
// запроса getChat на каждую карточку, а <img> и так умеет падать в инициалы
// по onError.
func (h *AdminUsersHandler) avatarURL(c *database.Customer) string {
	if c == nil || len(h.avatarSecret) == 0 {
		return ""
	}
	if c.IsWebOnly || utils.IsSyntheticTelegramID(c.TelegramID) {
		return ""
	}
	token, err := avatartoken.IssueTelegram(h.avatarSecret, c.TelegramID, adminAvatarTTL, time.Now())
	if err != nil {
		return ""
	}
	return "/cabinet/api/avatar?t=" + url.QueryEscape(token)
}

// fillPanelLogins проставляет логины web-клиентам одной пачкой.
//
// Нужен, потому что до первой покупки у web-клиента нет ни профиля в панели,
// ни @username — в списке он выглядел просто как "#222", и найти его было нечем.
// Ошибка запроса не фатальна: логин — вспомогательная подпись, а не данные операции.
func (h *AdminUsersHandler) fillPanelLogins(ctx context.Context, items []adminCustomerDTO) {
	if h.customers == nil || len(items) == 0 {
		return
	}
	ids := make([]int64, 0, len(items))
	for i := range items {
		if items[i].IsWebOnly {
			ids = append(ids, items[i].ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	emails, err := h.customers.CabinetAccountEmailsByCustomerIDs(ctx, ids)
	if err != nil {
		slog.Warn("admin users: не удалось загрузить логины web-клиентов", "error", err)
		return
	}
	for i := range items {
		if !items[i].IsWebOnly {
			continue
		}
		login := utils.PanelLoginForCustomer(items[i].ID, emails[items[i].ID])
		if login != "" {
			l := login
			items[i].PanelLogin = &l
		}
	}
}

func mapCustomerToDTO(c *database.Customer) adminCustomerDTO {
	dto := adminCustomerDTO{
		ID:         c.ID,
		TelegramID: c.TelegramID,
		Language:   c.Language,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		ExtraHwid:  c.ExtraHwid,
		LoyaltyXP:  c.LoyaltyXP,
		IsWebOnly:  c.IsWebOnly,
	}
	if c.TelegramUsername != nil {
		u := *c.TelegramUsername
		dto.TelegramUsername = &u
	}
	if c.ExpireAt != nil {
		s := c.ExpireAt.Format(time.RFC3339)
		dto.ExpireAt = &s
	}
	if c.SubscriptionLink != nil {
		s := *c.SubscriptionLink
		dto.SubscriptionLink = &s
	}
	if c.ExtraHwidExpiresAt != nil {
		s := c.ExtraHwidExpiresAt.Format(time.RFC3339)
		dto.ExtraHwidExpiresAt = &s
	}
	if c.CurrentTariffID != nil {
		id := *c.CurrentTariffID
		dto.CurrentTariffID = &id
	}
	if c.HwidLimitOverride != nil {
		v := *c.HwidLimitOverride
		dto.HwidLimitOverride = &v
	}
	if c.SubscriptionPeriodStart != nil {
		s := c.SubscriptionPeriodStart.Format(time.RFC3339)
		dto.SubscriptionPeriodStart = &s
	}
	if c.SubscriptionPeriodMonths != nil {
		m := *c.SubscriptionPeriodMonths
		dto.SubscriptionPeriodMonths = &m
	}

	dto.Status = deriveCustomerStatus(c, nil)

	return dto
}

// deriveCustomerStatus вычисляет статус для админки.
// paidSubs == nil — без проверки оплат (активная подписка без детализации trial/paid).
func deriveCustomerStatus(c *database.Customer, paidSubs *int) string {
	if c == nil {
		return "inactive"
	}
	now := time.Now()
	if c.ExpireAt != nil && c.ExpireAt.After(now) {
		hasLink := c.SubscriptionLink != nil && strings.TrimSpace(*c.SubscriptionLink) != ""
		if paidSubs != nil && *paidSubs == 0 && hasLink {
			return "trial"
		}
		return "active"
	}
	if c.ExpireAt != nil {
		return "expired"
	}
	return "inactive"
}

func (h *AdminUsersHandler) customerDTOWithStatus(ctx context.Context, c *database.Customer) adminCustomerDTO {
	dto := mapCustomerToDTO(c)
	if c == nil {
		return dto
	}
	h.enrichLoyaltyDTO(ctx, c.LoyaltyXP, &dto)
	if h.purchases == nil {
		return dto
	}
	if c.ExpireAt == nil || !c.ExpireAt.After(time.Now()) {
		return dto
	}
	n, err := h.purchases.CountPaidSubscriptionsByCustomer(ctx, c.ID)
	if err != nil {
		slog.Warn("admin users: paid subscription count failed", "customer_id", c.ID, "error", err)
		return dto
	}
	dto.Status = deriveCustomerStatus(c, &n)
	return dto
}

func (h *AdminUsersHandler) enrichLoyaltyDTO(ctx context.Context, loyaltyXP int64, dto *adminCustomerDTO) {
	if dto == nil || !config.LoyaltyEnabled() || h.loyalty == nil {
		return
	}
	prog, err := h.loyalty.ProgressForXP(ctx, loyaltyXP)
	if err != nil {
		slog.Warn("admin users: loyalty progress failed", "error", err)
		return
	}
	lvl := prog.CurrentTier.SortOrder
	pct := prog.CurrentTier.DiscountPercent
	dto.LoyaltyLevel = &lvl
	dto.LoyaltyDiscountPercent = &pct
}

func applyRWStatusToDTO(dto *adminCustomerDTO, rw *remnawave.User) {
	if dto == nil || rw == nil {
		return
	}
	st := strings.TrimSpace(rw.Status)
	if st == "" {
		return
	}
	dto.RwStatus = &st
	if strings.EqualFold(st, "DISABLED") {
		dto.Status = "disabled"
	}
}

// --- List -------------------------------------------------------------------

type adminUsersListResp struct {
	Items []adminCustomerDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

// List — GET /cabinet/api/admin/users?scope=all|active|inactive|expiring|trial&page=1&limit=20.
func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	var scope database.CustomerListScope
	switch q.Get("scope") {
	case "active":
		scope = database.CustomerListScopeActive
	case "inactive":
		scope = database.CustomerListScopeInactive
	case "expiring":
		scope = database.CustomerListScopeExpiringSoon
	case "trial", "trials":
		scope = database.CustomerListScopeTrial
	default:
		scope = database.CustomerListScopeAll
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	ctx := r.Context()

	total, err := h.customers.CountByListScope(ctx, scope)
	if err != nil {
		slog.Error("admin users: count failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	items, err := h.customers.ListPaged(ctx, scope, offset, limit)
	if err != nil {
		slog.Error("admin users: list failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]adminCustomerDTO, 0, len(items))
	for i := range items {
		dtos = append(dtos, h.customerDTOWithStatus(ctx, &items[i]))
	}

	h.fillPanelLogins(ctx, dtos)
	writeJSON(w, http.StatusOK, adminUsersListResp{
		Items: dtos,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// --- Search -----------------------------------------------------------------

type adminUsersSearchResp struct {
	Items []adminCustomerDTO `json:"items"`
}

// Search — GET /cabinet/api/admin/users/search?q=...
func (h *AdminUsersHandler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	needle := normalizeAdminSearchQuery(r.URL.Query().Get("q"))
	if needle == "" {
		writeJSON(w, http.StatusOK, adminUsersSearchResp{Items: []adminCustomerDTO{}})
		return
	}

	ctx := r.Context()
	const searchLimit = 24

	if isAdminSearchDigitsOnly(needle) {
		if tgID, err := strconv.ParseInt(needle, 10, 64); err == nil && tgID > 0 {
			cust, err := h.customers.FindByTelegramId(ctx, tgID)
			if err == nil && cust != nil {
				single := []adminCustomerDTO{h.customerDTOWithStatus(ctx, cust)}
				h.fillPanelLogins(ctx, single)
				writeJSON(w, http.StatusOK, adminUsersSearchResp{
					Items: single,
				})
				return
			}
		}
	}

	dbRows, err := h.customers.SearchForAdmin(ctx, needle, searchLimit)
	if err != nil {
		slog.Error("admin users: search failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var rwRows []remnawave.User
	if h.rw != nil {
		if rows, rwErr := h.rw.FindUsersMatchingAdminSearch(ctx, needle); rwErr != nil {
			slog.Warn("admin users: rw search failed", "error", rwErr.Error())
		} else {
			rwRows = rows
		}
	}

	merged := adminUsersMergeSearchResults(ctx, h.customers, dbRows, rwRows, searchLimit)
	dtos := make([]adminCustomerDTO, 0, len(merged))
	for i := range merged {
		dtos = append(dtos, h.customerDTOWithStatus(ctx, &merged[i]))
	}

	h.fillPanelLogins(ctx, dtos)
	writeJSON(w, http.StatusOK, adminUsersSearchResp{Items: dtos})
}

// --- Get --------------------------------------------------------------------

// Get — GET /cabinet/api/admin/users/{id}.
func (h *AdminUsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	cust, err := h.customers.FindById(r.Context(), id)
	if err != nil {
		slog.Error("admin users: get failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	dto := h.customerDTOWithStatus(r.Context(), cust)
	dto.AvatarURL = h.avatarURL(cust)
	writeJSON(w, http.StatusOK, dto)
}

// --- Remnawave lookup (mirrors internal/handler/admin_remnawave_lookup.go) --

func (h *AdminUsersHandler) findRWUser(ctx context.Context, cust *database.Customer) (*remnawave.User, error) {
	if cust == nil {
		return nil, fmt.Errorf("nil customer")
	}
	if h.rw == nil {
		return nil, fmt.Errorf("remnawave client not configured")
	}
	return h.rw.FindUserForAdminCustomer(ctx, cust.ID, cust.TelegramID, cust.SubscriptionLink, cust.IsWebOnly)
}

func adminAccountID(r *http.Request) int64 {
	if c := middleware.AuthClaims(r); c != nil {
		return c.AccountID
	}
	return 0
}

func (h *AdminUsersHandler) syncCustomerAfterPatch(ctx context.Context, custID int64, rwUser *remnawave.User) error {
	return h.customers.UpdateFields(ctx, custID, map[string]interface{}{
		"subscription_link": rwUser.SubscriptionUrl,
		"expire_at":         rwUser.ExpireAt,
	})
}

// --- Mutation operations ----------------------------------------------------

// SetExpire — PATCH /cabinet/api/admin/users/{id}/expire.
func (h *AdminUsersHandler) SetExpire(w http.ResponseWriter, r *http.Request) {
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
		ExpireAt string `json:"expire_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	parsed, err := time.Parse(time.RFC3339, body.ExpireAt)
	if err != nil {
		http.Error(w, "invalid expire_at format, use RFC3339", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: set expire — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rwUser, err := h.findRWUser(ctx, cust)
	if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Error("admin users: set expire — rw lookup failed", "error", err.Error())
		http.Error(w, "remnawave lookup failed", http.StatusInternalServerError)
		return
	}
	if rwUser != nil {
		req := &remnawave.UpdateUserRequest{
			ID:       &rwUser.ID,
			Status:   "ACTIVE",
			ExpireAt: &parsed,
		}
		out, patchErr := h.rw.PatchUser(ctx, req)
		if patchErr != nil {
			slog.Error("admin users: set expire — rw patch failed", "error", patchErr.Error())
			http.Error(w, "remnawave update failed", http.StatusInternalServerError)
			return
		}
		if syncErr := h.syncCustomerAfterPatch(ctx, cust.ID, out); syncErr != nil {
			slog.Warn("admin users: set expire — db sync failed", "error", syncErr.Error())
		}
	} else {
		if err := h.customers.UpdateFields(ctx, id, map[string]interface{}{"expire_at": parsed}); err != nil {
			slog.Error("admin users: set expire — db update failed", "error", err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	updated, err := h.customers.FindById(ctx, id)
	if err != nil || updated == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: set expire", "customer_id", id, "expire_at", parsed.Format(time.RFC3339), "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, mapCustomerToDTO(updated))
}

// Extend — POST /cabinet/api/admin/users/{id}/extend.
func (h *AdminUsersHandler) Extend(w http.ResponseWriter, r *http.Request) {
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
		Days int `json:"days"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Days < 1 || body.Days > 3650 {
		http.Error(w, "days must be between 1 and 3650", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if !h.requireRW(w) {
		return
	}

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: extend — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var rwOut *remnawave.User
	hasActive := cust.ExpireAt != nil && cust.ExpireAt.After(time.Now())
	if hasActive {
		rwOut, err = h.rw.ExtendSubscriptionByDaysPreserveSquads(ctx, cust.ID, cust.TelegramID, body.Days)
	} else {
		rwOut, err = h.rw.CreateOrUpdateUserFromNow(ctx, cust.ID, cust.TelegramID, config.TrafficLimit(), body.Days, false)
	}
	if err != nil {
		slog.Error("admin users: extend — rw failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	if syncErr := h.syncCustomerAfterPatch(ctx, cust.ID, rwOut); syncErr != nil {
		slog.Warn("admin users: extend — db sync failed", "error", syncErr.Error())
	}

	updated, err := h.customers.FindById(ctx, id)
	if err != nil || updated == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: extended user", "customer_id", id, "days", body.Days, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, mapCustomerToDTO(updated))
}

// Disable — POST /cabinet/api/admin/users/{id}/disable.
func (h *AdminUsersHandler) Disable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		slog.Error("admin users: disable — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rwUser, err := h.findRWUser(ctx, cust)
	if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Error("admin users: disable — rw lookup failed", "error", err.Error())
		http.Error(w, "remnawave lookup failed", http.StatusInternalServerError)
		return
	}
	if rwUser == nil {
		http.Error(w, "remnawave user not found", http.StatusNotFound)
		return
	}

	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:     &rwUser.ID,
		Status: "DISABLED",
	})
	if err != nil {
		slog.Error("admin users: disable — rw patch failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	if syncErr := h.syncCustomerAfterPatch(ctx, cust.ID, out); syncErr != nil {
		slog.Warn("admin users: disable — db sync failed", "error", syncErr.Error())
	}

	slog.Info("admin: disabled user", "customer_id", id, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// Enable — POST /cabinet/api/admin/users/{id}/enable.
func (h *AdminUsersHandler) Enable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		slog.Error("admin users: enable — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rwUser, err := h.findRWUser(ctx, cust)
	if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Error("admin users: enable — rw lookup failed", "error", err.Error())
		http.Error(w, "remnawave lookup failed", http.StatusInternalServerError)
		return
	}
	if rwUser == nil {
		http.Error(w, "remnawave user not found", http.StatusNotFound)
		return
	}

	out, err := h.rw.PatchUser(ctx, &remnawave.UpdateUserRequest{
		ID:     &rwUser.ID,
		Status: "ACTIVE",
	})
	if err != nil {
		slog.Error("admin users: enable — rw patch failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	if syncErr := h.syncCustomerAfterPatch(ctx, cust.ID, out); syncErr != nil {
		slog.Warn("admin users: enable — db sync failed", "error", syncErr.Error())
	}

	slog.Info("admin: enabled user", "customer_id", id, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// Delete — POST /cabinet/api/admin/users/{id}/delete.
func (h *AdminUsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: delete — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rwUser, findErr := h.findRWUser(ctx, cust)
	if findErr != nil && !errors.Is(findErr, remnawave.ErrUserNotFound) {
		slog.Warn("admin users: delete — rw lookup failed", "error", findErr.Error())
	}
	if rwUser != nil {
		if delErr := h.rw.DeleteUser(ctx, rwUser.ID); delErr != nil {
			slog.Error("admin users: delete — rw delete failed", "error", delErr.Error())
			http.Error(w, "remnawave delete failed", http.StatusInternalServerError)
			return
		}
	}

	if err := h.customers.DeleteByIDWithCabinetCascade(ctx, cust.ID); err != nil {
		slog.Error("admin users: delete — db delete failed", "error", err.Error())
		http.Error(w, "db delete failed", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: deleted user", "customer_id", id, "telegram_id", cust.TelegramID, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ResetTraffic — POST /cabinet/api/admin/users/{id}/reset-traffic.
func (h *AdminUsersHandler) ResetTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		slog.Error("admin users: reset traffic — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rwUser, err := h.findRWUser(ctx, cust)
	if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
		slog.Error("admin users: reset traffic — rw lookup failed", "error", err.Error())
		http.Error(w, "remnawave lookup failed", http.StatusInternalServerError)
		return
	}
	if rwUser == nil {
		http.Error(w, "remnawave user not found", http.StatusNotFound)
		return
	}

	if err := h.rw.ResetUserTraffic(ctx, rwUser.ID); err != nil {
		slog.Error("admin users: reset traffic — rw failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: reset traffic", "customer_id", id, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetHwidLimit — PATCH /cabinet/api/admin/users/{id}/hwid-limit.
func (h *AdminUsersHandler) SetHwidLimit(w http.ResponseWriter, r *http.Request) {
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
		Limit int `json:"limit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Limit < 1 || body.Limit > 100 {
		http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if !h.requireRW(w) {
		return
	}

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: set hwid — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if _, err := h.rw.UpdateUserDeviceLimitByCustomer(ctx, cust.ID, cust.TelegramID, body.Limit); err != nil {
		slog.Error("admin users: set hwid — rw failed", "error", err.Error())
		http.Error(w, "remnawave operation failed", http.StatusInternalServerError)
		return
	}

	slog.Info("admin: set hwid limit", "customer_id", id, "limit", body.Limit, "admin_account_id", adminAccountID(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Payments / Referrals / Spend -------------------------------------------

type adminPurchaseDTO struct {
	ID         int64   `json:"id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	PaidAt     *string `json:"paid_at"`
	Month      int     `json:"month"`
	Type       string  `json:"invoice_type"`
	Kind       string  `json:"purchase_kind"`
	TariffID   *int64  `json:"tariff_id"`
	PromoID    *int64  `json:"promo_code_id"`
	DiscountPc *int    `json:"discount_percent"`
}

type adminPaymentsResp struct {
	Items         []adminPurchaseDTO `json:"items"`
	Total         int                `json:"total"`
	RubCount      int64              `json:"rub_count"`
	RubSum        float64            `json:"rub_sum"`
	StarsCount    int64              `json:"stars_count"`
	StarsSum      float64            `json:"stars_sum"`
	RubPerStar    float64            `json:"rub_per_star"`
	StarsRubEquiv float64            `json:"stars_rub_equiv"`
}

// Payments — GET /cabinet/api/admin/users/{id}/payments?page=&limit=.
func (h *AdminUsersHandler) Payments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminUsersExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	ctx := r.Context()

	total, err := h.purchases.CountPaidByCustomer(ctx, id)
	if err != nil {
		slog.Error("admin users: payments count failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	items, err := h.purchases.FindPaidByCustomer(ctx, id, limit, offset)
	if err != nil {
		slog.Error("admin users: payments list failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rubCount, rubSum, starsCount, starsSum, err := h.purchases.SumPaidSpendBreakdown(ctx, id)
	if err != nil {
		slog.Error("admin users: spend breakdown failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]adminPurchaseDTO, 0, len(items))
	for _, p := range items {
		dto := adminPurchaseDTO{
			ID:         p.ID,
			Amount:     p.Amount,
			Currency:   p.Currency,
			Month:      p.Month,
			Type:       string(p.InvoiceType),
			Kind:       string(p.PurchaseKind),
			TariffID:   p.TariffID,
			PromoID:    p.PromoCodeID,
			DiscountPc: p.DiscountPercentApplied,
		}
		if p.PaidAt != nil {
			s := p.PaidAt.Format(time.RFC3339)
			dto.PaidAt = &s
		}
		dtos = append(dtos, dto)
	}

	writeJSON(w, http.StatusOK, adminPaymentsResp{
		Items:         dtos,
		Total:         total,
		RubCount:      rubCount,
		RubSum:        rubSum,
		StarsCount:    starsCount,
		StarsSum:      starsSum,
		RubPerStar:    config.RubPerStar(),
		StarsRubEquiv: starsRubEquiv(starsSum, config.RubPerStar()),
	})
}

func starsRubEquiv(starsSum float64, rubPerStar float64) float64 {
	if rubPerStar <= 0 || starsSum <= 0 {
		return 0
	}
	return starsSum * rubPerStar
}

type adminReferralDTO struct {
	TelegramID       int64   `json:"telegram_id,string"`
	TelegramUsername *string `json:"telegram_username"`
	Active           bool    `json:"active"`
	Email            *string `json:"email"`
}

type adminReferralStatsDTO struct {
	Total           int `json:"total"`
	Paid            int `json:"paid"`
	Active          int `json:"active"`
	Conversion      int `json:"conversion"`
	EarnedTotal     int `json:"earned_total"`
	EarnedLastMonth int `json:"earned_last_month"`
}

func mapReferralStatsDTO(s database.ReferralStats) adminReferralStatsDTO {
	return adminReferralStatsDTO{
		Total:           s.Total,
		Paid:            s.Paid,
		Active:          s.Active,
		Conversion:      s.Conversion,
		EarnedTotal:     s.EarnedTotal,
		EarnedLastMonth: s.EarnedLastMonth,
	}
}

type adminReferralsResp struct {
	Stats    adminReferralStatsDTO `json:"stats"`
	Referees []adminReferralDTO    `json:"referees"`
}

// Referrals — GET /cabinet/api/admin/users/{id}/referrals.
func (h *AdminUsersHandler) Referrals(w http.ResponseWriter, r *http.Request) {
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

	cust, err := h.customers.FindById(ctx, id)
	if err != nil {
		slog.Error("admin users: referrals — find failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cust == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	stats, err := h.referrals.GetStats(ctx, cust.TelegramID)
	if err != nil {
		slog.Error("admin users: referral stats failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	summaries, err := h.referrals.FindRefereeSummariesByReferrerPage(ctx, cust.TelegramID, limit, offset)
	if err != nil {
		slog.Warn("admin users: referral summaries failed", "error", err.Error())
		summaries = nil
	}

	dtos := make([]adminReferralDTO, 0, len(summaries))
	for _, s := range summaries {
		dtos = append(dtos, adminReferralDTO{
			TelegramID:       s.TelegramID,
			TelegramUsername: s.TelegramUsername,
			Active:           s.Active,
			Email:            s.Email,
		})
	}

	writeJSON(w, http.StatusOK, adminReferralsResp{
		Stats:    mapReferralStatsDTO(stats),
		Referees: dtos,
	})
}

// HandleByID dispatches /cabinet/api/admin/users/{id}[/action] requests.
func (h *AdminUsersHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/extend"):
		h.Extend(w, r)
	case strings.HasSuffix(path, "/disable"):
		h.Disable(w, r)
	case strings.HasSuffix(path, "/enable"):
		h.Enable(w, r)
	case strings.HasSuffix(path, "/delete"):
		h.Delete(w, r)
	case strings.HasSuffix(path, "/reset-traffic"):
		h.ResetTraffic(w, r)
	case strings.HasSuffix(path, "/expire"):
		h.SetExpire(w, r)
	case strings.HasSuffix(path, "/hwid-limit"):
		h.SetHwidLimit(w, r)
	case strings.HasSuffix(path, "/hwid-pin"):
		h.SetHwidPin(w, r)
	case strings.HasSuffix(path, "/payments"):
		h.Payments(w, r)
	case strings.HasSuffix(path, "/referrals"):
		h.Referrals(w, r)
	case strings.HasSuffix(path, "/panel"):
		h.Panel(w, r)
	case strings.HasSuffix(path, "/squads"):
		h.SetSquads(w, r)
	case strings.HasSuffix(path, "/traffic"):
		h.SetTraffic(w, r)
	case strings.HasSuffix(path, "/strategy"):
		h.SetStrategy(w, r)
	case strings.HasSuffix(path, "/description"):
		h.SetDescription(w, r)
	case strings.HasSuffix(path, "/tariff"):
		h.SetTariff(w, r)
	case strings.Contains(path, "/devices/"):
		if r.Method == http.MethodDelete {
			h.DeleteDevice(w, r)
		} else {
			h.Devices(w, r)
		}
	case strings.HasSuffix(path, "/devices"):
		h.Devices(w, r)
	case strings.HasSuffix(path, "/extra-hwid"):
		h.ExtraHwid(w, r)
	default:
		h.Get(w, r)
	}
}

// --- Helpers ----------------------------------------------------------------

func adminUsersExtractID(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "users" && i+1 < len(parts) {
			id, err := strconv.ParseInt(parts[i+1], 10, 64)
			if err == nil {
				return id, true
			}
		}
	}
	return 0, false
}

func adminUsersExtractDeviceHwid(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "devices" && i+1 < len(parts) {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return ""
}

func normalizeAdminSearchQuery(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	return strings.TrimSpace(s)
}

func isAdminSearchDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func adminUsersMergeSearchResults(ctx context.Context, customers *database.CustomerRepository, dbRows []database.Customer, rwRows []remnawave.User, limit int) []database.Customer {
	if limit <= 0 {
		limit = 24
	}
	seen := make(map[int64]struct{})
	out := make([]database.Customer, 0, limit)
	add := func(c *database.Customer) {
		if c == nil {
			return
		}
		if _, ok := seen[c.ID]; ok {
			return
		}
		if len(out) >= limit {
			return
		}
		seen[c.ID] = struct{}{}
		out = append(out, *c)
	}
	for i := range dbRows {
		add(&dbRows[i])
		if len(out) >= limit {
			return out
		}
	}
	for _, u := range rwRows {
		cust, err := remnawave.CustomerFromAdminSearchUser(ctx, customers, u)
		if err != nil || cust == nil {
			continue
		}
		add(cust)
		if len(out) >= limit {
			break
		}
	}
	return out
}
