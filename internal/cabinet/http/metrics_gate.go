package cabinethttp

import (
	"crypto/subtle"
	"net/http"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
)

func wrapMetricsBasicAuth(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user := cabcfg.MetricsUser()
		pass := cabcfg.MetricsPassword()
		if user == "" && pass == "" {
			// Без логина и пароля метрики закрыты: раньше они отдавались любому,
			// а в них число сессий, клиентов и карта всех маршрутов API.
			http.NotFound(w, r)
			return
		}
		if user == "" || pass == "" {
			http.Error(w, "metrics auth misconfigured", http.StatusInternalServerError)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || !ctEq(u, user) || !ctEq(p, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="cabinet metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func ctEq(a, b string) bool {
	ba, bb := []byte(a), []byte(b)
	if len(ba) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(ba, bb) == 1
}
