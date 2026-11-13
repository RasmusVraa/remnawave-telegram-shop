package trafficquota

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

// TelegramNotifier — предупреждения в ленту кабинета и в бот. Push не отправляется:
// вид уведомления «traffic» не заведён в настройках push.
type TelegramNotifier struct {
	bot *bot.Bot
	tm  *translation.Manager
}

func NewTelegramNotifier(b *bot.Bot, tm *translation.Manager) *TelegramNotifier {
	return &TelegramNotifier{bot: b, tm: tm}
}

// kindTitle — как называется вид трафика в тексте уведомления.
func kindTitle(tm *translation.Manager, lang, kind string) string {
	if kind == database.TrafficKindBypass {
		return config.TrafficBypassLabel()
	}
	return tm.GetText(lang, "traffic_kind_main")
}

// FormatGB — «6 ГБ», «5,5 ГБ» (en — «5.5 GB»).
func FormatGB(bytes int64, lang string) string {
	gb := float64(bytes) / (1 << 30)
	unit := "ГБ"
	if lang == "en" {
		unit = "GB"
	}
	var s string
	if gb >= 10 || gb == float64(int64(gb)) {
		s = fmt.Sprintf("%.0f", gb)
	} else {
		s = fmt.Sprintf("%.1f", gb)
	}
	if lang != "en" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s + " " + unit
}

func resetDate(t time.Time) string { return cabinetnotify.Date(t) }

// alertTone — цвет кнопки «Докупить» в ленте кабинета: красная при исчерпании, жёлтая при приближении.
func alertTone(level int) string {
	if level >= 100 {
		return "red"
	}
	return "amber"
}

// gbNumber — размер в ГБ без единицы: «30», «5,5».
func gbNumber(bytes int64, lang string) string {
	s := FormatGB(bytes, lang)
	for _, unit := range []string{" ГБ", " GB"} {
		s = strings.TrimSuffix(s, unit)
	}
	return s
}

// Alert реализует Notifier.
//
// Вид сообщения — по макету: в Telegram перед заголовком ⚠️ (приближение к лимиту) или 🚫 (исчерпан),
// ниже кнопка «Докупить трафик», при исчерпании ещё «Мой трафик». В ленте кабинета те же тексты без эмодзи.
func (n *TelegramNotifier) Alert(ctx context.Context, c database.Customer, a Alert) error {
	lang := c.Language
	if lang == "" {
		lang = "ru"
	}
	kind := kindTitle(n.tm, lang, a.Kind)
	kindLower := strings.ToLower(kind)
	full := a.Level >= 100

	var title, body, emoji string
	switch {
	case full && a.Kind == database.TrafficKindBypass:
		emoji = "🚫"
		title = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_full_title"), kindLower)
		body = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_full_bypass_body"), kind, resetDate(a.ResetAt))
	case full:
		emoji = "🚫"
		title = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_full_title"), kindLower)
		body = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_full_main_body"), kind, resetDate(a.ResetAt))
	default:
		emoji = "⚠️"
		title = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_warn_title"), a.Level, kindLower)
		body = fmt.Sprintf(n.tm.GetText(lang, "traffic_alert_warn_body"), FormatGB(a.LimitBytes-a.UsedBytes, lang), gbNumber(a.LimitBytes, lang), resetDate(a.ResetAt))
	}

	cabinetnotify.Notify(ctx, c.ID, lang, cabinetnotify.Event{
		Kind:     "traffic",
		Title:    title,
		BodyHTML: body,
		Buttons:  []string{"traffic_buy_" + a.Kind + "_" + alertTone(a.Level)},
		// Лимит в ключе: после докупки он вырос, и новое приближение к нему — новое уведомление, а не дубль.
		DedupeKey: fmt.Sprintf("traffic:%d:%s:%s:%d:%d", c.ID, a.Kind, a.ResetAt.AddDate(0, -1, 0).UTC().Format("2006-01"), a.Level, a.LimitBytes),
	})

	if c.IsWebOnly || utils.IsSyntheticTelegramID(c.TelegramID) || n.bot == nil {
		return nil
	}
	text := emoji + " <b>" + title + "</b>\n" + body
	params := &bot.SendMessageParams{ChatID: c.TelegramID, Text: text, ParseMode: models.ParseModeHTML}
	if url := cabcfg.MiniAppEntryURL(); url != "" {
		base := strings.TrimRight(url, "/")
		var rows [][]models.InlineKeyboardButton
		if config.TrafficPacksEnabled() {
			rows = append(rows, []models.InlineKeyboardButton{
				n.tm.WithButton(lang, "traffic_buy_button", models.InlineKeyboardButton{WebApp: &models.WebAppInfo{URL: base + "/dashboard?buy=" + a.Kind}}),
			})
		}
		if full {
			rows = append(rows, []models.InlineKeyboardButton{
				n.tm.WithButton(lang, "traffic_my_button", models.InlineKeyboardButton{WebApp: &models.WebAppInfo{URL: base + "/dashboard"}}),
			})
		}
		if len(rows) > 0 {
			params.ReplyMarkup = models.InlineKeyboardMarkup{InlineKeyboard: rows}
		}
	}
	if _, err := n.bot.SendMessage(ctx, params); err != nil {
		// Уведомление в ленте кабинета уже создано, поэтому недоставленное сообщение в Telegram не повод слать всё заново.
		slog.Warn("trafficquota: telegram alert", "customer_id", c.ID, "error", err)
	}
	return nil
}
