//go:build integration

package linking

import (
	"context"
	"testing"
)

// Приглашёнными были обе стороны слияния: у приглашённого одна связь
// (UNIQUE referee_id), поэтому перенос оставляет строку с начисленным бонусом,
// а не падает на уникальности.
func TestMoveReferrals_BothSidesWereReferees(t *testing.T) {
	e := newMergeEnv(t)
	referrerA := e.newCustomer(customerSpec{TelegramID: e.nextTelegramID()})
	referrerB := e.newCustomer(customerSpec{TelegramID: e.nextTelegramID()})
	from := e.newCustomer(customerSpec{TelegramID: e.nextTelegramID()})
	to := e.newCustomer(customerSpec{TelegramID: e.nextTelegramID()})
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(),
			`DELETE FROM referral WHERE referee_id IN ($1, $2)`, from.TelegramID, to.TelegramID)
	})

	e.addReferral(referrerA.TelegramID, from.TelegramID)
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO referral (referrer_id, referee_id, bonus_granted) VALUES ($1, $2, TRUE)`,
		referrerB.TelegramID, to.TelegramID); err != nil {
		t.Fatalf("insert referral: %v", err)
	}

	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := moveReferrals(e.ctx, tx, from.TelegramID, to.TelegramID); err != nil {
		t.Fatalf("moveReferrals: %v", err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}

	var n int
	var referrer int64
	if err := e.pool.QueryRow(e.ctx,
		`SELECT count(*), max(referrer_id) FROM referral WHERE referee_id IN ($1, $2)`,
		from.TelegramID, to.TelegramID).Scan(&n, &referrer); err != nil {
		t.Fatal(err)
	}
	if n != 1 || referrer != referrerB.TelegramID {
		t.Fatalf("referrals after merge: %d, referrer %d; want 1 with referrer %d (bonus granted)",
			n, referrer, referrerB.TelegramID)
	}
}
