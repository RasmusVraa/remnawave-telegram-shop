package statusprobe

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTargetsAcceptsPublicIP(t *testing.T) {
	got, err := NormalizeTargets([]ConfiguredTarget{{
		Name:    "Нидерланды",
		Country: "nl",
		Address: "1.1.1.1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Country != "NL" || got[0].Address != "1.1.1.1" || got[0].ID == "" {
		t.Fatalf("%+v", got)
	}
	if got[0].IntervalMin != 20 {
		t.Fatalf("interval %d", got[0].IntervalMin)
	}
}

func TestNormalizeTargetsAcceptsDomain(t *testing.T) {
	got, err := NormalizeTargets([]ConfiguredTarget{{
		Name:    "Нидерланды",
		Country: "NL",
		Address: "https://nl.example.com/path",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Address != "nl.example.com" {
		t.Fatalf("address %q", got[0].Address)
	}
}

func TestRepeatCitiesDefaultsOn(t *testing.T) {
	var stored []ConfiguredTarget
	if err := json.Unmarshal([]byte(`[{"name":"Нидерланды","country":"NL","address":"1.1.1.1"}]`), &stored); err != nil {
		t.Fatal(err)
	}
	if !stored[0].AllowsRepeatCities() {
		t.Fatal("missing flag should allow repeats")
	}
	got, err := NormalizeTargets(stored)
	if err != nil || !got[0].AllowsRepeatCities() || got[0].RepeatCities == nil || !*got[0].RepeatCities {
		t.Fatalf("%+v %v", got, err)
	}
	off := false
	got, err = NormalizeTargets([]ConfiguredTarget{{
		Name: "Нидерланды", Country: "NL", Address: "1.1.1.1", Enabled: true, RepeatCities: &off,
	}})
	if err != nil || got[0].AllowsRepeatCities() {
		t.Fatalf("off %+v %v", got, err)
	}
	enabled := EnabledTargets(got)
	if len(enabled) != 1 || enabled[0].RepeatCities {
		t.Fatalf("enabled %+v", enabled)
	}
}

func TestTargetsFromGivesEveryRowItsOwnProbe(t *testing.T) {
	got := TargetsFrom([]ConfiguredTarget{
		{ID: "de-plain", Name: "Германия", Country: "DE", Address: "1.1.1.1", Enabled: true},
		{ID: "de-list", Name: "Германия", Country: "DE", Address: "8.8.8.8", Enabled: true, Whitelist: true},
		{Name: "Финляндия", Country: "FI", Address: "9.9.9.9", Enabled: true},
		{Name: "Финляндия", Country: "FI", Address: "9.9.9.10", Enabled: true, Whitelist: true},
	})
	if len(got) != 4 {
		t.Fatalf("rows %d", len(got))
	}
	seen := map[string]struct{}{}
	for _, item := range got {
		if _, ok := seen[item.Key]; ok {
			t.Fatalf("shared probe key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
	}
	if got[0].Whitelist || !got[1].Whitelist || got[2].Whitelist || !got[3].Whitelist {
		t.Fatalf("flags %+v", got)
	}
	if got[2].Key == got[3].Key {
		t.Fatal("same name without id still shared a probe")
	}
}

func TestNormalizeTargetsKeepsNote(t *testing.T) {
	got, err := NormalizeTargets([]ConfiguredTarget{{
		Name:    "Албания",
		Country: "AL",
		Address: "1.1.1.1",
		Note:    "  + белые списки  ",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Note != "+ белые списки" {
		t.Fatalf("note %q", got[0].Note)
	}
}

func TestSummarizeStoresProbePlace(t *testing.T) {
	var measurement gpMeasurement
	raw := []byte(`{"results":[{"probe":{"country":"RU","city":"Moscow","network":"Timeweb","latitude":55.75,"longitude":37.62},"result":{"status":"finished","stats":{"avg":118,"loss":0,"rcv":2}}}]}`)
	if err := json.Unmarshal(raw, &measurement); err != nil {
		t.Fatal(err)
	}
	sample := summarize(measurement)
	if sample.RussiaOK != 1 || len(sample.Probes) != 1 || sample.Probes[0].City != "Moscow" || sample.Probes[0].Lat != 55.75 {
		t.Fatalf("%+v", sample)
	}
}

func TestNormalizeTargetsRejectsPrivateIP(t *testing.T) {
	_, err := NormalizeTargets([]ConfiguredTarget{{
		Name:    "Дом",
		Country: "RU",
		Address: "192.168.0.1",
	}})
	if err == nil || err.Error() != "address is not a public host" {
		t.Fatalf("err=%v", err)
	}
}
