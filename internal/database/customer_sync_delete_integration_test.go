//go:build integration

package database

// Интеграционные тесты удаления клиентов при синхронизации с панелью.
// Запуск — как partner_integration_test.go.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// syncTestCustomers создаёт n бот-клиентов и возвращает их telegram_id.
func syncTestCustomers(t *testing.T, pool *pgxpool.Pool, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	customers := make([]Customer, n)
	for i := range ids {
		ids[i] = uniqueTelegramID()
		customers[i] = Customer{TelegramID: ids[i], Language: "ru"}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM customer WHERE telegram_id = ANY($1)`, ids)
	})
	if err := NewCustomerRepository(pool).CreateBatch(context.Background(), customers); err != nil {
		t.Fatalf("create customers: %v", err)
	}
	return ids
}

// panelIDsWithout — Telegram ID всех бот-клиентов в БД, кроме missing: так
// выглядел бы ответ панели, из которой пропали только они.
func panelIDsWithout(t *testing.T, pool *pgxpool.Pool, missing []int64) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT telegram_id FROM customer WHERE is_web_only = FALSE AND telegram_id <> ALL($1)`, missing)
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

func customerExists(t *testing.T, pool *pgxpool.Pool, telegramID int64) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM customer WHERE telegram_id = $1)`, telegramID).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// Из панели пропали трое: без истории, с неоплаченным счётом и приглашённый
// по рефералке. Удаляется только первый — у остальных каскад унёс бы покупку
// и реферальную связь.
func TestDeleteMissingFromPanel_KeepsCustomersWithHistory(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewCustomerRepository(pool)
	ctx := context.Background()

	ids := syncTestCustomers(t, pool, 4)
	plain, withPurchase, referee, referrer := ids[0], ids[1], ids[2], ids[3]
	if _, err := pool.Exec(ctx, `
		INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type)
		SELECT 99, id, 1, 'RUB', 'pending', 'yookasa' FROM customer WHERE telegram_id = $1`, withPurchase); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral (referrer_id, referee_id) VALUES ($1, $2)`, referrer, referee); err != nil {
		t.Fatalf("insert referral: %v", err)
	}

	res, err := repo.DeleteMissingFromPanel(ctx, panelIDsWithout(t, pool, []int64{plain, withPurchase, referee}))
	if err != nil {
		t.Fatalf("DeleteMissingFromPanel: %v", err)
	}
	if res.Deleted != 1 || res.Kept != 2 {
		t.Fatalf("result = %+v, want Deleted=1 Kept=2", res)
	}
	if customerExists(t, pool, plain) {
		t.Fatal("customer without history must be deleted")
	}
	for _, id := range []int64{withPurchase, referee, referrer} {
		if !customerExists(t, pool, id) {
			t.Fatalf("customer %d with history was deleted", id)
		}
	}
	var purchases int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM purchase p JOIN customer c ON c.id = p.customer_id
		 WHERE c.telegram_id = $1`, withPurchase).Scan(&purchases); err != nil || purchases != 1 {
		t.Fatalf("purchase lost: count=%d err=%v", purchases, err)
	}
}

// Пустой ответ панели раньше удалял всех клиентов бота.
func TestDeleteMissingFromPanel_EmptyPanelDeletesNothing(t *testing.T) {
	pool := partnerTestPool(t)
	ids := syncTestCustomers(t, pool, 1)

	_, err := NewCustomerRepository(pool).DeleteMissingFromPanel(context.Background(), nil)
	if !errors.Is(err, ErrSyncTooManyDeletions) {
		t.Fatalf("err = %v, want ErrSyncTooManyDeletions", err)
	}
	if !customerExists(t, pool, ids[0]) {
		t.Fatal("customer deleted on empty panel response")
	}
}

// Если разом пропадает заметная доля клиентов, это сбой панели, а не чистка:
// не удаляется никто.
func TestDeleteMissingFromPanel_GuardStopsMassDeletion(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	var total int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM customer WHERE is_web_only = FALSE`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	const n = syncDeleteFreeRows + 5
	if (total+n)*syncDeleteMaxPercent >= n*100 {
		t.Skipf("database has %d customers: %d missing would stay under the guard", total, n)
	}
	ids := syncTestCustomers(t, pool, n)

	_, err := NewCustomerRepository(pool).DeleteMissingFromPanel(ctx, panelIDsWithout(t, pool, ids))
	if !errors.Is(err, ErrSyncTooManyDeletions) {
		t.Fatalf("err = %v, want ErrSyncTooManyDeletions", err)
	}
	for _, id := range ids {
		if !customerExists(t, pool, id) {
			t.Fatalf("customer %d deleted despite the guard", id)
		}
	}
}
