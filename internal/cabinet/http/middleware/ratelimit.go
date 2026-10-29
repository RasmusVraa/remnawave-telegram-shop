package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/auth/ratelimit"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
)

// KeyFunc — вычисляет ключ для rate-limiter'а на основе запроса.
// Например: IP + route; IP + email; account_id + route.
type KeyFunc func(r *http.Request) string

// RateLimit — middleware, отдаёт 429 при превышении. Возвращает чистую 429
// без тела — SPA ориентируется на status code.
//
// Если key пустой (например, IP не извлёкся из X-Forwarded-For) — пропускаем,
// не блокируя запрос: лучше ложноотрицательный rate-limit, чем ложноположительный.
func RateLimit(lim *ratelimit.Limiter, keyFn KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key != "" && !lim.Allow(key) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP — IP клиента для rate-limit, логов и Turnstile.
//
// Forwarded-заголовкам верим, только если запрос пришёл от доверенного прокси
// (см. isTrustedProxy). X-Forwarded-For разбираем справа налево: каждый прокси
// дописывает адрес, с которого к нему пришли, в конец, а всё левее мог
// прислать сам клиент. Поэтому первый справа адрес, не принадлежащий нашим
// прокси, и есть клиент. Раньше брался левый элемент — его подставлял клиент,
// и случайный X-Forwarded-For в каждом запросе обходил все лимиты по IP.
func ClientIP(r *http.Request) string {
	return clientIP(r, cabcfg.TrustedProxies())
}

func clientIP(r *http.Request, extraTrusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if !isTrustedProxy(remote, extraTrusted) {
		return remote.String()
	}

	// Заголовков X-Forwarded-For может быть несколько — это один список.
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	if len(hops) == 0 {
		if ip, ok := parseHop(r.Header.Get("X-Real-IP")); ok {
			return ip.String()
		}
		return remote.String()
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip, ok := parseHop(hops[i])
		if !ok {
			// Левее мусора верить нечему.
			return remote.String()
		}
		if !isTrustedProxy(ip, extraTrusted) || i == 0 {
			// Первый недоверенный адрес; если доверенные все — самый дальний.
			return ip.String()
		}
	}
	return remote.String()
}

// parseHop разбирает элемент X-Forwarded-For: «1.2.3.4», «1.2.3.4:5678», «[::1]:80».
func parseHop(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	if ip, err := netip.ParseAddr(raw); err == nil {
		return ip.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(raw); err == nil {
		return ap.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}

// isTrustedProxy — адрес нашего прокси: loopback и частные сети (nginx/Caddy на
// том же хосте или в docker-сети) плюс диапазоны из CABINET_TRUSTED_PROXIES
// (например, CDN перед nginx).
func isTrustedProxy(ip netip.Addr, extra []netip.Prefix) bool {
	// IsPrivate покрывает 10/8, 172.16/12, 192.168/16 и fc00::/7.
	if ip.IsLoopback() || ip.IsPrivate() {
		return true
	}
	for _, p := range extra {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
