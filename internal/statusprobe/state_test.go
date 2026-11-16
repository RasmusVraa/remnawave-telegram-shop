package statusprobe

import "testing"

func TestEffectiveState(t *testing.T) {
	dead := func(streak int) *Sample {
		return &Sample{WorldTotal: 3, RussiaTotal: 20, FailStreak: streak}
	}
	alive := &Sample{WorldTotal: 3, WorldOK: 3, RussiaTotal: 20, RussiaOK: 19}

	cases := []struct {
		name   string
		entry  string
		sample *Sample
		exit   string
		want   string
	}{
		{"panel up, no data", StateUp, nil, "", StateUp},
		{"no panel node, no probes", "", nil, "", StateUnknown},
		{"panel down, no probes", StateDown, nil, "", StateDown},
		{"panel down but probes answer", StateDown, alive, "", StatePartial},
		{"panel down and probes dead once", StateDown, dead(1), "", StateDown},
		{"unknown entry, exit down", "", nil, StateDown, StateDown},
		{"unknown entry, exit up", "", nil, StateUp, StateUnknown},
		{"no panel node, probes alive", "", alive, "", StateUp},
		{"one failed measurement is only partial", StateUp, dead(1), "", StatePartial},
		{"two failed measurements are down", StateUp, dead(2), "", StateDown},
		{"one partial russia failure keeps up", StateUp, alive, "", StateUp},
		{"panel down softened by live probes", StateDown, alive, "", StatePartial},
		{"cascade exit down drags entry", StateUp, alive, StateDown, StateDown},
		{"cascade exit partial", StateUp, nil, StatePartial, StatePartial},
		{"exit up does not improve entry", StatePartial, nil, StateUp, StatePartial},
		{"unknown exit is ignored", StateUp, alive, "", StateUp},
	}
	for _, tc := range cases {
		if got := EffectiveState(tc.entry, tc.sample, false, tc.exit); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestProbeState(t *testing.T) {
	ru := func(ok, low int) Sample {
		return Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: ok, RussiaLowStreak: low}
	}
	ops := func(ok, total, low int) Sample {
		s := Sample{WorldTotal: 1, RussiaTotal: 10, RussiaOK: 10, WhitelistTotal: total, WhitelistOK: ok, WhitelistLowStreak: low}
		return s
	}
	cases := []struct {
		name   string
		sample Sample
		bypass bool
		want   string
	}{
		{"all answered", ru(10, 0), false, StateUp},
		{"one russian probe is noise", ru(9, 0), false, StateUp},
		{"two russian probes failed", ru(8, 0), false, StatePartial},
		{"half of russia once", ru(5, 1), false, StatePartial},
		{"half of russia twice is down", ru(5, 2), false, StateDown},
		{"russia dead, world alive, twice", ru(0, 2), false, StateDown},
		{"world silent", Sample{WorldTotal: 1, RussiaTotal: 10, RussiaOK: 10}, false, StatePartial},
		{"nobody once", Sample{WorldTotal: 1, RussiaTotal: 10, FailStreak: 1, RussiaLowStreak: 1}, false, StatePartial},
		{"nobody twice", Sample{WorldTotal: 1, RussiaTotal: 10, FailStreak: 2, RussiaLowStreak: 2}, false, StateDown},
		{"bypass: all operators", ops(4, 4, 0), true, StateUp},
		{"bypass: one operator silent", ops(3, 4, 0), true, StatePartial},
		{"bypass: operators silent once", ops(0, 4, 1), true, StatePartial},
		{"bypass: operators silent twice", ops(0, 4, 2), true, StateDown},
		{"bypass ignores world", Sample{WorldTotal: 1, RussiaTotal: 10, RussiaOK: 10, WhitelistTotal: 4, WhitelistOK: 4}, true, StateUp},
		{"bypass without operators falls back to russia", ru(7, 0), true, StatePartial},
		{"bypass without operators ignores world", Sample{WorldTotal: 1, RussiaTotal: 10, RussiaOK: 10}, true, StateUp},
	}
	for _, tc := range cases {
		if got := ProbeState(tc.sample, tc.bypass, 2); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if got := ProbeState(ru(8, 0), false, 3); got != StateUp {
		t.Errorf("threshold 3 keeps two failures green, got %q", got)
	}
}

func TestEffectiveStateRussiaMatters(t *testing.T) {
	dead := &Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaLowStreak: 2}
	if got := EffectiveState(StateUp, dead, false, ""); got != StateDown {
		t.Fatalf("russia dead twice with live world must be down, got %q", got)
	}
	shaky := &Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 7}
	if got := EffectiveState(StateUp, shaky, false, ""); got != StatePartial {
		t.Fatalf("three russian failures must be partial, got %q", got)
	}
	if got := EffectiveState(StateDown, shaky, false, ""); got != StatePartial {
		t.Fatalf("panel down but probes answer: got %q", got)
	}
}

