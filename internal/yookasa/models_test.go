package yookasa

import (
	"testing"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/database"
)

func TestPaymentMatchesPurchase(t *testing.T) {
	id := uuid.New()
	other := uuid.New()
	purchase := func() *database.Purchase {
		return &database.Purchase{InvoiceType: database.InvoiceTypeYookasa, YookasaID: &id, Amount: 199}
	}
	payment := func(value string) *Payment {
		return &Payment{ID: id, Amount: Amount{Value: value, Currency: "RUB"}}
	}

	if err := payment("199.00").MatchesPurchase(purchase()); err != nil {
		t.Fatalf("matching payment rejected: %v", err)
	}
	// Счёт выставляется на целые рубли: 199.9 в покупке — платёж на 199.
	p := purchase()
	p.Amount = 199.9
	if err := payment("199.00").MatchesPurchase(p); err != nil {
		t.Fatalf("payment for int(amount) rejected: %v", err)
	}

	cases := map[string]struct {
		pay *Payment
		pur *database.Purchase
	}{
		"cheaper payment":  {payment("99.00"), purchase()},
		"another payment":  {&Payment{ID: other, Amount: Amount{Value: "199.00", Currency: "RUB"}}, purchase()},
		"other currency":   {&Payment{ID: id, Amount: Amount{Value: "199.00", Currency: "USD"}}, purchase()},
		"no yookasa id":    {payment("199.00"), &database.Purchase{InvoiceType: database.InvoiceTypeYookasa, Amount: 199}},
		"not yookassa":     {payment("199.00"), &database.Purchase{InvoiceType: database.InvoiceTypeCrypto, YookasaID: &id, Amount: 199}},
		"malformed amount": {payment("abc"), purchase()},
	}
	for name, c := range cases {
		if err := c.pay.MatchesPurchase(c.pur); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
