package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestMergeStatusTargetsBindsByNodeUUID(t *testing.T) {
	panel := []remnawave.ProbeTarget{
		{UUID: "entry-1", Name: "ru-entry", State: "up", Address: "ru.example.com"},
		{UUID: "exit-1", Name: "nl-exit", State: "down", Address: "9.9.9.9"},
	}
	// Адрес в списке — IP, а в панели у узла домен: привязка по UUID всё равно находит узел.
	got, _ := mergeStatusTargets(panel, []statusprobe.ConfiguredTarget{
		{ID: "a", Name: "RU", Country: "RU", Address: "203.0.113.40", Enabled: true, Kind: statusprobe.KindBypass, NodeUUID: "entry-1", ExitNodeUUID: "exit-1"},
		{ID: "b", Name: "Plain", Country: "NL", Address: "8.8.8.8", Enabled: true, NodeUUID: "entry-1"},
	})
	if len(got) != 2 {
		t.Fatalf("nodes: %+v", got)
	}
	if got[0].State != "up" || got[0].exit != "down" || !got[0].Whitelist || got[0].Kind != statusprobe.KindBypass {
		t.Fatalf("cascade row: %+v", got[0])
	}
	if got[1].State != "up" || got[1].exit != "" || got[1].Kind != statusprobe.KindRegular {
		t.Fatalf("plain row: %+v", got[1])
	}
	final := statusprobe.EffectiveState(got[0].State, nil, got[0].exit)
	if final != statusprobe.StateDown {
		t.Fatalf("dead exit must pull the cascade down, got %s", final)
	}
}

func TestPublicStatusBodyHidesAddresses(t *testing.T) {
	h := NewPublicStatus(nil, nil, statusprobe.NewKVTargets(&memKV{data: map[string]string{}}))
	h.cfgCached = []statusprobe.ConfiguredTarget{{
		ID: "a", Name: "NL", Country: "NL", Address: "198.51.100.7", Enabled: true,
		NodeUUID: "11111111-2222", ExitNodeUUID: "33333333-4444",
	}}
	h.cfgOK = true
	h.cfgAt = time.Now()
	raw := string(h.cachedBody())
	if !strings.Contains(raw, `"name":"NL"`) {
		t.Fatalf("the row must be in the response, otherwise the leak check proves nothing: %s", raw)
	}
	for _, secret := range []string{"198.51.100.7", "11111111-2222", "33333333-4444"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("public JSON leaks %q: %s", secret, raw)
		}
	}
}

func TestUsableForState(t *testing.T) {
	fresh := statusprobe.Sample{MeasuredAt: time.Now().Add(-time.Hour)}
	stale := statusprobe.Sample{MeasuredAt: time.Now().Add(-7 * time.Hour)}
	if !usableForState(fresh) {
		t.Fatal("fresh sample must count")
	}
	if usableForState(stale) || usableForState(statusprobe.Sample{}) {
		t.Fatal("stale or empty sample must not pin a status")
	}
	t.Setenv("STATUS_PROBES_ENABLED", "false")
	if usableForState(fresh) {
		t.Fatal("disabled probes must not keep a stale failure")
	}
}

func TestHasServersAndBootstrapFlag(t *testing.T) {
	h := NewPublicStatus(nil, nil, statusprobe.NewKVTargets(&memKV{data: map[string]string{}}))
	h.cfgOK, h.cfgAt = true, time.Now()
	h.cfgCached = nil
	if h.HasServers(context.Background()) {
		t.Fatal("empty list must hide the page")
	}
	h.cfgCached = []statusprobe.ConfiguredTarget{{Name: "x", Address: "8.8.8.8", Enabled: false}}
	if h.HasServers(context.Background()) {
		t.Fatal("only disabled rows must hide the page")
	}
	h.cfgCached = []statusprobe.ConfiguredTarget{{Name: "x", Address: "8.8.8.8", Enabled: true}}
	if !h.HasServers(context.Background()) {
		t.Fatal("an enabled row must show the page")
	}

	a := &AuthHandler{}
	if !a.statusNavVisible(context.Background()) {
		t.Fatal("without a provider the flag defaults to visible")
	}
	a.SetStatusVisible(func(context.Context) bool { return false })
	if a.statusNavVisible(context.Background()) {
		t.Fatal("provider result must be passed through")
	}
}
