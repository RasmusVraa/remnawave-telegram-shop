//go:build integration

package payment

// Интеграционные тесты проверок при создании счёта: неактивный тариф, промокод
// на несколько счетов, блокировка выдачи пробного периода.
// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/db?sslmode=disable \
//	  go test ./internal/payment/ -tags=integration -count=1

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/promo"
)

func guardsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CABINET_INTEGRATION_PG")
	if dsn == "" {
		t.Skip("set CABINET_INTEGRATION_PG to run integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	_, f, _, _ := runtime.Caller(0)
	dir := filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", "db", "migrations"))
	if err := database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: dir}, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return pool
}

func guardsCustomer(t *testing.T, pool *pgxpool.Pool) *database.Customer {
	t.Helper()
	ctx := context.Background()
	tg := time.Now().UnixNano() % 1_000_000_000_000
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO customer (telegram_id, language) VALUES ($1, 'ru') RETURNING id`, tg).Scan(&id); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM customer WHERE id = $1`, id) })
	c, err := database.NewCustomerRepository(pool).FindById(ctx, id)
	if err != nil || c == nil {
		t.Fatalf("find customer: %v", err)
	}
	return c
}

func guardsTariff(t *testing.T, pool *pgxpool.Pool, active bool) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	slug := fmt.Sprintf("guard-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO tariff (slug, name, is_active) VALUES ($1, $1, $2) RETURNING id`, slug, active).Scan(&id); err != nil {
		t.Fatalf("insert tariff: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tariff_price (tariff_id, months, amount_rub) VALUES ($1, 1, 199)`, id); err != nil {
		t.Fatalf("insert price: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `UPDATE customer SET current_tariff_id = NULL WHERE current_tariff_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE tariff_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tariff_price WHERE tariff_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tariff WHERE id = $1`, id)
	})
	return id
}

// Снятый с продажи тариф нельзя купить, а свой текущий — можно продлить.
func TestResolveTariffPurchase_InactiveTariff(t *testing.T) {
	pool := guardsTestPool(t)
	ctx := context.Background()
	tr := database.NewTariffRepository(pool)
	cust := guardsCustomer(t, pool)
	inactive := guardsTariff(t, pool, false)
	active := guardsTariff(t, pool, true)

	if _, _, _, _, err := ResolveTariffPurchase(ctx, tr, cust, active, 1, false); err != nil {
		t.Fatalf("active tariff: %v", err)
	}
	if _, _, _, _, err := ResolveTariffPurchase(ctx, tr, cust, inactive, 1, false); !errors.Is(err, ErrTariffUnavailable) {
		t.Fatalf("inactive tariff: err = %v, want ErrTariffUnavailable", err)
	}
	cust.CurrentTariffID = &inactive
	if _, _, _, _, err := ResolveTariffPurchase(ctx, tr, cust, inactive, 1, false); err != nil {
		t.Fatalf("renewing own inactive tariff: %v", err)
	}
}

