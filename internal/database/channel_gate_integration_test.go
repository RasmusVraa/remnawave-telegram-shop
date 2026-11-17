//go:build integration

package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Отметки «увидел экран / прошёл / попыток» и подсчёт новых и старых клиентов.
func TestChannelGateMarksAndStats(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewCustomerRepository(pool)
	stats := NewStatsRepository(pool)

	from := time.Now().UTC().Add(-72 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	base, err := stats.ChannelGateStats(ctx, from, to)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	// A — новый, увидел экран только что: «ещё думает».
	a := makeCustomer(t, pool)
	// B — новый, увидел 2 дня назад, нажимал кнопку дважды, не подписался: «ушёл».
	b := makeCustomer(t, pool)
	// C — новый, увидел 2 дня назад и подписался.
	c := makeCustomer(t, pool)
	// D — старый (зарегистрирован давно), увидел экран 2 дня назад, не подписался.
	d := makeCustomer(t, pool)
	// E — прошёл молча, экрана не видел: ни в какие цифры не попадает.
	e := makeCustomer(t, pool)

	for _, id := range []int64{a.ID, b.ID, c.ID, d.ID} {
		if err := repo.MarkChannelGateShown(ctx, id); err != nil {
			t.Fatal(err)
		}
		// Повторный показ не двигает дату.
		if err := repo.MarkChannelGateShown(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.MarkChannelGatePassed(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	var ePassed *time.Time
	if err := pool.QueryRow(ctx, `SELECT channel_gate_passed_at FROM customer WHERE id=$1`, e.ID).Scan(&ePassed); err != nil || ePassed != nil {
		t.Fatalf("«прошёл» без показа экрана не должен записываться: %v %v", ePassed, err)
	}

	for _, id := range []int64{b.ID, b.ID} {
		if err := repo.MarkChannelGateAttempt(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.MarkChannelGatePassed(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	// Первое «прошёл» не перезаписывается следующим.
	if err := repo.MarkChannelGatePassed(ctx, c.ID); err != nil {
		t.Fatal(err)
	}

	// Состарим показы: B, C, D — два дня назад. D зарегистрирован за 10 дней до показа.
	if _, err := pool.Exec(ctx, `
		UPDATE customer SET channel_gate_shown_at = now() - interval '2 days',
		       created_at = now() - interval '2 days' - interval '10 minutes'
		WHERE id = ANY($1)`, []int64{b.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE customer SET channel_gate_shown_at = now() - interval '2 days',
		       created_at = now() - interval '12 days'
		WHERE id = $1`, d.ID); err != nil {
		t.Fatal(err)
	}

	got, err := stats.ChannelGateStats(ctx, from, to)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	check := func(name string, got, base, want int64) {
		t.Helper()
		if got-base != want {
			t.Errorf("%s: прирост %d, ожидали %d", name, got-base, want)
		}
	}
	check("Shown (новые)", got.Shown, base.Shown, 3)              // A, B, C
	check("Passed", got.Passed, base.Passed, 1)                   // C
	check("NotPassed", got.NotPassed, base.NotPassed, 1)          // B
	check("InProgress", got.InProgress, base.InProgress, 1)       // A
	check("TriedFailed", got.TriedFailed, base.TriedFailed, 1)    // B
	check("OldNotPassed", got.OldNotPassed, base.OldNotPassed, 1) // D

	var attempts int
	if err := pool.QueryRow(ctx, `SELECT channel_gate_attempts FROM customer WHERE id=$1`, b.ID).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts = %d err=%v, want 2", attempts, err)
	}

	// Вернулся и подписался позже: из «ушли» в «прошли».
	if err := repo.MarkChannelGatePassed(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got2, err := stats.ChannelGateStats(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	check("NotPassed после возврата", got2.NotPassed, base.NotPassed, 0)
	check("Passed после возврата", got2.Passed, base.Passed, 2)
}

// Откат миграции безопасен: данные клиентов не трогает, колонки аддитивные.
func TestChannelGateMigrationDownKeepsCustomers(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	cust := makeCustomer(t, pool)
	err := execDownInTx(t, pool, "000071_customer_channel_gate.down.sql", func(pgx.Tx) {})
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM customer WHERE id=$1`, cust.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("клиент должен остаться: n=%d err=%v", n, err)
	}
}

// Границы: «новый» — экран увидел не позже часа после регистрации, «ушёл» — не подписался за сутки.
func TestChannelGateStatsBoundaries(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewCustomerRepository(pool)
	stats := NewStatsRepository(pool)

	from := time.Now().UTC().Add(-72 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	base, err := stats.ChannelGateStats(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}

	mk := func(shownAgo, createdBeforeShown string) {
		c := makeCustomer(t, pool)
		if err := repo.MarkChannelGateShown(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE customer SET channel_gate_shown_at = now() - $2::interval,
			       created_at = now() - $2::interval - $3::interval
			WHERE id = $1`, c.ID, shownAgo, createdBeforeShown); err != nil {
			t.Fatal(err)
		}
	}
	mk("25 hours", "59 minutes")         // новый, ушёл
	mk("25 hours", "61 minutes")         // старый (зарегистрирован больше часа до показа)
	mk("23 hours", "10 minutes")         // новый, ещё думает (меньше суток)
	mk("24 hours 1 minute", "5 minutes") // новый, ушёл: пошли вторые сутки

	got, err := stats.ChannelGateStats(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got, base, want int64) {
		t.Helper()
		if got-base != want {
			t.Errorf("%s: прирост %d, ожидали %d", name, got-base, want)
		}
	}
	check("Shown (новые)", got.Shown, base.Shown, 3)
	check("NotPassed", got.NotPassed, base.NotPassed, 2)
	check("InProgress", got.InProgress, base.InProgress, 1)
	check("OldNotPassed", got.OldNotPassed, base.OldNotPassed, 1)
}

// Подписался и вернулся не кнопкой: «прошёл» ставится по telegram_id, но только тому, кто видел экран.
func TestChannelGatePassedByTelegramID(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewCustomerRepository(pool)

	seen := makeCustomer(t, pool)
	silent := makeCustomer(t, pool)
	if err := repo.MarkChannelGateShown(ctx, seen.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Customer{seen, silent} {
		if err := repo.MarkChannelGatePassedByTelegramID(ctx, c.TelegramID); err != nil {
			t.Fatal(err)
		}
	}
	var seenPassed, silentPassed *time.Time
	if err := pool.QueryRow(ctx, `SELECT channel_gate_passed_at FROM customer WHERE id=$1`, seen.ID).Scan(&seenPassed); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT channel_gate_passed_at FROM customer WHERE id=$1`, silent.ID).Scan(&silentPassed); err != nil {
		t.Fatal(err)
	}
	if seenPassed == nil {
		t.Error("видевший экран должен получить отметку «прошёл»")
	}
	if silentPassed != nil {
		t.Error("прошедший молча (экрана не видел) в статистику не попадает")
	}
}
