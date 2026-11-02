package handler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/yookasa"
)

// Способы ЮKassa в callback кнопки — одной буквой: callback_data у Telegram
// ограничен 64 байтами, а «&ym=bank_card» к длинному payment?tid=…&extra=…
// уже не влезает.
var yookasaMethodCallbackCodes = map[string]string{
	config.YookasaPaymentMethodAll: "a",
	"sbp":                          "s",
	"bank_card":                    "c",
	"sberbank":                     "p",
	"tinkoff_bank":                 "t",
	"yoo_money":                    "y",
}

// yookasaMethodFromCallback — способ по коду из callback. Неизвестный код —
// пусто: PaymentService сам выберет способ по настройке.
func yookasaMethodFromCallback(code string) string {
	for method, c := range yookasaMethodCallbackCodes {
		if c == code {
			return method
		}
	}
	return ""
}

// withYookasaMethodFromCallback кладёт в контекст способ ЮKassa из параметра
// ym callback-кнопки. Для остальных касс параметра нет, и контекст не меняется.
func withYookasaMethodFromCallback(ctx context.Context, params map[string]string) context.Context {
	if m := yookasaMethodFromCallback(params["ym"]); m != "" {
		return context.WithValue(ctx, yookasa.CtxKeyPaymentMethod, m)
	}
	return ctx
}

// Подписи кнопок, если в файлах переводов на диске ещё нет новых ключей:
// без них Telegram показал бы сам ключ «yookasa_sbp_button».
var yookasaMethodButtonFallback = map[string][2]string{ // [ru, en]
	"sbp":          {"⚡ СБП", "⚡ SBP"},
	"bank_card":    {"💳 Банковская карта", "💳 Bank card"},
	"sberbank":     {"🟢 SberPay", "🟢 SberPay"},
	"tinkoff_bank": {"🟡 T-Pay", "🟡 T-Pay"},
	"yoo_money":    {"👛 ЮMoney", "👛 YooMoney"},
}

// yookasaPaymentRows — кнопки ЮKassa: по одной на способ из
// YOOKASA_PAYMENT_METHODS. Без настройки — одна прежняя card_button с тем же
// callback, что и раньше. callbackFor получает суффикс «&ym=…» (или пусто).
func (h Handler) yookasaPaymentRows(langCode string, callbackFor func(suffix string) string) [][]models.InlineKeyboardButton {
	methods := config.YookasaPaymentMethods()
	if len(methods) == 1 && methods[0] == config.YookasaPaymentMethodAll {
		return [][]models.InlineKeyboardButton{{
			h.translation.WithButton(langCode, "card_button", models.InlineKeyboardButton{CallbackData: callbackFor("")}),
		}}
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(methods))
	for _, m := range methods {
		btn := models.InlineKeyboardButton{CallbackData: callbackFor("&ym=" + yookasaMethodCallbackCodes[m])}
		if m == config.YookasaPaymentMethodAll {
			btn = h.translation.WithButton(langCode, "card_button", btn)
		} else {
			key := "yookasa_" + m + "_button"
			btn = h.translation.WithButton(langCode, key, btn)
			if btn.Text == key {
				labels := yookasaMethodButtonFallback[m]
				btn.Text = labels[1]
				if strings.HasPrefix(langCode, "ru") {
					btn.Text = labels[0]
				}
			}
		}
		rows = append(rows, []models.InlineKeyboardButton{btn})
	}
	return rows
}
