package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testProviderURL = "https://oauth.example/authorize?client_id=x&state=abc123&code_challenge=y"

// cookieFromStart — cookie, которую старт OAuth поставил бы браузеру.
func cookieFromStart(t *testing.T, redirectURL string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	setOAuthStateCookie(rec, redirectURL, "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == oauthStateCookieName {
			return c
		}
	}
	return nil
}

func TestSetOAuthStateCookie(t *testing.T) {
	c := cookieFromStart(t, testProviderURL)
	if c == nil {
		t.Fatal("state cookie not set")
	}
	if c.Value != "abc123" {
		t.Fatalf("cookie value = %q, want state from provider URL", c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: httpOnly=%v secure=%v sameSite=%v", c.HttpOnly, c.Secure, c.SameSite)
	}
	if c.Path != oauthStateCookiePath || c.MaxAge <= 0 {
		t.Fatalf("cookie path=%q maxAge=%d", c.Path, c.MaxAge)
	}
}

func TestSetOAuthStateCookie_NoStateInURL(t *testing.T) {
	if c := cookieFromStart(t, "https://oauth.example/authorize?client_id=x"); c != nil {
		t.Fatalf("cookie set without state: %+v", c)
	}
}

func TestConsumeOAuthState(t *testing.T) {
	startCookie := cookieFromStart(t, testProviderURL)

	cases := []struct {
		name   string
		cookie *http.Cookie
		state  string
		want   bool
	}{
		{"same browser", startCookie, "abc123", true},
		// Ссылка на callback со state атакующего, открытая в браузере жертвы.
		{"no cookie", nil, "abc123", false},
		{"another state", startCookie, "attacker-state", false},
		{"empty state", startCookie, "", false},
		{"empty cookie", &http.Cookie{Name: oauthStateCookieName, Value: ""}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/cabinet/api/auth/telegram/callback?state="+tc.state, nil)
			if tc.cookie != nil {
				r.AddCookie(&http.Cookie{Name: tc.cookie.Name, Value: tc.cookie.Value})
			}
			rec := httptest.NewRecorder()
			if got := consumeOAuthState(rec, r, tc.state, ""); got != tc.want {
				t.Fatalf("consumeOAuthState = %v, want %v", got, tc.want)
			}
			// State одноразовый: cookie сбрасывается в любом исходе.
			cleared := false
			for _, c := range rec.Result().Cookies() {
				if c.Name == oauthStateCookieName && c.MaxAge < 0 {
					cleared = true
				}
			}
			if !cleared {
				t.Fatal("state cookie was not cleared")
			}
		})
	}
}
