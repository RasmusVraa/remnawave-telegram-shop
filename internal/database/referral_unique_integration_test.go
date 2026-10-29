//go:build integration

package database

import (
	"context"
	"testing"
)

// Второй /start по чужой реферальной ссылке не создаёт вторую связь:
// остаётся первая, и бонус получает тот, кто пригласил первым.
func TestReferralCreate_OneReferrerPerReferee(t *testing.T) {
	pool := partnerTestPool(t)
	ctx := context.Background()
	repo := NewReferralRepository(pool)
	first := makeCustomer(t, pool)
	second := makeCustomer(t, pool)
	referee := makeCustomer(t, pool)

	ref, err := repo.Create(ctx, first.TelegramID, referee.TelegramID)
	if err != nil || ref == nil {
		t.Fatalf("first referral: ref=%v err=%v", ref, err)
	}
	dup, err := repo.Create(ctx, second.TelegramID, referee.TelegramID)
	if err != nil {
		t.Fatalf("second referral: %v", err)
	}
	if dup != nil {
		t.Fatalf("second referral created: %+v", dup)
	}
	got, err := repo.FindByReferee(ctx, referee.TelegramID)
	if err != nil || got == nil || got.ReferrerID != first.TelegramID {
		t.Fatalf("referrer = %+v err=%v, want %d", got, err, first.TelegramID)
	}
}
