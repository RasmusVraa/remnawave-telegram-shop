package statusprobe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const chebStream = `event: started
data: {"id":"x","online_probes":43,"target":"203.0.113.7"}

event: result
data: {"asn":"AS42610","provider":"Rostelecom","region":"Москва","verdicts":["ok"]}

event: result
data: {"asn":"AS8402","provider":"Beeline","region":"Ярославская область","verdicts":["tspu_block"]}

event: result
data: {"asn":"AS1","provider":"MTS","region":"Тюмень","verdicts":["uncertain"]}

event: result
data: {"asn":"AS2","provider":"","region":"Омск","verdicts":["ok","sni_block"]}

event: done
data: {"id":"x","status":"done"}

`

func TestReadChebStreamCountsOnlyDecidedScanners(t *testing.T) {
	res, err := readChebStream(strings.NewReader(chebStream), false)
	if err != nil {
		t.Fatal(err)
	}
	// ok, tspu_block и ok+sni_block — три вердикта, uncertain не считается.
	if res.Total != 3 || res.Pass != 1 || len(res.Hits) != 3 {
		t.Fatalf("got %+v", res)
	}
	if res.Hits[0].Network != "Rostelecom" || res.Hits[0].City != "Москва" || !res.Hits[0].OK || res.Hits[0].Country != "RU" {
		t.Fatalf("hit: %+v", res.Hits[0])
	}
	if res.Hits[2].Network != "AS2" || res.Hits[2].OK {
		t.Fatalf("ok with a block verdict is a block, provider falls back to ASN: %+v", res.Hits[2])
	}
}

func TestReadChebStreamKeepsPartialResultOnBrokenStream(t *testing.T) {
	cut := chebStream[:strings.Index(chebStream, "event: result\ndata: {\"asn\":\"AS1\"")] + "event: result\ndata: {\"asn\":\"A"
	res, err := readChebStream(strings.NewReader(cut), false)
	if err != nil || res.Total != 2 {
		t.Fatalf("partial stream: %+v err %v", res, err)
	}
}

func chebServer(t *testing.T, probeStatus int, body string) *string {
	t.Helper()
	var auth string
	mux := http.NewServeMux()
	mux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("target") == "" {
			http.Error(w, "no target", 422)
			return
		}
		_, _ = w.Write([]byte(`{"id":"signed.id.value","blocked":false}`))
	})
	mux.HandleFunc("/probe/", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(probeStatus)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	prev := cheburcheckAPI
	cheburcheckAPI = srv.URL
	t.Cleanup(func() { cheburcheckAPI = prev })
	return &auth
}

func TestMeasureCheburcheckSendsBearerAndParses(t *testing.T) {
	t.Setenv("CHEBURCHECK_TOKEN", "secret-token")
	auth := chebServer(t, 200, chebStream)
	res, err := measureCheburcheck(context.Background(), "203.0.113.7")
	if err != nil || res.Total != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	if *auth != "Bearer secret-token" {
		t.Fatalf("auth header: %q", *auth)
	}
}

func TestMeasureCheburcheckTakesRegistryFromTheSameCheck(t *testing.T) {
	t.Setenv("CHEBURCHECK_TOKEN", "secret-token")
	chebServer(t, 200, chebStream)
	id, reg, err := chebCheckID(context.Background(), "203.0.113.7")
	if err != nil || id != "signed.id.value" || reg == nil || reg.Blocked {
		t.Fatalf("%q %+v %v", id, reg, err)
	}
	res, err := measureCheburcheck(context.Background(), "203.0.113.7")
	if err != nil || res.Registry == nil || res.Registry.Blocked {
		t.Fatalf("measure must carry the registry answer: %+v %v", res, err)
	}
}

func TestChebPassFillsRegistryWithoutSeparateCheck(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	prev := chebMeasure
	t.Cleanup(func() { chebMeasure = prev })
	chebMeasure = func(context.Context, string) (chebResult, error) {
		res := chebHits(10, 0)
		res.Registry = &registryResult{Blocked: true, Kind: RegistryKindCDN}
		return res, nil
	}
	runCheb(r, target)
	s := r.samples["de"]
	if !s.RegistryOK || !s.RegistryBlocked || s.RegistryKind != RegistryKindCDN || s.RegistryStreak != 1 {
		t.Fatalf("registry must come from the same /check: %+v", s)
	}
}

