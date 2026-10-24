package handlers

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// AdminPaymentsHandler — эндпоинты /cabinet/api/admin/payments*.
//
// Раздел «Платежи» показывает все покупки (purchase) со всеми статусами
// (new/pending/paid/cancel) — в отличие от /admin/users/{id}/payments,
// который отдаёт только успешные оплаты одного клиента.
type AdminPaymentsHandler struct {
	purchases *database.PurchaseRepository
	tariffs   *database.TariffRepository
	promos    *database.PromoRepository
	checkouts *repository.CheckoutRepo
	customers *database.CustomerRepository
}

// NewAdminPayments — конструктор.
func NewAdminPayments(
	purchases *database.PurchaseRepository,
	tariffs *database.TariffRepository,
	promos *database.PromoRepository,
	checkouts *repository.CheckoutRepo,
	customers *database.CustomerRepository,
) *AdminPaymentsHandler {
	return &AdminPaymentsHandler{
		purchases: purchases,
		tariffs:   tariffs,
		promos:    promos,
		checkouts: checkouts,
		customers: customers,
	}
}

// --- DTOs -------------------------------------------------------------------

type adminPaymentListItemDTO struct {
	ID             int64   `json:"id"`
	CustomerID     int64   `json:"customer_id"`
	TelegramID     int64   `json:"telegram_id,string"`
	Username       *string `json:"telegram_username"`
	PanelLogin     *string `json:"panel_login,omitempty"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Month          int     `json:"month"`
	ExtraHwid      int     `json:"extra_hwid"`
	InvoiceType    string  `json:"invoice_type"`
	PurchaseKind   string  `json:"purchase_kind"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	PaidAt         *string `json:"paid_at"`
	TariffID       *int64  `json:"tariff_id,omitempty"`
	TariffName     *string `json:"tariff_name,omitempty"`
	TariffLocation *string `json:"tariff_location_name,omitempty"`
	PromoCodeID    *int64  `json:"promo_code_id,omitempty"`
	PromoCode      *string `json:"promo_code,omitempty"`
}

type adminPaymentDetailDTO struct {
	adminPaymentListItemDTO
	DiscountPercent  *int    `json:"discount_percent,omitempty"`
	IsEarlyDowngrade bool    `json:"is_early_downgrade"`
	ExpireAt         *string `json:"expire_at,omitempty"`
	CryptoInvoiceID  *int64  `json:"crypto_invoice_id,omitempty"`
	CryptoInvoiceURL *string `json:"crypto_invoice_url,omitempty"`
	YookasaID        *string `json:"yookasa_id,omitempty"`
	YookasaURL       *string `json:"yookasa_url,omitempty"`
	PlategaID        *string `json:"platega_id,omitempty"`
	PlategaURL       *string `json:"platega_url,omitempty"`
	HeleketID        *string `json:"heleket_id,omitempty"`
	HeleketURL       *string `json:"heleket_url,omitempty"`
	// ProviderTxnID — унифицированный внешний ID (yookasa/platega/heleket/crypto — что применимо).
	ProviderTxnID *string `json:"provider_txn_id,omitempty"`
	// IdempotencyKey/CheckoutProvider — заполнены, только если платёж создан через web-кабинет
	// (см. cabinet_checkout); для платежей из Telegram-бота остаются пустыми.
	IdempotencyKey   *string `json:"idempotency_key,omitempty"`
	CheckoutProvider *string `json:"checkout_provider,omitempty"`
}

func displayTariffName(t *database.Tariff) string {
	if t == nil {
		return ""
	}
	if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
		return strings.TrimSpace(*t.Name)
	}
	return t.Slug
}

