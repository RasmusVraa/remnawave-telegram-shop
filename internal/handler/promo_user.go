package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
)

var userPromoInput = struct {
	mu      sync.Mutex
	waiting map[int64]bool
}{waiting: make(map[int64]bool)}

func userPromoSetWait(tgID int64, on bool) {
	userPromoInput.mu.Lock()
	defer userPromoInput.mu.Unlock()
	if on {
		userPromoInput.waiting[tgID] = true
	} else {
		delete(userPromoInput.waiting, tgID)
	}
}

// UserPromoWaiting is true while the user is expected to send a promo code text.
func UserPromoWaiting(tgID int64) bool {
	userPromoInput.mu.Lock()
	defer userPromoInput.mu.Unlock()
	return userPromoInput.waiting[tgID]
}

// EnterPromoCallbackHandler asks user to send promo code in the next message.
func (h Handler) EnterPromoCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if h.promoService == nil || update.CallbackQuery == nil {
		return
	}
	cb := update.CallbackQuery
	userPromoSetWait(cb.From.ID, true)
	lang := cb.From.LanguageCode
	err := SendOrEditAfterInlineCallback(ctx, b, update, h.translation.GetText(lang, "promo_enter_code"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart})},
	}}, nil)
	if err != nil {
		slog.Error("enter promo edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// UserPromoMessageHandler handles text when user is entering a promo code (must register before ForwardUserMessageToAdmin).
func (h Handler) UserPromoMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if h.promoService == nil || update.Message == nil || update.Message.Text == "" {
		return
	}
	if !UserPromoWaiting(update.Message.From.ID) {
		return
	}
	userPromoSetWait(update.Message.From.ID, false)

	lang := update.Message.From.LanguageCode
	ctxT, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	customer, err := h.customerRepository.FindByTelegramId(ctxT, update.Message.Chat.ID)
	if err != nil || customer == nil {
		customer, err = h.customerRepository.Create(ctxT, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   lang,
		})
		if err != nil {
			slog.Error("promo user customer", "error", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: h.translation.GetText(lang, "promo_apply_failed")})
			return
		}
	}

	ctxU := context.WithValue(ctxT, remnawave.CtxKeyUsername, update.Message.From.Username)
	res, err := h.promoService.Activate(ctxU, update.Message.From.ID, update.Message.From.Username, customer, update.Message.Text)
	if err != nil {
		back := [][]models.InlineKeyboardButton{
			{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart})},
		}
		if promo.IsPromoValidationErr(err) {
			switch promo.ClassifyActivateError(err) {
			case promo.ActivateErrAlreadyUsed:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_already_used"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			case promo.ActivateErrInactive:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_inactive"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			case promo.ActivateErrNotFound:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_not_found"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			case promo.ActivateErrPendingDiscount:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_pending_discount"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			case promo.ActivateErrTariffMismatch:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_tariff_mismatch"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			case promo.ActivateErrHwidPinned:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_err_hwid_pinned"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			default:
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_apply_failed"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
				return
			}
		}
		slog.Error("promo activate", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: h.translation.GetText(lang, "promo_apply_failed"), ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
		return
	}

	back := [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart})},
	}
	var text string
	switch res.Type {
	case promo.ResultTypeDiscount:
		text = fmt.Sprintf(h.translation.GetText(lang, "promo_apply_ok_discount"), res.DiscountPercent)
	case promo.ResultTypeTrial:
		if res.TrialSkippedActiveSub {
			text = h.translation.GetText(lang, "promo_apply_ok_trial_has_sub")
		} else {
			text = fmt.Sprintf(h.translation.GetText(lang, "promo_apply_ok_trial"), res.TrialDays)
		}
	case promo.ResultTypeSubscriptionDays:
		text = formatPromoSubDaysExtended(lang, h, res.SubscriptionDays)
	case promo.ResultTypeExtraHwid:
		text = fmt.Sprintf(h.translation.GetText(lang, "promo_apply_ok_hwid"), res.ExtraHwidDelta)
	default:
		text = h.translation.GetText(lang, "promo_apply_ok")
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, ParseMode: models.ParseModeHTML, Text: text, ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: back}})
}

func formatPromoSubDaysExtended(lang string, h Handler, days int) string {
	if lang == "ru" {
		return fmt.Sprintf(h.translation.GetText(lang, "promo_apply_ok_sub_days"), days, ruDaysWord(days))
	}
	word := h.translation.GetText(lang, "promo_day_word_many")
	if days == 1 {
		word = h.translation.GetText(lang, "promo_day_word_one")
	}
	return fmt.Sprintf(h.translation.GetText(lang, "promo_apply_ok_sub_days"), days, word)
}

func ruDaysWord(n int) string {
	n = n % 100
	if n >= 11 && n <= 14 {
		return "дней"
	}
	switch n % 10 {
	case 1:
		return "день"
	case 2, 3, 4:
		return "дня"
	default:
		return "дней"
	}
}
