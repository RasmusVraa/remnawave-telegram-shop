package broadcast

import (
	"net/url"

	"github.com/go-telegram/bot/models"

	botcfg "remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

const (
	callbackBuy        = "buy"
	callbackStart      = "start"
	callbackConnect    = "connect"
	callbackEnterPromo = "enter_promo"
	inlineQuerySuffix  = "?bc=1"
)

// BuildReplyMarkup строит inline-клавиатуру под рассылку для языка получателя.
// telegramID — получатель: в кнопку «Пригласить друга» уходит его реферальная ссылка.
func BuildReplyMarkup(tm *translation.Manager, lang string, flags RecipientButtons, telegramID int64) models.ReplyMarkup {
	if tm == nil || flags.IsEmpty() {
		return nil
	}
	var rows [][]models.InlineKeyboardButton
	if flags.Buy {
		rows = append(rows, []models.InlineKeyboardButton{
			tm.WithButton(lang, "buy_button", models.InlineKeyboardButton{CallbackData: callbackBuy + inlineQuerySuffix}),
		})
	}
	if flags.Connect {
		rows = append(rows, connectRow(tm, lang))
	}
	rows = append(rows, cabinetLinkRows(tm, lang, flags.Links)...)
	if flags.Promo {
		rows = append(rows, []models.InlineKeyboardButton{
			tm.WithButton(lang, "promo_code_button", models.InlineKeyboardButton{CallbackData: callbackEnterPromo + inlineQuerySuffix}),
		})
	}
	if flags.ShareReferral {
		if u := referralShareURL(tm, lang, telegramID); u != "" {
			rows = append(rows, []models.InlineKeyboardButton{
				tm.WithButton(lang, "broadcast_share_referral_button", models.InlineKeyboardButton{URL: u}),
			})
		}
	}
	if flags.MainMenu {
		rows = append(rows, []models.InlineKeyboardButton{
			tm.WithButton(lang, "broadcast_inline_main", models.InlineKeyboardButton{CallbackData: callbackStart + inlineQuerySuffix}),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// cabinetLinkRows строит кнопки на выбранные разделы. Раздел без URL пропускается: так
// отваливаются и разделы кабинета без CABINET_MINI_APP_ENTRY_URL, и выключенные в конфиге
// (колесо фортуны, поддержка без SUPPORT_URL) — рассылка уйдёт, просто без этой кнопки.
func cabinetLinkRows(tm *translation.Manager, lang string, keys []string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	for _, key := range NormalizeCabinetLinkKeys(keys) {
		for _, link := range cabinetLinks {
			if link.Key != key {
				continue
			}
			u := link.resolveURL()
			if u == "" {
				break
			}
			button := models.InlineKeyboardButton{URL: u}
			if link.webApp {
				button = models.InlineKeyboardButton{WebApp: &models.WebAppInfo{URL: u}}
			}
			rows = append(rows, []models.InlineKeyboardButton{
				tm.WithButton(lang, link.TranslationKey, button),
			})
			break
		}
	}
	return rows
}

// referralShareURL — t.me/share/url: Telegram открывает выбор чата, и другу
// уходит текст приглашения с личной ссылкой получателя рассылки. Ссылку
// собрать не из чего (бот ещё не узнал своё имя) — кнопку не рисуем.
func referralShareURL(tm *translation.Manager, lang string, telegramID int64) string {
	ref := utils.ReferralStartURL(botcfg.BotURL(), telegramID)
	if ref == "" {
		return ""
	}
	q := url.Values{}
	q.Set("url", ref)
	q.Set("text", tm.GetText(lang, "broadcast_share_referral_text"))
	return "https://t.me/share/url?" + q.Encode()
}

func connectRow(tm *translation.Manager, lang string) []models.InlineKeyboardButton {
	if u := cabinetWebAppURL("/cabinet/dashboard"); u != "" {
		return []models.InlineKeyboardButton{
			tm.WithButton(lang, "connect_button", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: u},
			}),
		}
	}
	return []models.InlineKeyboardButton{
		tm.WithButton(lang, "connect_button", models.InlineKeyboardButton{
			CallbackData: callbackConnect + inlineQuerySuffix,
		}),
	}
}
