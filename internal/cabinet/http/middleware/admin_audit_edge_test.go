package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditActionEdgeCases(t *testing.T) {
	cases := []struct {
		path   string
		action string
		id     int64
	}{
		{"/cabinet/api/admin/users/12/devices/34", "users/:id/devices/:id", 12},
		{"/cabinet/api/admin/users/12/", "users/:id", 12},
		{"/cabinet/api/admin/partners/5/approve", "partners/:id/approve", 0}, // id партнёра — не клиент
		{"/cabinet/api/admin/", "", 0},
		{"/cabinet/api/admin", "", 0},
		{"/cabinet/api/admin/alerts/test", "", 0},
		{"/cabinet/api/admin/broadcast/audiences", "", 0},
		{"/cabinet/api/admin/broadcast/upload-media", "", 0},
		{"/cabinet/api/admin/broadcast/send", "broadcast/send", 0},
	}
	for _, c := range cases {
		a, id := auditAction(c.path)
		if a != c.action || id != c.id {
			t.Errorf("auditAction(%q) = %q, %d; want %q, %d", c.path, a, id, c.action, c.id)
		}
	}
}

func TestAuditFieldsEdgeCases(t *testing.T) {
	// Секреты отсекаются без учёта регистра и в составе имени.
	got := auditFields([]byte(`{"Password":"p","refreshToken":"t","client_secret":"s","new_password":"n","days":1}`))
	if len(got) != 1 || got["days"] != float64(1) {
		t.Fatalf("secrets must be dropped: %v", got)
	}
	// Строка ровно в предел (в рунах, не байтах) остаётся.
	s64 := strings.Repeat("я", auditMaxString)
	if got := auditFields([]byte(`{"name":"` + s64 + `"}`)); got["name"] != s64 {
		t.Fatalf("64-rune string must be kept: %v", got)
	}
	// Не объект, битый JSON, пусто, слишком большое тело — без параметров.
	for _, body := range []string{`[1,2]`, `{"a":`, ``, `"str"`, `{"t":"` + strings.Repeat("a", auditMaxBody) + `"}`} {
		if got := auditFields([]byte(body)); len(got) != 0 {
			t.Errorf("auditFields(%.20q) = %v, want empty", body, got)
		}
	}
	// Не больше auditMaxFields полей.
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 30; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"f` + strings.Repeat("x", i) + `":1`)
	}
	b.WriteString("}")
	if got := auditFields([]byte(b.String())); len(got) > auditMaxFields {
		t.Fatalf("fields = %d, cap %d", len(got), auditMaxFields)
	}
	// Вложенные объекты (настройки бота: {"settings":{...}}): в журнал идут только
	// имена ключей — значения, в том числе ключи провайдеров, не попадают.
	got = auditFields([]byte(`{"settings":{"YOOKASA_SECRET":"x","SOME":"y"}}`))
	if got["settings"] != "SOME,YOOKASA_SECRET" {
		t.Fatalf("nested keys: %v", got)
	}
	if strings.Contains(fmt.Sprint(got), `"x"`) || strings.Contains(fmt.Sprint(got), ":y") {
		t.Fatalf("nested values must not be logged: %v", got)
	}
}

// Тело больше, чем журнал читает себе (auditMaxBody), доходит до обработчика целиком.
func TestAuditKeepsLargeBodyForHandler(t *testing.T) {
	big := `{"text":"` + strings.Repeat("ж", 5000) + `"}`
	var seen string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPatch, "/cabinet/api/admin/tariffs/3", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	auditAdminAction(rec, req, 42, h)
	if seen != big {
		t.Fatalf("handler saw %d bytes, want %d", len(seen), len(big))
	}
}

// GET и не-JSON тела проходят нетронутыми, статус ответа не меняется.
func TestAuditPassThrough(t *testing.T) {
	for _, c := range []struct {
		method, ctype string
	}{
		{http.MethodGet, "application/json"},
		{http.MethodPost, "multipart/form-data; boundary=x"},
	} {
		var seen string
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			seen = string(b)
			w.WriteHeader(http.StatusTeapot)
		})
		req := httptest.NewRequest(c.method, "/cabinet/api/admin/users/5/extend", strings.NewReader("raw-body"))
		req.Header.Set("Content-Type", c.ctype)
		rec := httptest.NewRecorder()
		auditAdminAction(rec, req, 42, h)
		if seen != "raw-body" || rec.Code != http.StatusTeapot {
			t.Errorf("%s %s: handler saw %q, code %d", c.method, c.ctype, seen, rec.Code)
		}
	}
}

// statusWriter: первый WriteHeader побеждает, Write без заголовка — 200,
// Flush через ResponseController доходит до исходного writer (SSE/стриминг).
func TestStatusWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	_, _ = sw.Write([]byte("x"))
	sw.WriteHeader(http.StatusInternalServerError)
	if sw.status != http.StatusOK {
		t.Fatalf("status after implicit 200 = %d", sw.status)
	}

	rec = httptest.NewRecorder()
	sw = &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	sw.WriteHeader(http.StatusForbidden)
	sw.WriteHeader(http.StatusOK)
	if sw.status != http.StatusForbidden {
		t.Fatalf("first WriteHeader must win, got %d", sw.status)
	}
	if err := http.NewResponseController(sw).Flush(); err != nil {
		t.Fatalf("flush through statusWriter: %v", err)
	}
	if !rec.Flushed {
		t.Fatal("underlying recorder not flushed")
	}
}
