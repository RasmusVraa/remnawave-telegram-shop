// Package csrf — double-submit cookie паттерн.
//
// Идея: на логине backend ставит cookie `csrf_token` (НЕ httpOnly, чтобы SPA
// мог прочитать), а на каждом мутирующем запросе SPA передаёт то же значение
// в заголовке `X-CSRF-Token`. Backend сверяет cookie == header. Злоумышленник
// с другого origin'а не сможет прочитать cookie (same-origin policy) и не
// сможет выставить заголовок — поэтому атакующий запрос не пройдёт.
//
// Это стандартная защита для SPA с cookie-based refresh и намного проще, чем
// хранить токен на сервере.
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
)

const (
	// CookieName — имя cookie с CSRF-токеном.
	CookieName = "csrf_token"
	// HeaderName — заголовок, в котором SPA должен присылать тот же токен.
	HeaderName = "X-CSRF-Token"
)

// ErrMissing — ни cookie, ни header не пришли.
var ErrMissing = errors.New("csrf: missing token")

// ErrMismatch — cookie и header не совпадают.
var ErrMismatch = errors.New("csrf: token mismatch")

// GenerateToken — 32-байтный случайный токен в base64url (без паддинга).
// Достаточно для защиты от предугадывания (2^256).
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// SetCookie выставляет csrf_token cookie. Параметры:
//   - Secure задаёт вызывающий код: true на публичном HTTPS, false на localhost по HTTP;
//   - HttpOnly=false (SPA должен читать значение);
//   - SameSite=Lax (достаточно, refresh-cookie и так Lax, top-level navigation
//     безопасен для логина через OAuth-редиректы).
//
// Domain задаётся вызывающим кодом — должен совпадать с refresh-cookie Domain.
func SetCookie(w http.ResponseWriter, token, domain, path string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     path,
		Domain:   domain,
		MaxAge:   maxAge,
		Secure:   secure,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie сбрасывает csrf_token cookie (logout, reset password).
// secure должен совпадать с тем, как cookie была выставлена, иначе браузер её не сотрёт.
func ClearCookie(w http.ResponseWriter, domain, path string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     path,
		Domain:   domain,
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

// Validate сверяет cookie == header. Constant-time сравнение, чтобы не утечь
// ничего таймингом (хотя токен рандомный и атака практически невозможна).
func Validate(r *http.Request) error {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return ErrMissing
	}
	header := r.Header.Get(HeaderName)
	if header == "" {
		return ErrMissing
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
		return ErrMismatch
	}
	return nil
}