// Скидка «на одну оплату»: открытых счетов с ней не больше MinOpenInvoicesWithPromo.
// Брошенный счёт Stars освобождает скидку.
func TestPendingDiscount_ReservedByOpenInvoices(t *testing.T) {
	pool := guardsTestPool(t)
	ctx := context.Background()
	promoRepo := database.NewPromoRepository(pool)
	svc := promo.NewService(promoRepo, database.NewCustomerRepository(pool), database.NewPurchaseRepository(pool), nil)
	cust := guardsCustomer(t, pool)

	var promoID int64
	code := fmt.Sprintf("G%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO promo_code (code, type, discount_percent) VALUES ($1, 'discount', 50) RETURNING id`, code).Scan(&promoID); err != nil {
		t.Fatalf("insert promo: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE promo_code_id = $1`, promoID)
		_, _ = pool.Exec(ctx, `DELETE FROM customer_pending_discount WHERE promo_code_id = $1`, promoID)
		_, _ = pool.Exec(ctx, `DELETE FROM promo_code WHERE id = $1`, promoID)
	})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := promoRepo.UpsertPendingDiscount(ctx, tx, cust.ID, promoID, 50, nil, true, 1); err != nil {
		t.Fatalf("upsert discount: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID)
	if err != nil || pct != 50 {
		t.Fatalf("first invoice: pct=%d err=%v, want 50", pct, err)
	}
	openInvoice := func(invoiceType database.InvoiceType) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, promo_code_id, discount_percent_applied)
			VALUES (99, $1, 1, 'RUB', 'pending', $2, $3, 50)`, cust.ID, invoiceType, promoID); err != nil {
			t.Fatalf("insert purchase: %v", err)
		}
	}
	openInvoice(database.InvoiceTypeYookasa)
	if pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID); err != nil || pct != 50 {
		t.Fatalf("second invoice while first is open: pct=%d err=%v, want 50", pct, err)
	}
	// Скидку получают счета, пока открыто меньше MinOpenInvoicesWithPromo: пятый счёт ещё со скидкой,
	// шестой уже без неё.
	for open := 2; open < promo.MinOpenInvoicesWithPromo; open++ {
		openInvoice(database.InvoiceTypeYookasa)
		if pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID); err != nil || pct != 50 {
			t.Fatalf("invoice #%d (open before it: %d, limit %d): pct=%d err=%v, want 50", open+1, open, promo.MinOpenInvoicesWithPromo, pct, err)
		}
	}
	openInvoice(database.InvoiceTypeYookasa)
	if pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID); err != nil || pct != 0 {
		t.Fatalf("invoice over the limit of %d: pct=%d err=%v, want 0", promo.MinOpenInvoicesWithPromo, pct, err)
	}

	// Внешний счёт отменился (истёк в кассе), открыт брошенный счёт Stars.
	if _, err := pool.Exec(ctx, `UPDATE purchase SET status = 'cancel' WHERE promo_code_id = $1`, promoID); err != nil {
		t.Fatal(err)
	}
	// Брошенные счета Stars не истекают сами: когда их набралось на весь лимит, они освобождаются
	// (отменяются), и скидка снова достаётся новому счёту.
	for i := 0; i < promo.MinOpenInvoicesWithPromo; i++ {
		openInvoice(database.InvoiceTypeTelegram)
	}
	if pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID); err != nil || pct != 50 {
		t.Fatalf("stale stars invoices must release discount: pct=%d err=%v, want 50", pct, err)
	}
	var cancelled int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM purchase WHERE promo_code_id = $1 AND invoice_type = 'telegram' AND status = 'cancel'`, promoID).Scan(&cancelled); err != nil || cancelled != promo.MinOpenInvoicesWithPromo {
		t.Fatalf("cancelled stale stars invoices = %d, err = %v; want %d", cancelled, err, promo.MinOpenInvoicesWithPromo)
	}

	// Счёт кассы, который так и остался new (Platega месячной давности), скидку
	// не держит.
	if _, err := pool.Exec(ctx, `
		INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, promo_code_id, discount_percent_applied, created_at)
		VALUES (99, $1, 1, 'RUB', 'new', 'plt_ww', $2, 50, now() - interval '30 days')`, cust.ID, promoID); err != nil {
		t.Fatalf("insert old purchase: %v", err)
	}
	if pct, _, err := svc.PendingDiscountForPayment(ctx, cust.ID); err != nil || pct != 50 {
		t.Fatalf("month-old open invoice must not hold discount: pct=%d err=%v, want 50", pct, err)
	}
}

// Выдача пробного периода сериализуется по клиенту.
func TestLockTrial_Exclusive(t *testing.T) {
	pool := guardsTestPool(t)
	ctx := context.Background()
	repo := database.NewCustomerRepository(pool)
	cust := guardsCustomer(t, pool)

	unlock, err := repo.LockTrial(ctx, cust.ID)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := repo.LockTrial(ctx, cust.ID); !errors.Is(err, database.ErrTrialLocked) {
		t.Fatalf("second lock: err = %v, want ErrTrialLocked", err)
	}
	unlock()
	unlock2, err := repo.LockTrial(ctx, cust.ID)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	unlock2()
}
