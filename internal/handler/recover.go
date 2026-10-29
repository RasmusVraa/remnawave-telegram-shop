package handler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Inflight считает обработчики апдейтов, которые сейчас выполняются, чтобы при
// остановке дать им закончить: иначе процесс обрывает, например, выдачу
// подписки после уже списанной оплаты на полпути.
type Inflight struct {
	mu sync.Mutex
	n  int
}

func (f *Inflight) add(d int) {
	f.mu.Lock()
	f.n += d
	f.mu.Unlock()
}

// Wait ждёт, пока все обработчики закончатся, но не дольше, чем живёт ctx.
// Возвращает число тех, кто не успел.
func (f *Inflight) Wait(ctx context.Context) int {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		f.mu.Lock()
		n := f.n
		f.mu.Unlock()
		if n == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return n
		case <-t.C:
		}
	}
}

// RecoverMiddleware — внешний middleware бота. go-telegram/bot запускает каждый
// обработчик в своей горутине без recover, и паника в любом из них завершала
// весь процесс: бот, кабинет и приём вебхуков оплат. Теперь апдейт пишется в
// лог со стеком и отбрасывается, остальные обрабатываются дальше.
func RecoverMiddleware(inflight *Inflight) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if inflight != nil {
				inflight.add(1)
				defer inflight.add(-1)
			}
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovered panic in bot handler",
						"update_id", update.ID, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
				}
			}()
			next(ctx, b, update)
		}
	}
}
