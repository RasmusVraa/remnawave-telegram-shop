package service

import (
	"math"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// Пока у клиента лежит активная скидка, колесо не может выбить ещё одну.
func TestPickRewardTypeNoDiscountWhenActive(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()
	for i := 0; i < 5000; i++ {
		rt, err := svc.pickRewardType(cfg, false, true)
		if err != nil {
			t.Fatal(err)
		}
		if isDiscountReward(rt) {
			t.Fatalf("discount dropped for a customer with an active discount: %s", rt)
		}
	}
}

// Без активной скидки скидки выпадают, как раньше.
func TestPickRewardTypeDiscountPossibleWithoutActive(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		rt, err := svc.pickRewardType(cfg, false, false)
		if err != nil {
			t.Fatal(err)
		}
		seen[rt] = true
	}
	if !seen["discount_3"] || !seen["discount_5"] {
		t.Errorf("discounts must stay on the wheel for a customer without one: %v", seen)
	}
}

// Остальные призы делят шансы в прежних пропорциях: скидки просто вычеркнуты из таблицы.
func TestPickRewardTypeProportionsKeptWithoutDiscounts(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()
	const n = 200000
	got := map[string]int{}
	for i := 0; i < n; i++ {
		rt, err := svc.pickRewardType(cfg, false, true)
		if err != nil {
			t.Fatal(err)
		}
		got[rt]++
	}
	// вес без скидок: 478+355+25+15+10+5+2+10 = 900
	const total = 900.0
	for rt, w := range map[string]float64{"micro": 478, "xp": 355, "days_3": 25, "free_spin": 10} {
		want := w / total
		have := float64(got[rt]) / n
		if math.Abs(have-want) > 0.01 {
			t.Errorf("%s share = %.4f, want about %.4f", rt, have, want)
		}
	}
}

// Если на колесе остались только скидки, спин не должен падать молча на скидку.
func TestPickRewardTypeOnlyDiscountsConfigured(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()
	cfg.WeightMicro, cfg.WeightXP, cfg.WeightDays3, cfg.WeightDays5 = 0, 0, 0, 0
	cfg.WeightDays7, cfg.WeightDays15, cfg.WeightDays30, cfg.WeightFreeSpin = 0, 0, 0, 0
	rt, err := svc.pickRewardType(cfg, false, true)
	if err == nil && isDiscountReward(rt) {
		t.Fatalf("got %s with an active discount", rt)
	}
}

func TestPendingDiscountActive(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		pd   *database.PendingDiscount
		want bool
	}{
		{"нет строки", nil, false},
		{"до первой оплаты", &database.PendingDiscount{UntilFirstPurchase: true}, true},
		{"до первой оплаты, срок в прошлом не важен", &database.PendingDiscount{UntilFirstPurchase: true, ExpiresAt: &past}, true},
		{"без срока", &database.PendingDiscount{}, true},
		{"срок впереди", &database.PendingDiscount{ExpiresAt: &future}, true},
		{"срок вышел", &database.PendingDiscount{ExpiresAt: &past}, false},
	}
	for _, c := range cases {
		if got := pendingDiscountActive(c.pd, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
