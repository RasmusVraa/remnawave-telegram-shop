package statusprobe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func registryServer(t *testing.T, status int, body string) (host string, seen *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("target")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev := registryCheckURL
	registryCheckURL = srv.URL
	t.Cleanup(func() { registryCheckURL = prev })
	return srv.URL, &got
}

func TestCheckRegistryVerdicts(t *testing.T) {
	cases := []struct {
		name string
		body string
		want registryResult
	}{
		{"clean", `{"blocked":false,"blocked_subnets":[],"cdn_providers":{}}`, registryResult{}},
		{"registry", `{"blocked":true,"blocked_subnets":["203.0.113.0/24"],"cdn_providers":{}}`, registryResult{Blocked: true, Kind: RegistryKindRegistry}},
		{"cdn only", `{"blocked":true,"blocked_subnets":[],"cdn_providers":{"cloudflare":[{"cidr":"1.1.1.0/24"}]}}`, registryResult{Blocked: true, Kind: RegistryKindCDN}},
		{"registry wins over cdn", `{"blocked":true,"blocked_subnets":["x"],"cdn_providers":{"a":[1]}}`, registryResult{Blocked: true, Kind: RegistryKindRegistry}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, seen := registryServer(t, 200, c.body)
			got, err := checkRegistry(context.Background(), http.DefaultClient, "198.51.100.7")
			if err != nil || got != c.want {
				t.Fatalf("got %+v err %v, want %+v", got, err, c.want)
			}
			if *seen != "198.51.100.7" {
				t.Fatalf("target not sent: %q", *seen)
			}
		})
	}
}

func TestCheckRegistryFailures(t *testing.T) {
	registryServer(t, 429, `{}`)
	var limited *RateLimitError
	if _, err := checkRegistry(context.Background(), http.DefaultClient, "198.51.100.7"); !errors.As(err, &limited) {
		t.Fatalf("429 must be a rate limit: %v", err)
	}
	registryServer(t, 500, `oops`)
	if _, err := checkRegistry(context.Background(), http.DefaultClient, "198.51.100.7"); err == nil {
		t.Fatal("500 must fail")
	}
	// Ответ без вердикта — не «чисто», а сбой: молчаливое «чисто» спрятало бы блокировку.
	registryServer(t, 200, `{"target":"x"}`)
	if _, err := checkRegistry(context.Background(), http.DefaultClient, "198.51.100.7"); err == nil {
		t.Fatal("no verdict must fail")
	}
}

func stubRegistry(t *testing.T, results ...any) {
	t.Helper()
	prev := registryCheck
	t.Cleanup(func() { registryCheck = prev })
	i := 0
	registryCheck = func(context.Context, *http.Client, string) (registryResult, error) {
		if i >= len(results) {
			t.Fatalf("unexpected extra registry call")
		}
		v := results[i]
		i++
		if err, ok := v.(error); ok {
			return registryResult{}, err
		}
		return v.(registryResult), nil
	}
}

func runRegistry(r *Runner, target Target) []AlertEvent {
	var got []AlertEvent
	r.OnAlerts(func(ev []AlertEvent) { got = append(got, ev...) })
	r.registryPass(context.Background(), []Target{target})
	return got
}

func TestRegistryAlertOpensOnSecondFindingAndClosesOnClean(t *testing.T) {
	r := newAlertRunner(t)
	t.Setenv("STATUS_ALERT_REGISTRY", "")
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	listed := registryResult{Blocked: true, Kind: RegistryKindRegistry}
	stubRegistry(t, listed, listed, listed, registryResult{})

	if ev := runRegistry(r, target); len(ev) != 0 {
		t.Fatalf("first finding only arms: %+v", ev)
	}
	ev := runRegistry(r, target)
	if len(ev) != 1 || !ev[0].Open || ev[0].Reason != AlertRegistry || ev[0].Note != RegistryKindRegistry {
		t.Fatalf("second finding opens: %+v", ev)
	}
	if ev := runRegistry(r, target); len(ev) != 0 {
		t.Fatalf("no reminders while listed: %+v", ev)
	}
	ev = runRegistry(r, target)
	if len(ev) != 1 || ev[0].Open || ev[0].Reason != AlertRegistry {
		t.Fatalf("clean answer closes: %+v", ev)
	}
	if s := r.samples["de"]; len(s.Alerts) != 0 || s.RegistryBlocked || s.RegistryStreak != 0 {
		t.Fatalf("state not reset: %+v", s)
	}
}

func TestRegistryFailureKeepsPreviousAnswerAndIsRetriedSoon(t *testing.T) {
	r := newAlertRunner(t)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	stubRegistry(t, registryResult{Blocked: true, Kind: RegistryKindRegistry}, errors.New("boom"))
	runRegistry(r, target)
	runRegistry(r, target)
	s := r.samples["de"]
	if !s.RegistryBlocked || s.RegistryStreak != 1 || s.RegistryOK {
		t.Fatalf("a failed call must not erase or advance the answer: %+v", s)
	}
	if !registryDue(target, s, true, s.RegistryTryAt.Add(registryRecheck)) {
		t.Fatal("failed attempt must be retried after the short wait")
	}
	if registryDue(target, s, true, s.RegistryTryAt.Add(registryRecheck-time.Minute)) {
		t.Fatal("not due before the short wait")
	}
}

