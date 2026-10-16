package handlers

import (
	"testing"

	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/statusprobe"
)

func TestMergeStatusTargetsUsesOnlyManualList(t *testing.T) {
	panel := []remnawave.ProbeTarget{{
		Name: "lte", CountryCode: "PL", State: "up", Address: "1.2.3.4",
	}}

	got, kick := mergeStatusTargets(panel, nil)
	if len(got) != 0 || kick != nil {
		t.Fatalf("empty list leaked panel nodes: %+v kick=%+v", got, kick)
	}

	got, _ = mergeStatusTargets(panel, []statusprobe.ConfiguredTarget{{
		Name: "Poland", Country: "PL", Address: "5.6.7.8", Enabled: false,
	}})
	if len(got) != 0 {
		t.Fatalf("disabled rows must not fall back to the panel: %+v", got)
	}

	got, kick = mergeStatusTargets(panel, []statusprobe.ConfiguredTarget{{
		Name: "Poland", Country: "PL", Address: "1.2.3.4", Enabled: true,
	}})
	if len(got) != 1 || got[0].Name != "Poland" || got[0].State != "up" {
		t.Fatalf("manual row: %+v", got)
	}
	if len(kick) != 1 || kick[0].Address != "1.2.3.4" {
		t.Fatalf("probe kick: %+v", kick)
	}
}

func TestMergeStatusTargetsSeparatesSameName(t *testing.T) {
	_, kick := mergeStatusTargets(nil, []statusprobe.ConfiguredTarget{
		{ID: "ee-plain", Name: "Эстония", Country: "EE", Address: "1.1.1.1", Enabled: true},
		{ID: "ee-list", Name: "Эстония", Country: "EE", Address: "1.1.1.2", Enabled: true, Whitelist: true},
	})
	if len(kick) != 2 || kick[0].Key == kick[1].Key {
		t.Fatalf("same name must not share a probe: %+v", kick)
	}
	if kick[0].Whitelist || !kick[1].Whitelist {
		t.Fatalf("whitelist flags: %+v", kick)
	}
}
