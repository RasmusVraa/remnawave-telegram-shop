package service

import (
	"testing"
	"time"
)

func TestNextTrafficReset(t *testing.T) {
	// Пятница, 30 октября 2026, 15:00 UTC.
	now := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	last := day(2026, 10, 18)
	oldLast := day(2026, 8, 18)

	cases := []struct {
		name     string
		strategy string
		last     *time.Time
		want     *time.Time
	}{
		{"day", "DAY", nil, resetTimePtr(day(2026, 10, 31))},
		{"week — ближайший понедельник", "WEEK", nil, resetTimePtr(day(2026, 11, 2))},
		{"month — первое число", "MONTH", nil, resetTimePtr(day(2026, 11, 1))},
		{"rolling — месяц от сброса", "MONTH_ROLLING", &last, resetTimePtr(day(2026, 11, 18))},
		{"rolling — пропущенные месяцы", "MONTH_ROLLING", &oldLast, resetTimePtr(day(2026, 11, 18))},
		{"rolling без даты сброса", "MONTH_ROLLING", nil, nil},
		{"без сброса", "NO_RESET", &last, nil},
		{"неизвестная", "WHATEVER", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextTrafficReset(tc.strategy, tc.last, now)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("want nil, got %v", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("want %v, got nil", *tc.want)
			case tc.want != nil && !got.Equal(*tc.want):
				t.Fatalf("want %v, got %v", *tc.want, *got)
			}
		})
	}

	// В понедельник следующий сброс — через неделю, а не сегодня в полночь.
	monday := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	if got := nextTrafficReset("WEEK", nil, monday); got == nil || !got.Equal(day(2026, 11, 9)) {
		t.Fatalf("week on monday: got %v", got)
	}
}

func resetTimePtr(t time.Time) *time.Time { return &t }
