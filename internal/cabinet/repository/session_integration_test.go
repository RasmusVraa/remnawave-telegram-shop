//go:build integration

package repository

// Интеграционные тесты с PostgreSQL.
// Запуск из корня репозитория:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/dbname?sslmode=disable go test ./internal/cabinet/repository/... -tags=integration -count=1
//
// Перед запуском примените миграции (или доверьтесь RunMigrations в тесте — см. комментарий в теле).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/cabinet/auth/tokens"
	"remnawave-tg-shop-bot/internal/database"
)

func migrationsDir(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	// .../internal/cabinet/repository/session_integration_test.go → repo root (3× ..)
	return filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", "..", "db", "migrations"))
}

func pgPoolIntegration(t *testing.T) *pgxpool.Pool {
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
	if err := database.RunMigrations(ctx, &database.MigrationConfig{
		Direction:      "up",
		MigrationsPath: migrationsDir(t),
		Steps:          0,
	}, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return pool
}

func TestSessionRotate_secondRotateSameOldID_returnsErrReused(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)

	accRepo := NewAccountRepo(pool)
	acc, err := accRepo.Create(ctx, "cabinet-int-session-"+time.Now().Format("150405")+"@example.com", "", "ru")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	sessRepo := NewSessionRepo(pool)
	family := uuid.New()
	h1 := shaOfRandomRefresh(t)
	exp := time.Now().UTC().Add(24 * time.Hour)

	s1, err := sessRepo.Create(ctx, CreateInput{
		AccountID: acc.ID,
		TokenHash: h1,
		FamilyID:  family,
		ExpiresAt: exp,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	h2 := shaOfRandomRefresh(t)
	s2, err := sessRepo.Rotate(ctx, s1.ID, CreateInput{
		AccountID: acc.ID,
		TokenHash: h2,
		ExpiresAt: exp,
	})
	if err != nil {
		t.Fatalf("first rotate: %v", err)
	}
	if s2 == nil {
		t.Fatal("nil new session")
	}

	h3 := shaOfRandomRefresh(t)
	_, err = sessRepo.Rotate(ctx, s1.ID, CreateInput{
		AccountID: acc.ID,
		TokenHash: h3,
		ExpiresAt: exp,
	})
	if !errors.Is(err, ErrReused) {
		t.Fatalf("second rotate on old session id: want ErrReused, got %v", err)
	}
}

func shaOfRandomRefresh(t *testing.T) [32]byte {
	t.Helper()
	_, h, err := tokens.Generate(tokens.DefaultRefreshBytes)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
