package statusprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var belspiskiURL = "https://belspiski.online/check_api.php"

// Все операторы со страницы belspiski.online: три города и режим инкогнито.
var belspiskiPicks = []string{
	"moscow|Ростелеком",
	"moscow|T2",
	"moscow|МТС",
	"moscow|Мегафон",
	"moscow|Билайн",
	"spb|Ростелеком",
	"spb|T2",
	"spb|МТС",
	"spb|Мегафон",
	"spb|Билайн",
	"novosibirsk|Ростелеком",
	"novosibirsk|T2",
	"novosibirsk|МТС",
	"novosibirsk|Мегафон",
	"novosibirsk|Билайн",
	"incognito|Мегафон",
}

type whitelistResult struct {
	OK     int
	Total  int
	PingMs *int
	Hits   []ProbeHit
}

const belspiskiBatch = 3
const belspiskiProxyRetries = 2

// belspiskiWait ждёт Retry-After. В тесте подменяется, чтобы не спать по-настоящему.
var belspiskiWait = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// MeasureWhitelist проверяет адрес только через belspiski.online:
// открывается ли он у операторов, когда у них включены белые списки.
func MeasureWhitelist(ctx context.Context, client *http.Client, host string) (whitelistResult, error) {
	return measureWhitelist(ctx, client, host, nil)
}

func measureWhitelist(ctx context.Context, client *http.Client, host string, onHit func(ProbeHit)) (whitelistResult, error) {
	var out whitelistResult
	var pings []int
	var firstErr error
	var lastLimit error
	for batch := 0; batch < len(belspiskiPicks); batch += belspiskiBatch {
		if err := ctx.Err(); err != nil {
			break
		}
		end := batch + belspiskiBatch
		if end > len(belspiskiPicks) {
			end = len(belspiskiPicks)
		}
		chunk := belspiskiPicks[batch:end]
		batchID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), batch)
		for _, pick := range chunk {
			if err := ctx.Err(); err != nil {
				break
			}
			var hit ProbeHit
			var err error
			proxyTries := 0
			for {
				size := len(chunk)
				id := batchID
				if proxyTries > 0 {
					size = 1
					id = fmt.Sprintf("%d-p%d", time.Now().UnixNano(), proxyTries)
				}
				hit, err = checkBelspiski(ctx, client, host, pick, id, size)
				if isBelspiskiLimit(err) {
					lastLimit = err
					var limited belspiskiLimitError
					errorsAsLimit(err, &limited)
					wait := limited.RetryAfter
					if wait < time.Second {
						wait = 15 * time.Second
					}
					slog.Info("whitelist probe waiting", "retry_in", wait.String(), "operators", out.Total)
					if waitErr := belspiskiWait(ctx, wait); waitErr != nil {
						if out.Total == 0 {
							return out, lastLimit
						}
						return finishWhitelist(out, pings, firstErr)
					}
					batchID = fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), batch, out.Total)
					continue
				}
				if isBelspiskiProxy(err) && proxyTries < belspiskiProxyRetries {
					proxyTries++
					slog.Info("whitelist probe proxy retry", "operators", out.Total)
					continue
				}
				break
			}
			if isBelspiskiProxy(err) {
				slog.Warn("whitelist probe proxy failed")
				continue
			}
			if err != nil {
				slog.Warn("whitelist probe skipped", "error", err.Error())
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			out.Total++
			if hit.OK {
				out.OK++
			}
			if hit.PingMs != nil {
				pings = append(pings, *hit.PingMs)
			}
			out.Hits = append(out.Hits, hit)
			if onHit != nil {
				onHit(hit)
			}
		}
	}
	if out.Total == 0 && lastLimit != nil {
		return out, lastLimit
	}
	return finishWhitelist(out, pings, firstErr)
}

func finishWhitelist(out whitelistResult, pings []int, firstErr error) (whitelistResult, error) {
	if out.Total == 0 && firstErr != nil {
		return out, firstErr
	}
	if len(pings) > 0 {
		sum := 0
		for _, ms := range pings {
			sum += ms
		}
		avg := sum / len(pings)
		out.PingMs = &avg
	}
	return out, nil
}

type belspiskiProxyError struct{}

func (belspiskiProxyError) Error() string { return "whitelist probe: proxy error" }

func isBelspiskiProxy(err error) bool {
	_, ok := err.(belspiskiProxyError)
	return ok
}

func hitProxyError(hit ProbeHit) bool {
	return hit.Kind == "whitelist" && strings.HasPrefix(hit.Answer, "Ошибка прокси")
}

type belspiskiLimitError struct {
	RetryAfter time.Duration
}

func (belspiskiLimitError) Error() string { return "whitelist probe: rate limit" }

func isBelspiskiLimit(err error) bool {
	_, ok := err.(belspiskiLimitError)
	return ok
}

func errorsAsLimit(err error, dst *belspiskiLimitError) bool {
	got, ok := err.(belspiskiLimitError)
	if ok && dst != nil {
		*dst = got
	}
	return ok
}

