package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
)

func TestCookieScopeLoopbackDropsSecureDomain(t *testing.T) {
	for _, host := range []string{"127.0.0.1:3000", "localhost:3000", "[::1]:3000"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = host
		domain, secure := cookieScope(r, "cabinet.example.ru")
		if domain != "" || secure {
			t.Fatalf("%s: domain=%q secure=%v", host, domain, secure)
		}
	}
}

func TestSetRefreshCookieLoopbackIsHostOnly(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/cabinet/api/auth/login", nil)
	r.Host = "127.0.0.1:3000"
	setRefreshCookie(w, r, &service.TokenPair{
		RefreshToken: "refresh",
		CSRFToken:    "csrf",
		RefreshExp:   time.Now().Add(time.Hour),
	}, "cabinet.example.ru", "/cabinet/api/auth")

	byName := map[string]*http.Cookie{}
	for _, c := range w.Result().Cookies() {
		byName[c.Name] = c
	}
	for _, name := range []string{service.RefreshCookieName, "csrf_token"} {
		c := byName[name]
		if c == nil {
			t.Fatalf("missing cookie %s", name)
		}
		if c.Secure || c.Domain != "" {
			t.Fatalf("%s: secure=%v domain=%q", name, c.Secure, c.Domain)
		}
	}
}

func TestCookieScopePublicKeepsConfiguredDomain(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "cabinet.example.ru"
	domain, secure := cookieScope(r, "cabinet.example.ru")
	if domain != "cabinet.example.ru" || !secure {
		t.Fatalf("domain=%q secure=%v", domain, secure)
	}
}
