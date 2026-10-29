//go:build integration

package database

// Индексы горячих запросов и таймауты пула. Запуск — как partner_integration_test.go.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var hotIndexes = []string{
	"idx_purchase_open_by_type",
	"idx_purchase_customer_status",
	"idx_purchase_paid_at",
	"idx_purchase_created_at",
	"idx_customer_created_at",
}

func validIndexes(t *testing.T, pool *pgxpool.Pool) map[string]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT c.relname, i.indisvalid
		  FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		 WHERE c.relname = ANY($1)`, hotIndexes)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		var valid bool
		if err := rows.Scan(&name, &valid); err != nil {
			t.Fatal(err)
		}
		got[name] = valid
	}
	return got
}

// Индексы создаются CONCURRENTLY — это работает только вне транзакции, и
// невалидный индекс после сбоя остался бы молча. Down-миграции тоже рабочие.
func TestHotIndexes_CreatedAndReversible(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	check := func(stage string) {
		got := validIndexes(t, pool)
		for _, name := range hotIndexes {
			if !got[name] {
				t.Fatalf("%s: index %s missing or invalid (got %v)", stage, name, got)
			}
		}
	}
	check("up")

	// Откатываем до версии перед первым индексом (000050): над ними могли
	// появиться новые миграции, поэтому шаги считаем от текущей версии.
	const lastBeforeIndexes = 49
	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("schema version: %v", err)
	}
	dir := partnerMigrationsDir(t)
	if err := RunMigrations(ctx, &MigrationConfig{Direction: "down", Steps: version - lastBeforeIndexes, MigrationsPath: dir}, pool); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := validIndexes(t, pool); len(got) != 0 {
		t.Fatalf("after down indexes remain: %v", got)
	}
	if err := RunMigrations(ctx, &MigrationConfig{Direction: "up", MigrationsPath: dir}, pool); err != nil {
		t.Fatalf("up again: %v", err)
	}
	check("up again")
}

// Миграции идут отдельным пулом без statement_timeout, а пул приложения свой
// таймаут сохраняет.
func TestRunMigrations_KeepsAppPoolStatementTimeout(t *testing.T) {
	_ = partnerTestPool(t) // миграции
	cfg, err := pgxpool.ParseConfig(os.Getenv("CABINET_INTEGRATION_PG"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "2s"
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	if err := RunMigrations(ctx, &MigrationConfig{Direction: "up", MigrationsPath: partnerMigrationsDir(t)}, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	var v string
	if err := pool.QueryRow(ctx, `SHOW statement_timeout`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "2s" {
		t.Fatalf("app pool statement_timeout = %q, want 2s", v)
	}
	// Таймаут действительно режет долгий запрос.
	if _, err := pool.Exec(ctx, `SELECT pg_sleep(3)`); err == nil {
		t.Fatal("pg_sleep(3) finished despite statement_timeout=2s")
	}
}
