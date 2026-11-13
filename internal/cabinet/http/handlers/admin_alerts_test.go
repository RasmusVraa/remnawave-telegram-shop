package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/adminalert"
)

func TestAdminAlertsTest(t *testing.T) {
	var gotDraft *adminalert.Draft
	h := &AdminAlerts{test: func(_ context.Context, kind string, draft *adminalert.Draft) (adminalert.TestResult, error) {
		gotDraft = draft
		if kind == "nope" {
			return adminalert.TestResult{}, adminalert.ErrUnknownKind
		}
		return adminalert.TestResult{Error: "Bad Request: chat not found", Hint: adminalert.HintNotMember, ChatID: -100}, nil
	}}
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.Test(rec, httptest.NewRequest(http.MethodPost, "/cabinet/api/admin/alerts/test", strings.NewReader(body)))
		return rec
	}

	if rec := call(`{"kind":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind: %d", rec.Code)
	}
	h.last = h.last.AddDate(-1, 0, 0)
	rec := call(`{"kind":"servers"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("telegram error is still 200: %d", rec.Code)
	}
	var got adminalert.TestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.OK || got.Hint != adminalert.HintNotMember {
		t.Fatalf("body %s err %v", rec.Body.String(), err)
	}
	if rec := call(`{"kind":"servers"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second press within the gap: %d", rec.Code)
	}

	h.last = h.last.AddDate(-1, 0, 0)
	if rec := call(`{"kind":"servers","draft":{"shared_chat_id":"-100500","chat_id":"","thread_id":"12"}}`); rec.Code != http.StatusOK {
		t.Fatalf("draft: %d", rec.Code)
	}
	if gotDraft == nil || gotDraft.SharedChat != -100500 || gotDraft.Chat != 0 || gotDraft.Thread != 12 {
		t.Fatalf("draft parsed: %+v", gotDraft)
	}
	h.last = h.last.AddDate(-1, 0, 0)
	if rec := call(`{"kind":"servers","draft":{"thread_id":"abc"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad thread: %d", rec.Code)
	}
}
