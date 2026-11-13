package notification

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/adminalert"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

// Запасные тексты на случай, если ключей нет в translations/ на диске: папка
// смонтирована в контейнер, и после обновления образа новых ключей там может не
// оказаться. Оповещение о поломке не должно ломаться само.
const (
	mailDownFallback = "⚠️ <b>Почта кабинета не отправляется</b>\n\n" +
		"Письма с кодом подтверждения и ссылкой сброса пароля не доходят до клиентов, " +
		"а кабинет при этом пишет им «письмо отправлено».\n\n" +
		"Ошибка:\n<code>%s</code>\n\n" +
		"Проверьте CABINET_SMTP_* и что сервер достаёт до <code>%s</code>. " +
		"Хостеры часто закрывают исходящие порты 465 и 587 — у Resend есть запасные 2465 и 2587.\n\n" +
		"Пока не починится, напомню раз в 6 часов."
	mailUpFallback = "✅ <b>Почта кабинета снова отправляется</b>"
)

// MailHealthAlert возвращает обработчик для mail.Sender.OnHealthChange: пишет
// админу, когда почта кабинета перестала отправляться и когда заработала снова.
// Куда — вид «Система» в «Оповещениях админу» (по умолчанию личка).
// nil, если слать нечем.
func MailHealthAlert(b *bot.Bot, tm *translation.Manager, smtpHost string, smtpPort int) func(down bool, reason string) {
	if b == nil || tm == nil {
		return nil
	}
	addr := smtpHost + ":" + strconv.Itoa(smtpPort)
	return func(down bool, reason string) {
		lang := config.DefaultLanguage()
		var text string
		if down {
			text = fmt.Sprintf(mailText(tm, lang, "cabinet_mail_down_admin", mailDownFallback),
				html.EscapeString(trunc(strings.TrimSpace(reason), 500)), html.EscapeString(addr))
		} else {
			text = mailText(tm, lang, "cabinet_mail_up_admin", mailUpFallback)
		}
		utils.Go("mail health alert", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := adminalert.Send(ctx, config.AdminAlertSystem, text, nil); err != nil {
				slog.Error("mail health alert: send failed", "error", err)
			}
		})
	}
}

func mailText(tm *translation.Manager, lang, key, fallback string) string {
	if s := strings.TrimSpace(tm.GetText(lang, key)); s != "" && s != key {
		return s
	}
	return fallback
}
