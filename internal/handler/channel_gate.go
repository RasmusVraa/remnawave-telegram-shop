package handler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/channelgate"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// Обязательная подписка на канал. Экран идёт после согласия с документами:
// «Старт» → согласие → подписка → меню. Спека: .cursor/work-in-progress/channel-gate/SPEC.md.

// SetChannelGate подключает проверку подписки. Вызывать до регистрации
// обработчиков: методы Handler с value-receiver копируют структуру в момент
// регистрации, поздняя установка до них не дойдёт.
func (h *Handler) SetChannelGate(svc *channelgate.Service) {
	h.channelGate = svc
}

// channelGateBlocks — true, если пользователя нужно остановить на экране подписки.
// При выключенной функции, у админа и при любом сбое проверки — false.
func (h Handler) channelGateBlocks(ctx context.Context, telegramID int64) bool {
	return h.channelGate != nil && h.channelGate.Blocks(ctx, telegramID)
}

func (h Handler) buildChannelGateKeyboard(langCode string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	if u := config.ChannelGateURL(); u != "" {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "channel_gate_open_button", models.InlineKeyboardButton{URL: u}),
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "channel_gate_check_button", models.InlineKeyboardButton{CallbackData: CallbackChannelGateCheck}),
	})
	return rows
}

// editOrSendChannelGate показывает экран подписки: заменяет сообщение, если
// пришёл callback, и присылает новое, если пришло сообщение.
func (h Handler) editOrSendChannelGate(ctx context.Context, b *bot.Bot, update *models.Update, langCode string) {
	text := h.translation.GetText(langCode, "channel_gate_text")
	markup := models.InlineKeyboardMarkup{InlineKeyboard: h.buildChannelGateKeyboard(langCode)}

	if update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		err := SendOrEditAfterInlineCallback(ctx, b, update, text, models.ParseModeHTML, markup, nil)
		logEditError("error showing channel gate", err)
		return
	}
	var chatID int64
	switch {
	case update.Message != nil:
		chatID = update.Message.Chat.ID
	case update.CallbackQuery != nil:
		chatID = update.CallbackQuery.From.ID
	default:
		return
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if err != nil {
		slog.Error("error sending channel gate", "error", err)
	}
}

// showChannelGate показывает экран и запоминает, что клиент его увидел (для статистики).
func (h Handler) showChannelGate(ctx context.Context, b *bot.Bot, update *models.Update, customer *database.Customer, langCode string) {
	if customer != nil && h.customerRepository != nil {
		if err := h.customerRepository.MarkChannelGateShown(ctx, customer.ID); err != nil {
			slog.Error("channel gate: mark shown", "error", err)
		}
	}
	h.editOrSendChannelGate(ctx, b, update, langCode)
}

// gateBeforeMenu — общий вход перед главным меню: согласие, затем подписка.
// Возвращает true, если показан один из экранов и меню рисовать не нужно.
func (h Handler) gateBeforeMenu(ctx context.Context, b *bot.Bot, update *models.Update, customer *database.Customer, telegramID int64, langCode string) bool {
	if CustomerNeedsLegalGate(customer) {
		h.editOrSendLegalGate(ctx, b, update, langCode, false)
		return true
	}
	if h.channelGateBlocks(ctx, telegramID) {
		h.showChannelGate(ctx, b, update, customer, langCode)
		return true
	}
	return false
}

// RequireChannelSubscriptionMiddleware останавливает пользовательские действия
// на экране подписки. Идёт после RequireLegalAcceptanceMiddleware: сначала
// согласие, потом канал.
//
// Не трогает оплату уже выставленного счёта Stars (PreCheckoutQuery): человек,
// который начал платить до включения функции, не должен терять платёж.
func (h Handler) RequireChannelSubscriptionMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if h.channelGate == nil || !h.channelGate.Active() {
			next(ctx, b, update)
			return
		}
		if update.CallbackQuery != nil {
			switch update.CallbackQuery.Data {
			case CallbackChannelGateCheck, CallbackLegalAccept, CallbackLegalDecline:
				next(ctx, b, update)
				return
			}
		}

		var telegramID int64
		var langCode string
		switch {
		case update.Message != nil && update.Message.From != nil:
			telegramID = update.Message.From.ID
			langCode = update.Message.From.LanguageCode
		case update.CallbackQuery != nil:
			telegramID = update.CallbackQuery.From.ID
			langCode = update.CallbackQuery.From.LanguageCode
		default:
			next(ctx, b, update)
			return
		}

		if !h.channelGateBlocks(ctx, telegramID) {
			next(ctx, b, update)
			return
		}

		// Клиент нужен только для статистики показов: не нашли — экран всё равно показываем.
		var customer *database.Customer
		if h.customerRepository != nil {
			var err error
			if customer, err = h.customerRepository.FindByTelegramId(ctx, telegramID); err != nil {
				slog.Error("channel gate: find customer", "error", err)
				customer = nil
			}
		}
		// Ответ на callback здесь: AnswerCallbackQueryMiddleware без next не сработает.
		if update.CallbackQuery != nil {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
			})
		}
		h.showChannelGate(ctx, b, update, customer, langCode)
	}
}

// ChannelGateCheckCallbackHandler — кнопка «Да, я подписался».
// Регистрируется без AnswerCallbackQueryMiddleware: на неудачу нужно ответить
// всплывающим окном с текстом, а ответить на callback можно один раз.
func (h Handler) ChannelGateCheckCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}
	langCode := callback.From.LanguageCode
	answer := func(text string, alert bool) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            text,
			ShowAlert:       alert,
		})
	}
	if h.channelGate == nil {
		answer("", false)
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil || customer == nil {
		slog.Error("channel gate check: find customer", "error", err)
		answer("", false)
		return
	}

	state, err := h.channelGate.CheckNow(ctx, callback.From.ID)
	if errors.Is(err, channelgate.ErrTooFast) {
		answer("", false)
		return
	}
	if err := h.customerRepository.MarkChannelGateAttempt(ctx, customer.ID); err != nil {
		slog.Error("channel gate check: mark attempt", "error", err)
	}
	if state == channelgate.NotSubscribed {
		answer(h.translation.GetText(langCode, "channel_gate_not_subscribed"), true)
		return
	}
	// Подписан, а при сбое проверки пускаем, как и везде.
	if state == channelgate.Subscribed {
		if err := h.customerRepository.MarkChannelGatePassed(ctx, customer.ID); err != nil {
			slog.Error("channel gate check: mark passed", "error", err)
		}
	}
	answer("", false)

	if CustomerNeedsLegalGate(customer) {
		h.editOrSendLegalGate(ctx, b, update, langCode, false)
		return
	}
	displayName := buildDisplayName(callback.From.FirstName, callback.From.LastName, callback.From.Username)
	inlineKeyboard := h.buildStartKeyboard(customer, langCode)
	if err := h.sendStartMenuAfterCallback(ctx, b, update, langCode, inlineKeyboard, customer, displayName); err != nil {
		slog.Error("channel gate check: start menu", "error", err)
	}
}
