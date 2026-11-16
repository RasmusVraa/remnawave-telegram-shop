package statusprobe

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

var limitsURL = "https://api.globalping.io/v1/limits"

// rateLimitFrom собирает ошибку 429: время ожидания из заголовка и то, что сервис написал в теле.
// Тело идёт в лог и в админку: по нему видно, кончились кредиты или сработал другой лимит.
func rateLimitFrom(res *http.Response, raw []byte) *RateLimitError {
	out := &RateLimitError{
		RetryAfter:    retryAfter(res.Header),
		HasRetryAfter: strings.TrimSpace(res.Header.Get("Retry-After")) != "",
		Message:       rateLimitMessage(raw),
	}
	where := ""
	if res.Request != nil && res.Request.URL != nil {
		where = res.Request.URL.Host + res.Request.URL.Path
	}
	slog.Warn("status probe: service answered 429",
		"url", where,
		"retry_after_header", res.Header.Get("Retry-After"),
		"body", out.Message)
	return out
}

func rateLimitMessage(raw []byte) string {
	var parsed struct {
		Error json.RawMessage `json:"error"`
		Msg   string          `json:"message"`
	}
	text := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &parsed) == nil {
		var obj struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		var plain string
		switch {
		case len(parsed.Error) > 0 && json.Unmarshal(parsed.Error, &obj) == nil && obj.Message != "":
			text = obj.Message
		case len(parsed.Error) > 0 && json.Unmarshal(parsed.Error, &plain) == nil && plain != "":
			text = plain
		case parsed.Msg != "":
			text = parsed.Msg
		}
	}
	runes := []rune(text)
	if len(runes) > 200 {
		text = string(runes[:200])
	}
	return text
}

// Note — короткая причина паузы для админки.
func (e *RateLimitError) Note() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "HTTP 429"
}

// PauseNotes — что ответил сервис, когда поставил проверки на паузу: обычный пинг и белые списки.
func (r *Runner) PauseNotes() (ping, list string) {
	if r == nil {
		return "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cooldownNote, r.listCooldownNote
}

// GlobalpingLimits — настоящий остаток у Globalping, не наша прикидка.
type GlobalpingLimits struct {
	// Type — «user» (по токену) или «ip» (без токена).
	Type string `json:"type,omitempty"`
	// Limit и Remaining — измерений в час. -1, если сервис не сказал.
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
	// ResetSec — через сколько секунд лимит обновится.
	ResetSec int `json:"reset_sec"`
	// Credits — остаток кредитов сверх бесплатного лимита; nil, если сервис не отдал.
	Credits *int `json:"credits,omitempty"`
}

var (
	limitsMu     sync.Mutex
	limitsCached *GlobalpingLimits
	limitsAt     time.Time
)

// FetchGlobalpingLimits спрашивает GET /v1/limits. Запрос кредитов не тратит.
// Ответ держится 20 секунд, чтобы открытая страница не долбила сервис.
func FetchGlobalpingLimits(ctx context.Context, client *http.Client) (*GlobalpingLimits, error) {
	limitsMu.Lock()
	if limitsCached != nil && time.Since(limitsAt) < 20*time.Second {
		out := *limitsCached
		limitsMu.Unlock()
		return &out, nil
	}
	limitsMu.Unlock()
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, limitsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "remnawave-telegram-shop/status")
	authorizeGlobalping(req)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &limitsStatusError{code: res.StatusCode}
	}
	var parsed struct {
		RateLimit struct {
			Measurements struct {
				Create struct {
					Type      string `json:"type"`
					Limit     *int   `json:"limit"`
					Remaining *int   `json:"remaining"`
					Reset     *int   `json:"reset"`
				} `json:"create"`
			} `json:"measurements"`
		} `json:"rateLimit"`
		Credits *struct {
			Remaining *int `json:"remaining"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	c := parsed.RateLimit.Measurements.Create
	out := &GlobalpingLimits{Type: c.Type, Limit: -1, Remaining: -1}
	if c.Limit != nil {
		out.Limit = *c.Limit
	}
	if c.Remaining != nil {
		out.Remaining = *c.Remaining
	}
	if c.Reset != nil {
		out.ResetSec = *c.Reset
	}
	if parsed.Credits != nil && parsed.Credits.Remaining != nil {
		out.Credits = parsed.Credits.Remaining
	}
	limitsMu.Lock()
	cp := *out
	limitsCached, limitsAt = &cp, time.Now()
	limitsMu.Unlock()
	return out, nil
}

type limitsStatusError struct{ code int }

func (e *limitsStatusError) Error() string {
	return "globalping limits: http " + http.StatusText(e.code)
}

// CheckState — итог одной проверки у строки для значка в админке.
const (
	CheckOK      = "ok"
	CheckPartial = "partial"
	CheckBad     = "bad"
	CheckNone    = "none"
)

// CheckResult — сколько зондов ответило. Total 0 — замера нет.
type CheckResult struct {
	State string `json:"state"`
	OK    int    `json:"ok"`
	Total int    `json:"total"`
}

// RowChecks — что видно у строки: «мир» и «РФ». У обхода «РФ» — это операторы с белыми списками.
type RowChecks struct {
	World  CheckResult `json:"world"`
	Russia CheckResult `json:"russia"`
	// Operators — «РФ» посчитано по операторам, а не по зондам в России.
	Operators  bool       `json:"operators,omitempty"`
	MeasuredAt *time.Time `json:"measured_at,omitempty"`
	// Registry — значок «РКН»: числится ли адрес в списках блокировок.
	Registry RegistryCheck `json:"registry"`
	// RussiaSource — кто дал «Россию»: globalping или cheburcheck. RussiaFailed — кто не открыл сервер
	// (до четырёх, «провайдер (регион)»), только у Cheburcheck.
	RussiaSource string   `json:"russia_source,omitempty"`
	RussiaFailed []string `json:"russia_failed,omitempty"`
}

func checkOf(ok, total int) CheckResult {
	switch {
	case total <= 0:
		return CheckResult{State: CheckNone}
	case ok >= total:
		return CheckResult{State: CheckOK, OK: ok, Total: total}
	case ok <= 0:
		return CheckResult{State: CheckBad, OK: ok, Total: total}
	}
	return CheckResult{State: CheckPartial, OK: ok, Total: total}
}

// ChecksOf превращает последний замер в значки. sample nil — замера ещё нет.
func ChecksOf(sample *Sample, bypass bool) RowChecks {
	if sample == nil {
		return RowChecks{World: CheckResult{State: CheckNone}, Russia: CheckResult{State: CheckNone}, Registry: RegistryCheck{State: CheckNone}}
	}
	out := RowChecks{World: checkOf(sample.WorldOK, sample.WorldTotal), Registry: registryOf(sample)}
	if bypass && sample.WhitelistTotal > 0 {
		out.Russia = checkOf(sample.WhitelistOK, sample.WhitelistTotal)
		out.Operators = true
	} else {
		out.Russia = checkOf(sample.RussiaOK, sample.RussiaTotal)
		if sample.RussiaTotal > 0 {
			out.RussiaSource = SourceGlobalping
			if sample.RuSource == SourceCheburcheck {
				out.RussiaSource = SourceCheburcheck
				out.RussiaFailed = failedScanners(sample, 4)
			}
		}
	}
	if !sample.MeasuredAt.IsZero() {
		at := sample.MeasuredAt
		out.MeasuredAt = &at
	}
	return out
}
