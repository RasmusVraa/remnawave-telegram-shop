package promo

import (
	"strings"

	"remnawave-tg-shop-bot/internal/database"
)

// ActivateResult describes a successful promo activation for user-facing messages.
type ActivateResult struct {
	Type                  string
	SubscriptionDays      int
	TrialDays             int
	ExtraHwidDelta        int
	DiscountPercent       int
	TrialSkippedActiveSub bool
}

// ActivateErrKind classifies validation failures for differentiated UX.
type ActivateErrKind int

const (
	ActivateErrGeneric ActivateErrKind = iota
	ActivateErrAlreadyUsed
	ActivateErrInactive
	ActivateErrNotFound
	ActivateErrPendingDiscount
	ActivateErrTariffMismatch
	ActivateErrHwidPinned
)

// ClassifyActivateError maps validation errors to UI kind (generic = neutral fail + back).
func ClassifyActivateError(err error) ActivateErrKind {
	if err == nil || !IsPromoValidationErr(err) {
		return ActivateErrGeneric
	}
	reason := extractValidationReason(err)
	switch reason {
	case "already redeemed":
		return ActivateErrAlreadyUsed
	case "inactive", "expired", "max uses":
		return ActivateErrInactive
	case "not found":
		return ActivateErrNotFound
	case "pending discount":
		return ActivateErrPendingDiscount
	case "tariff mismatch":
		return ActivateErrTariffMismatch
	case "hwid pinned":
		return ActivateErrHwidPinned
	default:
		return ActivateErrGeneric
	}
}

func extractValidationReason(err error) string {
	msg := err.Error()
	const prefix = "promo validation failed: "
	if i := strings.Index(msg, prefix); i >= 0 {
		return strings.TrimSpace(msg[i+len(prefix):])
	}
	return ""
}

// Type const shortcuts for handlers.
const (
	ResultTypeSubscriptionDays = database.PromoTypeSubscriptionDays
	ResultTypeTrial            = database.PromoTypeTrial
	ResultTypeExtraHwid        = database.PromoTypeExtraHwid
	ResultTypeDiscount         = database.PromoTypeDiscount
)
