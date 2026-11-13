package database

import (
	"testing"
	"time"
)

func TestTrafficPeriod_calendarMonth(t *testing.T) {
	now := time.Date(2026, 11, 25, 13, 40, 0, 0, time.FixedZone("MSK", 3*3600))
	if got, want := TrafficPeriodStart(now), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("start = %v, want %v", got, want)
	}
	if got, want := TrafficPeriodEnd(now), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("end = %v, want %v", got, want)
	}
	// Декабрь → январь следующего года.
	dec := time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)
	if got, want := TrafficPeriodEnd(dec), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("end(dec) = %v, want %v", got, want)
	}
}
