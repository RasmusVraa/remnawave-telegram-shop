package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
)

type PromoCodesHandler struct {
	boot      *bootstrap.CustomerBootstrap
	customers *database.CustomerRepository
	promo     *promo.Service
}

func NewPromoCodes(
	boot *bootstrap.CustomerBootstrap,
	customers *database.CustomerRepository,
	links *repository.AccountCustomerLinkRepo,
	promoSvc *promo.Service,
) *PromoCodesHandler {
	_ = links
	return &PromoCodesHandler{boot: boot, customers: customers, promo: promoSvc}
}

type promoApplyReq struct {
	Code string `json:"code"`
}

type promoApplyResp struct {
	Applied bool   `json:"applied"`
	Type    string `json:"type,omitempty"`

	SubscriptionDays      int  `json:"subscription_days,omitempty"`
	TrialDays             int  `json:"trial_days,omitempty"`
	ExtraHwidDelta        int  `json:"extra_hwid_delta,omitempty"`
	DiscountPercent       int  `json:"discount_percent,omitempty"`
	TrialSkippedActiveSub bool `json:"trial_skipped_active_sub,omitempty"`
}

type pendingDiscountDTO struct {
	PromoCodeID                   int    `json:"promo_code_id"`
	Percent                       int    `json:"percent"`
	UntilFirstPurchase            bool   `json:"until_first_purchase"`
	SubscriptionPaymentsRemaining int    `json:"subscription_payments_remaining"`
	ExpiresAt                     string `json:"expires_at,omitempty"`
}

type promoStateResp struct {
	HasPendingDiscount bool                `json:"has_pending_discount"`
	PendingDiscount    *pendingDiscountDTO `json:"pending_discount,omitempty"`
}

func promoErrorKindSlug(kind promo.ActivateErrKind) string {
	switch kind {
	case promo.ActivateErrAlreadyUsed:
		return "already_used"
	case promo.ActivateErrInactive:
		return "inactive"
	case promo.ActivateErrNotFound:
		return "not_found"
	case promo.ActivateErrPendingDiscount:
		return "pending_discount"
	case promo.ActivateErrTariffMismatch:
		return "tariff_mismatch"
	case promo.ActivateErrHwidPinned:
		return "hwid_pinned"
	default:
		return "generic"
	}
}

func (h *PromoCodesHandler) loadCustomer(ctx context.Context, accountID int64) (*database.Customer, error) {
	if h.boot == nil || h.customers == nil {
		return nil, errors.New("promocodes not initialized")
	}
	link, err := h.boot.EnsureForAccount(ctx, accountID, "")
	if err != nil || link == nil {
		return nil, err
	}
	return h.customers.FindById(ctx, link.CustomerID)
}

// GetState — GET /cabinet/api/promocodes/state.
func (h *PromoCodesHandler) GetState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	customer, err := h.loadCustomer(r.Context(), claims.AccountID)
	if err != nil || customer == nil {
		if handleAccountGone(w, err, "promocodes.state", claims.AccountID) {
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := promoStateResp{}
	if h.promo != nil && h.promo.PromoRepo != nil {
		d, derr := h.promo.PromoRepo.GetPendingDiscountByCustomerID(r.Context(), customer.ID)
		if derr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if d != nil {
			resp.HasPendingDiscount = true
			dto := &pendingDiscountDTO{
				PromoCodeID:                   int(d.PromoCodeID),
				Percent:                       d.Percent,
				UntilFirstPurchase:            d.UntilFirstPurchase,
				SubscriptionPaymentsRemaining: d.SubscriptionPaymentsRemaining,
			}
			if d.ExpiresAt != nil {
				dto.ExpiresAt = d.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			resp.PendingDiscount = dto
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// Apply — POST /cabinet/api/promocodes/apply.
func (h *PromoCodesHandler) Apply(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req promoApplyReq
	if !decodeJSON(w, r, &req) {
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		http.Error(w, "promo code is required", http.StatusBadRequest)
		return
	}
	customer, err := h.loadCustomer(r.Context(), claims.AccountID)
	if err != nil || customer == nil {
		if handleAccountGone(w, err, "promocodes.activate", claims.AccountID) {
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	username := ""
	if customer.TelegramUsername != nil {
		username = strings.TrimSpace(*customer.TelegramUsername)
	}
	ctx := context.WithValue(r.Context(), remnawave.CtxKeyUsername, username)
	res, err := h.promo.Activate(ctx, customer.TelegramID, username, customer, code)
	if err != nil {
		if promo.IsPromoValidationErr(err) {
			kind := promo.ClassifyActivateError(err)
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":      "promo_validation_failed",
				"error_kind": promoErrorKindSlug(kind),
			})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := promoApplyResp{
		Applied:               true,
		Type:                  res.Type,
		SubscriptionDays:      res.SubscriptionDays,
		TrialDays:             res.TrialDays,
		ExtraHwidDelta:        res.ExtraHwidDelta,
		DiscountPercent:       res.DiscountPercent,
		TrialSkippedActiveSub: res.TrialSkippedActiveSub,
	}
	writeJSON(w, http.StatusOK, out)
}
