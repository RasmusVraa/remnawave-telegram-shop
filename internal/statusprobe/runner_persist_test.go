package statusprobe

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memKV struct {
	data   map[string]string
	getErr error
}

func (m *memKV) GetAll(context.Context) (map[string]string, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	out := make(map[string]string, len(m.data))
	for key, value := range m.data {
		out[key] = value
	}
	return out, nil
}

func (m *memKV) UpsertBatch(_ context.Context, settings map[string]string, _ *int64) error {
	if m.data == nil {
		m.data = map[string]string{}
	}
	for key, value := range settings {
		m.data[key] = value
	}
	return nil
}

func TestSaveLatestKeepsCountriesNotMeasuredThisRun(t *testing.T) {
	store := NewKVHistory(&memKV{data: map[string]string{
		latestKey: `{"pl":{"world_ok":2,"world_total":3,"measured_at":"2026-10-09T12:00:00Z"},"fi":{"world_ok":1,"world_total":1,"measured_at":"2026-10-09T12:00:00Z"}}`,
	}})
	err := store.SaveLatest(context.Background(), map[string]Sample{
		"fi": {WorldOK: 4, WorldTotal: 4, MeasuredAt: time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := store.LoadLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest["pl"].WorldTotal != 3 {
		t.Fatalf("poland was erased: %+v", latest["pl"])
	}
	if latest["fi"].WorldOK != 4 {
		t.Fatalf("finland was not updated: %+v", latest["fi"])
	}
}

func TestEnsureLoadedRestoresPingsBeforeNextProbe(t *testing.T) {
	store := NewKVHistory(&memKV{data: map[string]string{
		latestKey:  `{"pl\nPoland":{"world_ok":2,"world_total":3,"russia_ok":1,"russia_total":1,"measured_at":"2026-10-09T12:00:00Z"}}`,
		historyKey: `{"pl\nPoland":{"2026-10-09":{"wOk":2,"wN":3,"rOk":1,"rN":1}}}`,
	}})
	runner := NewRunner(store)
	runner.EnsureLoaded(context.Background())
	sample, history, ok := runner.Snapshot("pl\nPoland")
	if !ok || sample.WorldTotal != 3 || sample.RussiaTotal != 1 {
		t.Fatalf("snapshot %+v ok=%v", sample, ok)
	}
	saved := false
	for _, day := range history {
		if day != nil {
			saved = true
		}
	}
	if len(history) != 30 || !saved {
		t.Fatal("saved day is missing from the window")
	}
}

func TestPersistDoesNotOverwriteWhenLoadFailed(t *testing.T) {
	repo := &memKV{getErr: errors.New("db down")}
	store := NewKVHistory(repo)
	runner := NewRunner(store)
	runner.EnsureLoaded(context.Background())
	runner.storeSample("pl\nPoland", Sample{WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now()})
	runner.persist()
	if len(repo.data) != 0 {
		t.Fatalf("failed load still wrote %+v", repo.data)
	}

	repo.getErr = nil
	repo.data = map[string]string{
		latestKey: `{"fi\nFinland":{"world_ok":4,"world_total":4,"measured_at":"2026-10-09T12:00:00Z"}}`,
	}
	runner.EnsureLoaded(context.Background())
	sample, _, ok := runner.Snapshot("fi\nFinland")
	if !ok || sample.WorldTotal != 4 {
		t.Fatalf("finland after retry %+v ok=%v", sample, ok)
	}
	sample, _, ok = runner.Snapshot("pl\nPoland")
	if !ok || sample.WorldTotal != 1 {
		t.Fatalf("poland measured during the outage %+v ok=%v", sample, ok)
	}
}
