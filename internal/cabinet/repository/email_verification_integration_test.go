//go:build integration

package repository

// Интеграционные тесты кодов подтверждения email: поиск в пределах аккаунта,
// лимит попыток под параллельной нагрузкой, одноразовость, совпадение хэшей.
// Запуск — как session_integration_test.go.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/cabinet/auth/tokens"
)

func makeEVAccount(t *testing.T, pool *pgxpool.Pool) *Account {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("cabinet-int-ev-%d@example.com", time.Now().UnixNano())
	acc, err := NewAccountRepo(pool).Create(ctx, email, "", "ru")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM cabinet_account WHERE id = $1`, acc.ID)
	})
	return acc
}

func TestEmailVerification_FindActiveForAccount(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)
	repo := NewEmailVerificationRepo(pool)
	a := makeEVAccount(t, pool)
	b := makeEVAccount(t, pool)

	if _, err := repo.FindActiveForAccount(ctx, a.ID); err != ErrNotFound {
		t.Fatalf("no code yet: err = %v, want ErrNotFound", err)
	}
	ev, err := repo.Create(ctx, a.ID, tokens.HashEmailVerifyOTPKeyed([]byte("k"), "123456"), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.FindActiveForAccount(ctx, a.ID)
	if err != nil || got.ID != ev.ID {
		t.Fatalf("active for a: %+v, %v", got, err)
	}
	// Чужой аккаунт код a не видит.
	if _, err := repo.FindActiveForAccount(ctx, b.ID); err != ErrNotFound {
		t.Fatalf("active for b: err = %v, want ErrNotFound", err)
	}
	// Истёкший код не живой.
	if _, err := pool.Exec(ctx, `UPDATE cabinet_email_verification SET expires_at = NOW() - interval '1 minute' WHERE id = $1`, ev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindActiveForAccount(ctx, a.ID); err != ErrNotFound {
		t.Fatalf("expired: err = %v, want ErrNotFound", err)
	}
}

// Пятьдесят параллельных догадок по одному коду получают ровно лимит попыток.
func TestEmailVerification_ReserveAttemptConcurrentLimit(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)
	repo := NewEmailVerificationRepo(pool)
	a := makeEVAccount(t, pool)
	ev, err := repo.Create(ctx, a.ID, tokens.HashEmailVerifyOTPKeyed([]byte("k"), "654321"), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	const guesses, limit = 50, 5
	var allowed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < guesses; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, err := repo.ReserveAttempt(ctx, ev.ID, limit)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := allowed.Load(); got != limit {
		t.Fatalf("allowed attempts = %d, want %d", got, limit)
	}
	// Исчерпанный код не принимает и следующую попытку.
	if ok, _ := repo.ReserveAttempt(ctx, ev.ID, limit); ok {
		t.Fatal("attempt after limit was allowed")
	}
}

// Верная догадка на последней разрешённой попытке ещё принимается, а погасить
// код можно только один раз.
func TestEmailVerification_ConsumeOnceAndLastAttempt(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)
	repo := NewEmailVerificationRepo(pool)
	a := makeEVAccount(t, pool)
	ev, err := repo.Create(ctx, a.ID, tokens.HashEmailVerifyOTPKeyed([]byte("k"), "111111"), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		if ok, err := repo.ReserveAttempt(ctx, ev.ID, 5); !ok || err != nil {
			t.Fatalf("attempt %d: ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, err := repo.Consume(ctx, ev.ID)
	if err != nil || !ok {
		t.Fatalf("consume after last attempt: ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.Consume(ctx, ev.ID); ok {
		t.Fatal("code consumed twice")
	}
}

// После миграции 000049 одинаковый хэш у двух аккаунтов не ломает выдачу кода.
func TestEmailVerification_SameHashDifferentAccounts(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)
	repo := NewEmailVerificationRepo(pool)
	a := makeEVAccount(t, pool)
	b := makeEVAccount(t, pool)
	h := tokens.HashEmailVerifyOTP("222222")
	if _, err := repo.Create(ctx, a.ID, h, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := repo.Create(ctx, b.ID, h, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create b with same hash: %v", err)
	}
}
