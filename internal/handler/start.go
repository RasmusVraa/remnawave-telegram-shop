package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func (h Handler) StartCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	langCode := update.Message.From.LanguageCode
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}

		// Источник, из которого пришёл клиент: партнёрская ссылка или
		// реферальная. Взаимоисключающие — см. attachStartSource.
		h.attachStartSource(ctx, update.Message.Text, existingCustomer)
	} else {
		updates := map[string]interface{}{
			"language": langCode,
		}

		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return
		}
	}

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "🧹",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})

	if err != nil {
		slog.Error("Error sending removing reply keyboard", "error", err)
		return
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: m.ID,
	})

	if err != nil {
		slog.Error("Error deleting message", "error", err)
		return
	}

	if CustomerNeedsLegalGate(existingCustomer) {
		h.sendLegalGateMessage(ctx, b, update.Message.Chat.ID, langCode, false)
		return
	}

	displayName := buildDisplayName(update.Message.From.FirstName, update.Message.From.LastName, update.Message.From.Username)
	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)
	err = h.sendStartMenu(ctx, b, update.Message.Chat.ID, langCode, inlineKeyboard, existingCustomer, displayName)
	logEditError("Error sending /start message", err)
}

func (h Handler) StartCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctxWithTime, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if CustomerNeedsLegalGate(existingCustomer) {
		h.editOrSendLegalGate(ctxWithTime, b, update, langCode, false)
		return
	}

	displayName := buildDisplayName(callback.From.FirstName, callback.From.LastName, callback.From.Username)
	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)
	err = h.sendStartMenuAfterCallback(ctxWithTime, b, update, langCode, inlineKeyboard, existingCustomer, displayName)
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) startMenuGreetingText(ctx context.Context, langCode string, cust *database.Customer, displayName string) string {
	if cabinetTelegramMinimalismActive() {
		if cust != nil && cust.SubscriptionLink != nil {
			return h.buildCabinetMinimalismGreetingHTML(ctx, cust, langCode, displayName)
		}
		return h.translation.GetText(langCode, "greeting")
	}
	return h.translation.GetText(langCode, "greeting")
}

func (h Handler) sendStartMenu(ctx context.Context, b *bot.Bot, chatID int64, langCode string, inlineKeyboard [][]models.InlineKeyboardButton, cust *database.Customer, displayName string) error {
	greeting := h.startMenuGreetingText(ctx, langCode, cust, displayName)
	markup := models.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}
	imageValue := config.GreetingImage()

	if imageValue != "" {
		if utf8.RuneCountInString(greeting) > 1024 {
			slog.Warn("Greeting text is too long for photo caption, fallback to text greeting", "caption_runes", utf8.RuneCountInString(greeting))
		} else {
			photo, cleanup, err := resolveGreetingImageInputFile(imageValue)
			if err != nil {
				slog.Warn("Invalid GREETING_IMAGE, fallback to text greeting", "value", imageValue, "error", err)
			} else {
				defer cleanup()
				_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
					ChatID:      chatID,
					Photo:       photo,
					Caption:     greeting,
					ParseMode:   models.ParseModeHTML,
					ReplyMarkup: markup,
				})
				if err == nil {
					return nil
				}
				slog.Warn("Failed to send greeting image, fallback to text greeting", "error", err)
			}
		}
	}

	linkPreviewOff := true
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &linkPreviewOff,
		},
		ReplyMarkup: markup,
		Text:        greeting,
	})
	return err
}

func (h Handler) sendStartMenuAfterCallback(ctx context.Context, b *bot.Bot, update *models.Update, langCode string, inlineKeyboard [][]models.InlineKeyboardButton, cust *database.Customer, displayName string) error {
	callback := update.CallbackQuery
	if callback == nil || callback.Message.Message == nil {
		return nil
	}

	msg := callback.Message.Message
	if config.GreetingImage() != "" {
		// Рассылка (?bc=1): не удалять исходное сообщение — иначе пропадает пост; новое меню только отдельным сообщением.
		if !IsCallbackFromBroadcast(callback.Data) {
			_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
			})
		}
		return h.sendStartMenu(ctx, b, msg.Chat.ID, langCode, inlineKeyboard, cust, displayName)
	}

	linkPreviewOff := true
	lp := &models.LinkPreviewOptions{IsDisabled: &linkPreviewOff}
	return SendOrEditAfterInlineCallback(ctx, b, update, h.startMenuGreetingText(ctx, langCode, cust, displayName), models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: inlineKeyboard,
	}, lp)
}

func resolveGreetingImageInputFile(raw string) (models.InputFile, func(), error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, func() {}, os.ErrInvalid
	}
	lv := strings.ToLower(value)
	if strings.HasPrefix(lv, "http://") || strings.HasPrefix(lv, "https://") {
		return &models.InputFileString{Data: value}, func() {}, nil
	}
	var absPath string
	if !filepath.IsAbs(value) {
		absPath = filepath.Clean(filepath.Join(".", value))
	} else {
		absPath = filepath.Clean(value)
	}
	file, err := os.Open(absPath)
	if err != nil {
		return nil, func() {}, err
	}
	return &models.InputFileUpload{
		Filename: filepath.Base(absPath),
		Data:     file,
	}, func() {
		_ = file.Close()
	}, nil
}

