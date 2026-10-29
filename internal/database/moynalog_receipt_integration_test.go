//go:build integration

package database

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Отправку одного чека захватывает только одна попытка: фоновая отправка после
// оплаты и тик воркера больше не шлют в ФНС один доход дважды.
func TestMoynalogReceiptClaim_OneWinner(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewMoynalogReceiptRepository(pool)
	c := makeCustomer(t, pool)
	purchaseID := makePaidPurchase(t, pool, c.ID, 199)

	row, err := repo.Enqueue(ctx, purchaseID, 199, "Подписка", time.Now())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.Claim(ctx, row.ID, time.Now().Add(3*time.Minute))
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := winners.Load(); n != 1 {
		t.Fatalf("claims won: %d, want 1", n)
	}

	due, err := repo.ListDue(ctx, 100)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	for _, d := range due {
		if d.ID == row.ID {
			t.Fatal("claimed receipt must not be listed as due")
		}
	}

	// Отправленный чек не захватывается, даже когда срок захвата истёк.
	if err := repo.MarkSent(ctx, row.ID, "fns-1"); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE moynalog_receipt SET next_attempt_at = now() - interval '1 hour' WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Claim(ctx, row.ID, time.Now().Add(time.Minute)); err != nil || ok {
		t.Fatalf("claim of sent receipt: ok=%v err=%v, want false", ok, err)
	}
}

// Захват истёк (процесс упал посреди отправки) — чек снова доступен воркеру.
func TestMoynalogReceiptClaim_ExpiredLeaseIsRetried(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewMoynalogReceiptRepository(pool)
	c := makeCustomer(t, pool)
	purchaseID := makePaidPurchase(t, pool, c.ID, 99)

	row, err := repo.Enqueue(ctx, purchaseID, 99, "Подписка", time.Now())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if ok, err := repo.Claim(ctx, row.ID, time.Now().Add(-time.Second)); err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Claim(ctx, row.ID, time.Now().Add(time.Minute)); err != nil || !ok {
		t.Fatalf("claim after expired lease: ok=%v err=%v, want true", ok, err)
	}
}
