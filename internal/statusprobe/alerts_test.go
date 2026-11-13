package statusprobe

import (
	"testing"
	"time"
)

func newAlertRunner(t *testing.T) *Runner {
	t.Helper()
	t.Setenv("STATUS_PROBES_ENABLED", "")
	t.Setenv("STATUS_RU_FAIL_THRESHOLD", "")
	t.Setenv("STATUS_ALERT_WORLD", "")
	t.Setenv("LATENCYLAB_TOKEN", "")
	return &Runner{samples: map[string]Sample{}, history: map[string][]RunRecord{}}
}

func ruSample(ok int) Sample {
	s := Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: ok}
	for i := 0; i < 10; i++ {
		s.Probes = append(s.Probes, ProbeHit{Country: "RU", City: "Moscow", Network: "Net" + string(rune('A'+i)), OK: i < ok})
	}
	return s
}

func (r *Runner) pingAndCheck(target Target, s Sample, at time.Time) []AlertEvent {
	r.storeSample(target.Key, s)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pingAlertsLocked(target, at)
}

func TestRussiaAlertOpensAfterTwoAndClosesOnFirstGood(t *testing.T) {
	r := newAlertRunner(t)
	target := Target{Key: "de", Name: "Германия", Country: "DE"}
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

	if ev := r.pingAndCheck(target, ruSample(9), t0); len(ev) != 0 {
		t.Fatalf("one failure is noise: %+v", ev)
	}
	if ev := r.pingAndCheck(target, ruSample(7), t0); len(ev) != 0 {
		t.Fatalf("first bad measurement only arms: %+v", ev)
	}
	ev := r.pingAndCheck(target, ruSample(7), t0.Add(10*time.Minute))
	if len(ev) != 1 || !ev[0].Open || ev[0].Reason != AlertRussia || ev[0].RussiaOK != 7 || len(ev[0].Failed) != 3 {
		t.Fatalf("second bad measurement opens: %+v", ev)
	}
	if ev := r.pingAndCheck(target, ruSample(6), t0.Add(20*time.Minute)); len(ev) != 0 {
		t.Fatalf("no reminders while open: %+v", ev)
	}
	ev = r.pingAndCheck(target, ruSample(9), t0.Add(50*time.Minute))
	if len(ev) != 1 || ev[0].Open || ev[0].At.Sub(ev[0].Since) != 40*time.Minute {
		t.Fatalf("first good measurement closes with duration: %+v", ev)
	}
	if len(r.samples["de"].Alerts) != 0 {
		t.Fatalf("alerts left: %+v", r.samples["de"].Alerts)
	}
}

func TestWorldAlertRespectsSetting(t *testing.T) {
	r := newAlertRunner(t)
	target := Target{Key: "se", Name: "Швеция", Country: "SE"}
	silent := ruSample(10)
	silent.WorldOK = 0
	r.pingAndCheck(target, silent, time.Now())
	ev := r.pingAndCheck(target, silent, time.Now())
	if len(ev) != 1 || ev[0].Reason != AlertWorld || !ev[0].Open {
		t.Fatalf("world opens: %+v", ev)
	}
	t.Setenv("STATUS_ALERT_WORLD", "false")
	if ev := r.pingAndCheck(target, silent, time.Now()); len(ev) != 0 || len(r.samples["se"].Alerts) != 0 {
		t.Fatalf("switched off — forgotten silently: %+v %+v", ev, r.samples["se"].Alerts)
	}
}

func TestBypassJudgedByOperators(t *testing.T) {
	r := newAlertRunner(t)
	t.Setenv("LATENCYLAB_TOKEN", "x")
	target := Target{Key: "lte", Name: "Обход", Country: "RU", Whitelist: true}
	// Датацентры России и мир молчат — у обхода с операторами это не повод.
	dead := ruSample(3)
	dead.WorldOK = 0

	// Пока операторов нет, обход судится по России: две плохие проверки — тревога.
	r.pingAndCheck(target, dead, time.Now())
	if ev := r.pingAndCheck(target, dead, time.Now()); len(ev) != 1 || ev[0].Reason != AlertRussia {
		t.Fatalf("without operators russia decides: %+v", ev)
	}

	ops := []ProbeHit{{Network: "МегаФон", Kind: "whitelist"}, {Network: "Билайн", Kind: "whitelist", OK: true}}
	check := func() []AlertEvent {
		r.finishWhitelist("lte", ops, nil, true)
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.operatorAlertsLocked(target, time.Now())
	}
	if ev := check(); len(ev) != 0 {
		t.Fatalf("one operator pass only arms: %+v", ev)
	}
	ev := check()
	if len(ev) != 1 || ev[0].Reason != AlertOperators || len(ev[0].Failed) != 1 || ev[0].Failed[0] != "МегаФон" {
		t.Fatalf("operators open: %+v", ev)
	}
	if ev := r.pingAndCheck(target, dead, time.Now()); len(ev) != 0 {
		t.Fatalf("with operator data russia and world are ignored: %+v", ev)
	}
	if _, open := r.samples["lte"].Alerts[AlertRussia]; open {
		t.Fatal("russia alert is forgotten once operators judge")
	}
	ops[0].OK = true
	if ev := check(); len(ev) != 1 || ev[0].Open {
		t.Fatalf("all operators back: %+v", ev)
	}
}

func TestRetainForgetsAlertsOfDisabledRows(t *testing.T) {
	r := newAlertRunner(t)
	r.samples["a"] = Sample{Alerts: map[string]time.Time{AlertRussia: time.Now()}}
	r.retainLocked([]Target{{Key: "a", Disabled: true}})
	if len(r.samples["a"].Alerts) != 0 {
		t.Fatal("disabled row keeps no alerts")
	}
}
