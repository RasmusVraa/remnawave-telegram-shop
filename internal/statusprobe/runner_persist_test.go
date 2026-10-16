package statusprobe

import (
	"context"
	"errors"
	"net/http"
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

func TestSchedulePingsDueNodeWithoutAPageView(t *testing.T) {
	t.Setenv("STATUS_PROBES_ENABLED", "true")
	prevPing := pingTarget
	t.Cleanup(func() { pingTarget = prevPing })

	var pinged int
	pingTarget = func(context.Context, *http.Client, string, int, int, bool) (Sample, error) {
		pinged++
		return Sample{WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now()}, nil
	}

	runner := NewRunner(nil)
	runner.loaded = true
	runner.samples["pl\nPoland"] = Sample{
		WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now().Add(-31 * time.Minute),
	}
	runner.schedule(context.Background(), func(context.Context) ([]Target, error) {
		return []Target{{Key: "pl\nPoland", Address: "203.0.113.10", Interval: 30 * time.Minute}}, nil
	})
	waitUntil(t, func() bool { return pinged == 1 })

	runner.schedule(context.Background(), func(context.Context) ([]Target, error) {
		return []Target{{Key: "pl\nPoland", Address: "203.0.113.10", Interval: 30 * time.Minute}}, nil
	})
	time.Sleep(50 * time.Millisecond)
	if pinged != 1 {
		t.Fatalf("fresh sample was pinged again: %d", pinged)
	}
}

func TestScheduleLeavesFreshNodeAlone(t *testing.T) {
	t.Setenv("STATUS_PROBES_ENABLED", "true")
	prevPing := pingTarget
	t.Cleanup(func() { pingTarget = prevPing })
	pingTarget = func(context.Context, *http.Client, string, int, int, bool) (Sample, error) {
		t.Fatal("ping started inside the interval")
		return Sample{}, nil
	}
	runner := NewRunner(nil)
	runner.loaded = true
	runner.samples["pl\nPoland"] = Sample{
		WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now().Add(-2 * time.Minute),
	}
	runner.schedule(context.Background(), func(context.Context) ([]Target, error) {
		return []Target{{Key: "pl\nPoland", Address: "203.0.113.10", Interval: 30 * time.Minute}}, nil
	})
	time.Sleep(40 * time.Millisecond)
}

func waitUntil(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestFreshPingWaitsForTheConfiguredInterval(t *testing.T) {
	now := time.Now()
	target := Target{Key: "pl", Interval: 30 * time.Minute, Whitelist: true}
	sample := Sample{
		MeasuredAt:     now.Add(-2 * time.Minute),
		WhitelistAt:    now.Add(-2 * time.Minute),
		WhitelistDone:  true,
		WorldTotal:     3,
		WhitelistTotal: 1,
		Probes:         []ProbeHit{{Kind: "whitelist", City: "Москва", Network: "МТС", OK: true}},
	}
	if pingDue(target, sample, true, now) {
		t.Fatal("ping ran again before the interval")
	}
	if whitelistDue(target, sample, true, now) {
		t.Fatal("whitelist ran again before the interval")
	}
	incomplete := sample
	incomplete.WhitelistDone = false
	incomplete.WhitelistAt = time.Time{}
	if !whitelistDue(target, incomplete, true, now) {
		t.Fatal("unfinished whitelist should run again")
	}
	if len(dueTargets([]Target{target}, map[string]Sample{target.Key: sample}, now)) != 0 {
		t.Fatal("target was scheduled inside the interval")
	}
	later := now.Add(31 * time.Minute)
	if !pingDue(target, sample, true, later) || !whitelistDue(target, sample, true, later) {
		t.Fatal("both checks should be due after the interval")
	}
}

func TestFailedPingRetriesInTenMinutes(t *testing.T) {
	now := time.Now()
	target := Target{Key: "pl", Interval: 30 * time.Minute}
	failed := Sample{MeasuredAt: now.Add(-11 * time.Minute), WorldOK: 1, WorldTotal: 3}
	if !pingDue(target, failed, true, now) {
		t.Fatal("failed ping should be due after 10 minutes")
	}
	failed.MeasuredAt = now.Add(-2 * time.Minute)
	if pingDue(target, failed, true, now) {
		t.Fatal("failed ping should wait 10 minutes")
	}
	okSample := Sample{MeasuredAt: now.Add(-11 * time.Minute), WorldOK: 3, WorldTotal: 3}
	if pingDue(target, okSample, true, now) {
		t.Fatal("successful ping should keep the configured interval")
	}
	if NextPingMinutes(30, failed) != 10 || NextPingMinutes(30, okSample) != 30 {
		t.Fatalf("next %d %d", NextPingMinutes(30, failed), NextPingMinutes(30, okSample))
	}
}

func TestIncompleteWhitelistResumesMissingOperators(t *testing.T) {
	hits := make([]ProbeHit, 0, 6)
	for _, pick := range belspiskiPicks[:6] {
		city, network := splitPick(pick, "")
		hits = append(hits, ProbeHit{Kind: "whitelist", City: city, Network: network, OK: true})
	}
	missing := missingWhitelistPicks(hits)
	if len(missing) != len(belspiskiPicks)-6 || missing[0] != belspiskiPicks[6] {
		t.Fatalf("missing %v", missing)
	}
	now := time.Now()
	target := Target{Key: "pl", Interval: 30 * time.Minute, Whitelist: true}
	sample := Sample{WhitelistAt: now.Add(-2 * time.Minute), Probes: hits}
	if whitelistDue(target, sample, true, now) {
		t.Fatal("partial whitelist retried too soon")
	}
	sample.WhitelistAt = now.Add(-11 * time.Minute)
	if !whitelistDue(target, sample, true, now) {
		t.Fatal("partial whitelist should resume after 10 minutes")
	}
}

func TestNewPingKeepsPreviousWhitelist(t *testing.T) {
	runner := NewRunner(nil)
	when := time.Now().Add(-time.Hour)
	runner.samples["pl"] = Sample{
		WhitelistOK:    1,
		WhitelistTotal: 1,
		WhitelistAt:    when,
		Probes:         []ProbeHit{{Kind: "whitelist", City: "Москва", Network: "МТС", OK: true}},
	}
	runner.storeSample("pl", Sample{
		WorldOK: 2, WorldTotal: 2, MeasuredAt: time.Now(),
		Probes: []ProbeHit{{City: "Berlin", OK: true}},
	})
	sample, _, ok := runner.Snapshot("pl")
	if !ok || sample.WorldTotal != 2 || sample.WhitelistTotal != 1 || sample.WhitelistAt.IsZero() {
		t.Fatalf("whitelist was dropped: %+v ok=%v", sample, ok)
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

func TestRetainSplitsSameNameAndDropsWhitelist(t *testing.T) {
	runner := NewRunner(nil)
	legacy := NodeKey("EE", "Эстония")
	runner.samples[legacy] = Sample{
		WorldOK: 1, WorldTotal: 1,
		WhitelistOK: 2, WhitelistTotal: 2, WhitelistDone: true,
		Probes: []ProbeHit{
			{City: "Berlin", OK: true},
			{Kind: "whitelist", City: "Москва", Network: "МТС", OK: true},
		},
	}
	plain := Target{Key: ProbeKey("ee-plain", "EE", "Эстония", "1.1.1.1"), Country: "EE", Name: "Эстония", Address: "1.1.1.1"}
	lists := Target{Key: ProbeKey("ee-list", "EE", "Эстония", "1.1.1.2"), Country: "EE", Name: "Эстония", Address: "1.1.1.2", Whitelist: true}
	runner.Retain([]Target{plain, lists})

	if _, _, ok := runner.Snapshot(legacy); ok {
		t.Fatal("shared legacy sample must not stay")
	}
	plainSample, _, ok := runner.Snapshot(plain.Key)
	if ok && (plainSample.WhitelistTotal != 0 || len(plainSample.Probes) != 0) {
		t.Fatalf("plain estonia inherited whitelist: %+v", plainSample)
	}
	if _, _, ok := runner.Snapshot(lists.Key); ok {
		t.Fatal("whitelist estonia must start clean when the name was shared")
	}

	runner.samples[legacy] = Sample{
		WorldOK: 1, WorldTotal: 1,
		WhitelistTotal: 1,
		Probes: []ProbeHit{
			{City: "Berlin", OK: true},
			{Kind: "whitelist", City: "Москва", Network: "МТС", OK: true},
		},
	}
	runner.Retain([]Target{plain})
	kept, _, ok := runner.Snapshot(plain.Key)
	if !ok || kept.WorldTotal != 1 || kept.WhitelistTotal != 0 || len(kept.Probes) != 1 || kept.Probes[0].Kind == "whitelist" {
		t.Fatalf("recreated estonia still has whitelist: %+v ok=%v", kept, ok)
	}
	if _, _, ok := runner.Snapshot(legacy); ok {
		t.Fatal("legacy key survived the move")
	}
}
