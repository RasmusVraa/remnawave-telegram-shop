package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
	"remnawave-tg-shop-bot/internal/cabinet/payments"
)

// PaymentsHandler — HTTP-адаптер над payments.CheckoutService.
// Ничего кроме парсинга запроса / ответа и маппинга ошибок.
type PaymentsHandler struct {
	svc *payments.CheckoutService
}

// NewPayments — конструктор.
func NewPayments(svc *payments.CheckoutService) *PaymentsHandler {
	return &PaymentsHandler{svc: svc}
}

type checkoutReq struct {
	Period   int    `json:"period"`
	TariffID *int64 `json:"tariff_id,omitempty"`
	Provider string `json:"provider"`
	// RenewExtraHwid — продлевать активные доп. HWID вместе с подпиской.
	RenewExtraHwid bool `json:"renew_extra_hwid"`
	// LocationID — выбранная площадка, если у тарифа заданы локации.
	LocationID *int64 `json:"location_id,omitempty"`
}

// Checkout — POST /cabinet/api/payments/checkout.
//
// Заголовок Idempotency-Key обязателен: повторный запрос с тем же значением
// не создаёт нового счёта, а возвращает ранее выданный payment_url.
func (h *PaymentsHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req checkoutReq
	if !decodeJSON(w, r, &req) {
		return
	}
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))

	result, err := h.svc.Create(r.Context(), claims.AccountID, payments.CreateRequest{
		Period:         req.Period,
		TariffID:       req.TariffID,
		Provider:       strings.ToLower(strings.TrimSpace(req.Provider)),
		IdempotencyKey: idemKey,
		RenewExtraHwid: req.RenewExtraHwid,
		LocationID:     req.LocationID,
	})
	if err != nil {
		writePaymentsErr(w, err, "checkout")
		return
	}

	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	} else {
		cabmetrics.RecordCheckoutStarted(result.Provider)
	}
	writeJSON(w, status, result)
}

// Preview — GET /cabinet/api/payments/preview?period=&tariff_id= (tariff_id в режиме tariffs).
//
// Сумма и сценарий совпадают с Create без создания счёта.
func (h *PaymentsHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	periodStr := strings.TrimSpace(r.URL.Query().Get("period"))
	period, err := strconv.Atoi(periodStr)
	if err != nil || period <= 0 {
		http.Error(w, "bad period", http.StatusBadRequest)
		return
	}
	var tariffID *int64
	if v := strings.TrimSpace(r.URL.Query().Get("tariff_id")); v != "" {
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil || id <= 0 {
			http.Error(w, "bad tariff_id", http.StatusBadRequest)
			return
		}
		tariffID = &id
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	renewExtraHwid := false
	if v := strings.TrimSpace(r.URL.Query().Get("renew_extra_hwid")); v != "" {
		b, berr := strconv.ParseBool(v)
		if berr != nil {
			http.Error(w, "bad renew_extra_hwid", http.StatusBadRequest)
			return
		}
		renewExtraHwid = b
	}

	ctx := payments.WithPreviewRenewExtraHwid(r.Context(), renewExtraHwid)
	result, err := h.svc.Preview(ctx, claims.AccountID, period, tariffID, provider)
	if err != nil {
		writePaymentsErr(w, err, "preview")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type hwidCheckoutReq struct {
	TargetLimit int    `json:"target_limit"`
	Provider    string `json:"provider"`
}

// HwidPreview — GET /cabinet/api/payments/hwid/preview?target_limit=&provider=.
func (h *PaymentsHandler) HwidPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tStr := strings.TrimSpace(r.URL.Query().Get("target_limit"))
	target, err := strconv.Atoi(tStr)
	if err != nil || target <= 0 {
		http.Error(w, "bad target_limit", http.StatusBadRequest)
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	result, err := h.svc.PreviewHwid(r.Context(), claims.AccountID, target, provider)
	if err != nil {
		writePaymentsErr(w, err, "hwid_preview")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// HwidCheckout — POST /cabinet/api/payments/hwid/checkout.
func (h *PaymentsHandler) HwidCheckout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req hwidCheckoutReq
	if !decodeJSON(w, r, &req) {
		return
	}
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	result, err := h.svc.CreateHwid(r.Context(), claims.AccountID, payments.HwidCreateRequest{
		TargetLimit:    req.TargetLimit,
		Provider:       req.Provider,
		IdempotencyKey: idemKey,
	})
	if err != nil {
		writePaymentsErr(w, err, "hwid_checkout")
		return
	}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	} else {
		cabmetrics.RecordCheckoutStarted(result.Provider)
	}
	writeJSON(w, status, result)
}

// Status — GET /cabinet/api/payments/{id}/status.
//
// Пытаемся максимально быть frontend-friendly: статус + payment_url (пока
// не оплачено) + subscription_link/expire_at (когда paid).
//
// На ServeMux роут зарегистрирован префиксом /cabinet/api/payments/ и должен
// отдавать 405/404 для всего, что не GET /…/status.
func (h *PaymentsHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := parsePaymentStatusID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.svc.GetStatus(r.Context(), claims.AccountID, id)
	if err != nil {
		writePaymentsErr(w, err, "status")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ============================================================================
// helpers
// ============================================================================

// parsePaymentStatusID достаёт :id из пути /cabinet/api/payments/{id}/status.
// Если когда-нибудь мы переедем на http.ServeMux с path-params (Go 1.22),
// эту функцию можно будет выкинуть и заменить на r.PathValue("id").
func parsePaymentStatusID(path string) (int64, error) {
	const prefix = "/cabinet/api/payments/"
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return 0, errors.New("invalid path")
	}
	rest = strings.TrimSuffix(rest, "/status")
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.ContainsRune(rest, '/') {
		return 0, errors.New("invalid checkout id")
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid checkout id")
	}
	return id, nil
}

// writePaymentsErr маппит sentinel-ошибки сервиса в HTTP-статусы.
// Стараемся не подмешивать сюда лишних классов ошибок — всё, что не sentinel,
// логируется и превращается в 500 без деталей.
func writePaymentsErr(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, bootstrap.ErrAccountGone):
		// Аккаунт удалён, а access-JWT ещё жив: 401 заставит фронт разлогиниться.
		handleAccountGone(w, err, op, 0)
	case errors.Is(err, payments.ErrInvalidInput),
		errors.Is(err, payments.ErrProviderDisabled):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, payments.ErrCheckoutNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, payments.ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		slog.Error("cabinet payments handler error", "op", op, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
