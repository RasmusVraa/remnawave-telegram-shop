package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/database"
)

// parseStarsPayload разбирает payload счёта Stars вида "<purchaseId>&<username>".
// Раньше payload без «&» ронял обработчик: обращение к payload[1].
func parseStarsPayload(payload string) (purchaseID int64, username string, err error) {
	idPart, user, _ := strings.Cut(payload, "&")
	purchaseID, err = strconv.ParseInt(idPart, 10, 64)
	if err != nil || purchaseID <= 0 {
		return 0, "", fmt.Errorf("invalid stars payload %q", payload)
	}
	return purchaseID, user, nil
}

// checkStarsPurchase проверяет, что платёж Stars относится к открытому счёту
// Stars на ту же сумму. Вызывается в pre_checkout (до списания — можно
// отказать) и после оплаты (зачисляем только совпавшее).
func (h Handler) checkStarsPurchase(ctx context.Context, payload, currency string, totalAmount int) (*database.Purchase, string, error) {
	purchaseID, username, err := parseStarsPayload(payload)
	if err != nil {
		return nil, "", err
	}
	if currency != "XTR" {
		return nil, "", fmt.Errorf("currency %q, want XTR", currency)
	}
	purchase, err := h.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return nil, "", fmt.Errorf("find purchase %d: %w", purchaseID, err)
	}
	if purchase == nil {
		return nil, "", fmt.Errorf("purchase %d not found", purchaseID)
	}
	if purchase.InvoiceType != database.InvoiceTypeTelegram {
		return nil, "", fmt.Errorf("purchase %d invoice type %q", purchaseID, purchase.InvoiceType)
	}
	// Счёт выставляется на int(amount) — см. PaymentService.createTelegramInvoice.
	if totalAmount != int(purchase.Amount) {
		return nil, "", fmt.Errorf("purchase %d amount %d, paid %d", purchaseID, int(purchase.Amount), totalAmount)
	}
	return purchase, username, nil
}
