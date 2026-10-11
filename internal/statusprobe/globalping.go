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
	"sort"
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
	// WhitelistAt — когда последний раз заканчивалась проверка белых списков.
	// Отдельно от MeasuredAt, чтобы обычный пинг шёл по своему интервалу.
	WhitelistAt time.Time
	// WhitelistDone — полный проход по всем операторам, а не оборванный набор.
	WhitelistDone bool
	MeasuredAt    time.Time
	Probes        []ProbeHit
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
	for _, row := range m.Results {
		ok := row.Result.Status == "finished" && row.Result.Stats.Rcv > 0 && row.Result.Stats.Loss < 100
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
	sample.Probes = keepTwoPerCity(sample.Probes)
	recountPing(sample.Probes, &sample)
	return sample
}

const maxProbesPerCity = 2
const maxProbeHits = 40

// limitCities оставляет не больше двух зондов Globalping на город и сортирует
// их по названию. Строки белых списков не режет: у одного города несколько операторов.
func limitCities(sample Sample) Sample {
	regular := make([]ProbeHit, 0, len(sample.Probes))
	lists := make([]ProbeHit, 0)
	for _, hit := range sample.Probes {
		if hit.Kind == "whitelist" {
			lists = append(lists, hit)
			continue
		}
		regular = append(regular, hit)
	}
	if len(regular) == 0 {
		return sample
	}
	capped := keepTwoPerCity(regular)
	sample.Probes = append(capped, lists...)
	if len(capped) != len(regular) {
		recountPing(capped, &sample)
	}
	return sample
}

func keepTwoPerCity(hits []ProbeHit) []ProbeHit {
	counts := map[string]int{}
	kept := make([]ProbeHit, 0, len(hits))
	for _, hit := range hits {
		key := strings.ToLower(strings.TrimSpace(hit.Country)) + "\n" + strings.ToLower(strings.TrimSpace(hit.City))
		if counts[key] >= maxProbesPerCity {
			continue
		}
		counts[key]++
		kept = append(kept, hit)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		ai, aj := citySortKey(kept[i].City), citySortKey(kept[j].City)
		if ai != aj {
			return ai < aj
		}
		ni, nj := strings.ToLower(kept[i].Network), strings.ToLower(kept[j].Network)
		if ni != nj {
			return ni < nj
		}
		return strings.ToLower(kept[i].Country) < strings.ToLower(kept[j].Country)
	})
	if len(kept) > maxProbeHits {
		kept = kept[:maxProbeHits]
	}
	return kept
}

func citySortKey(city string) string {
	city = strings.ToLower(strings.TrimSpace(city))
	if city == "" {
		return "\uffff"
	}
	return city
}

func recountPing(hits []ProbeHit, sample *Sample) {
	sample.WorldOK, sample.WorldTotal = 0, 0
	sample.RussiaOK, sample.RussiaTotal = 0, 0
	sample.WorldPingMs = nil
	var pingSum, pingN int
	for _, hit := range hits {
		if strings.EqualFold(hit.Country, "RU") {
			sample.RussiaTotal++
			if hit.OK {
				sample.RussiaOK++
			}
			continue
		}
		sample.WorldTotal++
		if !hit.OK {
			continue
		}
		sample.WorldOK++
		if hit.PingMs == nil {
			continue
		}
		pingSum += *hit.PingMs
		pingN++
	}
	if pingN > 0 {
		ms := int(float64(pingSum)/float64(pingN) + 0.5)
		sample.WorldPingMs = &ms
	}
}
