package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/pgerr"
)

func extractPromoID(path string) (int64, bool) {
	s := strings.TrimPrefix(path, "/cabinet/api/admin/promos/")
	s = strings.TrimSuffix(s, "/redemptions")
	s = strings.TrimRight(s, "/")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

type AdminPromosHandler struct {
	promos *database.PromoRepository
}

func NewAdminPromos(promos *database.PromoRepository) *AdminPromosHandler {
	return &AdminPromosHandler{promos: promos}
}

type promoDTO struct {
	ID                                         int64      `json:"id"`
	Code                                       string     `json:"code"`
	Type                                       string     `json:"type"`
	SubscriptionDays                           *int       `json:"subscription_days"`
	TrialDays                                  *int       `json:"trial_days"`
	ExtraHwidDelta                             *int       `json:"extra_hwid_delta"`
	DiscountPercent                            *int       `json:"discount_percent"`
	DiscountTTLHours                           *int       `json:"discount_ttl_hours"`
	MaxUses                                    *int       `json:"max_uses"`
	UsesCount                                  int        `json:"uses_count"`
	ValidUntil                                 *time.Time `json:"valid_until"`
	Active                                     bool       `json:"active"`
	FirstPurchaseOnly                          bool       `json:"first_purchase_only"`
	RequireCustomerInDB                        bool       `json:"require_customer_in_db"`
	AllowTrialWithoutPayment                   bool       `json:"allow_trial_without_payment"`
	CreatedAt                                  time.Time  `json:"created_at"`
	DiscountMaxSubscriptionPaymentsPerCustomer int        `json:"discount_max_subscription_payments_per_customer"`
	TariffID                                   *int64     `json:"tariff_id"`
}

func promoToDTO(p *database.PromoCode) promoDTO {
	return promoDTO{
		ID: p.ID, Code: p.Code, Type: p.Type,
		SubscriptionDays: p.SubscriptionDays, TrialDays: p.TrialDays,
		ExtraHwidDelta: p.ExtraHwidDelta, DiscountPercent: p.DiscountPercent,
		DiscountTTLHours: p.DiscountTTLHours, MaxUses: p.MaxUses,
		UsesCount: p.UsesCount, ValidUntil: p.ValidUntil,
		Active: p.Active, FirstPurchaseOnly: p.FirstPurchaseOnly,
		RequireCustomerInDB: p.RequireCustomerInDB, AllowTrialWithoutPayment: p.AllowTrialWithoutPayment,
		CreatedAt: p.CreatedAt, TariffID: p.TariffID,
		DiscountMaxSubscriptionPaymentsPerCustomer: p.DiscountMaxSubscriptionPaymentsPerCustomer,
	}
}

type promoListResp struct {
	Items []promoDTO `json:"items"`
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
}

// List — GET /cabinet/api/admin/promos
func (h *AdminPromosHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	items, total, err := h.promos.List(r.Context(), offset, limit)
	if err != nil {
		slog.Error("admin promos list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]promoDTO, 0, len(items))
	for i := range items {
		dtos = append(dtos, promoToDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, promoListResp{
		Items: dtos,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

type createPromoReq struct {
	Code                                       string     `json:"code"`
	Type                                       string     `json:"type"`
	SubscriptionDays                           *int       `json:"subscription_days"`
	TrialDays                                  *int       `json:"trial_days"`
	ExtraHwidDelta                             *int       `json:"extra_hwid_delta"`
	DiscountPercent                            *int       `json:"discount_percent"`
	DiscountTTLHours                           *int       `json:"discount_ttl_hours"`
	MaxUses                                    *int       `json:"max_uses"`
	ValidUntil                                 *time.Time `json:"valid_until"`
	FirstPurchaseOnly                          bool       `json:"first_purchase_only"`
	TariffID                                   *int64     `json:"tariff_id"`
	DiscountMaxSubscriptionPaymentsPerCustomer int        `json:"discount_max_subscription_payments_per_customer"`
}

// validateCreatePromo проверяет параметры нового промокода и возвращает
// машинное сообщение для админки (см. formatAdminApiError) или "". Раньше
// проверялось только наличие кода и типа: скидка 0 % или 150 %, отрицательные
// дни и неизвестный тип сохранялись как есть.
func validateCreatePromo(req *createPromoReq, now time.Time) string {
	code := strings.TrimSpace(req.Code)
	if n := utf8.RuneCountInString(code); n < 2 || n > 64 || strings.HasPrefix(code, "__") {
		return "invalid promo code"
	}
	for _, r := range code {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return "invalid promo code"
		}
	}
	positive := func(v *int) bool { return v != nil && *v > 0 }
	switch req.Type {
	case database.PromoTypeSubscriptionDays:
		if !positive(req.SubscriptionDays) {
			return "subscription days must be positive"
		}
	case database.PromoTypeTrial:
		if !positive(req.TrialDays) {
			return "trial days must be positive"
		}
	case database.PromoTypeExtraHwid:
		if !positive(req.ExtraHwidDelta) {
			return "extra hwid delta must be positive"
		}
	case database.PromoTypeDiscount:
		if req.DiscountPercent == nil || *req.DiscountPercent < 1 || *req.DiscountPercent > 100 {
			return "discount percent must be 1-100"
		}
		if req.DiscountTTLHours != nil && *req.DiscountTTLHours < 0 {
			return "discount ttl must not be negative"
		}
		if req.DiscountMaxSubscriptionPaymentsPerCustomer < 0 {
			return "discount max payments must not be negative"
		}
	default:
		return "invalid promo type"
	}
	if req.MaxUses != nil && *req.MaxUses <= 0 {
		return "max uses must be positive"
	}
	if req.ValidUntil != nil && !req.ValidUntil.After(now) {
		return "valid until is in the past"
	}
	return ""
}

// Create — POST /cabinet/api/admin/promos
func (h *AdminPromosHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createPromoReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Type) == "" {
		http.Error(w, "code and type are required", http.StatusBadRequest)
		return
	}
	if msg := validateCreatePromo(&req, time.Now()); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	p := database.PromoCode{
		Code:                     strings.ToUpper(strings.TrimSpace(req.Code)),
		Type:                     req.Type,
		SubscriptionDays:         req.SubscriptionDays,
		TrialDays:                req.TrialDays,
		ExtraHwidDelta:           req.ExtraHwidDelta,
		DiscountPercent:          req.DiscountPercent,
		DiscountTTLHours:         req.DiscountTTLHours,
		MaxUses:                  req.MaxUses,
		ValidUntil:               req.ValidUntil,
		Active:                   true,
		FirstPurchaseOnly:        req.FirstPurchaseOnly,
		AllowTrialWithoutPayment: true,
		TariffID:                 req.TariffID,
		DiscountMaxSubscriptionPaymentsPerCustomer: req.DiscountMaxSubscriptionPaymentsPerCustomer,
	}

	id, err := h.promos.Create(r.Context(), &p)
	if err != nil {
		// Код уникален без учёта регистра, в том числе среди выключенных
		// промокодов. Раньше повтор отдавал 500, и было непонятно, что не так.
		if pgerr.IsUniqueViolation(err) {
			http.Error(w, "promo code already exists", http.StatusConflict)
			return
		}
		slog.Error("admin promos create", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.ID = id
	writeJSON(w, http.StatusCreated, promoToDTO(&p))
}

type promoGetResp struct {
	Promo            promoDTO `json:"promo"`
	Redemptions      int      `json:"redemptions"`
	RedemptionsToday int      `json:"redemptions_today"`
}

// Get — GET /cabinet/api/admin/promos/{id}
func (h *AdminPromosHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractPromoID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	p, err := h.promos.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("admin promos get", "error", err.Error())
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	redemptions, _ := h.promos.CountRedemptions(r.Context(), id)
	redemptionsToday, _ := h.promos.CountRedemptionsToday(r.Context(), id)

	writeJSON(w, http.StatusOK, promoGetResp{
		Promo:            promoToDTO(p),
		Redemptions:      redemptions,
		RedemptionsToday: redemptionsToday,
	})
}

// Update — PATCH /cabinet/api/admin/promos/{id}
func (h *AdminPromosHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractPromoID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing, err := h.promos.FindByID(r.Context(), id)
	if err != nil || existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var raw map[string]json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}

	allowed := map[string]bool{
		"active": true, "max_uses": true, "valid_until": true,
		"first_purchase_only": true, "subscription_days": true, "trial_days": true,
		"extra_hwid_delta": true, "discount_percent": true, "discount_ttl_hours": true,
		"discount_max_subscription_payments_per_customer": true, "tariff_id": true,
	}

	fields, err := parsePromoPatchFields(raw, allowed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(fields) == 0 {
		http.Error(w, "no valid fields", http.StatusBadRequest)
		return
	}

	if err := validatePromoUpdateFields(existing.Type, fields); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.promos.UpdateFields(r.Context(), id, fields); err != nil {
		slog.Error("admin promos update", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	p, err := h.promos.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("admin promos get after update", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, promoToDTO(p))
}

// Delete — DELETE /cabinet/api/admin/promos/{id}
func (h *AdminPromosHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractPromoID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.promos.Delete(r.Context(), id); err != nil {
		slog.Error("admin promos delete", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type promoRedemptionItemDTO struct {
	UsedAt           string  `json:"used_at"`
	CustomerID       int64   `json:"customer_id"`
	TelegramUsername *string `json:"telegram_username"`
	Nickname         *string `json:"nickname"`
}

type promoRedemptionsResp struct {
	Items []promoRedemptionItemDTO `json:"items"`
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Limit int                      `json:"limit"`
}

// Redemptions — GET /cabinet/api/admin/promos/{id}/redemptions
func (h *AdminPromosHandler) Redemptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := extractPromoID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if _, err := h.promos.FindByID(r.Context(), id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	total, err := h.promos.CountRedemptions(r.Context(), id)
	if err != nil {
		slog.Error("admin promos redemptions count", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows, err := h.promos.ListRedemptionsPage(r.Context(), id, offset, limit)
	if err != nil {
		slog.Error("admin promos redemptions list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]promoRedemptionItemDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, promoRedemptionItemDTO{
			UsedAt:           row.UsedAt.UTC().Format(time.RFC3339),
			CustomerID:       row.CustomerID,
			TelegramUsername: row.TelegramUsername,
			Nickname:         row.Nickname,
		})
	}

	writeJSON(w, http.StatusOK, promoRedemptionsResp{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// Handle dispatches /cabinet/api/admin/promos (no trailing path).
func (h *AdminPromosHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.List(w, r)
	case http.MethodPost:
		h.Create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleByID dispatches /cabinet/api/admin/promos/{id}[/redemptions].
func (h *AdminPromosHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/redemptions") {
		h.Redemptions(w, r)
		return
	}
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
}
