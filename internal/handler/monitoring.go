package handler

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/config"
)

// MonitoringMiddleware пишет в «Мониторинг» команды и нажатия кнопок в боте.
// Это мелочь (уровень «нажатия»): пишется, только если включено
// MONITORING_BOT_ACTIONS. Нажатия админа не пишем — это не поведение клиентов.
func MonitoringMiddleware() bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			emitBotAction(update)
			next(ctx, b, update)
		}
	}
}

func emitBotAction(update *models.Update) {
	if update == nil || !config.MonitoringBotActions() {
		return
	}
	var (
		from    int64
		typ     string
		payload map[string]any
	)
	switch {
	case update.CallbackQuery != nil:
		from = update.CallbackQuery.From.ID
		typ = adminevents.BotButton
		payload = map[string]any{"data": callbackAction(update.CallbackQuery.Data)}
	case update.Message != nil && update.Message.From != nil && strings.HasPrefix(update.Message.Text, "/"):
		from = update.Message.From.ID
		typ = adminevents.BotCommand
		cmd := strings.Fields(update.Message.Text)[0]
		payload = map[string]any{"command": clipRunes(cmd, 32)}
	default:
		return
	}
	if from == 0 || from == config.GetAdminTelegramId() {
		return
	}
	adminevents.Emit(adminevents.Event{
		Type: typ, Level: adminevents.LevelBot, TelegramID: from,
		Source: adminevents.SourceBot, Payload: payload,
	})
}

// emitBotMessage — клиент написал боту текст (не команду). Пишется всегда,
// независимо от того, пересылается ли текст админу.
func emitBotMessage(update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil {
		return
	}
	text := update.Message.Text
	if text == "" || strings.HasPrefix(text, "/") || update.Message.From.ID == config.GetAdminTelegramId() {
		return
	}
	adminevents.Emit(adminevents.Event{
		Type: adminevents.BotMessage, TelegramID: update.Message.From.ID, Source: adminevents.SourceBot,
		Payload: map[string]any{"preview": adminevents.Preview(text, 100)},
	})
}

// callbackAction — что за кнопка, без хвоста: «sell?tid=3&amount=…» → «sell»,
// «delete_device_abc» → «delete_device». Параметры оплаты и id в журнал не идут.
func callbackAction(data string) string {
	if i := strings.IndexAny(data, "?:"); i >= 0 {
		data = data[:i]
	}
	if strings.HasPrefix(data, CallbackDeleteDevice) {
		return strings.TrimSuffix(CallbackDeleteDevice, "_")
	}
	return clipRunes(data, 32)
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