func TestRegistryPassIsSkippedWhileCheburcheckCoversIt(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	r.mu.Lock()
	if got := r.pickRegistryLocked([]Target{target}, true); len(got) != 0 {
		t.Fatalf("cheburcheck measures list the registry itself: %+v", got)
	}
	r.chebCooldownUntil = time.Now().Add(time.Hour)
	if got := r.pickRegistryLocked([]Target{target}, true); len(got) != 1 {
		t.Fatalf("on pause the registry goes by itself: %+v", got)
	}
	r.mu.Unlock()
	t.Setenv("STATUS_RU_SOURCE", SourceGlobalping)
	r.mu.Lock()
	r.chebCooldownUntil = time.Time{}
	got := r.pickRegistryLocked([]Target{target}, true)
	r.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("without cheburcheck as the source the registry goes by itself: %+v", got)
	}
}

func TestMeasureCheburcheckErrors(t *testing.T) {
	t.Setenv("CHEBURCHECK_TOKEN", "secret-token")
	chebServer(t, 429, `{}`)
	var limited *RateLimitError
	if _, err := measureCheburcheck(context.Background(), "203.0.113.7"); !errors.As(err, &limited) {
		t.Fatalf("429 must be a rate limit: %v", err)
	}
	chebServer(t, 401, `{}`)
	if _, err := measureCheburcheck(context.Background(), "203.0.113.7"); !errors.Is(err, errChebToken) {
		t.Fatalf("401 must be a token error: %v", err)
	}
	chebServer(t, 200, "event: result\ndata: {\"verdicts\":[\"uncertain\"]}\n\nevent: done\ndata: {}\n\n")
	if _, err := measureCheburcheck(context.Background(), "203.0.113.7"); !errors.Is(err, errChebNoVerdicts) {
		t.Fatalf("only uncertain answers is no data, not «all fine»: %v", err)
	}
	t.Setenv("CHEBURCHECK_TOKEN", "")
	if _, err := measureCheburcheck(context.Background(), "203.0.113.7"); !errors.Is(err, errChebToken) {
		t.Fatalf("no token: %v", err)
	}
}

