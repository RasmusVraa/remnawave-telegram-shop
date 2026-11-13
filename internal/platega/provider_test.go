package platega

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

func createVia(t *testing.T, it database.InvoiceType, response string) (path string, body map[string]json.RawMessage, redirect string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Write([]byte(response))
	}))
	defer server.Close()

	client := NewClient("merchant", "secret")
	client.baseURL = server.URL
	provider, err := ProviderFor(client, it)
	if err != nil {
		t.Fatalf("provider for %s: %v", it, err)
	}
	redirect, _, err = provider.CreateInvoice(context.Background(), 42, 100, "RUB", "test", "https://t.me/bot", "")
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	return path, body, redirect
}

func TestGeneralFormSendsNoPaymentMethod(t *testing.T) {
	path, body, redirect := createVia(t, database.InvoiceTypePlatega,
		`{"transactionId":"tx-1","status":"PENDING","url":"https://pay.platega.io/?id=tx-1"}`)
	if path != "/v2/transaction/process" {
		t.Fatalf("expected v2 endpoint, got %s", path)
	}
	if _, ok := body["paymentMethod"]; ok {
		t.Fatalf("paymentMethod must not be sent for the general form")
	}
	if redirect != "https://pay.platega.io/?id=tx-1" {
		t.Fatalf("expected link from url, got %q", redirect)
	}
}

func TestSpecificMethodsSendDocumentedCodes(t *testing.T) {
	cases := map[database.InvoiceType]string{
		database.InvoiceTypePlategaSBP:       "2",
		database.InvoiceTypePlategaAcquiring: "11",
		database.InvoiceTypePlategaWorldwide: "12",
		database.InvoiceTypePlategaCrypto:    "13",
	}
	for it, want := range cases {
		path, body, redirect := createVia(t, it, `{"transactionId":"tx-1","redirect":"https://pay.test/tx-1"}`)
		if path != "/transaction/process" {
			t.Errorf("%s: unexpected endpoint %s", it, path)
		}
		if string(body["paymentMethod"]) != want {
			t.Errorf("%s: paymentMethod %s, want %s", it, body["paymentMethod"], want)
		}
		if redirect != "https://pay.test/tx-1" {
			t.Errorf("%s: link from redirect, got %q", it, redirect)
		}
	}
}

func TestRetiredCardsMethodIsRejected(t *testing.T) {
	if _, err := ProviderFor(NewClient("m", "s"), database.InvoiceTypePlategaCards); err == nil {
		t.Fatal("plt_cards (method 10) must not create new invoices")
	}
}
