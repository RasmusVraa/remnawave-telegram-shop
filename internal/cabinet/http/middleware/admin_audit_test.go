package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditAction(t *testing.T) {
	cases := []struct {
		path   string
		action string
		id     int64
	}{
		{"/cabinet/api/admin/users/1560/extend", "users/:id/extend", 1560},
		{"/cabinet/api/admin/promos/7", "promos/:id", 0},
		{"/cabinet/api/admin/settings", "settings", 0},
		{"/cabinet/api/admin/broadcast/preview", "", 0},
		{"/cabinet/api/admin/users/search", "", 0},
		{"/cabinet/api/me", "", 0},
	}
	for _, c := range cases {
		a, id := auditAction(c.path)
		if a != c.action || id != c.id {
			t.Errorf("auditAction(%q) = %q, %d; want %q, %d", c.path, a, id, c.action, c.id)
		}
	}
}

func TestAuditFieldsDropsSecretsAndLongText(t *testing.T) {
	got := auditFields([]byte(`{"days":7,"enabled":true,"tariff":"Базовый","api_token":"x","text":"` + strings.Repeat("a", 100) + `","list":[1]}`))
	if got["days"] != float64(7) || got["enabled"] != true || got["tariff"] != "Базовый" {
		t.Fatalf("simple fields lost: %v", got)
	}
	for _, k := range []string{"api_token", "text", "list"} {
		if _, ok := got[k]; ok {
			t.Fatalf("%s must be dropped: %v", k, got)
		}
	}
}

// Тело, прочитанное журналом, должно дойти до обработчика целиком.
func TestAuditKeepsBodyForHandler(t *testing.T) {
	var seen string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/cabinet/api/admin/users/5/extend", strings.NewReader(`{"days":3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	auditAdminAction(rec, req, 42, h)
	if seen != `{"days":3}` || rec.Code != http.StatusNoContent {
		t.Fatalf("handler saw %q, code %d", seen, rec.Code)
	}
}
