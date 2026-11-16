package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/cabinet/avatartoken"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/pgerr"
	"remnawave-tg-shop-bot/utils"
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
	// tariffs — проверка, что tariff_id у промокода «Дни подписки» существует.
	tariffs *database.TariffRepository
	// avatarSecret — тот же секрет, что у карточки пользователя: им подписываются
	// ссылки на аватарки в окне «Кто использовал».
	avatarSecret []byte
}

func NewAdminPromos(promos *database.PromoRepository, tariffs *database.TariffRepository, avatarSecret []byte) *AdminPromosHandler {
	return &AdminPromosHandler{promos: promos, tariffs: tariffs, avatarSecret: avatarSecret}
}

// redemptionAvatarURL — ссылка на аватарку активировавшего, как в карточке
// пользователя (AdminUsersHandler.avatarURL): существование фото не проверяем,
// <img> сам падает в инициалы.
func (h *AdminPromosHandler) redemptionAvatarURL(row database.PromoRedemptionDetailRow) string {
	if len(h.avatarSecret) == 0 || row.IsWebOnly || utils.IsSyntheticTelegramID(row.TelegramID) {
		return ""
	}
	token, err := avatartoken.IssueTelegram(h.avatarSecret, row.TelegramID, adminAvatarTTL, time.Now())
	if err != nil {
		return ""
	}
	return "/cabinet/api/avatar?t=" + url.QueryEscape(token)
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
	Note                                       *string    `json:"note"`
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
		CreatedAt: p.CreatedAt, TariffID: p.TariffID, Note: p.Note,
		DiscountMaxSubscriptionPaymentsPerCustomer: p.DiscountMaxSubscriptionPaymentsPerCustomer,
	}
}

type promoStatusCountsDTO struct {
	All    int `json:"all"`
	Active int `json:"active"`
	Ended  int `json:"ended"`
	Off    int `json:"off"`
}

type promoTypeCountsDTO struct {
	All              int `json:"all"`
	SubscriptionDays int `json:"subscription_days"`
	Discount         int `json:"discount"`
	Trial            int `json:"trial"`
	ExtraHwid        int `json:"extra_hwid"`
}

type promoListCountsDTO struct {
	Status promoStatusCountsDTO `json:"status"`
	Type   promoTypeCountsDTO   `json:"type"`
}

type promoListSummaryDTO struct {
	Redemptions24h   int `json:"redemptions_24h"`
	RedemptionsTotal int `json:"redemptions_total"`
}

type promoListResp struct {
	Items   []promoDTO          `json:"items"`
	Total   int                 `json:"total"`
	Page    int                 `json:"page"`
	Limit   int                 `json:"limit"`
	Counts  promoListCountsDTO  `json:"counts"`
	Summary promoListSummaryDTO `json:"summary"`
}

// promoSearchMaxRunes — поиск длиннее кода (64 символа) смысла не имеет.
const promoSearchMaxRunes = 64

// promoListParams — разобранные параметры GET /cabinet/api/admin/promos.
type promoListParams struct {
	Page   int
	Limit  int
	Filter database.PromoListFilter
}

// parsePromoListParams разбирает page/limit/status/type/q/sort. Неизвестные
// status/type/sort — ошибка (текст для 400), пустые — значения по умолчанию.
func parsePromoListParams(q url.Values) (promoListParams, string) {
	var p promoListParams
	p.Page, _ = strconv.Atoi(q.Get("page"))
	if p.Page < 1 {
		p.Page = 1
	}
	p.Limit, _ = strconv.Atoi(q.Get("limit"))
	if p.Limit < 1 || p.Limit > 100 {
		p.Limit = 20
	}

	switch status := strings.TrimSpace(q.Get("status")); status {
	case "", database.PromoStatusAll:
		p.Filter.Status = database.PromoStatusAll
	case database.PromoStatusActive, database.PromoStatusEnded, database.PromoStatusOff:
		p.Filter.Status = status
	default:
		return p, "invalid status"
	}

	switch typ := strings.TrimSpace(q.Get("type")); typ {
	case "", "all":
		p.Filter.Type = ""
	case database.PromoTypeSubscriptionDays, database.PromoTypeTrial, database.PromoTypeExtraHwid, database.PromoTypeDiscount:
		p.Filter.Type = typ
	default:
		return p, "invalid type"
	}

	switch sort := strings.TrimSpace(q.Get("sort")); sort {
	case "", database.PromoSortNew:
		p.Filter.Sort = database.PromoSortNew
	case database.PromoSortUses, database.PromoSortExpiring, database.PromoSortCode:
		p.Filter.Sort = sort
	default:
		return p, "invalid sort"
	}

	switch dir := strings.TrimSpace(q.Get("dir")); dir {
	case "", database.PromoSortAsc, database.PromoSortDesc:
		p.Filter.Dir = dir
	default:
		return p, "invalid dir"
	}

	query := strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(query) > promoSearchMaxRunes {
		query = strings.TrimSpace(string([]rune(query)[:promoSearchMaxRunes]))
	}
	p.Filter.Query = query
	return p, ""
}

