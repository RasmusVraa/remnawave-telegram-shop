package handlers

import (
	"testing"
	"time"
)

func TestValidateCreatePromo(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	i := func(v int) *int { return &v }
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	ok := []createPromoReq{
		{Code: "SALE10", Type: "discount", DiscountPercent: i(10), DiscountMaxSubscriptionPaymentsPerCustomer: 1},
		{Code: "СКИДКА-2026", Type: "discount", DiscountPercent: i(100)},
		{Code: "DAYS_30", Type: "subscription_days", SubscriptionDays: i(30), MaxUses: i(5), ValidUntil: &future},
		{Code: "TRY", Type: "trial", TrialDays: i(3)},
		{Code: "DEV", Type: "extra_hwid", ExtraHwidDelta: i(1)},
	}
	for _, r := range ok {
		if msg := validateCreatePromo(&r, now); msg != "" {
			t.Errorf("%+v rejected: %s", r, msg)
		}
	}

	bad := map[string]createPromoReq{
		"invalid promo code":                         {Code: "A", Type: "trial", TrialDays: i(1)},
		"invalid promo code ":                        {Code: "__SYSTEM__", Type: "trial", TrialDays: i(1)},
		"invalid promo code  ":                       {Code: "SALE 10", Type: "trial", TrialDays: i(1)},
		"invalid promo type":                         {Code: "X1", Type: "bonus"},
		"subscription days must be positive":         {Code: "X1", Type: "subscription_days", SubscriptionDays: i(0)},
		"trial days must be positive":                {Code: "X1", Type: "trial"},
		"extra hwid delta must be positive":          {Code: "X1", Type: "extra_hwid", ExtraHwidDelta: i(-1)},
		"discount percent must be 1-100":             {Code: "X1", Type: "discount", DiscountPercent: i(150)},
		"discount percent must be 1-100 ":            {Code: "X1", Type: "discount"},
		"discount ttl must not be negative":          {Code: "X1", Type: "discount", DiscountPercent: i(5), DiscountTTLHours: i(-1)},
		"discount max payments must not be negative": {Code: "X1", Type: "discount", DiscountPercent: i(5), DiscountMaxSubscriptionPaymentsPerCustomer: -1},
		"max uses must be positive":                  {Code: "X1", Type: "trial", TrialDays: i(1), MaxUses: i(0)},
		"valid until is in the past":                 {Code: "X1", Type: "trial", TrialDays: i(1), ValidUntil: &past},
	}
	for want, r := range bad {
		if got := validateCreatePromo(&r, now); got != trimSpaces(want) {
			t.Errorf("%+v: got %q, want %q", r, got, trimSpaces(want))
		}
	}
}

func trimSpaces(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
