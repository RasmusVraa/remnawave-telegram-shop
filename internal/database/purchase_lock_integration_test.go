//go:build integration

package database

// Интеграционные тесты блокировки обработки покупки.
//
// Двойное продление подписки возникает только при настоящей конкуренции за
// строку в PostgreSQL, поэтому проверяем на живой базе, а не на моках.
//
// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/dbname?sslmode=disable \
//	  go test ./internal/database/... -tags=integration -run Purchase -count=1

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func makePurchaseWithStatus(t *testing.T, pool *pgxpool.Pool, customerID int64, status PurchaseStatus) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type)
VALUES (100, $1, 1, 'RUB', $2, 'yookasa')
RETURNING id`, customerID, status).Scan(&id)
	if err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	return id
}

func purchaseStatus(t *testing.T, repo *PurchaseRepository, id int64) PurchaseStatus {
	t.Helper()
	p, err := repo.FindById(context.Background(), id)
	if err != nil || p == nil {
		t.Fatalf("reload purchase %d: %v", id, err)
	}
	return p.Status
}

// Пока покупку держит один обработчик, остальные сразу получают
// ErrPurchaseLocked, а после unlock покупку снова можно взять.
func TestPurchaseLock_ExclusiveWhileHeld(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPurchaseRepository(pool)
	ctx := context.Background()
	c := makeCustomer(t, pool)
	id := makePurchaseWithStatus(t, pool, c.ID, PurchaseStatusPending)

	unlock, err := repo.LockForProcessing(ctx, id)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	const contenders = 10
	var wg sync.WaitGroup
	var locked, busy atomic.Int32
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := repo.LockForProcessing(ctx, id)
			switch {
			case err == nil:
				locked.Add(1)
				u()
			case errors.Is(err, ErrPurchaseLocked):
				busy.Add(1)
			default:
				t.Errorf("unexpected lock error: %v", err)
			}
		}()
	}
	wg.Wait()
	if locked.Load() != 0 || busy.Load() != contenders {
		t.Fatalf("while held: locked=%d busy=%d, want 0/%d", locked.Load(), busy.Load(), contenders)
	}

	unlock()
	unlock() // повторный вызов безопасен

	again, err := repo.LockForProcessing(ctx, id)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	again()
}

// Разные покупки друг друга не блокируют.
func TestPurchaseLock_IndependentPurchases(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPurchaseRepository(pool)
	ctx := context.Background()
	c := makeCustomer(t, pool)
	a := makePurchaseWithStatus(t, pool, c.ID, PurchaseStatusPending)
	b := makePurchaseWithStatus(t, pool, c.ID, PurchaseStatusPending)

	ua, err := repo.LockForProcessing(ctx, a)
	if err != nil {
		t.Fatalf("lock a: %v", err)
	}
	defer ua()
	ub, err := repo.LockForProcessing(ctx, b)
	if err != nil {
		t.Fatalf("lock b while a is held: %v", err)
	}
	ub()
}

// Однокомпонентная advisory-блокировка с тем же числом (так чат поддержки
// блокирует account_id) не должна мешать блокировке покупки.
func TestPurchaseLock_DoesNotCollideWithBigintKeys(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPurchaseRepository(pool)
	ctx := context.Background()
	c := makeCustomer(t, pool)
	id := makePurchaseWithStatus(t, pool, c.ID, PurchaseStatusPending)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, id); err != nil {
		t.Fatalf("bigint lock: %v", err)
	}
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, id) }()

	unlock, err := repo.LockForProcessing(ctx, id)
	if err != nil {
		t.Fatalf("purchase lock collided with bigint key: %v", err)
	}
	unlock()
}

// Главный сценарий: много обработчиков одной оплаченной покупки одновременно
// (вебхук, его повторы, поллер). Каждый ведёт себя как ProcessPurchaseById:
// захват → перечитать статус → если ещё не paid, «продлить» и записать paid.
// Продление должно случиться ровно один раз.
func TestPurchaseLock_ConcurrentProcessingExtendsOnce(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPurchaseRepository(pool)
	c := makeCustomer(t, pool)
	id := makePurchaseWithStatus(t, pool, c.ID, PurchaseStatusPending)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const workers = 20
	var extensions atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				unlock, err := repo.LockForProcessing(ctx, id)
				if errors.Is(err, ErrPurchaseLocked) {
					// Касса повторит вебхук позже — имитируем повтор.
					time.Sleep(5 * time.Millisecond)
					continue
				}
				if err != nil {
					t.Errorf("lock: %v", err)
					return
				}
				p, err := repo.FindById(ctx, id)
				if err != nil || p == nil {
					unlock()
					t.Errorf("reload: %v", err)
					return
				}
				if p.Status != PurchaseStatusPaid {
					extensions.Add(1)
					// Окно, в котором без блокировки соседи увидели бы pending.
					time.Sleep(20 * time.Millisecond)
					if err := repo.MarkAsPaid(ctx, id); err != nil {
						t.Errorf("mark paid: %v", err)
					}
				}
				unlock()
				return
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := extensions.Load(); got != 1 {
		t.Fatalf("subscription extended %d times, want exactly 1", got)
	}
	if st := purchaseStatus(t, repo, id); st != PurchaseStatusPaid {
		t.Fatalf("status = %s, want paid", st)
	}
}

// Отмена не затирает уже оплаченную покупку и срабатывает только для открытых.
func TestCancelIfOpen(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPurchaseRepository(pool)
	ctx := context.Background()
	c := makeCustomer(t, pool)

	for _, tc := range []struct {
		status     PurchaseStatus
		wantCancel bool
		wantStatus PurchaseStatus
	}{
		{PurchaseStatusNew, true, PurchaseStatusCancel},
		{PurchaseStatusPending, true, PurchaseStatusCancel},
		{PurchaseStatusPaid, false, PurchaseStatusPaid},
		{PurchaseStatusCancel, false, PurchaseStatusCancel},
	} {
		id := makePurchaseWithStatus(t, pool, c.ID, tc.status)
		got, err := repo.CancelIfOpen(ctx, id)
		if err != nil {
			t.Fatalf("%s: cancel: %v", tc.status, err)
		}
		if got != tc.wantCancel {
			t.Fatalf("%s: cancelled = %v, want %v", tc.status, got, tc.wantCancel)
		}
		if st := purchaseStatus(t, repo, id); st != tc.wantStatus {
			t.Fatalf("%s: status = %s, want %s", tc.status, st, tc.wantStatus)
		}
	}
}
