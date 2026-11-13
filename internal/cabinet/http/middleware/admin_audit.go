package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/adminevents"
)

// Журнал действий админов для «Мониторинга». Каждый успешный изменяющий запрос
// к /cabinet/api/admin/* пишется событием admin.action: что за действие (путь
// с id вместо чисел), над каким клиентом и короткие параметры из тела — сколько
// дней, какой тариф. Так новая кнопка в админке попадает в журнал сама, без
// правок в каждом обработчике.

const adminAPIPrefix = "/cabinet/api/admin/"

// auditSkip — изменяющие запросы, которые ничего не меняют: превью, подсчёты,
// поиск, загрузка картинки до отправки.
var auditSkip = []string{"/preview", "/audiences", "/upload-media", "/search", "/test"}

// auditNoBody — тело не пишем: текст рассылки длинный, а смысл и так ясен.
var auditNoBody = []string{"broadcast/send"}

const (
	auditMaxBody   = 4 << 10
	auditMaxFields = 12
	auditMaxString = 64
)

func auditAdminAction(w http.ResponseWriter, r *http.Request, actor int64, next http.Handler) {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		next.ServeHTTP(w, r)
		return
	}
	action, customerID := auditAction(r.URL.Path)
	if action == "" {
		next.ServeHTTP(w, r)
		return
	}
	var body []byte
	if r.Body != nil && !auditHasAny(action, auditNoBody) && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, _ = io.ReadAll(io.LimitReader(r.Body, auditMaxBody+1))
		rest := r.Body
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), rest), rest}
	}
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(sw, r)
	if sw.status < 200 || sw.status >= 300 {
		return
	}
	payload := map[string]any{"action": action, "method": r.Method}
	if fields := auditFields(body); len(fields) > 0 {
		payload["params"] = fields
	}
	adminevents.Emit(adminevents.Event{
		Type: adminevents.AdminAction, CustomerID: customerID, ActorTgID: actor,
		Source: adminevents.SourceCabinet, Payload: payload,
	})
}

// auditAction: «/cabinet/api/admin/users/1560/extend» → «users/:id/extend», 1560.
// Пустое действие — запрос в журнал не идёт.
func auditAction(path string) (string, int64) {
	if !strings.HasPrefix(path, adminAPIPrefix) {
		return "", 0
	}
	rest := strings.Trim(strings.TrimPrefix(path, adminAPIPrefix), "/")
	if rest == "" || auditHasAny("/"+rest, auditSkip) {
		return "", 0
	}
	parts := strings.Split(rest, "/")
	var customerID int64
	for i, p := range parts {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			if i == 1 && parts[0] == "users" {
				customerID = n
			}
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/"), customerID
}

func auditHasAny(s string, list []string) bool {
	for _, x := range list {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// auditFields — простые поля тела: числа, да/нет и короткие строки.
func auditFields(body []byte) map[string]any {
	if len(body) == 0 || len(body) > auditMaxBody {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range raw {
		if len(out) >= auditMaxFields {
			break
		}
		if auditSecretKey(k) {
			continue
		}
		switch t := v.(type) {
		case float64, bool:
			out[k] = t
		case string:
			if utf8.RuneCountInString(t) <= auditMaxString {
				out[k] = t
			}
		case map[string]any:
			// «Настройки бота» шлют {"settings":{...}}: значения не пишем,
			// но какие ключи меняли — видно.
			names := make([]string, 0, len(t))
			for name := range t {
				names = append(names, name)
			}
			sort.Strings(names)
			if len(names) > auditMaxFields {
				names = names[:auditMaxFields]
			}
			if len(names) > 0 {
				out[k] = strings.Join(names, ",")
			}
		}
	}
	return out
}

// auditSecretKey — поле, значение которого в журнал не пишем никогда.
func auditSecretKey(k string) bool {
	lk := strings.ToLower(k)
	for _, s := range []string{"password", "pass", "token", "secret", "key", "cookie", "init_data", "hash", "credential", "auth"} {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap — чтобы http.ResponseController видел исходный writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
