// Package metrics регистрирует Prometheus-метрики web-кабинета (отдельный registry,
// чтобы не смешивать с возможными другими экспортёрами процесса).
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const ns = "cabinet"

var reg = prometheus.NewRegistry()

var (
	authAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "auth_attempts_total",
			Help:      "Попытки входа/регистрации/OAuth по методу и исходу.",
		},
		[]string{"method", "outcome"},
	)
	checkoutStarted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "checkout_started_total",
			Help:      "Успешно созданные web-checkout по провайдеру.",
		},
		[]string{"provider"},
	)
	checkoutPaidSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "checkout_paid_duration_seconds",
			Help:      "Время от создания checkout до первого зафиксированного статуса paid (секунды).",
			Buckets:   []float64{5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200, 86400},
		},
		[]string{"provider"},
	)
	mergeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "merge_operations_total",
			Help:      "Операции merge/link по исходу.",
		},
		[]string{"outcome"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_duration_seconds",
			Help:      "Длительность HTTP-запросов кабинета по нормализованному пути.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	activeSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "active_sessions",
			Help:      "Число неотозванных cabinet_session с expires_at > now().",
		},
	)
	webOnlyCustomers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "web_only_customers",
			Help:      "Число customer с is_web_only=true.",
		},
	)
)

func init() {
	reg.MustRegister(authAttempts, checkoutStarted, checkoutPaidSeconds, mergeTotal, httpDuration, activeSessions, webOnlyCustomers)
}

// Handler возвращает HTTP-обработчик /metrics (без auth — его оборачивает router).
func Handler() http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// RecordAuth фиксирует попытку auth/OAuth (method: email_login, email_register, google_callback, …).
func RecordAuth(method, outcome string) {
	authAttempts.WithLabelValues(method, outcome).Inc()
}

// RecordCheckoutStarted — успешный POST checkout.
func RecordCheckoutStarted(provider string) {
	checkoutStarted.WithLabelValues(provider).Inc()
}

// RecordCheckoutPaidDuration — первый переход checkout → paid (секунды).
func RecordCheckoutPaidDuration(provider string, seconds float64) {
	checkoutPaidSeconds.WithLabelValues(provider).Observe(seconds)
}

// RecordMerge фиксирует исход merge (success, noop, conflict, already_done, client_error, server_error).
func RecordMerge(outcome string) {
	mergeTotal.WithLabelValues(outcome).Inc()
}

// ObserveHTTPDuration записывает длительность запроса (path — нормализованный, см. middleware).
func ObserveHTTPDuration(method, normPath string, seconds float64) {
	httpDuration.WithLabelValues(method, normPath).Observe(seconds)
}

// NormalizeAPIPath снижает кардинальность label'ов: числовые сегменты → :id.
func NormalizeAPIPath(path string) string {
	if !strings.HasPrefix(path, "/cabinet/api/") {
		if strings.HasPrefix(path, "/cabinet/") {
			return "/cabinet/*"
		}
		return "other"
	}
	parts := strings.Split(strings.TrimPrefix(path, "/cabinet/api/"), "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if _, err := strconv.ParseInt(p, 10, 64); err == nil {
			parts[i] = ":id"
		}
	}
	return "/cabinet/api/" + strings.Join(parts, "/")
}

// StartGaugeRefresh периодически обновляет gauge'и из БД (интервал по умолчанию 5 мин).
func StartGaugeRefresh(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	refresh := func() {
		var n int64
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM cabinet_session
			WHERE revoked_at IS NULL AND expires_at > now()
		`).Scan(&n); err != nil {
			slog.Warn("cabinet metrics: active_sessions query failed", "error", err)
		} else {
			activeSessions.Set(float64(n))
		}
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM customer WHERE is_web_only = true`).Scan(&n); err != nil {
			slog.Warn("cabinet metrics: web_only_customers query failed", "error", err)
		} else {
			webOnlyCustomers.Set(float64(n))
		}
	}
	refresh()
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh()
			}
		}
	}()
}