func TestStreaksInRunner(t *testing.T) {
	r := &Runner{samples: map[string]Sample{}, history: map[string][]RunRecord{}}
	low := Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 4}
	r.storeSample("k", low)
	r.storeSample("k", low)
	if got := r.samples["k"].RussiaLowStreak; got != 2 {
		t.Fatalf("russia low streak: %d", got)
	}
	r.storeSample("k", Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 10})
	if got := r.samples["k"].RussiaLowStreak; got != 0 {
		t.Fatalf("good sample resets streak: %d", got)
	}

	silent := []ProbeHit{{Network: "A", Kind: "whitelist"}, {Network: "B", Kind: "whitelist"}}
	r.finishWhitelist("k", silent, nil, false)
	if got := r.samples["k"].WhitelistLowStreak; got != 0 {
		t.Fatalf("partial pass must not move the streak: %d", got)
	}
	r.finishWhitelist("k", silent, nil, true)
	r.storeSample("k", Sample{WorldTotal: 1, WorldOK: 1, RussiaTotal: 10, RussiaOK: 10})
	r.finishWhitelist("k", silent, nil, true)
	if got := r.samples["k"].WhitelistLowStreak; got != 2 {
		t.Fatalf("operator streak survives ping and counts full passes: %d", got)
	}
	r.finishWhitelist("k", []ProbeHit{{Network: "A", Kind: "whitelist", OK: true}, {Network: "B", Kind: "whitelist"}}, nil, true)
	if got := r.samples["k"].WhitelistLowStreak; got != 0 {
		t.Fatalf("an answering operator resets streak: %d", got)
	}
}

func TestNormalizeTCPPort(t *testing.T) {
	out, err := NormalizeTargets([]ConfiguredTarget{
		{Name: "A", Country: "NL", Address: "8.8.8.8:8443", Enabled: true},
		{Name: "B", Country: "NL", Address: "8.8.4.4", Enabled: true, TCPPort: 2053},
		{Name: "C", Country: "NL", Address: "9.9.9.9", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Address != "8.8.8.8" || out[0].TCPPort != 8443 {
		t.Fatalf("port from address: %+v", out[0])
	}
	if out[1].TCPPort != 2053 || out[2].TCPPort != 0 {
		t.Fatalf("explicit and empty port: %+v %+v", out[1], out[2])
	}
	if got := TargetsFrom(out)[2].TCPPort; got != DefaultTCPPort {
		t.Fatalf("empty port pings 443, got %d", got)
	}
	if _, err := NormalizeTargets([]ConfiguredTarget{{Name: "X", Country: "NL", Address: "8.8.8.8", TCPPort: 70000}}); err == nil {
		t.Fatal("port out of range must be rejected")
	}
}

func TestSampleAllFailed(t *testing.T) {
	if (Sample{}).AllFailed() {
		t.Fatal("empty sample is not a failure")
	}
	if !(Sample{WorldTotal: 3, RussiaTotal: 20}).AllFailed() {
		t.Fatal("nobody answered")
	}
	if (Sample{WorldTotal: 3, RussiaTotal: 20, RussiaOK: 1}).AllFailed() {
		t.Fatal("one russian answer is not a total failure")
	}
	if (Sample{WorldTotal: 3, RussiaTotal: 20, WhitelistTotal: 4, WhitelistOK: 1}).AllFailed() {
		t.Fatal("an answering operator is not a total failure")
	}
}

func TestEstimateQuota(t *testing.T) {
	t.Setenv("GLOBALPING_TOKEN", "")
	got := EstimateQuota([]ConfiguredTarget{
		{Enabled: true, Address: "1.1.1.1", IntervalMin: 20, WorldProbes: 3, RussiaProbes: 20},
		{Enabled: true, Address: "1.1.1.2", IntervalMin: 60, WorldProbes: 3, RussiaProbes: 7, NoExternal: true},
		{Enabled: false, Address: "1.1.1.3", IntervalMin: 20},
	})
	if got.PerHour != 69 || got.Limit != 250 || got.MaxCost != 23 || got.TokenSet {
		t.Fatalf("estimate: %+v", got)
	}
	t.Setenv("GLOBALPING_TOKEN", "x")
	if EstimateQuota(nil).Limit != 500 {
		t.Fatal("token raises limit")
	}
}

func TestNormalizeKindAndNodes(t *testing.T) {
	in := []ConfiguredTarget{
		{Name: "A", Country: "nl", Address: "8.8.8.8", Enabled: true, Whitelist: true, NodeUUID: " ABC-123 "},
		{Name: "B", Country: "RU", Address: "9.9.9.9", Enabled: true, Kind: KindBypass, ExitNodeUUID: "dead-beef"},
		{Name: "C", Country: "DE", Address: "9.9.9.10", Enabled: true, Kind: KindRegular, Whitelist: true},
	}
	out, err := NormalizeTargets(in)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Kind != KindBypass || !out[0].Whitelist || out[0].NodeUUID != "abc-123" {
		t.Fatalf("legacy whitelist -> bypass: %+v", out[0])
	}
	if out[1].Kind != KindBypass || out[1].ExitNodeUUID != "dead-beef" {
		t.Fatalf("bypass row: %+v", out[1])
	}
	if out[2].Kind != KindRegular || out[2].Whitelist {
		t.Fatalf("explicit regular wins over stale whitelist flag: %+v", out[2])
	}
	if _, err := NormalizeTargets([]ConfiguredTarget{{Name: "X", Country: "NL", Address: "8.8.8.8", Kind: "x"}}); err == nil {
		t.Fatal("unknown kind must be rejected")
	}
	if _, err := NormalizeTargets([]ConfiguredTarget{{Name: "X", Country: "NL", Address: "8.8.8.8", NodeUUID: "../etc"}}); err == nil {
		t.Fatal("bad node id must be rejected")
	}
}

func TestNoExternalTargetsAreNotMeasured(t *testing.T) {
	r := &Runner{samples: map[string]Sample{}}
	ping, list := r.pickLocked([]Target{
		{Key: "a", Address: "8.8.8.8", Whitelist: true, NoExternal: true},
		{Key: "b", Address: "8.8.4.4"},
	}, true)
	if len(ping) != 1 || ping[0].Key != "b" || len(list) != 0 {
		t.Fatalf("ping=%+v list=%+v", ping, list)
	}
}
