package service

import (
	"context"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestRemnawaveUserIDForUsesSavedID(t *testing.T) {
	s := NewSupport(nil, nil, nil, nil, nil, nil, nil, nil)
	saved := int64(4242)
	got := s.remnawaveUserIDFor(context.Background(), &database.Customer{ID: 1, RemnawaveUserID: &saved})
	if got == nil || *got != saved {
		t.Fatalf("want saved id %d, got %v", saved, got)
	}
}

func TestRemnawaveUserIDForSkipsLookupWithoutSubscription(t *testing.T) {
	// Панели нет (sub == nil): без подписки поиск не нужен, с подпиской — некуда идти.
	s := NewSupport(nil, nil, nil, nil, nil, nil, nil, nil)
	if got := s.remnawaveUserIDFor(context.Background(), &database.Customer{ID: 1}); got != nil {
		t.Fatalf("no subscription: want nil, got %d", *got)
	}
	exp := time.Now().Add(24 * time.Hour)
	if got := s.remnawaveUserIDFor(context.Background(), &database.Customer{ID: 1, ExpireAt: &exp}); got != nil {
		t.Fatalf("no panel client: want nil, got %d", *got)
	}
}
