package handler

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Паника в обработчике не выходит за middleware, а счётчик работающих
// обработчиков возвращается к нулю — иначе остановка ждала бы до таймаута.
func TestRecoverMiddleware_SwallowsPanicAndReleasesInflight(t *testing.T) {
	inflight := &Inflight{}
	h := RecoverMiddleware(inflight)(func(context.Context, *bot.Bot, *models.Update) {
		panic("boom")
	})
	h(context.Background(), nil, &models.Update{ID: 42})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if left := inflight.Wait(ctx); left != 0 {
		t.Fatalf("inflight = %d after panic, want 0", left)
	}
}

func TestInflight_WaitStopsAtDeadline(t *testing.T) {
	inflight := &Inflight{}
	release := make(chan struct{})
	started := make(chan struct{})
	h := RecoverMiddleware(inflight)(func(context.Context, *bot.Bot, *models.Update) {
		close(started)
		<-release
	})
	go h(context.Background(), nil, &models.Update{})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if left := inflight.Wait(ctx); left != 1 {
		t.Fatalf("Wait with running handler = %d, want 1", left)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if left := inflight.Wait(ctx2); left != 0 {
		t.Fatalf("Wait after handler finished = %d, want 0", left)
	}
}

// Кнопка под сообщением старше 48 часов приходит без Message: middleware
// раньше разыменовывал nil и ронял процесс.
func TestSuspiciousUserFilter_InaccessibleCallbackMessage(t *testing.T) {
	called := false
	mw := Handler{}.SuspiciousUserFilterMiddleware(func(context.Context, *bot.Bot, *models.Update) {
		called = true
	})
	update := &models.Update{CallbackQuery: &models.CallbackQuery{
		From: models.User{ID: 1001, FirstName: "Ivan", Username: "ivan"},
		Message: models.MaybeInaccessibleMessage{
			Type:                models.MaybeInaccessibleMessageTypeInaccessibleMessage,
			InaccessibleMessage: &models.InaccessibleMessage{Chat: models.Chat{ID: 1001}},
		},
	}}
	mw(context.Background(), nil, update)
	if !called {
		t.Fatal("next handler not called")
	}
}
