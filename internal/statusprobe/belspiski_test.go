package statusprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMeasureWhitelistUsesBelspiski(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("ip") != "203.0.113.10" || !strings.Contains(r.Form.Get("proxy_pick"), "|") {
			t.Fatalf("form %#v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"ui":{"chartLabel":"Санкт-Петербург · МТС","inBs":true,"logLine":"Санкт-Петербург\tМТС\ton\tBS:yes\treach:yes\tOK\tOK\tOK\t41","metrics":{"icmp":{"label":"41 ms","ok":true}}}}`))
	}))
	defer srv.Close()

	prev := belspiskiURL
	belspiskiURL = srv.URL
	t.Cleanup(func() { belspiskiURL = prev })

	got, err := MeasureWhitelist(context.Background(), srv.Client(), "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if calls != len(belspiskiPicks) || got.Total != len(belspiskiPicks) || got.OK != len(belspiskiPicks) {
		t.Fatalf("calls=%d %+v", calls, got)
	}
	if len(got.Hits) != len(belspiskiPicks) || got.Hits[0].City != "Санкт-Петербург" || got.Hits[0].Network != "МТС" || !got.Hits[0].OK {
		t.Fatalf("%+v", got.Hits)
	}
	if got.Hits[0].PingMs == nil || *got.Hits[0].PingMs != 41 {
		t.Fatalf("ping %+v", got.Hits[0].PingMs)
	}
	if got.Hits[0].Kind != "whitelist" || !strings.Contains(got.Hits[0].Answer, "В белом списке") || !strings.Contains(got.Hits[0].Answer, "ICMP 41 ms") {
		t.Fatalf("answer %+v", got.Hits[0])
	}
}

func TestMergeWhitelistOrdersCities(t *testing.T) {
	got := mergeWhitelist([]ProbeHit{{City: "Amsterdam", Network: "Example"}}, []ProbeHit{
		{Kind: "whitelist", City: "Москва", Network: "МТС"},
		{Kind: "whitelist", City: "Инкогнито", Network: "Мегафон"},
		{Kind: "whitelist", City: "Москва", Network: "Ростелеком"},
		{Kind: "whitelist", City: "Новосибирск", Network: "Мегафон"},
		{Kind: "whitelist", City: "Санкт-Петербург", Network: "T2"},
	})
	if got[0].City != "Amsterdam" {
		t.Fatalf("regular hit moved: %+v", got[0])
	}
	var rows []string
	for _, hit := range got[1:] {
		rows = append(rows, hit.City+" "+hit.Network)
	}
	want := []string{"Москва Ростелеком", "Москва МТС", "Санкт-Петербург T2", "Новосибирск Мегафон", "Инкогнито Мегафон"}
	if strings.Join(rows, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", rows)
	}
}

func TestMeasureWhitelistStopsOnRateLimit(t *testing.T) {
	prevWait := belspiskiWait
	belspiskiWait = func(context.Context, time.Duration) error { return context.DeadlineExceeded }
	t.Cleanup(func() { belspiskiWait = prevWait })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"code":"rate_limited","error":"too many","retry_after":30}`))
	}))
	defer srv.Close()
	prev := belspiskiURL
	belspiskiURL = srv.URL
	t.Cleanup(func() { belspiskiURL = prev })

	_, err := MeasureWhitelist(context.Background(), srv.Client(), "203.0.113.10")
	if err == nil || !isBelspiskiLimit(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestMeasureWhitelistRetriesProxyError(t *testing.T) {
	const okBody = `{"ok":true,"ui":{"chartLabel":"Москва · МТС","inBs":true,"logLine":"Москва\tМТС\ton\tBS:yes\treach:yes\tOK\tOK\tOK\t41","metrics":{"icmp":{"label":"41 ms","ok":true}}}}`
	const proxyBody = `{"ok":true,"ui":{"chartLabel":"Москва · Мегафон","inBs":false,"situation":"proxy_error","filter":"proxy_error","metrics":{"tcp":{"label":"FAIL","ok":false},"icmp":{"label":"FAIL","ok":false}}}}`
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body := okBody
		if r.Form.Get("proxy_pick") == belspiskiPicks[0] && calls == 1 {
			body = proxyBody
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	prev := belspiskiURL
	belspiskiURL = srv.URL
	t.Cleanup(func() { belspiskiURL = prev })

	got, err := MeasureWhitelist(context.Background(), srv.Client(), "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if calls != len(belspiskiPicks)+1 || got.Total != len(belspiskiPicks) || got.OK != len(belspiskiPicks) {
		t.Fatalf("calls=%d %+v", calls, got)
	}
	if hitProxyError(got.Hits[0]) {
		t.Fatalf("proxy error kept: %+v", got.Hits[0])
	}
}

func TestMeasureWhitelistDropsPersistentProxyError(t *testing.T) {
	const okBody = `{"ok":true,"ui":{"chartLabel":"Москва · МТС","inBs":true,"logLine":"Москва\tМТС\ton\tBS:yes\treach:yes\tOK\tOK\tOK\t41","metrics":{"icmp":{"label":"41 ms","ok":true}}}}`
	const proxyBody = `{"ok":true,"ui":{"chartLabel":"Москва · Мегафон","inBs":false,"situation":"proxy_error","filter":"proxy_error","metrics":{"tcp":{"label":"FAIL","ok":false},"icmp":{"label":"FAIL","ok":false}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body := okBody
		if r.Form.Get("proxy_pick") == belspiskiPicks[0] {
			body = proxyBody
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	prev := belspiskiURL
	belspiskiURL = srv.URL
	t.Cleanup(func() { belspiskiURL = prev })

	got, err := MeasureWhitelist(context.Background(), srv.Client(), "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != len(belspiskiPicks)-1 {
		t.Fatalf("total %+v", got)
	}
	for _, hit := range got.Hits {
		if hitProxyError(hit) {
			t.Fatalf("proxy error kept: %+v", hit)
		}
	}
}
