package payments

import (
	"context"
	"errors"
	"fmt"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// LastMethod — id кнопки кабинета, которой клиент оплатил последнюю покупку
// (в боте или кабинете): "yookassa:sbp", "platega_sbp", "telegram"… Пусто —
// оплат ещё не было или способа в кабинете нет.
//
// Включён ли этот способ сейчас, не проверяем: фронт и так знает список
// включённых и просто не подставит выключенный.
func (s *CheckoutService) LastMethod(ctx context.Context, accountID int64) (string, error) {
	link, err := s.links.FindByAccountID(ctx, accountID)
	if err != nil {
		// Без привязки к клиенту оплат быть не может; создавать web-only
		// клиента ради ответа «ничего» незачем.
		if errors.Is(err, repository.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("payments: find link: %w", err)
	}
	invoiceType, yookassaMethod, err := s.purchases.LastPaidMethod(ctx, link.CustomerID)
	if err != nil {
		return "", err
	}
	return checkoutMethodForInvoice(invoiceType, yookassaMethod), nil
}

// checkoutMethodForInvoice — обратное к mapProviderToInvoiceType, плюс способ
// ЮKassa. Покупки ЮKassa до появления колонки и через форму со всеми
// способами — "yookassa:all".
func checkoutMethodForInvoice(t database.InvoiceType, yookassaMethod *string) string {
	switch t {
	case database.InvoiceTypeYookasa:
		m := config.YookasaPaymentMethodAll
		if yookassaMethod != nil && *yookassaMethod != "" {
			m = *yookassaMethod
		}
		return config.CheckoutMethodYookassaPrefix + m
	case database.InvoiceTypeCrypto:
		return repository.CheckoutProviderCryptoPay
	case database.InvoiceTypeTelegram:
		return repository.CheckoutProviderTelegram
	case database.InvoiceTypePlategaSBP:
		return repository.CheckoutProviderPlategaSBP
	case database.InvoiceTypePlategaCards:
		return repository.CheckoutProviderPlategaCards
	case database.InvoiceTypePlategaAcquiring:
		return repository.CheckoutProviderPlategaAcquiring
	case database.InvoiceTypePlategaWorldwide:
		return repository.CheckoutProviderPlategaWorldwide
	case database.InvoiceTypePlategaCrypto:
		return repository.CheckoutProviderPlategaCrypto
	case database.InvoiceTypeHeleket:
		return repository.CheckoutProviderHeleket
	}
	return ""
}
