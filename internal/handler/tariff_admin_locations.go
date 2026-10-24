package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

func (h Handler) AdminTariffLocationsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	msg := cb.Message.Message
	if msg == nil || h.tariffRepository == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	prefix := strings.SplitN(cb.Data, "?", 2)[0]
	q := parseCallbackData(cb.Data)
	tariffID, _ := strconv.ParseInt(q["i"], 10, 64)
	locID, _ := strconv.ParseInt(q["l"], 10, 64)
	if tariffID <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	lang := cb.From.LanguageCode
	switch prefix {
	case tariffCallbackLocTog:
		if err := h.tariffRepository.ToggleLocationSoldOut(ctx, tariffID, locID); err != nil {
			slog.Error("toggle tariff location", "error", err)
		}
	case tariffCallbackLocDel:
		if err := h.tariffRepository.DeleteLocation(ctx, tariffID, locID); err != nil {
			slog.Error("delete tariff location", "error", err)
		}
	case tariffCallbackLocAdd:
		adminTariffReset(cb.From.ID)
		adminTariffCtx.mu.Lock()
		adminTariffCtx.mode[cb.From.ID] = "edit"
		adminTariffCtx.editID[cb.From.ID] = tariffID
		adminTariffCtx.editField[cb.From.ID] = "location"
		adminTariffCtx.mu.Unlock()
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(lang, "tariff_locations_prompt"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
				h.translation.WithButton(lang, "tariff_btn_cancel_edit", models.InlineKeyboardButton{
					CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackCancel, tariffID),
				}),
			}}},
		})
		if err != nil {
			slog.Error("tariff location prompt", "error", err)
		}
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	h.sendAdminTariffLocations(ctx, b, msg.Chat.ID, msg.ID, lang, tariffID)
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func (h Handler) sendAdminTariffLocations(ctx context.Context, b *bot.Bot, chatID int64, messageID int, lang string, tariffID int64) {
	locs, err := h.tariffRepository.ListLocations(ctx, tariffID)
	if err != nil {
		slog.Error("list tariff locations", "error", err)
		return
	}
	var text strings.Builder
	text.WriteString(h.translation.GetText(lang, "tariff_locations_title"))
	if len(locs) == 0 {
		text.WriteString(h.translation.GetText(lang, "tariff_locations_empty"))
	} else {
		text.WriteString(h.translation.GetText(lang, "tariff_locations_hint"))
	}
	keyboard := make([][]models.InlineKeyboardButton, 0, len(locs)+2)
	sold := h.translation.GetText(lang, "tariff_location_sold_out")
	for _, loc := range locs {
		label := loc.Name
		if loc.SoldOut {
			label = sold + " · " + loc.Name
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("%s?i=%d&l=%d", tariffCallbackLocTog, tariffID, loc.ID)},
			{Text: "✕", CallbackData: fmt.Sprintf("%s?i=%d&l=%d", tariffCallbackLocDel, tariffID, loc.ID)},
		})
	}
	keyboard = append(keyboard,
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "tariff_locations_add", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackLocAdd, tariffID),
			}),
		},
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackView, tariffID),
			}),
		},
	)
	markup := models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	if messageID > 0 {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			ParseMode:   models.ParseModeHTML,
			Text:        text.String(),
			ReplyMarkup: markup,
		})
	} else {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			ParseMode:   models.ParseModeHTML,
			Text:        text.String(),
			ReplyMarkup: markup,
		})
	}
	if err != nil {
		slog.Error("tariff locations screen", "error", err)
	}
}

func parseTariffLocationInput(text string) (string, *string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", nil, false
	}
	if len(fields) >= 2 && len([]rune(fields[0])) == 2 && isLatinCode(fields[0]) {
		code := strings.ToUpper(fields[0])
		name := strings.Join(fields[1:], " ")
		return name, &code, name != ""
	}
	name := strings.Join(fields, " ")
	if code, ok := countryCodeByName(name); ok {
		return name, &code, true
	}
	return "", nil, false
}

func isLatinCode(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func countryCodeByName(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "германия", "germany":
		return "DE", true
	case "нидерланды", "голландия", "netherlands":
		return "NL", true
	case "финляндия", "finland":
		return "FI", true
	case "эстония", "estonia":
		return "EE", true
	case "латвия", "latvia":
		return "LV", true
	case "литва", "lithuania":
		return "LT", true
	case "польша", "poland":
		return "PL", true
	case "швеция", "sweden":
		return "SE", true
	case "франция", "france":
		return "FR", true
	case "великобритания", "англия", "uk", "united kingdom":
		return "GB", true
	case "сша", "usa", "united states":
		return "US", true
	case "казахстан", "kazakhstan":
		return "KZ", true
	case "турция", "turkey":
		return "TR", true
	default:
		return "", false
	}
}
