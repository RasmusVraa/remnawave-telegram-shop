package yookasa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Способ из контекста должен доехать до ЮKassa в payment_method_data, а без
// него поле не передаётся вовсе: иначе форма ЮKassa покажет не все способы.
func TestCreateInvoicePaymentMethodData(t *testing.T) {
	cases := []struct {
		name   string
		method string
		want   string // "" — поля нет в запросе
	}{
		{"not set", "", ""},
		{"all", "all", ""},
		{"sbp", "sbp", "sbp"},
		{"bank card", "bank_card", "bank_card"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Payment{ID: uuid.New(), Status: "pending"})
			}))
			defer srv.Close()

			ctx := context.Background()
			if c.method != "" {
				ctx = context.WithValue(ctx, CtxKeyPaymentMethod, c.method)
			}
			if _, err := NewClient(srv.URL, "shop", "secret").CreateInvoice(ctx, 100, "test", 1, 2); err != nil {
				t.Fatal(err)
			}

			raw, ok := body["payment_method_data"]
			if c.want == "" {
				if ok {
					t.Fatalf("payment_method_data sent: %s", raw)
				}
				return
			}
			var got PaymentMethodData
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("payment_method_data = %s: %v", raw, err)
			}
			if got.Type != c.want {
				t.Fatalf("type = %q, want %q", got.Type, c.want)
			}
		})
	}
}
