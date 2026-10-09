package statusprobe

import (
	"testing"
	"time"
)

func TestWindowLeavesGaps(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	per := map[string]DayRecord{
		"2026-10-09": {WorldOK: 3, WorldTotal: 3, RussiaOK: 5, RussiaTotal: 5},
		"2026-09-01": {WorldOK: 1, WorldTotal: 1},
	}
	history := map[string]map[string]DayRecord{
		"NL\nАмстердам": {"2026-09-01": {WorldOK: 1, WorldTotal: 1}},
	}
	RecordDay(history, "NL\nАмстердам", Sample{WorldOK: 1, WorldTotal: 1}, now)
	window := Window(per, now)
	if len(window) != 30 {
		t.Fatalf("len %d", len(window))
	}
	if window[29] == nil || *window[29] != 1 {
		t.Fatalf("today %+v", window[29])
	}
	if window[0] != nil {
		t.Fatal("day outside the measured set must stay empty")
	}
	if _, old := history["NL\nАмстердам"]["2026-09-01"]; old {
		t.Fatal("stale day should be dropped when recording")
	}
}
