package payment

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/database"
)

// Тип события оплаты решает, в какой счётчик и строку ленты она попадёт:
// «купил» и «продлил» различаются только по тому, шла ли подписка на момент оплаты.
func TestPaidEventType(t *testing.T) {
	paid := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	future := paid.Add(24 * time.Hour)
	past := paid.Add(-time.Hour)
	sub := database.PurchaseKindSubscription

	cases := []struct {
		name   string
		kind   database.PurchaseKind
		paidAt *time.Time
		before *time.Time
		want   string
	}{
		{"first purchase", sub, &paid, nil, adminevents.PaymentSubscriptionNew},
		{"expired, bought again", sub, &paid, &past, adminevents.PaymentSubscriptionNew},
		{"active, renewed", sub, &paid, &future, adminevents.PaymentSubscriptionRenew},
		{"expires exactly at payment", sub, &paid, &paid, adminevents.PaymentSubscriptionNew},
		{"empty kind is subscription", "", &paid, &future, adminevents.PaymentSubscriptionRenew},
		{"upgrade", database.PurchaseKindTariffUpgrade, &paid, &future, adminevents.PaymentTariffUpgrade},
		{"extra hwid", database.PurchaseKindExtraHwid, &paid, &future, adminevents.PaymentExtraHwid},
		{"extra traffic", database.PurchaseKindExtraTraffic, &paid, nil, adminevents.PaymentExtraTraffic},
	}
	for _, c := range cases {
		p := &database.Purchase{PurchaseKind: c.kind, PaidAt: c.paidAt}
		if got := paidEventType(p, c.before); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	// PaidAt не заполнен — сравнение с «сейчас».
	far := time.Now().Add(48 * time.Hour)
	if got := paidEventType(&database.Purchase{PurchaseKind: sub}, &far); got != adminevents.PaymentSubscriptionRenew {
		t.Errorf("nil PaidAt with active sub: %s", got)
	}
	for _, typ := range []string{
		adminevents.PaymentSubscriptionNew, adminevents.PaymentSubscriptionRenew,
		adminevents.PaymentTariffUpgrade, adminevents.PaymentExtraHwid, adminevents.PaymentExtraTraffic,
	} {
		found := false
		for _, p := range adminevents.PaidTypes {
			found = found || p == typ
		}
		if !found {
			t.Errorf("%s is not counted in PaidTypes", typ)
		}
	}
}
