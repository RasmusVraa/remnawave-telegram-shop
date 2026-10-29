package middleware

import (
	"net/http"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/metrics"
)

// HTTPMetrics записывает histogram cabinet_http_request_duration_seconds.
func HTTPMetrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			metrics.ObserveHTTPDuration(r.Method, routeLabel(r), time.Since(start).Seconds())
		})
	}
}

// apiNotFoundPattern — шаблон catch-all, отвечающий 404 на неизвестные пути API.
const apiNotFoundPattern = "/cabinet/api/"

// routeLabel — метка пути для метрик. Берётся шаблон маршрута, который нашёл
// ServeMux (он записывает его в r.Pattern того же запроса — этот middleware
// стоит сразу перед роутером). Путь из URL метками не годится: любой мог
// перебирать случайные адреса, и каждый становился новой серией Prometheus —
// память росла без предела.
func routeLabel(r *http.Request) string {
	switch r.Pattern {
	case "":
		// Не через ServeMux (SPA): там путь сводится к двум значениям.
		return metrics.NormalizeAPIPath(r.URL.Path)
	case apiNotFoundPattern:
		return "unmatched"
	default:
		return r.Pattern
	}
}
