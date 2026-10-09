package statusprobe

import "testing"

func TestProbeLimitsDefaultsAndClamp(t *testing.T) {
	t.Setenv("STATUS_WORLD_PROBES", "")
	t.Setenv("STATUS_RUSSIA_PROBES", "")
	w, ru := ProbeLimits()
	if w != 3 || ru != 20 {
		t.Fatalf("defaults: world=%d russia=%d", w, ru)
	}

	t.Setenv("STATUS_WORLD_PROBES", "8")
	t.Setenv("STATUS_RUSSIA_PROBES", "99")
	w, ru = ProbeLimits()
	if w != 8 || ru != 20 {
		t.Fatalf("clamp: world=%d russia=%d", w, ru)
	}
	if ProbeIntervalMinutes() != 20 {
		t.Fatalf("interval default %d", ProbeIntervalMinutes())
	}
}

func TestProbesEnabledDefault(t *testing.T) {
	t.Setenv("STATUS_PROBES_ENABLED", "")
	if !ProbesEnabled() || !ShowMap() {
		t.Fatal("empty means on")
	}
	t.Setenv("STATUS_PROBES_ENABLED", "false")
	t.Setenv("STATUS_SHOW_MAP", "0")
	if ProbesEnabled() || ShowMap() {
		t.Fatal("explicit off")
	}
}