// promoTotalForStatus — строк под фильтрами: счётчик выбранной вкладки уже
// посчитан с учётом типа и поиска.
func promoTotalForStatus(c database.PromoStatusCounts, status string) int {
	switch status {
	case database.PromoStatusActive:
		return c.Active
	case database.PromoStatusEnded:
		return c.Ended
	case database.PromoStatusOff:
		return c.Off
	default:
		return c.All
	}
}

// List — GET /cabinet/api/admin/promos
func (h *AdminPromosHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	params, msg := parsePromoListParams(r.URL.Query())
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	offset := (params.Page - 1) * params.Limit

	items, counts, err := h.promos.ListFiltered(r.Context(), params.Filter, offset, params.Limit)
	if err != nil {
		slog.Error("admin promos list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	summary, err := h.promos.RedemptionsSummary(r.Context())
	if err != nil {
		slog.Error("admin promos summary", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]promoDTO, 0, len(items))
	for i := range items {
		dtos = append(dtos, promoToDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, promoListResp{
		Items: dtos,
		Total: promoTotalForStatus(counts.Status, params.Filter.Status),
		Page:  params.Page,
		Limit: params.Limit,
		Counts: promoListCountsDTO{
			Status: promoStatusCountsDTO{
				All: counts.Status.All, Active: counts.Status.Active,
				Ended: counts.Status.Ended, Off: counts.Status.Off,
			},
			Type: promoTypeCountsDTO{
				All: counts.Type.All, SubscriptionDays: counts.Type.SubscriptionDays,
				Discount: counts.Type.Discount, Trial: counts.Type.Trial, ExtraHwid: counts.Type.ExtraHwid,
			},
		},
		Summary: promoListSummaryDTO{
			Redemptions24h:   summary.Last24h,
			RedemptionsTotal: summary.Total,
		},
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
	Note                                       *string    `json:"note"`
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
	if msg := promoTariffAllowed(req.Type, req.TariffID != nil); msg != "" {
		return msg
	}
	if req.Note != nil {
		if _, ok := normalizePromoNote(*req.Note); !ok {
			return "note too long"
		}
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
	if req.TariffID != nil && !h.tariffExists(w, r, *req.TariffID) {
		return
	}
	var note *string
	if req.Note != nil {
		note, _ = normalizePromoNote(*req.Note)
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
		Note:                     note,
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

// tariffExists проверяет tariff_id из запроса. Пишет ответ и возвращает false,
// если тарифа нет или база не ответила.
func (h *AdminPromosHandler) tariffExists(w http.ResponseWriter, r *http.Request, id int64) bool {
	t, err := h.tariffs.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("admin promos tariff lookup", "error", err.Error(), "tariff_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if t == nil {
		http.Error(w, "tariff not found", http.StatusBadRequest)
		return false
	}
	return true
}

type promoPaybackDTO struct {
	PaidCustomers int     `json:"paid_customers"`
	Payments      int     `json:"payments"`
	RevenueRub    float64 `json:"revenue_rub"`
	RevenueStars  float64 `json:"revenue_stars"`
}

type promoGetResp struct {
	Promo       promoDTO `json:"promo"`
	Redemptions int      `json:"redemptions"`
	// RedemptionsToday — с 00:00 UTC; оставлено для совместимости, новое окно — Redemptions24h.
	RedemptionsToday int             `json:"redemptions_today"`
	Redemptions24h   int             `json:"redemptions_24h"`
	PendingDiscounts int             `json:"pending_discounts"`
	Payback          promoPaybackDTO `json:"payback"`
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
	if p == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	redemptions, _ := h.promos.CountRedemptions(r.Context(), id)
	redemptionsToday, _ := h.promos.CountRedemptionsToday(r.Context(), id)
	stats, err := h.promos.DetailStats(r.Context(), p)
	if err != nil {
		slog.Error("admin promos detail stats", "error", err.Error(), "promo_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, promoGetResp{
		Promo:            promoToDTO(p),
		Redemptions:      redemptions,
		RedemptionsToday: redemptionsToday,
		Redemptions24h:   stats.Redemptions24h,
		PendingDiscounts: stats.PendingDiscounts,
		Payback: promoPaybackDTO{
			PaidCustomers: stats.Payback.PaidCustomers,
			Payments:      stats.Payback.Payments,
			RevenueRub:    stats.Payback.RevenueRub,
			RevenueStars:  stats.Payback.RevenueStars,
		},
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
		"note": true,
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
	if tid, ok := fields["tariff_id"].(int64); ok && !h.tariffExists(w, r, tid) {
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
	// AvatarURL — подписанная ссылка на фото из Telegram; пусто — инициалы.
	AvatarURL string `json:"avatar_url,omitempty"`
	// Оплаты человека, засчитанные промокоду (как payback в GET /promos/{id}).
	PaidCount int     `json:"paid_count"`
	PaidRub   float64 `json:"paid_rub"`
	PaidStars float64 `json:"paid_stars"`
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

	if p, err := h.promos.FindByID(r.Context(), id); err != nil || p == nil {
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
			AvatarURL:        h.redemptionAvatarURL(row),
			PaidCount:        row.PaidCount,
			PaidRub:          row.PaidRub,
			PaidStars:        row.PaidStars,
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
