package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GET по ссылке из письма только показывает кнопку и ничего не привязывает:
// такие ссылки открывают почтовые сканеры. Сервис здесь nil — любое обращение
// к нему упало бы.
func TestOAuthLinkConfirm_GetShowsFormOnly(t *testing.T) {
	h := &OAuthHandler{}
	r := httptest.NewRequest(http.MethodGet, "/cabinet/api/auth/oauth/confirm?token=abc%22%3E", nil)
	rec := httptest.NewRecorder()
	h.GoogleLinkConfirm(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `method="post"`) || !strings.Contains(body, `action="/cabinet/api/auth/oauth/confirm"`) {
		t.Fatalf("no POST form in page: %s", body)
	}
	// Токен экранирован шаблоном, а не вставлен как есть.
	if strings.Contains(body, `value="abc">`) || !strings.Contains(body, "abc&#34;&gt;") {
		t.Fatalf("token not escaped: %s", body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("GET must not set session cookies")
	}
}

func TestOAuthLinkConfirm_PostWithoutToken(t *testing.T) {
	h := &OAuthHandler{}
	r := httptest.NewRequest(http.MethodPost, "/cabinet/api/auth/oauth/confirm", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.GoogleLinkConfirm(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
