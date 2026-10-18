package statusprobe

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestRunPingsEveryNodeBeforeWhitelist(t *testing.T) {
	prevPing := pingTarget
	prevList := listTarget
	t.Cleanup(func() {
		pingTarget = prevPing
		listTarget = prevList
	})

	var mu sync.Mutex
	var order []string
	pingTarget = func(_ context.Context, _ *http.Client, target string, _, _ int, _ bool) (Sample, error) {
		mu.Lock()
		order = append(order, "ping:"+target)
		mu.Unlock()
		return Sample{WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now()}, nil
	}
	listTarget = func(_ context.Context, _ *http.Client, host string, _ []string, onHit func(ProbeHit)) (whitelistResult, error) {
		mu.Lock()
		order = append(order, "list:"+host)
		mu.Unlock()
		hit := ProbeHit{Kind: "whitelist", Network: "МТС", Country: "RU", OK: true}
		if onHit != nil {
			onHit(hit)
		}
		return whitelistResult{OK: 1, Total: 1, Hits: []ProbeHit{hit}}, nil
	}

	runner := NewRunner(nil)
	runner.run([]Target{
		{Key: "pl", Address: "pl.example", Whitelist: true},
		{Key: "fi", Address: "fi.example"},
	})

	got := map[string]bool{}
	for _, item := range order {
		got[item] = true
	}
	for _, want := range []string{"ping:pl.example", "ping:fi.example", "list:pl.example"} {
		if !got[want] {
			t.Fatalf("order %v", order)
		}
	}
	if _, _, ok := runner.Snapshot("fi"); !ok {
		t.Fatal("finland ping was not stored")
	}
	sample, _, ok := runner.Snapshot("pl")
	if !ok || sample.WorldTotal != 1 || sample.WhitelistTotal != 1 {
		t.Fatalf("poland ping and whitelist %+v ok=%v", sample, ok)
	}
}
