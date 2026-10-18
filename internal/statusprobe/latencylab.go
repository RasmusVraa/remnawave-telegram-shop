package statusprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const latencyLabBase = "https://console.latencylab.ru"

var latencyLabOperatorsURL = latencyLabBase + "/api/lab/operators"
var latencyLabMultiscanURL = latencyLabBase + "/api/lab/multiscan"
var latencyLabJobURL = latencyLabBase + "/api/lab/job/"

// errWhitelistPartial — ответ оборвался, набор операторов неполный.
var errWhitelistPartial = errors.New("whitelist probe: incomplete")

type whitelistResult struct {
	OK     int
	Total  int
	PingMs *int
	Hits   []ProbeHit
}

var latencyLabWait = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var latencyLabLabels = map[string]string{
	"mts":     "МТС",
	"megafon": "Мегафон",
	"beeline": "Билайн",
	"t2":      "T2",
	"tmobile": "Т-Мобайл",
}

type latencyOperator struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Online bool   `json:"online"`
	WL     bool   `json:"wl"`
}

// MeasureWhitelist проверяет адрес через Latency Lab: модемы операторов,
// у которых включены белые списки.
func MeasureWhitelist(ctx context.Context, client *http.Client, host string) (whitelistResult, error) {
	return measureWhitelist(ctx, client, host, nil, nil)
}

func measureWhitelist(ctx context.Context, client *http.Client, host string, picks []string, onHit func(ProbeHit)) (whitelistResult, error) {
	token := strings.TrimSpace(os.Getenv("LATENCYLAB_TOKEN"))
	if token == "" {
		return whitelistResult{}, fmt.Errorf("whitelist probe: LATENCYLAB_TOKEN is empty")
	}
	if client == nil {
		client = http.DefaultClient
	}
	operators := picks
	labels := map[string]string{}
	if len(operators) == 0 {
		rows, err := latencyLabWhitelistOperators(ctx, client, token)
		if err != nil {
			return whitelistResult{}, err
		}
		for _, row := range rows {
			id := strings.ToLower(strings.TrimSpace(row.ID))
			if id == "" {
				continue
			}
			operators = append(operators, id)
			if label := strings.TrimSpace(row.Label); label != "" {
				labels[id] = label
			}
		}
	}
	if len(operators) == 0 {
		return whitelistResult{}, fmt.Errorf("whitelist probe: no whitelist operators online")
	}
	reqID, err := latencyLabStartMultiscan(ctx, client, token, host, operators)
	if err != nil {
		return whitelistResult{}, err
	}
	raw, err := latencyLabWaitJob(ctx, client, token, reqID)
	if err != nil {
		return whitelistResult{}, err
	}
	return latencyLabHits(raw, labels, onHit)
}

func latencyLabWhitelistOperators(ctx context.Context, client *http.Client, token string) ([]latencyOperator, error) {
	body, err := latencyLabGet(ctx, client, token, latencyLabOperatorsURL)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			Operators []latencyOperator `json:"operators"`
		} `json:"result"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if !parsed.OK {
		return nil, fmt.Errorf("whitelist probe: %s", strings.TrimSpace(parsed.Error))
	}
	out := make([]latencyOperator, 0, len(parsed.Result.Operators))
	for _, row := range parsed.Result.Operators {
		if row.Online && row.WL {
			out = append(out, row)
		}
	}
	return out, nil
}

func latencyLabStartMultiscan(ctx context.Context, client *http.Client, token, host string, operators []string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"target":    host,
		"operators": operators,
		"async":     true,
	})
	body, err := latencyLabPost(ctx, client, token, latencyLabMultiscanURL, payload)
	if err != nil {
		return "", err
	}
	var parsed struct {
		OK     bool   `json:"ok"`
		ReqID  string `json:"req_id"`
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if !parsed.OK && parsed.ReqID == "" {
		return "", fmt.Errorf("whitelist probe: %s", strings.TrimSpace(parsed.Error))
	}
	if parsed.ReqID == "" {
		return "", fmt.Errorf("whitelist probe: empty job")
	}
	return parsed.ReqID, nil
}

func latencyLabWaitJob(ctx context.Context, client *http.Client, token, reqID string) (json.RawMessage, error) {
	url := latencyLabJobURL + reqID
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, err := latencyLabGet(ctx, client, token, url)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			OK     bool            `json:"ok"`
			Status string          `json:"status"`
			Error  string          `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		switch parsed.Status {
		case "done":
			return parsed.Result, nil
		case "cancelled", "error":
			return nil, fmt.Errorf("whitelist probe: %s", strings.TrimSpace(parsed.Error))
		}
		if err := latencyLabWait(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
}

func latencyLabHits(raw json.RawMessage, labels map[string]string, onHit func(ProbeHit)) (whitelistResult, error) {
	var result struct {
		Results []struct {
			Operator string   `json:"operator"`
			OK       bool     `json:"ok"`
			RTT      *float64 `json:"rtt_ms"`
			ICMPOK   *bool    `json:"icmp_ok"`
			TCPOK    *bool    `json:"tcp_ok"`
			ICMPRTT  *float64 `json:"icmp_rtt_ms"`
			TCPRTT   *float64 `json:"tcp_rtt_ms"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return whitelistResult{}, err
	}
	var out whitelistResult
	var pings []int
	for _, row := range result.Results {
		id := strings.ToLower(strings.TrimSpace(row.Operator))
		name := labels[id]
		if name == "" {
			name = latencyLabLabels[id]
		}
		if name == "" {
			name = id
		}
		hit := ProbeHit{
			Network: name,
			Country: "RU",
			Kind:    "whitelist",
			OK:      row.OK,
			ICMPOK:  row.ICMPOK,
			TCPOK:   row.TCPOK,
			ICMPMs:  latencyMs(row.ICMPRTT),
			TCPMs:   latencyMs(row.TCPRTT),
		}
		if ms := latencyMs(row.RTT); ms != nil {
			hit.PingMs = ms
			pings = append(pings, *ms)
		}
		out.Hits = append(out.Hits, hit)
		out.Total++
		if hit.OK {
			out.OK++
		}
		if onHit != nil {
			onHit(hit)
		}
	}
	if len(pings) > 0 {
		sum := 0
		for _, ms := range pings {
			sum += ms
		}
		avg := int(float64(sum)/float64(len(pings)) + 0.5)
		out.PingMs = &avg
	}
	sortWhitelistHits(out.Hits)
	if out.Total == 0 {
		return out, fmt.Errorf("whitelist probe: empty result")
	}
	return out, nil
}

func latencyMs(value *float64) *int {
	if value == nil || *value <= 0 {
		return nil
	}
	ms := int(*value + 0.5)
	return &ms
}

func latencyLabGet(ctx context.Context, client *http.Client, token, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return latencyLabDo(client, token, req)
}

func latencyLabPost(ctx context.Context, client *http.Client, token, url string, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return latencyLabDo(client, token, req)
}

func latencyLabDo(client *http.Client, token string, req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "remnawave-telegram-shop/status")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{RetryAfter: retryAfter(res.Header)}
	}
	if res.StatusCode == http.StatusConflict {
		return nil, errWhitelistPartial
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var parsed struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &parsed)
		msg := strings.TrimSpace(parsed.Error)
		if msg == "" {
			msg = fmt.Sprintf("status %d", res.StatusCode)
		}
		return nil, fmt.Errorf("whitelist probe: %s", msg)
	}
	return raw, nil
}
