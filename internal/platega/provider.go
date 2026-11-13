package platega

import (
	"context"
	"fmt"
	"strings"

	"remnawave-tg-shop-bot/internal/database"
)

// Provider создаёт ссылку на оплату без заранее выбранного способа: СБП, карту
// или другой включённый в кабинете Platega способ плательщик выбирает на
// странице оплаты.
type Provider struct {
	client        *Client
	invoiceType   database.InvoiceType
	paymentMethod PaymentMethod
}

func (p *Provider) InvoiceType() database.InvoiceType {
	return p.invoiceType
}

func (p *Provider) IsConfigured() bool {
	return p.client != nil && p.client.IsConfigured()
}

func (p *Provider) CreateInvoice(
	ctx context.Context,
	purchaseID int64,
	amount float64,
	currency string,
	description string,
	returnURL string,
	username string,
) (redirectURL string, transactionID string, err error) {
	if p.client == nil {
		return "", "", fmt.Errorf("platega client not configured")
	}
	u := strings.ReplaceAll(strings.ReplaceAll(username, "&", ""), "=", "")
	payload := fmt.Sprintf("purchaseId=%d", purchaseID)
	if u != "" {
		payload += "&username=" + u
	}

	resp, err := p.client.CreateTransaction(ctx, &CreateTransactionRequest{
		PaymentMethod: p.paymentMethod,
		PaymentDetails: PaymentDetails{
			Amount:   amount,
			Currency: currency,
		},
		Description: description,
		Return:      returnURL,
		FailedUrl:   returnURL,
		Payload:     payload,
	})
	if err != nil {
		return "", "", fmt.Errorf("create platega transaction: %w", err)
	}

	return resp.PayURL(), resp.TransactionId, nil
}

// Общая форма (InvoiceTypePlatega) — без метода; остальные — с кодом способа.
var invoiceTypeToMethod = map[database.InvoiceType]PaymentMethod{
	database.InvoiceTypePlatega:          0,
	database.InvoiceTypePlategaSBP:       PaymentMethodSBPQR,
	database.InvoiceTypePlategaAcquiring: PaymentMethodCardAcquiring,
	database.InvoiceTypePlategaWorldwide: PaymentMethodInternationalAcquiring,
	database.InvoiceTypePlategaCrypto:    PaymentMethodCrypto,
}

func ProviderFor(client *Client, invoiceType database.InvoiceType) (*Provider, error) {
	method, ok := invoiceTypeToMethod[invoiceType]
	if !ok {
		return nil, fmt.Errorf("unsupported platega invoice type: %s", invoiceType)
	}
	return &Provider{client: client, invoiceType: invoiceType, paymentMethod: method}, nil
}

// methodIDs — id способа в PLATEGA_PAYMENT_METHODS для каждого типа счёта.
var methodIDs = map[database.InvoiceType]string{
	database.InvoiceTypePlatega:          "all",
	database.InvoiceTypePlategaSBP:       "sbp",
	database.InvoiceTypePlategaAcquiring: "acquiring",
	database.InvoiceTypePlategaWorldwide: "worldwide",
	database.InvoiceTypePlategaCrypto:    "crypto",
}

// InvoiceTypeForMethod — тип счёта для id способа; пусто, если способа нет.
func InvoiceTypeForMethod(id string) (database.InvoiceType, bool) {
	for t, m := range methodIDs {
		if m == id {
			return t, true
		}
	}
	return "", false
}

// MethodEnabled — можно ли сейчас создавать счёт этого типа: Platega включена
// и способ есть в списке кнопок. Прежний plt_cards новых счетов не создаёт.
func MethodEnabled(t database.InvoiceType, enabled func(id string) bool) bool {
	id, ok := methodIDs[t]
	return ok && enabled(id)
}
