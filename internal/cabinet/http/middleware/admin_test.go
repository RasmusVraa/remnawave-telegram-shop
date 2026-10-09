package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	adminauth "remnawave-tg-shop-bot/internal/cabinet/admin/auth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
)

func TestRequireAdmin_unauthorizedWithoutClaims(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireAdmin(nil)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rr.Code)
	}
}

func TestRequireAdmin_forbiddenWhenNotAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	checker := adminauth.NewChecker(nil, nil)
	handler := RequireAdmin(checker)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := &jwt.Claims{AccountID: 42}
	req = req.WithContext(SetClaims(req.Context(), claims))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rr.Code)
	}
}