func TestRegistryAlertRespectsSetting(t *testing.T) {
	r := newAlertRunner(t)
	t.Setenv("STATUS_ALERT_REGISTRY", "false")
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	listed := registryResult{Blocked: true, Kind: RegistryKindCDN}
	stubRegistry(t, listed, listed)
	runRegistry(r, target)
	if ev := runRegistry(r, target); len(ev) != 0 {
		t.Fatalf("setting off must silence the alert: %+v", ev)
	}
	// Значок при этом остаётся: настройка гасит только оповещение.
	sample := r.samples["de"]
	if got := registryOf(&sample); got.State != CheckBad || got.Kind != RegistryKindCDN {
		t.Fatalf("badge must still show the listing: %+v", got)
	}
}

func TestRegistryRateLimitPausesWithoutMarking(t *testing.T) {
	r := newAlertRunner(t)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	stubRegistry(t, &RateLimitError{RetryAfter: 20 * time.Minute})
	runRegistry(r, target)
	if until := r.RegistryPause(); time.Until(until) < 19*time.Minute {
		t.Fatalf("pause not set: %v", until)
	}
	if picks := r.pickRegistryLocked([]Target{target}, false); len(picks) != 0 {
		t.Fatalf("paused runner must not pick: %+v", picks)
	}
	if picks := r.pickRegistryLocked([]Target{target}, true); len(picks) != 1 {
		t.Fatalf("force ignores the pause: %+v", picks)
	}
}

func TestRegistrySkipsBypassDisabledAndNoExternal(t *testing.T) {
	r := newAlertRunner(t)
	picks := r.pickRegistryLocked([]Target{
		{Key: "a", Address: "8.8.8.8"},
		{Key: "b", Address: "ya.ru", Whitelist: true},
		{Key: "c", Address: "8.8.4.4", Disabled: true},
		{Key: "d", Address: "1.1.1.1", NoExternal: true},
	}, true)
	if len(picks) != 1 || picks[0].Key != "a" {
		t.Fatalf("only the plain external row is checked: %+v", picks)
	}
}

func TestRegistryKeptAcrossPingStore(t *testing.T) {
	r := newAlertRunner(t)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	stubRegistry(t, registryResult{Blocked: true, Kind: RegistryKindRegistry})
	runRegistry(r, target)
	// Обычный пинг пишет замер целиком: проверка списков не должна пропасть.
	r.storeSample("de", ruSample(10))
	s := r.samples["de"]
	if !s.RegistryBlocked || s.RegistryKind != RegistryKindRegistry || s.RegistryAt.IsZero() || s.RegistryStreak != 1 {
		t.Fatalf("ping erased the registry answer: %+v", s)
	}
}

func TestRegistryDueCadence(t *testing.T) {
	target := Target{Key: "a"}
	now := time.Now()
	clean := Sample{RegistryOK: true, RegistryTryAt: now.Add(-registryEvery + time.Minute)}
	if registryDue(target, clean, true, now) {
		t.Fatal("clean answer is rechecked every registryEvery")
	}
	if !registryDue(target, Sample{}, false, now) {
		t.Fatal("never checked is due")
	}
	firstFinding := Sample{RegistryOK: true, RegistryBlocked: true, RegistryStreak: 1, RegistryTryAt: now.Add(-registryRecheck)}
	if !registryDue(target, firstFinding, true, now) {
		t.Fatal("first finding is confirmed after the short wait")
	}
	confirmed := Sample{RegistryOK: true, RegistryBlocked: true, RegistryStreak: alertStreak, RegistryTryAt: now.Add(-registryRecheck)}
	if registryDue(target, confirmed, true, now) {
		t.Fatal("confirmed finding goes back to the normal cadence")
	}
}

func TestChecksOfRegistry(t *testing.T) {
	if got := ChecksOf(nil, false).Registry.State; got != CheckNone {
		t.Fatalf("no sample: %q", got)
	}
	if got := ChecksOf(&Sample{}, false).Registry.State; got != CheckNone {
		t.Fatalf("never checked: %q", got)
	}
	at := time.Now()
	ok := ChecksOf(&Sample{RegistryAt: at, RegistryOK: true}, false).Registry
	if ok.State != CheckOK || ok.CheckedAt == nil {
		t.Fatalf("clean: %+v", ok)
	}
	bad := ChecksOf(&Sample{RegistryAt: at, RegistryBlocked: true, RegistryKind: RegistryKindCDN}, false).Registry
	if bad.State != CheckBad || bad.Kind != RegistryKindCDN {
		t.Fatalf("listed: %+v", bad)
	}
}

func TestNoExternalDropsRegistryData(t *testing.T) {
	r := newAlertRunner(t)
	r.loaded = true
	r.samples["a"] = Sample{RegistryAt: time.Now(), RegistryTryAt: time.Now(), RegistryOK: true, RegistryBlocked: true, RegistryStreak: 2,
		Alerts: map[string]time.Time{AlertRegistry: time.Now()}}
	r.Retain([]Target{{Key: "a", Address: "8.8.8.8", NoExternal: true}})
	s := r.samples["a"]
	if !s.RegistryAt.IsZero() || s.RegistryBlocked || len(s.Alerts) != 0 {
		t.Fatalf("registry data must go with the external check: %+v", s)
	}
}
