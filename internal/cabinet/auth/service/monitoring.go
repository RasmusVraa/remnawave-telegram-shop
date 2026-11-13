package service

import (
	"context"

	"remnawave-tg-shop-bot/internal/adminevents"
)

// События аккаунта для «Мониторинга». Способ входа едет в контексте: сессию
// выдаёт одна функция issueSession, а знают, как человек вошёл, только её
// вызывающие. Привязка аккаунта тоже выдаёт сессию — там способ сбрасывается,
// чтобы привязка не считалась входом.

type loginMethodKey struct{}

func withLoginMethod(ctx context.Context, method string) context.Context {
	return context.WithValue(ctx, loginMethodKey{}, method)
}

func loginMethodFrom(ctx context.Context) string {
	m, _ := ctx.Value(loginMethodKey{}).(string)
	return m
}

// emitAccountEvent пишет событие аккаунта кабинета. Клиента по аккаунту журнал
// найдёт сам при записи.
func emitAccountEvent(eventType string, accountID int64, payload map[string]any) {
	level := adminevents.LevelImportant
	if eventType == adminevents.AccountEmailVerified || eventType == adminevents.AccountPasswordReset {
		level = adminevents.LevelBot
	}
	adminevents.Emit(adminevents.Event{
		Type: eventType, Level: level, AccountID: accountID,
		Source: adminevents.SourceCabinet, Payload: payload,
	})
}
