package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"remnawave-tg-shop-bot/internal/remnawave"
)

// Панель с двумя нодами; reorders считает, сколько раз ей прислали новый порядок.
func fakePanelWithNodes(t *testing.T, reorders *atomic.Int32) *remnawave.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/nodes":
			_, _ = io.WriteString(w, `{"response":[{"uuid":"AAAA","name":"a","viewPosition":1},{"uuid":"bbbb","name":"b","viewPosition":2}]}`)
		case "/api/nodes/actions/reorder":
			reorders.Add(1)
			_, _ = io.WriteString(w, `{"response":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return remnawave.NewClient(srv.URL, "t", "remote")
}

func postReorder(h *AdminNodesHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/cabinet/api/admin/nodes/reorder", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Reorder(rec, req)
	return rec
}

func TestAdminNodesReorder(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantStatus   int
		wantReorders int32
	}{
		{"ok, uuid в любом регистре", `{"uuids":["BBBB","aaaa"]}`, http.StatusOK, 1},
		{"неполный список", `{"uuids":["aaaa"]}`, http.StatusConflict, 0},
		{"чужая нода", `{"uuids":["aaaa","cccc"]}`, http.StatusConflict, 0},
		{"повтор", `{"uuids":["aaaa","aaaa"]}`, http.StatusBadRequest, 0},
		{"пустой uuid", `{"uuids":["aaaa",""]}`, http.StatusBadRequest, 0},
		{"не json", `nope`, http.StatusBadRequest, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var reorders atomic.Int32
			h := NewAdminNodes(fakePanelWithNodes(t, &reorders))
			if rec := postReorder(h, c.body); rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.wantStatus, rec.Body.String())
			}
			if got := reorders.Load(); got != c.wantReorders {
				t.Fatalf("panel reorders = %d, want %d", got, c.wantReorders)
			}
		})
	}
}

func TestAdminNodesReorder_dropsCache(t *testing.T) {
	var reorders atomic.Int32
	h := NewAdminNodes(fakePanelWithNodes(t, &reorders))
	h.cached = []remnawave.NodeOverview{{UUID: "stale"}}
	if rec := postReorder(h, `{"uuids":["aaaa","bbbb"]}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if h.cached != nil {
		t.Fatal("cache must be dropped so the next list shows the new order")
	}
}

func TestAdminNodesReorder_methodNotAllowed(t *testing.T) {
	var reorders atomic.Int32
	h := NewAdminNodes(fakePanelWithNodes(t, &reorders))
	rec := httptest.NewRecorder()
	h.Reorder(rec, httptest.NewRequest(http.MethodGet, "/cabinet/api/admin/nodes/reorder", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}
