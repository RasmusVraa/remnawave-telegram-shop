package cabinethttp

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

// Без CABINET_METRICS_USER/PASSWORD метрики закрыты: раньше отдавались любому.
func TestMetricsGate_ClosedWithoutCredentials(t *testing.T) {
	called := false
	h := wrapMetricsBasicAuth(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) { called = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, "/cabinet/api/metrics", nil))
	if rec.Code != nethttp.StatusNotFound || called {
		t.Fatalf("status %d, inner called %v; want 404 without calling metrics", rec.Code, called)
	}
}
