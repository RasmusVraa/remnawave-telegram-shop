//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Удаляются только записи, истёкшие раньше cutoff. Живая сессия и свежие коды
// остаются, а удаление старого звена ротации не ломает его преемника.
func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	pool := pgPoolIntegration(t)
	acc, err := NewAccountRepo(pool).Create(ctx, fmt.Sprintf("cabinet-int-purge-%d@example.com", time.Now().UnixNano()), "", "ru")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM cabinet_account WHERE id = $1`, acc.ID) })

	now := time.Now().UTC()
	old := now.Add(-30 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	cutoff := now.Add(-ExpiredRetention)

	sessions := NewSessionRepo(pool)
	family := uuid.New()
	oldSess, err := sessions.Create(ctx, CreateInput{AccountID: acc.ID, TokenHash: shaOfRandomRefresh(t), FamilyID: family, ExpiresAt: old})
	if err != nil {
		t.Fatal(err)
	}
	// Преемник старой сессии по ротации — живой.
	liveHash := shaOfRandomRefresh(t)
	if _, err := sessions.Rotate(ctx, oldSess.ID, CreateInput{AccountID: acc.ID, TokenHash: liveHash, ExpiresAt: now.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create(ctx, CreateInput{AccountID: acc.ID, TokenHash: shaOfRandomRefresh(t), FamilyID: uuid.New(), ExpiresAt: recent}); err != nil {
		t.Fatal(err)
	}

	for _, exp := range []time.Time{old, recent} {
		if _, err := pool.Exec(ctx, `INSERT INTO cabinet_email_verification (account_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			acc.ID, []byte(uuid.NewString()), exp); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO cabinet_password_reset_token (account_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			acc.ID, []byte(uuid.NewString()), exp); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := PurgeExpired(ctx, pool, cutoff); err != nil {
		t.Fatalf("purge: %v", err)
	}

	count := func(table string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE account_id = $1`, acc.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Остаются: живая сессия и недавно истёкшая; по одному свежему коду.
	if n := count("cabinet_session"); n != 2 {
		t.Fatalf("sessions left %d, want 2", n)
	}
	if n := count("cabinet_email_verification"); n != 1 {
		t.Fatalf("email codes left %d, want 1", n)
	}
	if n := count("cabinet_password_reset_token"); n != 1 {
		t.Fatalf("reset tokens left %d, want 1", n)
	}
	if s, err := sessions.FindByRefreshHash(ctx, liveHash); err != nil || s == nil || s.RevokedAt != nil {
		t.Fatalf("live session after purge: %+v err=%v", s, err)
	}
}
