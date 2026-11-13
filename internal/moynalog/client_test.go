package moynalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ФНС возвращает UUID чека в поле approvedReceiptUuid — его и должен отдать клиент.
func TestSendIncomeRequestReturnsApprovedReceiptUUID(t *testing.T) {
	const uuid = "200abc1def"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"approvedReceiptUuid":"` + uuid + `"}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), baseURL: srv.URL}
	c.token.Store("test-token")

	got, err := c.sendIncomeRequest(context.Background(), srv.URL+"/income", []byte(`{}`), 305)
	if err != nil {
		t.Fatalf("sendIncomeRequest: %v", err)
	}
	if got != uuid {
		t.Fatalf("receipt id = %q, want %q", got, uuid)
	}
}
