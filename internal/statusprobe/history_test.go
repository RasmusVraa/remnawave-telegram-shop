package statusprobe

import (
	"testing"
	"time"
)

func TestWindowIsOneBarPerRun(t *testing.T) {
	history := map[string][]RunRecord{}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	RecordRun(history, "pl", Sample{WorldOK: 1, WorldTotal: 1, RussiaOK: 10, RussiaTotal: 10}, now)
	RecordRun(history, "pl", Sample{WorldOK: 1, WorldTotal: 1, RussiaOK: 8, RussiaTotal: 10}, now.Add(time.Hour))

	window := Window(history["pl"])
	if len(window) != historyRuns {
		t.Fatalf("len %d", len(window))
	}
	if window[historyRuns-2] == nil || *window[historyRuns-2] != 1 {
		t.Fatalf("first run %+v", window[historyRuns-2])
	}
	last := window[historyRuns-1]
	if last == nil || *last != 9.0/11.0 {
		t.Fatalf("second run %+v", last)
	}
	if window[0] != nil {
		t.Fatal("places before the first run must stay empty")
	}
}

func TestRecordRunKeepsLastThirty(t *testing.T) {
	history := map[string][]RunRecord{}
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	for i := 0; i < historyRuns+5; i++ {
		ok := 1
		if i == 0 {
			ok = 0
		}
		RecordRun(history, "pl", Sample{WorldOK: ok, WorldTotal: 1}, now.Add(time.Duration(i)*time.Minute))
	}
	runs := history["pl"]
	if len(runs) != historyRuns {
		t.Fatalf("runs %d", len(runs))
	}
	// самый первый (провальный) замер вытеснен: в окне остались только удачные
	for i, v := range Window(runs) {
		if v == nil || *v != 1 {
			t.Fatalf("bar %d = %+v", i, v)
		}
	}
}

func TestRecordRunSkipsEmptySample(t *testing.T) {
	history := map[string][]RunRecord{}
	RecordRun(history, "pl", Sample{}, time.Now())
	if len(history) != 0 {
		t.Fatalf("empty sample recorded: %+v", history)
	}
}

func TestWindowForgivesSingleFailedProbe(t *testing.T) {
	runs := []RunRecord{
		{WorldOK: 1, WorldTotal: 1, RussiaOK: 9, RussiaTotal: 10},
		{WorldOK: 0, WorldTotal: 1, RussiaOK: 10, RussiaTotal: 10},
		{WorldOK: 1, WorldTotal: 1, RussiaOK: 8, RussiaTotal: 10},
		{WorldOK: 0, WorldTotal: 1, RussiaOK: 0, RussiaTotal: 1},
	}
	w := Window(runs)[historyRuns-len(runs):]
	if *w[0] != 1 || *w[1] != 1 {
		t.Fatalf("one failed probe must stay green: %v %v", *w[0], *w[1])
	}
	if *w[2] >= 1 {
		t.Fatalf("two failed probes must not be green: %v", *w[2])
	}
	if *w[3] != 0 {
		t.Fatalf("all failed must be zero: %v", *w[3])
	}
}

func TestWindowForBypassIgnoresWorld(t *testing.T) {
	runs := []RunRecord{{WorldOK: 0, WorldTotal: 3, RussiaOK: 10, RussiaTotal: 10}}
	if v := *WindowFor(runs, true)[historyRuns-1]; v != 1 {
		t.Fatalf("bypass reachable from Russia must be green, got %v", v)
	}
	if v := *WindowFor(runs, false)[historyRuns-1]; v == 1 {
		t.Fatalf("regular node keeps world probes, got %v", v)
	}
}
