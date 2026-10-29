package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Метка — шаблон маршрута, а не путь: случайные адреса не плодят серии.
func TestRouteLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cabinet/api/admin/users/", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("/cabinet/api/me", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc(apiNotFoundPattern, func(http.ResponseWriter, *http.Request) {})

	cases := map[string]string{
		"/cabinet/api/me":                  "/cabinet/api/me",
		"/cabinet/api/admin/users/42/edit": "/cabinet/api/admin/users/",
		"/cabinet/api/admin/users/xyz123":  "/cabinet/api/admin/users/",
		"/cabinet/api/random-a8f3":         "unmatched",
		"/cabinet/api/another/random/path": "unmatched",
	}
	for path, want := range cases {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		mux.ServeHTTP(httptest.NewRecorder(), r)
		if got := routeLabel(r); got != want {
			t.Errorf("routeLabel(%s) = %q, want %q", path, got, want)
		}
	}
	// Вне ServeMux (SPA) — прежняя нормализация.
	if got := routeLabel(httptest.NewRequest(http.MethodGet, "/cabinet/some/page", nil)); got != "/cabinet/*" {
		t.Errorf("spa label = %q", got)
	}
}
