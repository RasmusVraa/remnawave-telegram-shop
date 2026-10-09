package statusprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const globalpingURL = "https://api.globalping.io/v1/measurements"

// RateLimitError — Globalping ответил 429. RetryAfter берётся из заголовка, не из попытки обойти лимит.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return "globalping rate limit" }

func authorizeGlobalping(req *http.Request) {
	token := strings.TrimSpace(os.Getenv("GLOBALPING_TOKEN"))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func retryAfter(h http.Header) time.Duration {
	const fallback = 15 * time.Minute
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return fallback
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		wait := time.Duration(secs) * time.Second
		if wait > time.Hour {
			return time.Hour
		}
		return wait
	}
	if when, err := http.ParseTime(raw); err == nil {
		wait := time.Until(when)
		if wait < time.Minute {
			return time.Minute
		}
		if wait > time.Hour {
			return time.Hour
		}
		return wait
	}
	return fallback
}

// ProbeHit — один зонд Globalping. Адреса наших узлов здесь нет.
type ProbeHit struct {
	City    string  `json:"city,omitempty"`
	Network string  `json:"network,omitempty"`
	Country string  `json:"country,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	PingMs  *int    `json:"ping_ms,omitempty"`
	OK      bool    `json:"ok"`
	// Kind — "whitelist" для замера belspiski. Пусто у зондов Globalping.
	Kind string `json:"kind,omitempty"`
	// Answer — текст ответа оператора. Адреса узлов сюда не входят.
	Answer string `json:"answer,omitempty"`
}

// Sample — один замер узла. Адрес узла сюда не входит.
type Sample struct {
	WorldPingMs     *int
	WorldOK         int
	WorldTotal      int
	RussiaOK        int
	RussiaTotal     int
	WhitelistOK     int
	WhitelistTotal  int
	WhitelistPingMs *int
	MeasuredAt      time.Time
	Probes          []ProbeHit
}

// PublicTarget оставляет хост, который можно отдать внешнему зонду.
// Частные, локальные и пустые адреса отбрасываются: их пинг ничего не скажет
// и не должен уезжать в чужой сервис.
func PublicTarget(address string) (string, bool) {
	host := strings.TrimSpace(address)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.ContainsAny(host, " \t@") {
		return "", false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return "", false
		}
		return ip.String(), true
	}
	if strings.Contains(host, "..") || !strings.Contains(host, ".") {
		return "", false
	}
	return host, true
}

type gpCreate struct {
	Type               string         `json:"type"`
	Target             string         `json:"target"`
	Locations          []gpLocation   `json:"locations"`
	InProgressUpdates  bool           `json:"inProgressUpdates"`
	MeasurementOptions map[string]int `json:"measurementOptions"`
}

type gpLocation struct {
	Magic string `json:"magic"`
	Limit int    `json:"limit"`
}

type gpCreated struct {
	ID string `json:"id"`
}

type gpMeasurement struct {
	Status  string     `json:"status"`
	Results []gpResult `json:"results"`
}

type gpResult struct {
	Probe struct {
		Country   string  `json:"country"`
		City      string  `json:"city"`
		Network   string  `json:"network"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"probe"`
	Result struct {
		Status string `json:"status"`
		Stats  struct {
			Avg  float64 `json:"avg"`
			Loss float64 `json:"loss"`
			Rcv  int     `json:"rcv"`
		} `json:"stats"`
	} `json:"result"`
}

// MeasurePing запускает один замер Globalping: несколько зондов из мира и из России.
func MeasurePing(ctx context.Context, client *http.Client, target string) (Sample, error) {
	return MeasurePingCounts(ctx, client, target, 3, 5)
}

// MeasurePingCounts — то же, но число зондов задаёт админка.
func MeasurePingCounts(ctx context.Context, client *http.Client, target string, world, russia int) (Sample, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if world < 1 || world > 10 {
		world = 3
	}
	if russia < 1 || russia > 30 {
		russia = 20
	}
	body, _ := json.Marshal(gpCreate{
		Type:   "ping",
		Target: target,
		Locations: []gpLocation{
			{Magic: "world", Limit: world},
			{Magic: "Russia", Limit: russia},
		},
		InProgressUpdates: true,
		MeasurementOptions: map[string]int{
			"packets": 2,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, globalpingURL, bytes.NewReader(body))
	if err != nil {
		return Sample{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "remnawave-telegram-shop/status")
	authorizeGlobalping(req)
	res, err := client.Do(req)
	if err != nil {
		return Sample{}, err
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		return Sample{}, &RateLimitError{RetryAfter: retryAfter(res.Header)}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Sample{}, fmt.Errorf("globalping create: %d", res.StatusCode)
	}
	var created gpCreated
	if err := json.Unmarshal(raw, &created); err != nil || created.ID == "" {
		return Sample{}, fmt.Errorf("globalping create: empty id")
	}

	deadline := time.Now().Add(25 * time.Second)
	var last gpMeasurement
	for {
		if err := ctx.Err(); err != nil {
			return Sample{}, err
		}
		m, err := fetchMeasurement(ctx, client, created.ID)
		if err != nil {
			return Sample{}, err
		}
		last = m
		if m.Status == "finished" || time.Now().After(deadline) {
			break
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Sample{}, ctx.Err()
		case <-timer.C:
		}
	}
	sample := summarize(last)
	sample.MeasuredAt = time.Now().UTC()
	return sample, nil
}

func fetchMeasurement(ctx context.Context, client *http.Client, id string) (gpMeasurement, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, globalpingURL+"/"+id, nil)
	if err != nil {
		return gpMeasurement{}, err
	}
	req.Header.Set("User-Agent", "remnawave-telegram-shop/status")
	authorizeGlobalping(req)
	res, err := client.Do(req)
	if err != nil {
		return gpMeasurement{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusTooManyRequests {
		return gpMeasurement{}, &RateLimitError{RetryAfter: retryAfter(res.Header)}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return gpMeasurement{}, fmt.Errorf("globalping get: %d", res.StatusCode)
	}
	var m gpMeasurement
	if err := json.Unmarshal(raw, &m); err != nil {
		return gpMeasurement{}, err
	}
	return m, nil
}

func summarize(m gpMeasurement) Sample {
	var sample Sample
	var pingSum float64
	var pingN int
	for _, row := range m.Results {
		ok := row.Result.Status == "finished" && row.Result.Stats.Rcv > 0 && row.Result.Stats.Loss < 100
		if len(sample.Probes) < 40 {
			hit := ProbeHit{
				City:    strings.TrimSpace(row.Probe.City),
				Network: strings.TrimSpace(row.Probe.Network),
				Country: strings.ToUpper(strings.TrimSpace(row.Probe.Country)),
				Lat:     row.Probe.Latitude,
				Lon:     row.Probe.Longitude,
				OK:      ok,
			}
			if row.Result.Stats.Avg > 0 {
				ms := int(row.Result.Stats.Avg + 0.5)
				hit.PingMs = &ms
			}
			sample.Probes = append(sample.Probes, hit)
		}
		if strings.EqualFold(row.Probe.Country, "RU") {
			sample.RussiaTotal++
			if ok {
				sample.RussiaOK++
			}
			continue
		}
		sample.WorldTotal++
		if ok {
			sample.WorldOK++
			if row.Result.Stats.Avg > 0 {
				pingSum += row.Result.Stats.Avg
				pingN++
			}
		}
	}
	if pingN > 0 {
		ms := int(pingSum/float64(pingN) + 0.5)
		sample.WorldPingMs = &ms
	}
	return sample
}