func TestRuFromCheburcheck(t *testing.T) {
	now := time.Now()
	fresh, stale := now.Add(-time.Hour), now.Add(-chebFresh-time.Minute)
	cases := []struct {
		name       string
		mode       string
		configured bool
		paused     bool
		at         time.Time
		want       bool
	}{
		{"globalping mode", SourceGlobalping, true, false, fresh, false},
		{"no key", SourceAuto, false, false, fresh, false},
		{"no data yet", SourceCheburcheck, true, false, time.Time{}, false},
		{"auto fresh", SourceAuto, true, false, fresh, true},
		{"auto paused falls back", SourceAuto, true, true, fresh, false},
		{"auto stale falls back", SourceAuto, true, false, stale, false},
		{"explicit keeps stale data, no fallback", SourceCheburcheck, true, true, stale, true},
	}
	for _, c := range cases {
		if got := ruFromCheburcheck(c.mode, c.configured, c.paused, c.at, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func chebRunner(t *testing.T, mode string) *Runner {
	t.Helper()
	// Проходы в тестах идут без минутной паузы между серверами.
	prevGap := chebGap
	chebGap = 0
	t.Cleanup(func() { chebGap = prevGap })
	r := newAlertRunner(t)
	t.Setenv("CHEBURCHECK_TOKEN", "tok")
	t.Setenv("STATUS_RU_SOURCE", mode)
	return r
}

func chebStub(t *testing.T, results ...any) {
	t.Helper()
	prev := chebMeasure
	t.Cleanup(func() { chebMeasure = prev })
	i := 0
	chebMeasure = func(context.Context, string) (chebResult, error) {
		if i >= len(results) {
			t.Fatalf("unexpected extra measure")
		}
		v := results[i]
		i++
		if err, ok := v.(error); ok {
			return chebResult{}, err
		}
		return v.(chebResult), nil
	}
}

// chebHits собирает ответ: blocked сканеров из total не открыли сервер.
func chebHits(total, blocked int) chebResult {
	res := chebResult{Total: total, Pass: total - blocked}
	for i := 0; i < total; i++ {
		res.Hits = append(res.Hits, ProbeHit{Country: "RU", City: "Город" + string(rune('A'+i%26)), Network: "ISP" + string(rune('A'+i%26)), OK: i >= blocked})
	}
	return res
}

func runCheb(r *Runner, target Target) []AlertEvent {
	var got []AlertEvent
	r.OnAlerts(func(ev []AlertEvent) { got = append(got, ev...) })
	r.chebPass(context.Background(), []Target{target})
	return got
}

func TestChebPassFillsRussiaAndAlertsOnSecondBadPass(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 5), chebHits(40, 5), chebHits(40, 0))

	if ev := runCheb(r, target); len(ev) != 0 {
		t.Fatalf("one bad pass only arms: %+v", ev)
	}
	s := r.samples["de"]
	if s.RuSource != SourceCheburcheck || s.RussiaOK != 35 || s.RussiaTotal != 40 || s.RussiaFailStreak != 1 {
		t.Fatalf("russia must come from the scanners: %+v", s)
	}
	ev := runCheb(r, target)
	if len(ev) != 1 || !ev[0].Open || ev[0].Reason != AlertRussia || len(ev[0].Failed) != 5 || !strings.Contains(ev[0].Failed[0], "ISP") {
		t.Fatalf("second bad pass opens an alert naming the scanners: %+v", ev)
	}
	ev = runCheb(r, target)
	if len(ev) != 1 || ev[0].Open {
		t.Fatalf("clean pass closes it: %+v", ev)
	}
}

func TestGlobalpingSampleKeepsCheburcheckRussia(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 5))
	runCheb(r, target)

	// Обычный пинг Globalping приходит с одним зондом из России: Cheburcheck остаётся источником.
	gp := Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 1, RussiaOK: 1}
	gp.Probes = []ProbeHit{{Country: "DE", City: "Frankfurt", OK: true, PingMs: intp(20)}, {Country: "RU", City: "Moscow", OK: true}}
	r.storeSample("de", gp)
	s := r.samples["de"]
	if s.RuSource != SourceCheburcheck || s.RussiaTotal != 40 || s.RussiaOK != 35 {
		t.Fatalf("ping must not overwrite Russia: %+v", s)
	}
	if s.RussiaFailStreak != 1 {
		t.Fatalf("ping must not advance the Russia streak: %d", s.RussiaFailStreak)
	}
	var ru, world int
	for _, hit := range s.Probes {
		if hit.Country == "RU" {
			ru++
		} else {
			world++
		}
	}
	if ru != 40 || world != 1 {
		t.Fatalf("probes: %d RU (want 40 scanners), %d world (want 1)", ru, world)
	}
}

func TestAutoFallsBackToGlobalpingWhenPaused(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 0))
	runCheb(r, target)
	r.pauseCheb(time.Hour, "HTTP 429")

	gp := Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 9}
	r.storeSample("de", gp)
	s := r.samples["de"]
	if s.RuSource != SourceGlobalping || s.RussiaTotal != 10 || s.RussiaOK != 9 {
		t.Fatalf("paused auto must use Globalping: %+v", s)
	}
	if r.ActiveRussiaSource() != SourceGlobalping {
		t.Fatalf("active source: %s", r.ActiveRussiaSource())
	}
}

func TestExplicitCheburcheckHasNoFallback(t *testing.T) {
	r := chebRunner(t, SourceCheburcheck)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 0))
	runCheb(r, target)
	r.pauseCheb(time.Hour, "HTTP 429")
	r.storeSample("de", Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 2})
	if s := r.samples["de"]; s.RuSource != SourceCheburcheck || s.RussiaTotal != 40 {
		t.Fatalf("explicit mode keeps Cheburcheck: %+v", s)
	}
}

