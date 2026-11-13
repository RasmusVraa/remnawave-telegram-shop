//go:build integration

package database

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Популярный тариф всегда один: отметка на новом тарифе снимает её с прежнего,
// а база не пускает вторую отметку в обход кода (уникальный частичный индекс).
func TestTariffPopular_single(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewTariffRepository(pool)
	ctx := context.Background()

	// Тестовая база общая: чужая отметка мешала бы проверке, поэтому снимаем её и возвращаем после.
	var prevPopular *int64
	_ = pool.QueryRow(ctx, `SELECT id FROM tariff WHERE is_popular`).Scan(&prevPopular)
	_, _ = pool.Exec(ctx, `UPDATE tariff SET is_popular = false WHERE is_popular`)

	suffix := time.Now().Format("150405.000000000")
	newTariff := func(name string, popular bool) int64 {
		t.Helper()
		id, err := repo.CreateWithPrices(ctx, &Tariff{
			Slug: fmt.Sprintf("pop-%s-%s", name, suffix), Name: &name, IsActive: true, DeviceLimit: 1,
			TrafficLimitResetStrategy: "MONTH", IsPopular: popular,
		}, [4]int{100, 0, 0, 0}, [4]*int{})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return id
	}
	a := newTariff("a", true)
	b := newTariff("b", false)
	t.Cleanup(func() {
		_ = repo.DeleteTariff(ctx, a)
		_ = repo.DeleteTariff(ctx, b)
		if prevPopular != nil {
			_, _ = pool.Exec(ctx, `UPDATE tariff SET is_popular = true WHERE id = $1`, *prevPopular)
		}
	})

	popular := func() []int64 {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT id FROM tariff WHERE is_popular ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}

	if got := popular(); len(got) != 1 || got[0] != a {
		t.Fatalf("после создания популярным должен быть только %d, есть %v", a, got)
	}

	if err := repo.UpdateTariff(ctx, b, map[string]interface{}{"is_popular": true}); err != nil {
		t.Fatalf("update b popular: %v", err)
	}
	if got := popular(); len(got) != 1 || got[0] != b {
		t.Fatalf("отметка должна переехать на %d, есть %v", b, got)
	}
	tb, err := repo.GetByID(ctx, b)
	if err != nil || tb == nil || !tb.IsPopular {
		t.Fatalf("GetByID(b).IsPopular = false, err %v", err)
	}

	// Мимо UpdateTariff вторая отметка не встаёт — держит индекс.
	if _, err := pool.Exec(ctx, `UPDATE tariff SET is_popular = true WHERE id = $1`, a); err == nil {
		t.Fatal("база пустила второй популярный тариф")
	}

	if err := repo.UpdateTariff(ctx, b, map[string]interface{}{"is_popular": false}); err != nil {
		t.Fatalf("update b not popular: %v", err)
	}
	if got := popular(); len(got) != 0 {
		t.Fatalf("после снятия отметки популярных быть не должно, есть %v", got)
	}
}
