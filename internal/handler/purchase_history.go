package handler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

const purchaseHistoryPageSize = 10

func (h Handler) PurchaseHistoryCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return
	}

	callbackMessage := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	page := parseHistoryPage(update.CallbackQuery.Data)
	if page < 1 {
		page = 1
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID))
		return
	}

	total, err := h.purchaseRepository.CountPaidByCustomer(ctx, customer.ID)
	if err != nil {
		slog.Error("Error counting purchases", "error", err, "customer_id", utils.MaskHalfInt64(customer.ID))
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(purchaseHistoryPageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * purchaseHistoryPageSize
	purchases, err := h.purchaseRepository.FindPaidByCustomer(ctx, customer.ID, purchaseHistoryPageSize, offset)
	if err != nil {
		slog.Error("Error fetching purchases", "error", err, "customer_id", utils.MaskHalfInt64(customer.ID))
		return
	}

	text := h.buildPurchaseHistoryText(ctx, langCode, purchases, page, totalPages)
	markup := buildPurchaseHistoryMarkup(langCode, page, totalPages)

	isDisabled := true
	lp := &models.LinkPreviewOptions{IsDisabled: &isDisabled}
	_, err = editCallbackOriginToHTMLText(ctx, b, callbackMessage, text, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: markup,
	}, lp)
	logEditError("Error sending purchase history message", err)
}

func parseHistoryPage(data string) int {
	if !strings.HasPrefix(data, CallbackPurchaseHistory) {
		return 1
	}
	parts := strings.SplitN(data, "?", 2)
	if len(parts) != 2 {
		return 1
	}
	query := strings.Split(parts[1], "&")
	for _, item := range query {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 || kv[0] != "page" {
			continue
		}
		page, err := strconv.Atoi(kv[1])
		if err != nil {
			return 1
		}
		return page
	}
	return 1
}

func (h Handler) buildPurchaseHistoryText(ctx context.Context, langCode string, purchases []database.Purchase, page, totalPages int) string {
	tm := translation.GetInstance()
	title := tm.GetText(langCode, "purchase_history_title")
	if totalPages > 1 {
		title = fmt.Sprintf("%s (%d/%d)", title, page, totalPages)
	}
	if len(purchases) == 0 {
		return title + "\n\n" + tm.GetText(langCode, "purchase_history_empty")
	}

	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")

	for i, p := range purchases {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		amount := formatAmount(p.Amount, p.Currency)
		method := purchaseInvoiceLabel(langCode, p.InvoiceType)
		emoji := purchaseMethodEmoji(p.InvoiceType)
		sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_amount"), emoji, method, amount))
		sb.WriteString("\n")
		if p.TariffID != nil && *p.TariffID > 0 && h.tariffRepository != nil {
			if tr, err := h.tariffRepository.GetByID(ctx, *p.TariffID); err == nil && tr != nil {
				tname := escapeHTML(displayTariffName(tr))
				sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_tariff_line"), tname))
				sb.WriteString("\n")
			}
		}
		if p.ExtraHwid > 0 && p.Month > 0 {
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_subscription_combo"), formatMonthLabel(langCode, p.Month), p.ExtraHwid))
		} else if p.ExtraHwid > 0 {
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_subscription_hwid"), p.ExtraHwid))
		} else {
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_subscription"), formatMonthLabel(langCode, p.Month)))
		}
		sb.WriteString("\n")
		if p.PaidAt != nil {
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_date"), p.PaidAt.Format("02.01.2006 15:04")))
		} else {
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_date"), tm.GetText(langCode, "vpn_not_available")))
		}
		if config.LoyaltyEnabled() {
			xp := loyalty.XPRubEquivalentForPurchase(&p)
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(tm.GetText(langCode, "purchase_history_loyalty_xp"), xp))
		}
	}

	return sb.String()
}

func buildPurchaseHistoryMarkup(langCode string, page, totalPages int) [][]models.InlineKeyboardButton {
	var markup [][]models.InlineKeyboardButton
	var paginationRow []models.InlineKeyboardButton

	if page > 1 {
		paginationRow = append(paginationRow, models.InlineKeyboardButton{
			Text:         translation.GetInstance().GetText(langCode, "purchase_history_prev"),
			CallbackData: fmt.Sprintf("%s?page=%d", CallbackPurchaseHistory, page-1),
		})
	}
	if page < totalPages {
		paginationRow = append(paginationRow, models.InlineKeyboardButton{
			Text:         translation.GetInstance().GetText(langCode, "purchase_history_next"),
			CallbackData: fmt.Sprintf("%s?page=%d", CallbackPurchaseHistory, page+1),
		})
	}
	if len(paginationRow) > 0 {
		markup = append(markup, paginationRow)
	}

	markup = append(markup, []models.InlineKeyboardButton{
		translation.GetInstance().WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
	})
	return markup
}

func purchaseInvoiceLabel(langCode string, invoiceType database.InvoiceType) string {
	tm := translation.GetInstance()
	switch invoiceType {
	case database.InvoiceTypeYookasa:
		return tm.GetText(langCode, "purchase_history_method_yookasa")
	case database.InvoiceTypeTelegram:
		return tm.GetText(langCode, "purchase_history_method_stars")
	case database.InvoiceTypeCrypto:
		return tm.GetText(langCode, "purchase_history_method_crypto")
	case database.InvoiceTypeTribute:
		return tm.GetText(langCode, "purchase_history_method_tribute")
	case database.InvoiceTypePlatega:
		return "Platega"
	case database.InvoiceTypePlategaSBP:
		return tm.GetText(langCode, "purchase_history_method_platega_sbp")
	case database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring, database.InvoiceTypePlategaWorldwide:
		return tm.GetText(langCode, "purchase_history_method_platega_card")
	case database.InvoiceTypePlategaCrypto:
		return tm.GetText(langCode, "purchase_history_method_platega_crypto")
	case database.InvoiceTypeHeleket:
		return tm.GetText(langCode, "purchase_history_method_heleket")
	default:
		return tm.GetText(langCode, "purchase_history_method_unknown")
	}
}

func formatAmount(amount float64, currency string) string {
	formatted := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", amount), "0"), ".")
	if currency == "" || strings.EqualFold(currency, "RUB") {
		return fmt.Sprintf("+%s ₽", formatted)
	}
	if strings.EqualFold(currency, "XTR") || strings.EqualFold(currency, "STARS") {
		return fmt.Sprintf("+%s STARS", formatted)
	}
	return fmt.Sprintf("+%s %s", formatted, strings.ToUpper(currency))
}

func formatMonthLabel(langCode string, month int) string {
	if strings.ToLower(langCode) == "en" {
		switch month {
		case 1:
			return "1 mo."
		case 3:
			return "3 mo."
		case 6:
			return "6 mo."
		case 12:
			return "12 mo."
		default:
			return fmt.Sprintf("%d mo.", month)
		}
	}
	switch month {
	case 1:
		return "1 мес."
	case 3:
		return "3 мес."
	case 6:
		return "6 мес."
	case 12:
		return "12 мес."
	default:
		return fmt.Sprintf("%d мес.", month)
	}
}

func purchaseMethodEmoji(invoiceType database.InvoiceType) string {
	switch invoiceType {
	case database.InvoiceTypeTelegram:
		return "⭐️"
	case database.InvoiceTypeCrypto:
		return "₿"
	default:
		return "💰"
	}
}