func TestPingUsesOneRussiaProbeWhenCheburcheckIsSource(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 0))
	runCheb(r, target)

	prev := pingTarget
	t.Cleanup(func() { pingTarget = prev })
	var gotWorld, gotRussia int
	var gotRepeat bool
	pingTarget = func(_ context.Context, _ *http.Client, _ string, _, world, russia int, repeat bool) (Sample, error) {
		gotWorld, gotRussia, gotRepeat = world, russia, repeat
		return Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 1, RussiaOK: 1}, nil
	}
	r.pingPass(context.Background(), []Target{target})
	if gotRussia != 1 || !gotRepeat {
		t.Fatalf("Globalping must not spend its Russia probes: world %d russia %d repeat %v", gotWorld, gotRussia, gotRepeat)
	}

	// Cheburcheck на паузе: Globalping снова меряет Россию полностью.
	r.pauseCheb(time.Hour, "HTTP 429")
	r.pingPass(context.Background(), []Target{target})
	if gotRussia <= 1 {
		t.Fatalf("fallback needs the full Russia probe count: %d", gotRussia)
	}
}

func TestChebFailureKeepsPreviousAnswerAndPausesOn429(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	target := Target{Key: "de", Name: "Германия", Country: "DE", Address: "8.8.8.8"}
	chebStub(t, chebHits(40, 0), errors.New("boom"), &RateLimitError{RetryAfter: 20 * time.Minute})
	runCheb(r, target)
	runCheb(r, target)
	s := r.samples["de"]
	if s.ChebOK || s.ChebTotal != 40 || s.RussiaTotal != 40 {
		t.Fatalf("a failed pass keeps the answer: %+v", s)
	}
	if !chebDue(target, s, true, s.ChebTryAt.Add(chebRecheck)) {
		t.Fatal("failed pass is retried after the short wait")
	}
	runCheb(r, target)
	if until, note := r.CheburcheckPause(); time.Until(until) < 19*time.Minute || note == "" {
		t.Fatalf("pause not set: %v %q", until, note)
	}
}

func TestChebSkipsBypassDisabledNoExternalAndNeedsKey(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	picks := r.pickChebLocked([]Target{
		{Key: "a", Address: "8.8.8.8"},
		{Key: "b", Address: "ya.ru", Whitelist: true},
		{Key: "c", Address: "8.8.4.4", Disabled: true},
		{Key: "d", Address: "1.1.1.1", NoExternal: true},
	}, true)
	if len(picks) != 1 || picks[0].Key != "a" {
		t.Fatalf("only the plain external row: %+v", picks)
	}
	t.Setenv("CHEBURCHECK_TOKEN", "")
	if chebDue(Target{Key: "a"}, Sample{}, false, time.Now()) {
		t.Fatal("without a key nothing is due")
	}
	t.Setenv("CHEBURCHECK_TOKEN", "tok")
	t.Setenv("STATUS_RU_SOURCE", SourceGlobalping)
	if chebDue(Target{Key: "a"}, Sample{}, false, time.Now()) {
		t.Fatal("globalping mode does not call Cheburcheck")
	}
}

func TestChebCadenceIsAtLeastHalfAnHour(t *testing.T) {
	t.Setenv("CHEBURCHECK_TOKEN", "tok")
	t.Setenv("STATUS_RU_SOURCE", "")
	now := time.Now()
	target := Target{Key: "a", Interval: 5 * time.Minute}
	s := Sample{ChebOK: true, ChebTryAt: now.Add(-20 * time.Minute), ChebTotal: 40, ChebPass: 40}
	if chebDue(target, s, true, now) {
		t.Fatal("a row with a 5 minute interval still waits half an hour")
	}
	s.ChebTryAt = now.Add(-31 * time.Minute)
	if !chebDue(target, s, true, now) {
		t.Fatal("due after half an hour")
	}
}

