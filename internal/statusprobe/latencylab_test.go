package statusprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMeasureWhitelistUsesLatencyLab(t *testing.T) {
	var scans int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/lab/operators":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"operators": []map[string]any{
						{"id": "mts", "label": "МТС", "online": true, "wl": true},
						{"id": "t2", "label": "T2", "online": true, "wl": false},
						{"id": "megafon", "label": "Мегафон", "online": false, "wl": true},
					},
				},
			})
		case r.URL.Path == "/api/lab/multiscan":
			var body struct {
				Target    string   `json:"target"`
				Operators []string `json:"operators"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Target != "203.0.113.10" || len(body.Operators) != 1 || body.Operators[0] != "mts" {
				t.Errorf("scan body %+v", body)
			}
			scans++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "req_id": "job-1", "status": "pending"})
		case r.URL.Path == "/api/lab/job/job-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "status": "done", "req_id": "job-1",
				"result": map[string]any{
					"results": []map[string]any{
						{"operator": "mts", "ok": true, "rtt_ms": 12.4, "icmp_ok": true, "icmp_rtt_ms": 12.4, "tcp_ok": true, "tcp_rtt_ms": 18.2},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	prevOps, prevScan, prevJob := latencyLabOperatorsURL, latencyLabMultiscanURL, latencyLabJobURL
	latencyLabOperatorsURL = srv.URL + "/api/lab/operators"
	latencyLabMultiscanURL = srv.URL + "/api/lab/multiscan"
	latencyLabJobURL = srv.URL + "/api/lab/job/"
	t.Cleanup(func() {
		latencyLabOperatorsURL = prevOps
		latencyLabMultiscanURL = prevScan
		latencyLabJobURL = prevJob
	})
	t.Setenv("LATENCYLAB_TOKEN", "test-key")

	got, err := MeasureWhitelist(context.Background(), srv.Client(), "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if scans != 1 || got.Total != 1 || got.OK != 1 || len(got.Hits) != 1 {
		t.Fatalf("scans %d result %+v", scans, got)
	}
	if got.Hits[0].Network != "МТС" || got.Hits[0].Country != "RU" || got.Hits[0].Kind != "whitelist" || !got.Hits[0].OK {
		t.Fatalf("hit %+v", got.Hits[0])
	}
	if got.Hits[0].PingMs == nil || *got.Hits[0].PingMs != 12 || got.Hits[0].ICMPMs == nil || *got.Hits[0].ICMPMs != 12 || got.Hits[0].TCPMs == nil || *got.Hits[0].TCPMs != 18 {
		t.Fatalf("ping %+v", got.Hits[0])
	}
	if got.Hits[0].City != "" || got.Hits[0].Answer != "" {
		t.Fatalf("legacy fields %+v", got.Hits[0])
	}
}

func TestMeasureWhitelistRequiresToken(t *testing.T) {
	t.Setenv("LATENCYLAB_TOKEN", "")
	_, err := MeasureWhitelist(context.Background(), http.DefaultClient, "203.0.113.10")
	if err == nil || !strings.Contains(err.Error(), "LATENCYLAB_TOKEN") {
		t.Fatalf("err %v", err)
	}
}