func checkBelspiski(ctx context.Context, client *http.Client, host, pick, batchID string, batchSize int) (ProbeHit, error) {
	form := url.Values{}
	form.Set("ip", host)
	form.Set("proxy_pick", pick)
	if batchID != "" {
		form.Set("batch_id", batchID)
		form.Set("batch_size", strconv.Itoa(batchSize))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, belspiskiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ProbeHit{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return ProbeHit{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return ProbeHit{}, err
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusTooManyRequests {
		return ProbeHit{}, fmt.Errorf("whitelist probe: %d", res.StatusCode)
	}
	var payload struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		Code       string `json:"code"`
		RetryAfter int    `json:"retry_after"`
		UI         struct {
			ChartLabel string `json:"chartLabel"`
			InBs       bool   `json:"inBs"`
			Situation  string `json:"situation"`
			Filter     string `json:"filter"`
			LogLine    string `json:"logLine"`
			Metrics    struct {
				HTTP metricLabel `json:"http"`
				TCP  metricLabel `json:"tcp"`
				ICMP metricLabel `json:"icmp"`
			} `json:"metrics"`
		} `json:"ui"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ProbeHit{}, err
	}
	if payload.OK && (payload.UI.Situation == "proxy_error" || payload.UI.Filter == "proxy_error") {
		return ProbeHit{}, belspiskiProxyError{}
	}
	if !payload.OK {
		if payload.Code == "rate_limited" || res.StatusCode == http.StatusTooManyRequests {
			wait := time.Duration(payload.RetryAfter) * time.Second
			if wait <= 0 {
				wait = 15 * time.Second
			}
			return ProbeHit{}, belspiskiLimitError{RetryAfter: wait}
		}
		if payload.Error != "" {
			return ProbeHit{}, fmt.Errorf("whitelist probe: %s", payload.Error)
		}
		return ProbeHit{}, fmt.Errorf("whitelist probe: rejected")
	}
	city, network := splitPick(pick, payload.UI.ChartLabel)
	hit := ProbeHit{
		City:    city,
		Network: network,
		Country: "RU",
		Kind:    "whitelist",
		OK:      payload.UI.InBs,
		PingMs:  pingFromBelspiski(payload.UI.Metrics.ICMP.Label, payload.UI.LogLine),
		Answer:  belspiskiAnswer(payload.UI.InBs, payload.UI.Situation, payload.UI.Filter, payload.UI.Metrics.HTTP, payload.UI.Metrics.TCP, payload.UI.Metrics.ICMP),
	}
	return hit, nil
}

type metricLabel struct {
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

func belspiskiAnswer(inBs bool, situation, filter string, http, tcp, icmp metricLabel) string {
	head := "Не в белом списке"
	switch {
	case inBs:
		head = "В белом списке"
	case situation == "blocked_bs_on":
		head = "БС включены, сайт закрыт"
	case situation == "open_no_bs":
		head = "БС нет — сайт открывается"
	case situation == "proxy_error" || filter == "proxy_error":
		head = "Ошибка прокси"
	}
	parts := []string{head}
	if text := metricText("HTTP", http.Label); text != "" {
		parts = append(parts, text)
	}
	if text := metricText("TCP", tcp.Label); text != "" {
		parts = append(parts, text)
	}
	if text := metricText("ICMP", icmp.Label); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, " · ")
}

func metricText(name, label string) string {
	label = strings.TrimSpace(label)
	if label == "" || label == "—" || label == "-" {
		return ""
	}
	return name + " " + label
}

func splitPick(pick, label string) (city, network string) {
	if parts := strings.SplitN(label, "·", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	parts := strings.SplitN(pick, "|", 2)
	city = parts[0]
	if len(parts) == 2 {
		network = parts[1]
	}
	switch city {
	case "moscow":
		city = "Москва"
	case "spb":
		city = "Санкт-Петербург"
	case "novosibirsk":
		city = "Новосибирск"
	case "incognito":
		city = "Инкогнито"
	}
	return city, network
}

// Города и операторы в том порядке, в каком они заданы в belspiskiPicks.
func sortWhitelistHits(hits []ProbeHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		return whitelistHitLess(hits[i], hits[j])
	})
}

func whitelistHitLess(a, b ProbeHit) bool {
	ac, an := whitelistRanks(a)
	bc, bn := whitelistRanks(b)
	if ac != bc {
		return ac < bc
	}
	if a.City != b.City {
		return a.City < b.City
	}
	if an != bn {
		return an < bn
	}
	return a.Network < b.Network
}

func whitelistRanks(hit ProbeHit) (city, network int) {
	city, network = len(belspiskiPicks), len(belspiskiPicks)
	for i, pick := range belspiskiPicks {
		name, operator := splitPick(pick, "")
		if name != hit.City {
			continue
		}
		if city == len(belspiskiPicks) {
			city = i
		}
		if operator == hit.Network {
			network = i
			return city, network
		}
	}
	return city, network
}

func pingFromBelspiski(label, logLine string) *int {
	if ms, ok := leadingInt(label); ok {
		return &ms
	}
	fields := strings.Split(logLine, "\t")
	if len(fields) == 0 {
		return nil
	}
	if ms, ok := leadingInt(fields[len(fields)-1]); ok {
		return &ms
	}
	return nil
}

func leadingInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	i := 0
	for i < len(raw) && (unicode.IsDigit(rune(raw[i])) || raw[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(raw[:i], 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	if n < 20 && strings.Contains(raw[:i], ".") {
		return int(n*1000 + 0.5), true
	}
	return int(n + 0.5), true
}
