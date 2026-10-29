package handler

import (
	"context"
	"fmt"
	"strings"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func (h Handler) LoyaltyRootCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !config.LoyaltyEnabled() || h.loyaltyTierRepository == nil {
		return
	}
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("loyalty screen customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("loyalty screen customer nil", "telegramId", update.CallbackQuery.From.ID)
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode
	callbackMessage := update.CallbackQuery.Message.Message
	text := h.buildLoyaltyScreenHTML(ctx, customer, langCode)
	_, err = editCallbackOriginToHTMLText(ctx, b, callbackMessage, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
		},
	}}, nil)
	logEditError("loyalty screen edit", err)
}

func (h Handler) buildLoyaltyScreenHTML(ctx context.Context, customer *database.Customer, lang string) string {
	tm := h.translation
	if h.loyaltyTierRepository == nil {
		return tm.GetText(lang, "loyalty_screen_unavailable")
	}
	prog, err := h.loyaltyTierRepository.ProgressForXP(ctx, customer.LoyaltyXP)
	if err != nil {
		slog.Error("loyalty progress screen", "error", err)
		return tm.GetText(lang, "loyalty_screen_unavailable")
	}
	cur := prog.CurrentTier
	var b strings.Builder
	if cur.SortOrder == 0 && cur.DiscountPercent == 0 && prog.NextTier != nil {
		b.WriteString(tm.GetText(lang, "loyalty_screen_level0_hint_short"))
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_head_level"), cur.SortOrder))
	b.WriteString("\n\n")
	if prog.NextTier != nil {
		next := prog.NextTier
		need := next.XpMin - customer.LoyaltyXP
		if need < 0 {
			need = 0
		}
		ratio := loyaltySegmentProgressRatio(customer.LoyaltyXP, cur.XpMin, next.XpMin)
		bar := loyaltyProgressBarASCII(ratio, loyaltyProgressBarWidth)
		pct := loyaltyPercentInt(ratio)
		earnedInLevel, spanToNext := loyaltyWithinLevelXP(customer.LoyaltyXP, cur.XpMin, next.XpMin)
		if spanToNext <= 0 {
			b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_xp_total_only"), customer.LoyaltyXP))
		} else {
			b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_xp_fraction"), earnedInLevel, spanToNext))
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_progress_bar_line"), bar, pct))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_until_next"), next.SortOrder, need))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_discount_emoji"), cur.DiscountPercent))
	} else {
		bar := loyaltyProgressBarASCII(1, loyaltyProgressBarWidth)
		pct := loyaltyPercentInt(1.0)
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_xp_total_only"), customer.LoyaltyXP))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_progress_bar_line"), bar, pct))
		b.WriteString("\n")
		b.WriteString(tm.GetText(lang, "loyalty_screen_max_level_short"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_screen_discount_emoji"), cur.DiscountPercent))
	}
	b.WriteString("\n\n")
	b.WriteString(tm.GetText(lang, "loyalty_screen_how_it_works"))
	return b.String()
}

// buildLoyaltyConnectSummaryHTML — краткий блок лояльности для экрана «Мой VPN».
func (h Handler) buildLoyaltyConnectSummaryHTML(lang string, customer *database.Customer, prog database.LoyaltyProgress) string {
	tm := h.translation
	cur := prog.CurrentTier
	var b strings.Builder
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_connect_head_level"), cur.SortOrder))
	b.WriteString("\n")
	var ratio float64 = 1
	if prog.NextTier != nil {
		next := prog.NextTier
		ratio = loyaltySegmentProgressRatio(customer.LoyaltyXP, cur.XpMin, next.XpMin)
	}
	bar := loyaltyProgressBarASCII(ratio, loyaltyProgressBarWidth)
	pct := loyaltyPercentInt(ratio)
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_connect_progress_bar_line"), bar, pct))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_connect_discount"), cur.DiscountPercent))
	return b.String()
}

