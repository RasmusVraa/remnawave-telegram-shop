package statusprobe

import (
	"context"
	"net/http"
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

	var order []string
	pingTarget = func(_ context.Context, _ *http.Client, target string, _, _ int) (Sample, error) {
		order = append(order, "ping:"+target)
		return Sample{WorldOK: 1, WorldTotal: 1, MeasuredAt: time.Now()}, nil
	}
	listTarget = func(_ context.Context, _ *http.Client, host string, onHit func(ProbeHit)) (whitelistResult, error) {
		order = append(order, "list:"+host)
		if onHit != nil {
			onHit(ProbeHit{Kind: "whitelist", City: "Москва", Network: "МТС", OK: true})
		}
		return whitelistResult{OK: 1, Total: 1}, nil
	}

	runner := NewRunner(nil)
	runner.run([]Target{
		{Key: "pl", Address: "pl.example", Whitelist: true},
		{Key: "fi", Address: "fi.example"},
	})

	want := []string{"ping:pl.example", "ping:fi.example", "list:pl.example"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v", order)
		}
	}
	if _, _, ok := runner.Snapshot("fi"); !ok {
		t.Fatal("finland ping was not stored before whitelist")
	}
}
