package main

import "testing"

func TestPaymentMonths(t *testing.T) {
	cases := []struct {
		desc   string
		months int
		days   int
		ok     bool
	}{
		{"Покупка тарифа ⚡️ Базовый на 30 дней", 1, 30, true},
		{"Покупка тарифа '🔥 Премиум' на 90 дней", 3, 90, true},
		{"Автопродление подписки на 180 дней", 6, 180, true},
		{"Покупка тарифа ⚡️ Базовый на 360 дней", 12, 360, true},
		{"Переход на тариф '🔥 Премиум' (доплата за 12 дней)", 1, 12, true},
		{"Покупка 2 доп. устройств", 0, 0, false},
		{"Переход на тариф '🔥 Премиум'", 0, 0, false},
	}
	for _, c := range cases {
		m, d, ok := paymentMonths(c.desc)
		if ok != c.ok || m != c.months || d != c.days {
			t.Errorf("%q: got (%d,%d,%v), want (%d,%d,%v)", c.desc, m, d, ok, c.months, c.days, c.ok)
		}
	}
}