// mapRowsToDTOs пакетно подтягивает названия тарифов, коды промокодов и логины
// web-клиентов в панели — без N+1 на страницу списка.
func (h *AdminPaymentsHandler) mapRowsToDTOs(ctx context.Context, rows []database.AdminPurchaseRow) []adminPaymentListItemDTO {
	tariffNames := make(map[int64]string)
	promoCodes := make(map[int64]string)
	webOnlyIDs := make([]int64, 0)

	for i := range rows {
		r := &rows[i]
		if r.TariffID != nil {
			tariffNames[*r.TariffID] = ""
		}
		if r.PromoCodeID != nil {
			promoCodes[*r.PromoCodeID] = ""
		}
		if r.CustomerIsWebOnly {
			webOnlyIDs = append(webOnlyIDs, r.CustomerID)
		}
	}

	if h.tariffs != nil {
		for id := range tariffNames {
			t, err := h.tariffs.GetByID(ctx, id)
			if err != nil {
				slog.Warn("admin payments: tariff lookup failed", "tariff_id", id, "error", err.Error())
				continue
			}
			tariffNames[id] = displayTariffName(t)
		}
	}
	if h.promos != nil {
		for id := range promoCodes {
			p, err := h.promos.FindByID(ctx, id)
			if err != nil {
				slog.Warn("admin payments: promo lookup failed", "promo_id", id, "error", err.Error())
				continue
			}
			if p != nil {
				promoCodes[id] = p.Code
			}
		}
	}
	var panelLogins map[int64]string
	if h.customers != nil && len(webOnlyIDs) > 0 {
		emails, err := h.customers.CabinetAccountEmailsByCustomerIDs(ctx, webOnlyIDs)
		if err != nil {
			slog.Warn("admin payments: panel logins lookup failed", "error", err.Error())
		} else {
			panelLogins = make(map[int64]string, len(webOnlyIDs))
			for _, id := range webOnlyIDs {
				if login := utils.PanelLoginForCustomer(id, emails[id]); login != "" {
					panelLogins[id] = login
				}
			}
		}
	}

	items := make([]adminPaymentListItemDTO, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		dto := adminPaymentListItemDTO{
			ID:           r.ID,
			CustomerID:   r.CustomerID,
			TelegramID:   r.CustomerTelegramID,
			Username:     r.CustomerTelegramUsername,
			Amount:       r.Amount,
			Currency:     r.Currency,
			Month:        r.Month,
			ExtraHwid:    r.ExtraHwid,
			InvoiceType:  string(r.InvoiceType),
			PurchaseKind: string(r.PurchaseKind),
			Status:       string(r.Status),
			CreatedAt:    r.CreatedAt.Format(time.RFC3339),
			TariffID:     r.TariffID,
			PromoCodeID:  r.PromoCodeID,
		}
		if r.PaidAt != nil {
			s := r.PaidAt.Format(time.RFC3339)
			dto.PaidAt = &s
		}
		if r.TariffID != nil {
			if name, ok := tariffNames[*r.TariffID]; ok && name != "" {
				dto.TariffName = &name
			}
		}
		if r.TariffLocationName != nil && strings.TrimSpace(*r.TariffLocationName) != "" {
			name := strings.TrimSpace(*r.TariffLocationName)
			dto.TariffLocation = &name
		}
		if r.PromoCodeID != nil {
			if code, ok := promoCodes[*r.PromoCodeID]; ok && code != "" {
				dto.PromoCode = &code
			}
		}
		if panelLogins != nil {
			if login, ok := panelLogins[r.CustomerID]; ok {
				dto.PanelLogin = &login
			}
		}
		items = append(items, dto)
	}
	return items
}

func resolveProviderTxnID(r database.AdminPurchaseRow) *string {
	switch {
	case r.YookasaID != nil:
		s := r.YookasaID.String()
		return &s
	case r.PlategaID != nil:
		return r.PlategaID
	case r.HeleketID != nil:
		return r.HeleketID
	case r.CryptoInvoiceID != nil:
		s := strconv.FormatInt(*r.CryptoInvoiceID, 10)
		return &s
	default:
		return nil
	}
}

func (h *AdminPaymentsHandler) mapRowToDetailDTO(ctx context.Context, row database.AdminPurchaseRow) adminPaymentDetailDTO {
	base := h.mapRowsToDTOs(ctx, []database.AdminPurchaseRow{row})[0]
	dto := adminPaymentDetailDTO{
		adminPaymentListItemDTO: base,
		DiscountPercent:         row.DiscountPercentApplied,
		IsEarlyDowngrade:        row.IsEarlyDowngrade,
		CryptoInvoiceID:         row.CryptoInvoiceID,
		CryptoInvoiceURL:        row.CryptoInvoiceLink,
		YookasaURL:              row.YookasaURL,
		PlategaID:               row.PlategaID,
		PlategaURL:              row.PlategaURL,
		HeleketID:               row.HeleketID,
		HeleketURL:              row.HeleketURL,
		ProviderTxnID:           resolveProviderTxnID(row),
	}
	if row.ExpireAt != nil {
		s := row.ExpireAt.Format(time.RFC3339)
		dto.ExpireAt = &s
	}
	if row.YookasaID != nil {
		s := row.YookasaID.String()
		dto.YookasaID = &s
	}

	if h.checkouts != nil {
		checkout, err := h.checkouts.FindByPurchaseID(ctx, row.ID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			slog.Warn("admin payments: checkout lookup failed", "purchase_id", row.ID, "error", err.Error())
		} else if checkout != nil {
			key := checkout.IdempotencyKey
			provider := checkout.Provider
			dto.IdempotencyKey = &key
			dto.CheckoutProvider = &provider
		}
	}
	return dto
}

// --- Filter parsing -----------------------------------------------------------

func parseAdminPaymentFilter(r *http.Request) database.AdminPurchaseFilter {
	q := r.URL.Query()
	filter := database.AdminPurchaseFilter{Search: strings.TrimSpace(q.Get("q"))}
	switch strings.ToLower(strings.TrimSpace(q.Get("status"))) {
	case "new":
		filter.Status = database.PurchaseStatusNew
	case "pending":
		filter.Status = database.PurchaseStatusPending
	case "paid":
		filter.Status = database.PurchaseStatusPaid
	case "cancel", "cancelled", "canceled":
		filter.Status = database.PurchaseStatusCancel
	default:
		filter.Status = ""
	}
	return filter
}

