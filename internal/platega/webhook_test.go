package platega

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

func TestCheckTransaction(t *testing.T) {
	txID := "tx-1"
	pur := func() *database.Purchase {
		id := txID
		return &database.Purchase{InvoiceType: database.InvoiceTypePlategaSBP, PlategaID: &id, Amount: 199}
	}
	tx := func(id string, amount float64) *TransactionStatusResponse {
		return &TransactionStatusResponse{Id: id, Status: StatusConfirmed, PaymentDetails: PaymentDetails{Amount: amount, Currency: "RUB"}}
	}

	if err := CheckTransaction(tx(txID, 199), pur()); err != nil {
		t.Fatalf("matching transaction rejected: %v", err)
	}
	// Комиссия провайдера может увеличить сумму — это не повод отказывать.
	if err := CheckTransaction(tx(txID, 205), pur()); err != nil {
		t.Fatalf("transaction with commission rejected: %v", err)
	}
	if err := CheckTransaction(tx("tx-other", 199), pur()); err == nil {
		t.Error("transaction of another purchase accepted")
	}
	if err := CheckTransaction(tx(txID, 99), pur()); err == nil {
		t.Error("cheaper transaction accepted")
	}
	notPlatega := pur()
	notPlatega.InvoiceType = database.InvoiceTypeYookasa
	if err := CheckTransaction(tx(txID, 199), notPlatega); err == nil {
		t.Error("non-Platega purchase accepted")
	}
}

// Неверный секрет — 401 до чтения тела и обращения к БД (репозиторий nil).
func TestWebhook_RejectsWrongSecret(t *testing.T) {
	h := NewWebhookHandler(nil, nil, nil, "merchant", "secret")
	for _, c := range []struct{ merchant, secret string }{
		{"merchant", "secreT"}, {"merchant", "secret2"}, {"other", "secret"}, {"", ""},
	} {
		r := httptest.NewRequest(http.MethodPost, "/platega", strings.NewReader(`{}`))
		r.Header.Set("X-MerchantId", c.merchant)
		r.Header.Set("X-Secret", c.secret)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("merchant=%q secret=%q: status %d, want 401", c.merchant, c.secret, rec.Code)
		}
	}
}
