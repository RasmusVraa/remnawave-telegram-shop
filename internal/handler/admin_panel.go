package handler

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

// RenderAdminPanel edits an existing message to the admin root menu (broadcast, sync, promos).
func (h Handler) RenderAdminPanel(ctx context.Context, b *bot.Bot, msg *models.Message, lang string) error {
	if msg == nil {
		return nil
	}
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_users_submenu_button", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
			h.translation.WithButton(lang, "admin_tariffs", models.InlineKeyboardButton{CallbackData: CallbackAdminTariffs}),
		},
	}
	if config.LoyaltyEnabled() {
		kb = append(kb, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_promos", models.InlineKeyboardButton{CallbackData: CallbackAdminPromo}),
			h.translation.WithButton(lang, "admin_loyalty", models.InlineKeyboardButton{CallbackData: CallbackAdminLoyaltyRoot}),
		})
	} else {
		kb = append(kb, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_promos", models.InlineKeyboardButton{CallbackData: CallbackAdminPromo}),
		})
	}
	kb = append(kb,
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_broadcast", models.InlineKeyboardButton{CallbackData: CallbackAdminBroadcast}),
		},
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_stats", models.InlineKeyboardButton{CallbackData: CallbackAdminStatsRoot}),
			h.translation.WithButton(lang, "admin_infra_billing", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraRoot}),
		},
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_sync", models.InlineKeyboardButton{CallbackData: CallbackAdminSync}),
		},
		[]models.InlineKeyboardButton{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackStart}),
		},
	)
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "admin_panel_title"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	return err
}

// AdminPanelHandler shows admin menu (broadcast, sync, promos).
func (h Handler) AdminPanelHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if err := h.RenderAdminPanel(ctx, b, msg, lang); err != nil {
		slog.Error("admin panel edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminBroadcastShortcutHandler shows broadcast audience selection by editing the current message (same flow as /broadcast).
func (h Handler) AdminBroadcastShortcutHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	clearBroadcastState(cb.From.ID)
	broadcastState.mu.Lock()
	broadcastState.broadcastOpenedFromAdmin[cb.From.ID] = true
	broadcastState.mu.Unlock()
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_audience"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.BroadcastAudienceKeyboard(lang, true)}, nil)
	if err != nil {
		slog.Error("admin broadcast open", "error", err)
	}
}

// AdminSyncShortcutHandler runs Remnawave sync (same as /sync).
func (h Handler) AdminSyncShortcutHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            h.syncReport(ctx, update.CallbackQuery.From.LanguageCode),
		ShowAlert:       true,
	})
}
