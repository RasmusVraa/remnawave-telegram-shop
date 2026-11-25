package config

import (
	"os"
	"testing"
	"time"
)

func TestDecorTheme_defaultOff(t *testing.T) {
	_ = os.Unsetenv("CABINET_DECOR_THEME")
	if got := DecorTheme(); got != "off" {
		t.Fatalf("got %q", got)
	}
}

func TestDecorTheme_knownValue(t *testing.T) {
	t.Setenv("CABINET_DECOR_THEME", "green")
	if got := DecorTheme(); got != "green" {
		t.Fatalf("got %q", got)
	}
}

func TestDecorTheme_unknownFallsBackToOff(t *testing.T) {
	t.Setenv("CABINET_DECOR_THEME", "easter")
	if got := DecorTheme(); got != "off" {
		t.Fatalf("got %q", got)
	}
}

func TestDecorTheme_atmosphericPresets(t *testing.T) {
	for _, id := range []string{"violet", "slate", "aurora", "ocean", "cyber", "lime", "emerald", "sunset", "lavender", "wine"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv("CABINET_DECOR_THEME", id)
			if got := DecorTheme(); got != id {
				t.Fatalf("got %q", got)
			}
			if !IsValidDecorTheme(id) {
				t.Fatalf("expected %q valid", id)
			}
		})
	}
}

func TestEffectiveDecorTheme_autoDisabled(t *testing.T) {
	t.Setenv("CABINET_DECOR_AUTO_ENABLED", "false")
	t.Setenv("CABINET_DECOR_THEME", "carbon")
	t.Setenv("CABINET_DECOR_SCHEDULE", `[{"theme":"halloween","from":"01-01","to":"12-31"}]`)
	if got := EffectiveDecorTheme(); got != "carbon" {
		t.Fatalf("got %q, want carbon", got)
	}
}

func TestScheduledDecorTheme_windowWins(t *testing.T) {
	t.Setenv("CABINET_DECOR_AUTO_ENABLED", "true")
	t.Setenv("CABINET_DECOR_THEME", "carbon")
	t.Setenv("CABINET_DECOR_SCHEDULE", `[{"theme":"valentine","from":"02-07","to":"02-21"}]`)

	inside := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	if got := ScheduledDecorTheme(inside); got != "valentine" {
		t.Fatalf("inside window: got %q, want valentine", got)
	}
	outside := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	if got := ScheduledDecorTheme(outside); got != "" {
		t.Fatalf("outside window: got %q, want empty", got)
	}
}

func TestDecorScheduleRules_emptyUsesPreset(t *testing.T) {
	t.Setenv("CABINET_DECOR_SCHEDULE", "")
	if len(DecorScheduleRules()) == 0 {
		t.Fatal("empty CABINET_DECOR_SCHEDULE must fall back to the built-in preset")
	}
}

func TestDecorScheduleRules_brokenJSONDisablesAuto(t *testing.T) {
	t.Setenv("CABINET_DECOR_AUTO_ENABLED", "true")
	t.Setenv("CABINET_DECOR_SCHEDULE", `[{"theme":`)
	if rules := DecorScheduleRules(); rules != nil {
		t.Fatalf("broken JSON must yield no rules, got %+v", rules)
	}
	if got := ScheduledDecorTheme(time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
