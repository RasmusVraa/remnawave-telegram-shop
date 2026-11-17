package notification

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/adminalert"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

// Запасные тексты: translations/ смонтирована с диска, и после обновления
// образа новых ключей там может не оказаться (см. mail_health.go).
const (
	channelGateDownFallback = "⚠️ <b>Бот не может проверить подписку на канал</b>\n\n" +
		"Канал: <code>%s</code>\nОшибка: <code>%s</code>\n\n" +
		"Пока это не исправлено, пользователи проходят без проверки. " +
		"Добавьте бота администратором канала и убедитесь, что канал публичный: " +
		"«Настройки бота → Бот и доступ → Подписка на канал → Проверить».\n\n" +
		"Пока не починится, напомню раз в 6 часов."
	channelGateUpFallback = "✅ <b>Проверка подписки на канал снова работает</b>"
)

// ChannelGateHealthAlert возвращает обработчик для channelgate.New: пишет админу,
// когда проверка подписки сломалась (бота убрали из канала, канал стал
// приватным) и когда заработала снова. Куда — вид «Система» в «Оповещениях
// админу». Пока проверка сломана, пользователи проходят без неё — об этом
// админ должен знать.
func ChannelGateHealthAlert(tm *translation.Manager) func(down bool, reason string) {
	if tm == nil {
		return nil
	}
	return func(down bool, reason string) {
		lang := config.DefaultLanguage()
		var text string
		if down {
			text = fmt.Sprintf(mailText(tm, lang, "channel_gate_down_admin", channelGateDownFallback),
				html.EscapeString(config.ChannelGateChannel()),
				html.EscapeString(trunc(strings.TrimSpace(reason), 300)))
		} else {
			text = mailText(tm, lang, "channel_gate_up_admin", channelGateUpFallback)
		}
		utils.Go("channel gate health alert", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := adminalert.Send(ctx, config.AdminAlertSystem, text, nil); err != nil {
				slog.Error("channel gate health alert: send failed", "error", err)
			}
		})
	}
}
