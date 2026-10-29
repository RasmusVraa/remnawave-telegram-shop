package remnawave

import (
	"context"
	"errors"
	"testing"
)

func pinLookup(limit int, ok bool, err error) HwidLimitPinLookup {
	return func(context.Context, int64, int64) (int, bool, error) { return limit, ok, err }
}

// Закреплённый админом лимит подменяет любой расчётный: так ручное число
// переживает оплату, докупку, промокод и смену тарифа.
func TestPinnedHwidLimit(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		lookup HwidLimitPinLookup
		want   int
	}{
		{"без источника — расчётный", nil, 2},
		{"не закреплён — расчётный", pinLookup(0, false, nil), 2},
		{"закреплён — закреплённый", pinLookup(5, true, nil), 5},
		{"закреплён меньше расчётного — всё равно закреплённый", pinLookup(1, true, nil), 1},
		{"ошибка БД — расчётный, запись не падает", pinLookup(5, true, errors.New("db down")), 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Client{hwidPin: c.lookup}
			if got := r.pinnedHwidLimit(ctx, 1, 100, 2); got != c.want {
				t.Fatalf("pinnedHwidLimit = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

// Запросы, которые лимит не трогают (продление без триала), при закреплении
// всё равно получают его — заново созданный профиль панели не уйдёт с
// дефолтом панели.
func TestPinHwidLimitField(t *testing.T) {
	ctx := context.Background()

	t.Run("nil остаётся nil без закрепления", func(t *testing.T) {
		r := &Client{hwidPin: pinLookup(0, false, nil)}
		var field *int
		r.pinHwidLimitField(ctx, 1, 100, &field)
		if field != nil {
			t.Fatalf("лимит появился без закрепления: %d", *field)
		}
	})

	t.Run("nil получает закреплённый", func(t *testing.T) {
		r := &Client{hwidPin: pinLookup(5, true, nil)}
		var field *int
		r.pinHwidLimitField(ctx, 1, 100, &field)
		if field == nil || *field != 5 {
			t.Fatalf("ожидался закреплённый 5, получено %v", field)
		}
	})

	t.Run("база тарифа заменяется закреплённым", func(t *testing.T) {
		r := &Client{hwidPin: pinLookup(5, true, nil)}
		base := 2
		field := &base
		r.pinHwidLimitField(ctx, 1, 100, &field)
		if *field != 5 {
			t.Fatalf("ожидался 5, получено %d", *field)
		}
		if base != 2 {
			t.Fatal("исходное значение вызывающего изменено")
		}
	})

	t.Run("база тарифа остаётся без закрепления", func(t *testing.T) {
		r := &Client{hwidPin: pinLookup(0, false, nil)}
		base := 2
		field := &base
		r.pinHwidLimitField(ctx, 1, 100, &field)
		if *field != 2 {
			t.Fatalf("ожидался 2, получено %d", *field)
		}
	})
}
