package handler

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// resolveTrialButton — кнопка «Попробовать бесплатно».
//
// В режиме minimalism ведёт сразу в WebApp на /cabinet/dashboard, где человека
// встречает карточка пробного периода с кнопкой «Активировать бесплатно».
// Триал-флоу внутри Telegram там только дублировал кабинет: пользователь
// активировал пробный в боте, а подключаться всё равно уходил в мини-апп.
//
// В classic (и без кабинета вовсе) остаётся прежний callback —
// TrialCallbackHandler ниже: там весь путь покупки живёт в боте, и уводить
// из него один только пробный период значило бы рвать сценарий пополам.
// Тот же callback обслуживает кнопки в уже отправленных сообщениях.
func (h Handler) resolveTrialButton(langCode, buttonKey string) models.InlineKeyboardButton {
	if cabinetTelegramMinimalismActive() {
		if u := cabinetWebAppURL("/cabinet/dashboard"); u != "" {
			return h.translation.WithButton(langCode, buttonKey, models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: u},
			})
		}
	}
	return h.translation.WithButton(langCode, buttonKey, models.InlineKeyboardButton{CallbackData: CallbackTrial})
}

func (h Handler) TrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if config.TrialDays() == 0 {
		return
	}
	c, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if c == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID), "error", err)
		return
	}
	if c.SubscriptionLink != nil {
		return
	}
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	_, err = editCallbackOriginToHTMLText(ctx, b, callback, h.translation.GetText(langCode, "trial_text"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(langCode, "activate_trial_button", models.InlineKeyboardButton{CallbackData: CallbackActivateTrial}),
		},
		{
			h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
		},
	}}, nil)
	logEditError("Error sending /trial message", err)
}

func (h Handler) ActivateTrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if config.TrialDays() == 0 {
		return
	}
	c, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if c == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID), "error", err)
		return
	}
	if c.SubscriptionLink != nil {
		return
	}
	callback := update.CallbackQuery.Message.Message
	ctxWithUsername := context.WithValue(ctx, remnawave.CtxKeyUsername, update.CallbackQuery.From.Username)
	_, err = h.paymentService.ActivateTrial(ctxWithUsername, update.CallbackQuery.From.ID)
	if errors.Is(err, payment.ErrTrialAlreadyUsed) {
		return // повторное нажатие: пробный уже выдан этим или параллельным запросом
	}
	if err != nil {
		slog.Error("Error activating trial", "error", err)
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode
	_, err = editCallbackOriginToHTMLText(ctx, b, callback, h.translation.GetText(langCode, "trial_activated"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.createConnectKeyboard(langCode)}, nil)
	logEditError("Error sending /trial message", err)
}

func (h Handler) createConnectKeyboard(lang string) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton
	inlineCustomerKeyboard = append(inlineCustomerKeyboard, h.resolveConnectButton(lang))

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
	})
	return inlineCustomerKeyboard
}
