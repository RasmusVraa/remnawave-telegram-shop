package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
)

const slowRequestLogThreshold = 300 * time.Millisecond

var noisyAccessLogPaths = map[string]struct{}{
	"/cabinet/sw.js":                               {},
	"/cabinet/api/auth/bootstrap":                  {},
	"/cabinet/api/public/pwa-manifest.webmanifest": {},
}

// statusRecorder — обёртка над http.ResponseWriter, которая запоминает статус и размер ответа
// для последующего логирования. Без неё мы не знаем, что реально уехало клиенту.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.status = code
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Logger пишет строку access-лога на каждый завершённый HTTP-запрос (уровень зависит от CABINET_HTTP_ACCESS_LOG).
// Тело запроса/ответа не логируется — только метаданные, чтобы не утечь PII.
func Logger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			duration := time.Since(start)
			logCabinetHTTPAccess(rec, r, duration)
		})
	}
}

func logCabinetHTTPAccess(rec *statusRecorder, r *http.Request, duration time.Duration) {
	mode := cabcfg.HTTPAccessLogMode()
	if mode == cabcfg.AccessLogOff {
		return
	}
	if shouldSkipAccessLog(r.URL.Path, rec.status, duration) {
		return
	}
	attrs := []any{
		"request_id", RequestIDFromContext(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"status", rec.status,
		"bytes", rec.bytes,
		"duration_ms", duration.Milliseconds(),
		"remote", maskRemoteAddr(ClientIP(r)),
		"ua", truncateUserAgent(r.Header.Get("User-Agent"), 160),
	}
	problem := rec.status >= http.StatusBadRequest || duration >= slowRequestLogThreshold
	if mode == cabcfg.AccessLogFull {
		slog.Info("cabinet http", attrs...)
		return
	}
	// AccessLogMinimal
	if problem {
		slog.Info("cabinet http", attrs...)
	} else {
		slog.Debug("cabinet http", attrs...)
	}
}

func shouldSkipAccessLog(path string, status int, duration time.Duration) bool {
	// В логах всегда оставляем ошибки и медленные запросы: это критично для отладки.
	if status >= http.StatusBadRequest || duration >= slowRequestLogThreshold {
		return false
	}
	_, noisy := noisyAccessLogPaths[path]
	return noisy
}

func truncateUserAgent(ua string, max int) string {
	ua = strings.TrimSpace(ua)
	if max <= 0 || len(ua) <= max {
		return ua
	}
	return ua[:max] + "…"
}

// maskRemoteAddr укорачивает лог IP: host без последнего октета для IPv4 или /64 для IPv6 не делаем — только обрезка порта.
func maskRemoteAddr(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return ""
	}
	// X-Forwarded-For может быть списком — берём первый hop.
	if i := strings.IndexByte(hostport, ','); i >= 0 {
		hostport = strings.TrimSpace(hostport[:i])
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return hostport
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String()
	}
	// IPv6: маскируем младшие 8 байт (грубая анонимизация).
	v6 := ip.To16()
	if v6 == nil {
		return hostport
	}
	for i := 8; i < 16; i++ {
		v6[i] = 0
	}
	return v6.String()
}
