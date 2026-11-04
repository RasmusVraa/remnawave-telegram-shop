package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/statusprobe"
)

type memKV struct{ data map[string]string }

func (m *memKV) GetAll(context.Context) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range m.data {
		out[k] = v
	}
	return out, nil
}

func (m *memKV) UpsertBatch(_ context.Context, settings map[string]string, _ *int64) error {
	for k, v := range settings {
		m.data[k] = v
	}
	return nil
}

func TestAdminStatusTargetsSaveAndQuota(t *testing.T) {
	t.Setenv("GLOBALPING_TOKEN", "")
	kv := &memKV{data: map[string]string{}}
	changed := 0
	h := NewAdminStatusTargets(statusprobe.NewKVTargets(kv), nil, nil, func() { changed++ })

	body := `{"targets":[{"name":"RU entry","country":"ru","address":"203.0.113.40","enabled":true,"kind":"bypass","node_uuid":"AAAA-1111","exit_node_uuid":"bbbb-2222","no_external":true}]}`
	rec := httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	if changed != 1 {
		t.Fatal("public cache must be invalidated after save")
	}
	var saved statusTargetsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || len(saved.Targets) != 1 {
		t.Fatalf("response: %v %s", err, rec.Body.String())
	}
	got := saved.Targets[0]
	if got.Kind != statusprobe.KindBypass || !got.Whitelist || got.NodeUUID != "aaaa-1111" || got.ExitNodeUUID != "bbbb-2222" || !got.NoExternal || got.Country != "RU" {
		t.Fatalf("normalized: %+v", got)
	}

	// Список читается обратно с теми же полями.
	rec = httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), `"exit_node_uuid":"bbbb-2222"`) || !strings.Contains(rec.Body.String(), `"no_external":true`) {
		t.Fatalf("get: %s", rec.Body.String())
	}

	// Строка без внешних зондов квоту не тратит.
	rec = httptest.NewRecorder()
	h.Quota(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var quota statusprobe.QuotaEstimate
	if err := json.Unmarshal(rec.Body.Bytes(), &quota); err != nil {
		t.Fatal(err)
	}
	if quota.PerHour != 0 || quota.Limit != 250 {
		t.Fatalf("quota: %+v", quota)
	}

	// Без клиента панели список узлов пуст, а не ошибка.
	rec = httptest.NewRecorder()
	h.Nodes(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"nodes":[]`) {
		t.Fatalf("nodes: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAdminStatusTargetsRejectsBadInput(t *testing.T) {
	kv := &memKV{data: map[string]string{}}
	h := NewAdminStatusTargets(statusprobe.NewKVTargets(kv), nil, nil, nil)
	for _, body := range []string{
		`{"targets":[{"name":"x","country":"NL","address":"10.0.0.1"}]}`,
		`{"targets":[{"name":"x","country":"NL","address":"8.8.8.8","kind":"weird"}]}`,
		`{"targets":[{"name":"x","country":"NL","address":"8.8.8.8","node_uuid":"../x"}]}`,
	} {
		rec := httptest.NewRecorder()
		h.Handle(rec, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s -> %d", body, rec.Code)
		}
	}
	if len(kv.data) != 0 {
		t.Fatal("nothing must be stored on validation failure")
	}
}
