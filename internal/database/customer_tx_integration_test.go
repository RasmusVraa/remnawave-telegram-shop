//go:build integration

package database

// Интеграционные тесты записи клиентов: утечка соединений, пакетные запросы
// больше лимита параметров PostgreSQL, часовой пояс срока подписки.
// Запуск — как partner_integration_test.go.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateFields по исчезнувшему клиенту не должен оставлять соединение занятым.
// Пул из двух соединений раньше выбирался за два таких вызова.
func TestCustomerUpdateFields_MissingCustomerDoesNotLeakConnection(t *testing.T) {
	_ = partnerTestPool(t) // миграции
	cfg, err := pgxpool.ParseConfig(os.Getenv("CABINET_INTEGRATION_PG"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	small, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(small.Close)
	repo := NewCustomerRepository(small)

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := repo.UpdateFields(ctx, -1, map[string]interface{}{"language": "ru"})
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()
		if timedOut {
			t.Fatalf("call %d timed out: pool exhausted by leaked connections", i+1)
		}
		if err == nil {
			t.Fatal("expected not-found error for missing customer")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var one int
	if err := small.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("pool unusable after missing-customer updates: %v", err)
	}
}

// Синхронизация с панелью вставляет новых клиентов одним вызовом. Больше
// 65535/4 строк раньше не помещалось в один запрос.
func TestCustomerCreateBatch_AboveParameterLimit(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewCustomerRepository(pool)
	ctx := context.Background()

	const n = 17_000
	base := uniqueTelegramID() - n
	customers := make([]Customer, n)
	for i := range customers {
		customers[i] = Customer{TelegramID: base + int64(i), Language: "ru"}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM customer WHERE telegram_id BETWEEN $1 AND $2`, base, base+n-1)
	})
	if err := repo.CreateBatch(ctx, customers); err != nil {
		t.Fatalf("CreateBatch(%d): %v", n, err)
	}
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM customer WHERE telegram_id BETWEEN $1 AND $2`, base, base+n-1).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Fatalf("inserted %d customers, want %d", got, n)
	}
}

// Срок подписки из панели может прийти не в UTC — записаться должен тот же момент.
func TestCustomerUpdateBatch_KeepsInstantAcrossTimeZones(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewCustomerRepository(pool)
	ctx := context.Background()
	c := makeCustomer(t, pool)

	msk := time.FixedZone("UTC+3", 3*60*60)
	want := time.Date(2030, 1, 2, 12, 0, 0, 0, msk)
	link := "https://example.invalid/sub"
	if err := repo.UpdateBatch(ctx, []Customer{{TelegramID: c.TelegramID, ExpireAt: &want, SubscriptionLink: &link}}); err != nil {
		t.Fatalf("UpdateBatch: %v", err)
	}
	got, err := repo.FindById(ctx, c.ID)
	if err != nil || got == nil || got.ExpireAt == nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.ExpireAt.Equal(want) {
		t.Fatalf("expire_at = %s, want %s (shift %s)", got.ExpireAt.UTC(), want.UTC(), got.ExpireAt.Sub(want))
	}
}
