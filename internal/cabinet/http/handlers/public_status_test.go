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