// --- List ---------------------------------------------------------------------

type adminPaymentsListResp struct {
	Items []adminPaymentListItemDTO `json:"items"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Limit int                       `json:"limit"`
}

// List — GET /cabinet/api/admin/payments?status=&q=&page=&limit=.
func (h *AdminPaymentsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	filter := parseAdminPaymentFilter(r)
	ctx := r.Context()

	total, err := h.purchases.CountForAdmin(ctx, filter)
	if err != nil {
		slog.Error("admin payments: count failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	rows, err := h.purchases.ListForAdmin(ctx, filter, limit, offset)
	if err != nil {
		slog.Error("admin payments: list failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, adminPaymentsListResp{
		Items: h.mapRowsToDTOs(ctx, rows),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// --- Get ------------------------------------------------------------------

// Get — GET /cabinet/api/admin/payments/{id}.
func (h *AdminPaymentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := adminPaymentsExtractID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	row, err := h.purchases.GetForAdmin(ctx, id)
	if err != nil {
		slog.Error("admin payments: get failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if row == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, h.mapRowToDetailDTO(ctx, *row))
}

// --- Export (CSV) -----------------------------------------------------------

// adminPaymentsExportMaxRows — верхняя граница строк одной выгрузки CSV.
const adminPaymentsExportMaxRows = 20000

var invoiceTypeCsvLabels = map[database.InvoiceType]string{
	database.InvoiceTypeCrypto:           "Crypto Bot",
	database.InvoiceTypeYookasa:          "ЮKassa",
	database.InvoiceTypeTelegram:         "Telegram Stars",
	database.InvoiceTypeTribute:          "Tribute",
	database.InvoiceTypePlategaSBP:       "Platega СБП",
	database.InvoiceTypePlategaCards:     "Platega карты",
	database.InvoiceTypePlategaAcquiring: "Platega эквайринг",
	database.InvoiceTypePlategaWorldwide: "Platega worldwide",
	database.InvoiceTypePlategaCrypto:    "Platega crypto",
	database.InvoiceTypeHeleket:          "Heleket",
}

func invoiceTypeCsvLabel(t string) string {
	if label, ok := invoiceTypeCsvLabels[database.InvoiceType(t)]; ok {
		return label
	}
	return t
}

// Export — GET /cabinet/api/admin/payments/export?status=&q= — CSV с учётом текущего фильтра
// (без пагинации, до adminPaymentsExportMaxRows строк).
func (h *AdminPaymentsHandler) Export(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	filter := parseAdminPaymentFilter(r)

	rows, err := h.purchases.ListForAdmin(ctx, filter, adminPaymentsExportMaxRows, 0)
	if err != nil {
		slog.Error("admin payments: export list failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	items := h.mapRowsToDTOs(ctx, rows)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="payments_%s.csv"`, time.Now().UTC().Format("20060102_150405")))
	w.WriteHeader(http.StatusOK)

	// BOM — чтобы Excel корректно определил UTF-8 с кириллицей.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{
		"ID", "Пользователь", "Telegram ID", "Сумма", "Валюта", "Провайдер",
		"Период (мес.)", "Доп. устройства", "Статус", "Дата создания", "Дата оплаты",
		"Тариф", "Промокод",
	})
	for _, it := range items {
		username := ""
		switch {
		case it.Username != nil:
			username = "@" + *it.Username
		case it.PanelLogin != nil:
			username = *it.PanelLogin
		}
		tariff := ""
		if it.TariffName != nil {
			tariff = *it.TariffName
		}
		promo := ""
		if it.PromoCode != nil {
			promo = *it.PromoCode
		}
		paidAt := ""
		if it.PaidAt != nil {
			paidAt = *it.PaidAt
		}
		_ = cw.Write([]string{
			strconv.FormatInt(it.ID, 10),
			username,
			strconv.FormatInt(it.TelegramID, 10),
			strconv.FormatFloat(it.Amount, 'f', 2, 64),
			it.Currency,
			invoiceTypeCsvLabel(it.InvoiceType),
			strconv.Itoa(it.Month),
			strconv.Itoa(it.ExtraHwid),
			it.Status,
			it.CreatedAt,
			paidAt,
			tariff,
			promo,
		})
	}
	cw.Flush()
}

// HandleByID dispatches /cabinet/api/admin/payments/{id or export} requests.
func (h *AdminPaymentsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/export") {
		h.Export(w, r)
		return
	}
	h.Get(w, r)
}

func adminPaymentsExtractID(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "payments" && i+1 < len(parts) {
			id, err := strconv.ParseInt(parts[i+1], 10, 64)
			if err == nil {
				return id, true
			}
		}
	}
	return 0, false
}
