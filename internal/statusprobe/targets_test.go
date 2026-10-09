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
