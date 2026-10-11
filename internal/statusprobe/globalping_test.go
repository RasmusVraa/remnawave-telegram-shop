package statusprobe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicTarget(t *testing.T) {
	ok, yes := PublicTarget("185.1.2.3")
	if !yes || ok != "185.1.2.3" {
		t.Fatalf("public ip: %q %v", ok, yes)
	}
	if _, yes := PublicTarget("10.0.0.8"); yes {
		t.Fatal("private ip must be skipped")
	}
	if _, yes := PublicTarget("127.0.0.1"); yes {
		t.Fatal("loopback must be skipped")
	}
	host, yes := PublicTarget("https://node.example.com:443/path")
	if !yes || host != "node.example.com" {
		t.Fatalf("host: %q %v", host, yes)
	}
}

func TestSummarizeSplitsRussia(t *testing.T) {
	raw := []byte(`{
		"status": "finished",
		"results": [
			{"probe": {"country": "DE"}, "result": {"status": "finished", "stats": {"avg": 40, "loss": 0, "rcv": 2}}},
			{"probe": {"country": "US"}, "result": {"status": "finished", "stats": {"avg": 80, "loss": 0, "rcv": 2}}},
			{"probe": {"country": "RU"}, "result": {"status": "finished", "stats": {"avg": 20, "loss": 0, "rcv": 2}}},
			{"probe": {"country": "RU"}, "result": {"status": "failed", "stats": {"avg": 0, "loss": 100, "rcv": 0}}}
		]
	}`)
	var m gpMeasurement
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	sample := summarize(m)
	if sample.WorldOK != 2 || sample.WorldTotal != 2 {
		t.Fatalf("world %+v", sample)
	}
	if sample.RussiaOK != 1 || sample.RussiaTotal != 2 {
		t.Fatalf("russia %+v", sample)
	}
	if sample.WorldPingMs == nil || *sample.WorldPingMs != 60 {
		t.Fatalf("ping %+v", sample.WorldPingMs)
	}
}

func TestSummarizeKeepsTwoProbesPerCity(t *testing.T) {
	raw := []byte(`{"results":[
		{"probe":{"country":"RU","city":"Moscow","network":"A"},"result":{"status":"finished","stats":{"avg":10,"loss":0,"rcv":2}}},
		{"probe":{"country":"RU","city":"Moscow","network":"B"},"result":{"status":"finished","stats":{"avg":11,"loss":0,"rcv":2}}},
		{"probe":{"country":"RU","city":"Moscow","network":"C"},"result":{"status":"finished","stats":{"avg":12,"loss":0,"rcv":2}}},
		{"probe":{"country":"RU","city":"Moscow","network":"D"},"result":{"status":"finished","stats":{"avg":13,"loss":0,"rcv":2}}},
		{"probe":{"country":"RU","city":"Moscow","network":"E"},"result":{"status":"finished","stats":{"avg":14,"loss":0,"rcv":2}}},
		{"probe":{"country":"DE","city":"Berlin","network":"F"},"result":{"status":"finished","stats":{"avg":40,"loss":0,"rcv":2}}},
		{"probe":{"country":"RU","city":"Kazan","network":"G"},"result":{"status":"finished","stats":{"avg":20,"loss":0,"rcv":2}}}
	]}`)
	var m gpMeasurement
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	sample := summarize(m)
	if len(sample.Probes) != 4 || sample.RussiaTotal != 3 || sample.RussiaOK != 3 || sample.WorldTotal != 1 {
		t.Fatalf("%+v", sample)
	}
	var rows []string
	for _, hit := range sample.Probes {
		rows = append(rows, hit.City+" "+hit.Network)
	}
	want := "Berlin F|Kazan G|Moscow A|Moscow B"
	if strings.Join(rows, "|") != want {
		t.Fatalf("got %v", rows)
	}
}
