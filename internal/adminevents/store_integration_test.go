//go:build integration

package adminevents_test

// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/db?sslmode=disable go test ./internal/adminevents/ -tags=integration -count=1

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	. "remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/database"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
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
	if _, err := pool.Exec(ctx, `DELETE FROM admin_event`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return pool
}

func TestStoreIntegration(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t)
	store := NewStore(pool)

	// Регистрация через репозиторий клиентов: событие ровно одно, клиент найден.
	t.Setenv("MONITORING_BOT_ACTIONS", "true")
	w := NewWriter(store)
	w.Start()
	SetDefault(w)
	t.Cleanup(func() { SetDefault(nil) })

	tg := 990000000 + time.Now().Unix()%1000000
	customers := database.NewCustomerRepository(pool)
	c, err := customers.FindOrCreate(ctx, &database.Customer{TelegramID: tg, Language: "ru"})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM customer WHERE id = $1`, c.ID) })
	if _, err := customers.FindOrCreate(ctx, &database.Customer{TelegramID: tg, Language: "ru"}); err != nil {
		t.Fatalf("find customer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE customer SET telegram_username = 'mon_test' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	w.Stop(stopCtx)
	cancel()
	var regs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_event WHERE type = $1 AND customer_id = $2 AND source = 'bot'`,
		UserRegistered, c.ID).Scan(&regs); err != nil || regs != 1 {
		t.Fatalf("registration events: %d, %v", regs, err)
	}

	now := time.Now()
	batch := []Event{
		{Type: PaymentSubscriptionNew, Level: LevelImportant, CustomerID: c.ID, Source: SourceBot, At: now,
			Payload: map[string]any{"amount": 199.0, "currency": "RUB", "provider": "yookasa"}},
		{Type: PaymentFailed, Level: LevelImportant, TelegramID: tg, Source: SourceBot, At: now,
			Payload: map[string]any{"amount": 99, "currency": "RUB", "provider": "yookasa"}},
		{Type: BotButton, Level: LevelBot, TelegramID: tg, Source: SourceBot, At: now,
			Payload: map[string]any{"data": "buy"}},
		{Type: NodeDown, Level: LevelImportant, Source: SourcePanel, At: now, Payload: map[string]any{"name": "NL-1"}},
		{Type: AdminAction, Level: LevelImportant, CustomerID: c.ID, ActorTgID: tg, Source: SourceCabinet, At: now},
		{Type: UserLogin, Level: LevelImportant, CustomerID: c.ID, Source: SourceCabinet, At: now.AddDate(0, 0, -100)},
		{Type: PaymentSubscriptionRenew, Level: LevelImportant, CustomerID: c.ID, Source: SourceBot, At: now.AddDate(0, 0, -100),
			Payload: map[string]any{"amount": 150, "currency": "RUB"}},
	}
	for i := range batch {
		if batch[i].At.IsZero() {
			batch[i].At = now
		}
		if batch[i].Level == 0 {
			batch[i].Level = LevelImportant
		}
	}
	if err := store.Insert(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	all, err := store.List(ctx, ListFilter{HideExpired: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 7 { // нажатие в боте без переключателя не видно
		t.Fatalf("list important = %d rows, want 7", len(all))
	}
	var failed *Row
	for i := range all {
		if all[i].Type == PaymentFailed {
			failed = &all[i]
		}
		if all[i].Type == AdminAction && (all[i].Actor == nil || *all[i].Actor != "@mon_test") {
			t.Fatalf("admin actor label: %+v", all[i].Actor)
		}
	}
	if failed == nil || failed.CustomerID == nil || *failed.CustomerID != c.ID || failed.Customer == nil || *failed.Customer != "@mon_test" {
		t.Fatalf("customer resolved by telegram id: %+v", failed)
	}

	money, err := store.List(ctx, ListFilter{Categories: []string{CategoryMoney}, BotActions: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(money) != 4 { // 3 оплаты + нажатие
		t.Fatalf("money+bot = %d rows, want 4", len(money))
	}

	hist, err := store.List(ctx, ListFilter{CustomerID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 7 { // всё про клиента, включая нажатие; нода — не про него
		t.Fatalf("customer history = %d rows, want 7", len(hist))
	}

	newer, err := store.List(ctx, ListFilter{AfterID: all[1].ID})
	if err != nil || len(newer) != 1 || newer[0].ID != all[0].ID {
		t.Fatalf("after_id poll: %v %+v", err, newer)
	}

	sum, err := store.Summary(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Payments != 1 || sum.Revenue["RUB"] != 199 || sum.Failed != 1 || sum.FailedProvider != "yookasa" || sum.RegBot != 1 || sum.NodeDowns != 1 {
		b, _ := json.Marshal(sum)
		t.Fatalf("summary: %s", b)
	}

	deleted, err := store.Purge(ctx, now, 365, 90, 14)
	if err != nil || deleted != 1 { // только старый вход: старая оплата ещё в пределах года
		t.Fatalf("purge: deleted=%d err=%v", deleted, err)
	}

	// Истечение: клиент с подпиской, закончившейся полчаса назад, попадает в выборку один раз.
	if _, err := pool.Exec(ctx, `UPDATE customer SET expire_at = $2 WHERE id = $1`, c.ID, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	exp, err := store.ExpiredSince(ctx, now.Add(-time.Hour), now)
	if err != nil || len(exp) != 1 {
		t.Fatalf("expired since: %v %+v", err, exp)
	}
	if err := store.Insert(ctx, []Event{{Type: SubscriptionExpired, Level: LevelImportant, CustomerID: c.ID, Source: SourceSystem, At: now}}); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.ExpiredSince(ctx, now.Add(-time.Hour), now); len(again) != 0 {
		t.Fatalf("expired must not repeat: %+v", again)
	}
	expired, notRenewed, err := store.ExpiredDigest(ctx, now.Add(-time.Hour))
	if err != nil || expired != 1 || notRenewed != 1 {
		t.Fatalf("digest: %d %d %v", expired, notRenewed, err)
	}
}