func TestChecksOfNamesSourceAndFailedScanners(t *testing.T) {
	s := Sample{RussiaOK: 35, RussiaTotal: 40, RuSource: SourceCheburcheck, ChebHits: chebHits(40, 6).Hits}
	got := ChecksOf(&s, false)
	if got.RussiaSource != SourceCheburcheck || len(got.RussiaFailed) != 4 {
		t.Fatalf("%+v", got)
	}
	gp := ChecksOf(&Sample{RussiaOK: 9, RussiaTotal: 10}, false)
	if gp.RussiaSource != SourceGlobalping || len(gp.RussiaFailed) != 0 {
		t.Fatalf("%+v", gp)
	}
}

func intp(v int) *int { return &v }

func TestChebPassPacesMeasuresAcrossServersAndPasses(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	chebGap = 80 * time.Millisecond
	var starts []time.Time
	prev := chebMeasure
	t.Cleanup(func() { chebMeasure = prev })
	chebMeasure = func(context.Context, string) (chebResult, error) {
		starts = append(starts, time.Now())
		return chebHits(10, 0), nil
	}
	targets := []Target{
		{Key: "a", Address: "8.8.8.8"},
		{Key: "b", Address: "8.8.4.4"},
		{Key: "c", Address: "1.1.1.1"},
	}
	r.chebPass(context.Background(), targets[:2])
	// Второй проход сразу следом: слот общий, пауза выдерживается и между проходами.
	r.chebPass(context.Background(), targets[2:])
	if len(starts) != 3 {
		t.Fatalf("measures: %d", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 70*time.Millisecond {
			t.Fatalf("gap %d too short: %v", i, gap)
		}
	}
}

func TestChebPassStopsWhileWaitingForASlot(t *testing.T) {
	r := chebRunner(t, SourceAuto)
	chebGap = time.Hour
	calls := 0
	prev := chebMeasure
	t.Cleanup(func() { chebMeasure = prev })
	chebMeasure = func(context.Context, string) (chebResult, error) {
		calls++
		return chebHits(10, 0), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	r.chebPass(ctx, []Target{{Key: "a", Address: "8.8.8.8"}, {Key: "b", Address: "8.8.4.4"}})
	if calls != 1 {
		t.Fatalf("the second server must wait for its slot and give up with the context: %d calls", calls)
	}
}

func TestChebDisplayVerdictsFollowTheSite(t *testing.T) {
	scanner := func(unblocked bool, hosts int, verdicts ...string) chebScanner {
		return chebScanner{Verdicts: verdicts, CDNUnblocked: unblocked, HostResults: make([]json.RawMessage, hosts)}
	}
	cases := []struct {
		name          string
		sc            chebScanner
		staticCDN     bool
		wantOK, isBad bool
	}{
		{"cdn_block, server outside CDN ranges: the limit does not touch it", scanner(false, 13, "cdn_block"), false, true, false},
		{"cdn_block, server in a CDN range", scanner(false, 13, "cdn_block"), true, false, true},
		{"ok, server in a CDN range, scanner cannot reach control hosts", scanner(false, 13, "ok"), true, false, true},
		{"ok, server in a CDN range, scanner reaches them", scanner(true, 13, "ok"), true, true, false},
		{"ok outside CDN ranges", scanner(false, 13, "ok"), false, true, false},
		{"tspu_block stays a block anywhere", scanner(true, 13, "tspu_block"), false, false, true},
	}
	for _, c := range cases {
		ok, bad := chebVerdict(chebDisplayVerdicts(c.sc, c.staticCDN))
		if ok != c.wantOK || bad != c.isBad {
			t.Errorf("%s: got ok=%v blocked=%v", c.name, ok, bad)
		}
	}
}

func TestChebVerdictCountsWhitelistAsOpen(t *testing.T) {
	cases := []struct {
		verdicts        []string
		wantOK, wantBad bool
	}{
		{[]string{"ok"}, true, false},
		{[]string{"whitelist"}, true, false},
		{[]string{"cdn_block"}, false, true},
		{[]string{"whitelist", "cdn_block"}, false, true},
		{[]string{"uncertain"}, false, false},
		{nil, false, false},
	}
	for _, c := range cases {
		if ok, bad := chebVerdict(c.verdicts); ok != c.wantOK || bad != c.wantBad {
			t.Errorf("%v: got ok=%v blocked=%v", c.verdicts, ok, bad)
		}
	}
}
