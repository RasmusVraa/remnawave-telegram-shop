package cabinethttp

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// Лендинг отдаётся с корня домена (/), со старого /landing и с /cabinet/landing.
// В обоих случаях SPA-хендлер должен вернуть index.html, а не 404 — иначе
// прямой заход и обновление страницы ломаются.
func TestSPAHandlerServesIndexForLandingRoutes(t *testing.T) {
	const indexBody = "<!doctype html><title>cabinet</title>"

	spaFS := fstest.MapFS{
		"index.html":    {Data: []byte(indexBody)},
		"assets/app.js": {Data: []byte("console.log(1)")},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}

	mux := http.NewServeMux()
	spa := buildSPAHandler(spaFS)
	mux.Handle("/cabinet/", spa)
	mux.Handle("/{$}", spa)
	mux.Handle("/landing", spa)
	mux.Handle("/landing/", spa)
	mux.Handle("/status", spa)
	mux.Handle("/status/", spa)

	for _, path := range []string{
		"/",
		"/landing",
		"/landing/",
		"/status",
		"/status/",
		"/cabinet/landing",
		"/cabinet/",
		"/cabinet/dashboard",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("статус %d, ожидался 200", rec.Code)
			}
			if got := rec.Body.String(); got != indexBody {
				t.Fatalf("тело %q, ожидался index.html", got)
			}
		})
	}
}

// Пропавший hashed-ассет — 404, не index.html: иначе браузер/SW получают HTML
// как module script и чеки/админка падают до hard refresh.
func TestSPAHandlerMissingAssetIsNotFound(t *testing.T) {
	spaFS := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html>")},
	}

	rec := httptest.NewRecorder()
	buildSPAHandler(spaFS).ServeHTTP(
		rec,
		httptest.NewRequest(http.MethodGet, "/cabinet/assets/PaymentStatusPage-missing.js", nil),
	)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("статус %d, ожидался 404", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "<!doctype html>") {
		t.Fatalf("missing asset вернул index.html: %q", body)
	}
}

// Реальные файлы бандла должны отдаваться как файлы, а не подменяться index.html.
func TestSPAHandlerStillServesAssets(t *testing.T) {
	spaFS := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}

	rec := httptest.NewRecorder()
	buildSPAHandler(spaFS).ServeHTTP(
		rec,
		httptest.NewRequest(http.MethodGet, "/cabinet/assets/app.js", nil),
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("статус %d, ожидался 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log(1)" {
		t.Fatalf("тело %q, ожидался ассет", got)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control %q, ожидался immutable для assets/", cc)
	}
}

var _ fs.FS = fstest.MapFS{}
