package handlers

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// OAuth state живёт на сервере и сам по себе не привязан к браузеру. Тогда чужая
// ссылка на callback (со state и code атакующего) логинит жертву в аккаунт
// атакующего, а ссылка на старт привязки Telegram превращает клик жертвы в
// привязку её Telegram к чужому аккаунту. Поэтому старт кладёт state в cookie,
// а callback принимает только тот state, который пришёл из того же браузера.

const (
	oauthStateCookieName = "cabinet_oauth_state"
	// Все callback'и лежат под /cabinet/api/auth/…/callback.
	oauthStateCookiePath = "/cabinet/api/auth"
	// Как у state на сервере (auth/oauth: stateTTL, telegramOIDCStateTTL).
	oauthStateCookieTTL = 10 * time.Minute
)

// setOAuthStateCookie запоминает в браузере state из URL провайдера.
func setOAuthStateCookie(w http.ResponseWriter, redirectURL, cookieDomain string) {
	u, err := url.Parse(redirectURL)
	state := ""
	if err == nil {
		state = u.Query().Get("state")
	}
	if state == "" {
		// Callback такой вход отклонит — лучше заметить это в логах сразу.
		slog.Error("oauth start: no state in provider redirect URL")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     oauthStateCookiePath,
		Domain:   cookieDomain,
		MaxAge:   int(oauthStateCookieTTL / time.Second),
		Secure:   true,
		HttpOnly: true,
		// Lax: cookie уходит при возврате с провайдера (top-level GET), но не в
		// фоновых кросс-сайтовых запросах.
		SameSite: http.SameSiteLaxMode,
	})
}

// consumeOAuthState проверяет, что state из callback выдан этому браузеру, и
// сбрасывает cookie: state одноразовый.
func consumeOAuthState(w http.ResponseWriter, r *http.Request, state, cookieDomain string) bool {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     oauthStateCookiePath,
		Domain:   cookieDomain,
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	c, err := r.Cookie(oauthStateCookieName)
	if err != nil || c.Value == "" || state == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) == 1
}

// oauthStateMismatchRedirect — куда вести браузер, если state чужой.
const oauthStateMismatchRedirect = "/cabinet/login?status=error&reason_code=oauth_state_mismatch"
