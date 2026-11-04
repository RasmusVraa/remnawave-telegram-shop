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
		if got := EffectiveState(tc.entry, tc.sample, tc.exit); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
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
	if got.PerHour != 69 || got.Limit != 250 || got.TokenSet {
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