func (h Handler) resolveConnectButton(lang string) []models.InlineKeyboardButton {
	// При включённом кабинете «Мой VPN» сразу открывает WebApp кабинета (тот же функционал, что подменю).
	if u := cabinetWebAppURL("/cabinet/dashboard"); u != "" {
		return []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "connect_button", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: u},
			}),
		}
	}
	return []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "connect_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
	}
}

func (h Handler) HelpCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery

	if callback == nil || callback.Message.Message == nil {
		slog.Error("HelpCallbackHandler: callback.Message.Message is nil")
		return
	}

	langCode := callback.From.LanguageCode
	var helpKeyboard [][]models.InlineKeyboardButton

	// Ряд 1: «Какой сервер выбрать», «Видеоинструкция»
	var serverVideoRow []models.InlineKeyboardButton
	if config.ServerSelectionURL() != "" {
		serverVideoRow = append(serverVideoRow, h.translation.WithButton(langCode, "server_selection_button", models.InlineKeyboardButton{
			URL: config.ServerSelectionURL(),
		}))
	}
	if config.VideoGuideURL() != "" {
		serverVideoRow = append(serverVideoRow, h.translation.WithButton(langCode, "video_guide_button", models.InlineKeyboardButton{
			URL: config.VideoGuideURL(),
		}))
	}
	if len(serverVideoRow) > 0 {
		helpKeyboard = append(helpKeyboard, serverVideoRow)
	}

	// Ряд 2: только «Поддержка»
	if config.SupportURL() != "" {
		helpKeyboard = append(helpKeyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "support_button", models.InlineKeyboardButton{URL: config.SupportURL()}),
		})
	}

	// Ряд 3: «Публичная оферта», «Политика конфиденциальности»
	var offerPrivacyRow []models.InlineKeyboardButton
	if config.PublicOfferURL() != "" {
		offerPrivacyRow = append(offerPrivacyRow, h.translation.WithButton(langCode, "public_offer_button", models.InlineKeyboardButton{
			URL: config.PublicOfferURL(),
		}))
	}
	if config.PrivacyPolicyURL() != "" {
		offerPrivacyRow = append(offerPrivacyRow, h.translation.WithButton(langCode, "privacy_policy_button", models.InlineKeyboardButton{
			URL: config.PrivacyPolicyURL(),
		}))
	}
	if len(offerPrivacyRow) > 0 {
		helpKeyboard = append(helpKeyboard, offerPrivacyRow)
	}

	// Ряд 4: «Пользовательское соглашение»
	if config.TermsOfServiceURL() != "" {
		helpKeyboard = append(helpKeyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "terms_of_service_button", models.InlineKeyboardButton{URL: config.TermsOfServiceURL()}),
		})
	}

	// Кнопка "Назад"
	helpKeyboard = append(helpKeyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
	})

	_, err := editCallbackOriginToHTMLText(ctx, b, callback.Message.Message, h.translation.GetText(langCode, "help_title"), models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: helpKeyboard,
	}, nil)
	logEditError("Error sending help message", err)
}

func (h Handler) buildStartKeyboard(existingCustomer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	if cabinetTelegramMinimalismActive() {
		kb := h.buildCabinetMinimalismCoreRows(langCode, existingCustomer)
		if existingCustomer != nil && existingCustomer.TelegramID == config.GetAdminTelegramId() {
			kb = append(kb, h.adminStartKeyboardRow(langCode))
		}
		return kb
	}

	var inlineKeyboard [][]models.InlineKeyboardButton

	// 1. Попробовать бесплатно (если юзер новый)
	if existingCustomer.SubscriptionLink == nil && config.TrialDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "trial_button", models.InlineKeyboardButton{CallbackData: CallbackTrial}),
		})
	}

	// 2. Купить (всегда показывается)
	inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "buy_button", models.InlineKeyboardButton{CallbackData: CallbackBuy}),
	})

	// 3. Подключиться (если есть подписка)
	if existingCustomer.SubscriptionLink != nil {
		inlineKeyboard = append(inlineKeyboard, h.resolveConnectButton(langCode))
	}

	// 4. Собираем кнопки для 2-в-ряд: отзывы, канал
	var secondRow []models.InlineKeyboardButton
	if config.FeedbackURL() != "" {
		secondRow = append(secondRow, h.translation.WithButton(langCode, "feedback_button", models.InlineKeyboardButton{URL: config.FeedbackURL()}))
	}
	if config.ChannelURL() != "" {
		secondRow = append(secondRow, h.translation.WithButton(langCode, "channel_button", models.InlineKeyboardButton{URL: config.ChannelURL()}))
	}
	if len(secondRow) > 0 {
		inlineKeyboard = append(inlineKeyboard, secondRow)
	}

	inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "promo_code_button", models.InlineKeyboardButton{CallbackData: CallbackEnterPromo}),
	})

	inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "help_button", models.InlineKeyboardButton{CallbackData: "help"}),
	})

	if existingCustomer.TelegramID == config.GetAdminTelegramId() {
		inlineKeyboard = append(inlineKeyboard, h.adminStartKeyboardRow(langCode))
	}

	return inlineKeyboard
}

// adminStartKeyboardRow — одна строка кнопок админа: TG-админка и/или WebApp кабинета (/cabinet/admin).
func (h Handler) adminStartKeyboardRow(langCode string) []models.InlineKeyboardButton {
	if u := cabinetWebAppURL("/cabinet/admin"); u != "" {
		return []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "admin_panel_button_tg", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel}),
			h.translation.WithButton(langCode, "admin_panel_button_cab", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: u},
			}),
		}
	}
	return []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "admin_panel_button", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel}),
	}
}
