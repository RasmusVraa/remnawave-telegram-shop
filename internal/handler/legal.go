package handler

import (
	"context"
	"strings"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// LegalDocumentsConfigured — gate включается только если заданы политика и оферта.
func LegalDocumentsConfigured() bool {
	return strings.TrimSpace(config.PrivacyPolicyURL()) != "" &&
		strings.TrimSpace(config.PublicOfferURL()) != ""
}

// CustomerNeedsLegalGate — true, если нужно показать экран согласия.
func CustomerNeedsLegalGate(c *database.Customer) bool {
	if !LegalDocumentsConfigured() {
		return false
	}
	return c == nil || c.LegalAcceptedAt == nil
}

func (h Handler) buildLegalGateKeyboard(langCode string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton

	if u := strings.TrimSpace(config.PrivacyPolicyURL()); u != "" {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "privacy_policy_button", models.InlineKeyboardButton{URL: u}),
		})
	}
	if u := strings.TrimSpace(config.PublicOfferURL()); u != "" {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "public_offer_button", models.InlineKeyboardButton{URL: u}),
		})
	}
	if u := strings.TrimSpace(config.TermsOfServiceURL()); u != "" {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "terms_of_service_button", models.InlineKeyboardButton{URL: u}),
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "legal_accept_button", models.InlineKeyboardButton{CallbackData: CallbackLegalAccept}),
		h.translation.WithButton(langCode, "legal_decline_button", models.InlineKeyboardButton{CallbackData: CallbackLegalDecline}),
	})
	return rows
}

func (h Handler) sendLegalGateMessage(ctx context.Context, b *bot.Bot, chatID int64, langCode string, declined bool) {
	key := "legal_gate_text"
	if declined {
		key = "legal_gate_declined"
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      h.translation.GetText(langCode, key),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: h.buildLegalGateKeyboard(langCode),
		},
	})
	if err != nil {
		slog.Error("error sending legal gate", "error", err)
	}
}

func (h Handler) editOrSendLegalGate(ctx context.Context, b *bot.Bot, update *models.Update, langCode string, declined bool) {
	key := "legal_gate_text"
	if declined {
		key = "legal_gate_declined"
	}
	text := h.translation.GetText(langCode, key)
	markup := models.InlineKeyboardMarkup{InlineKeyboard: h.buildLegalGateKeyboard(langCode)}

	if update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		err := SendOrEditAfterInlineCallback(ctx, b, update, text, models.ParseModeHTML, markup, nil)
		logEditError("error showing legal gate", err)
		return
	}
	if update.Message != nil {
		h.sendLegalGateMessage(ctx, b, update.Message.Chat.ID, langCode, declined)
	}
}

// RequireLegalAcceptanceMiddleware блокирует пользовательские действия до принятия документов.
func (h Handler) RequireLegalAcceptanceMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if !LegalDocumentsConfigured() {
			next(ctx, b, update)
			return
		}
		if update.CallbackQuery != nil {
			data := update.CallbackQuery.Data
			if data == CallbackLegalAccept || data == CallbackLegalDecline {
				next(ctx, b, update)
				return
			}
		}

		var telegramID int64
		var langCode string
		switch {
		case update.Message != nil:
			telegramID = update.Message.From.ID
			langCode = update.Message.From.LanguageCode
		case update.CallbackQuery != nil:
			telegramID = update.CallbackQuery.From.ID
			langCode = update.CallbackQuery.From.LanguageCode
		case update.PreCheckoutQuery != nil:
			telegramID = update.PreCheckoutQuery.From.ID
			langCode = update.PreCheckoutQuery.From.LanguageCode
		default:
			next(ctx, b, update)
			return
		}

		customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
		if err != nil {
			slog.Error("legal gate: find customer", "error", err)
			return
		}
		if !CustomerNeedsLegalGate(customer) {
			next(ctx, b, update)
			return
		}

		if update.PreCheckoutQuery != nil {
			_, _ = b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
				PreCheckoutQueryID: update.PreCheckoutQuery.ID,
				OK:                 false,
				ErrorMessage:       h.translation.GetText(langCode, "legal_gate_declined"),
			})
			return
		}
		// Answer callback here: AnswerCallbackQueryMiddleware may not run if we don't call next.
		if update.CallbackQuery != nil {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
			})
		}
		h.editOrSendLegalGate(ctx, b, update, langCode, false)
	}
}

func (h Handler) LegalAcceptCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil {
		return
	}
	langCode := callback.From.LanguageCode
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil || customer == nil {
		slog.Error("legal accept: find customer", "error", err)
		return
	}
	if customer.LegalAcceptedAt == nil {
		now := time.Now().UTC()
		if err := h.customerRepository.SetLegalAcceptedAt(ctx, customer.ID, now); err != nil {
			slog.Error("legal accept: update", "error", err)
			return
		}
		customer.LegalAcceptedAt = &now
	}

	// После согласия — экран подписки на канал, если он включён.
	if h.channelGateBlocks(ctx, callback.From.ID) {
		h.showChannelGate(ctx, b, update, customer, langCode)
		return
	}

	displayName := buildDisplayName(callback.From.FirstName, callback.From.LastName, callback.From.Username)
	inlineKeyboard := h.buildStartKeyboard(customer, langCode)
	err = h.sendStartMenuAfterCallback(ctx, b, update, langCode, inlineKeyboard, customer, displayName)
	if err != nil {
		slog.Error("legal accept: start menu", "error", err)
	}
}

func (h Handler) LegalDeclineCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode
	h.editOrSendLegalGate(ctx, b, update, langCode, true)
}
