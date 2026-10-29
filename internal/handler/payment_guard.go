package handler

import (
	"context"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// callback_data присылает клиент Telegram. Кнопки бота задают её сами, но
// модифицированный клиент может прислать любую строку, поэтому всё, что влияет
// на цену и способ оплаты, сверяется здесь с тем, что бот показал бы сам.

// validSubscriptionMonths — периоды, которые продаёт бот (те же, что в кабинете
// и в CHECK таблицы tariff_price).
func validSubscriptionMonths(months int) bool {
	switch months {
	case 1, 3, 6, 12:
		return true
	}
	return false
}

// invoiceTypeAllowed — способ оплаты, кнопку которого бот показал бы этому клиенту.
func (h Handler) invoiceTypeAllowed(ctx context.Context, customer *database.Customer, t database.InvoiceType) bool {
	switch t {
	case database.InvoiceTypeCrypto:
		return config.IsCryptoPayEnabled()
	case database.InvoiceTypeYookasa:
		return config.IsYookasaEnabled()
	case database.InvoiceTypeHeleket:
		return config.IsHeleketEnabled()
	case database.InvoiceTypeTelegram:
		if !config.IsTelegramStarsEnabled() {
			return false
		}
		if !config.RequirePaidPurchaseForStars() {
			return true
		}
		if customer == nil {
			return false
		}
		paid, err := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
		if err != nil {
			slog.Error("payment guard: check paid purchase for stars", "error", err)
			return false
		}
		return paid != nil
	default:
		// Tribute сюда не попадает: его кнопка — ссылка на Tribute, а не callback,
		// и счёт Tribute через бота не создаётся. Подделанный invoiceType=tribute
		// раньше давал pending-покупку, которую ночное автопродление Tribute
		// проводило без оплаты.
		return plategaInvoiceEnabled(t)
	}
}

// extraHwidAllowed — число доп. устройств в счёте в пределах того, что можно купить.
func extraHwidAllowed(extra int) bool {
	if extra < 0 {
		return false
	}
	if maxDevices := config.HwidMaxDevices(); maxDevices > 0 && extra > maxDevices {
		return false
	}
	return true
}

// rejectForgedCallback пишет в лог callback, которого бот не мог выдать сам.
func rejectForgedCallback(handler, reason string, telegramID int64, data string) {
	slog.Warn("payment callback rejected", "handler", handler, "reason", reason, "telegram_id", utils.MaskHalfInt64(telegramID), "data", data)
}
