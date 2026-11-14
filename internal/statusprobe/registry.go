package statusprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Проверка по спискам блокировок: числится ли адрес сервера в реестре Роскомнадзора
// или в диапазонах заблокированных CDN. Источник — открытый API cheburcheck.ru
// (GET /api/v1/check, без токена). Это не пинг: так видно блокировку по списку,
// но не обрыв на лету. Адрес сервера в замер и на публичную страницу не попадает.

// RegistryKind — по какому списку нашёлся адрес.
const (
	RegistryKindRegistry = "registry"
	RegistryKindCDN      = "cdn"
)

var (
	registryCheckURL  = "https://cheburcheck.ru/api/v1/check"
	registryStatusURL = "https://cheburcheck.ru/api/v1/status"
)

const (
	// registryEvery — как часто смотреть список; сами списки у сервиса обновляются раз в 30–60 минут.
	registryEvery = 30 * time.Minute
	// registryRecheck — повтор после находки или сбоя: находку подтверждаем быстро, а не через полчаса.
	registryRecheck = 10 * time.Minute
	// registryGap — пауза между серверами, чтобы не бить по чужому API пачкой.
	registryGap = 700 * time.Millisecond
	// registryUserAgent — чтобы владельцы сервиса видели, кто ходит.
	registryUserAgent = "meows-status-probe/1 (+registry check)"
)

// registryResult — ответ сервиса по одному адресу.
type registryResult struct {
	Blocked bool
	Kind    string
}

// registryCheck — подменяется в тестах.
var registryCheck = checkRegistry

// checkRegistry спрашивает у cheburcheck, числится ли host в списках.
func checkRegistry(ctx context.Context, client *http.Client, host string) (registryResult, error) {
	q := url.Values{}
	q.Set("target", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryCheckURL+"?"+q.Encode(), nil)
	if err != nil {
		return registryResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", registryUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return registryResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return registryResult{}, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return registryResult{}, &RateLimitError{RetryAfter: retryAfter(resp.Header), Message: "cheburcheck"}
	case resp.StatusCode != http.StatusOK:
		return registryResult{}, fmt.Errorf("cheburcheck: http %d", resp.StatusCode)
	}
	var parsed struct {
		Blocked       *bool            `json:"blocked"`
		BlockedSubnet []any            `json:"blocked_subnets"`
		CDNProviders  map[string][]any `json:"cdn_providers"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return registryResult{}, err
	}
	// Без поля blocked ответ не наш: не гадаем, а считаем сбоем.
	if parsed.Blocked == nil {
		return registryResult{}, errors.New("cheburcheck: no verdict")
	}
	out := registryResult{Blocked: *parsed.Blocked}
	if out.Blocked {
		out.Kind = RegistryKindRegistry
		if len(parsed.BlockedSubnet) == 0 && len(parsed.CDNProviders) > 0 {
			out.Kind = RegistryKindCDN
		}
	}
	return out, nil
}

// RegistryInfo — состояние самих списков сервиса для карточки в админке.
type RegistryInfo struct {
	// UpdatedAt — когда сервис последний раз обновил базу.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// Domains — сколько доменов в реестре.
	Domains int `json:"domains,omitempty"`
}

var (
	registryInfoMu     sync.Mutex
	registryInfoCached *RegistryInfo
	registryInfoAt     time.Time
)

// FetchRegistryInfo спрашивает у сервиса, когда обновлены списки. Ответ живёт 5 минут.
func FetchRegistryInfo(ctx context.Context, client *http.Client) (*RegistryInfo, error) {
	registryInfoMu.Lock()
	if registryInfoCached != nil && time.Since(registryInfoAt) < 5*time.Minute {
		cp := *registryInfoCached
		registryInfoMu.Unlock()
		return &cp, nil
	}
	registryInfoMu.Unlock()
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryStatusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", registryUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cheburcheck status: http %d", resp.StatusCode)
	}
	var parsed struct {
		DomainCount int       `json:"domain_count"`
		LastUpdate  time.Time `json:"last_update"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&parsed); err != nil {
		return nil, err
	}
	out := &RegistryInfo{Domains: parsed.DomainCount}
	if !parsed.LastUpdate.IsZero() {
		at := parsed.LastUpdate
		out.UpdatedAt = &at
	}
	registryInfoMu.Lock()
	cp := *out
	registryInfoCached, registryInfoAt = &cp, time.Now()
	registryInfoMu.Unlock()
	return out, nil
}

// registryDue — пора ли смотреть список для этой строки.
func registryDue(target Target, sample Sample, ok bool, now time.Time) bool {
	if !registryApplies(target) {
		return false
	}
	if !ok || sample.RegistryTryAt.IsZero() {
		return true
	}
	wait := registryEvery
	// Находку или сбой перепроверяем быстро: алерт открывается по двум находкам подряд.
	if !sample.RegistryOK || (sample.RegistryBlocked && sample.RegistryStreak < alertStreak) {
		wait = registryRecheck
	}
	return now.Sub(sample.RegistryTryAt) >= wait
}

// registryApplies — у каких строк есть смысл смотреть списки. У обхода адрес — хост из белого
// списка, а не адрес сервера, поэтому его реестр не касается.
func registryApplies(target Target) bool {
	return !target.Disabled && !target.NoExternal && !target.Whitelist
}

func (r *Runner) pickRegistryLocked(targets []Target, force bool) []Target {
	if len(targets) > maxNodes {
		targets = targets[:maxNodes]
	}
	now := time.Now()
	paused := now.Before(r.registryCooldownUntil)
	var out []Target
	for _, target := range targets {
		if !registryApplies(target) {
			continue
		}
		sample, ok := r.samples[target.Key]
		if force || (!paused && registryDue(target, sample, ok, now)) {
			out = append(out, target)
		}
	}
	return out
}

// RegistryPause — до какого времени проверка списков стоит после ответа 429. Нулевое время — не на паузе.
func (r *Runner) RegistryPause() time.Time {
	if r == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registryCooldownUntil
}

// kickRegistry запускает проверку списков для строк, у которых подошёл срок.
// Идёт отдельным проходом и не ждёт ни пинг, ни белые списки.
func (r *Runner) kickRegistry(targets []Target, force bool) {
	if r == nil || !ProbesEnabled() {
		return
	}
	r.mu.Lock()
	if r.registryBusy {
		r.mu.Unlock()
		return
	}
	picks := r.pickRegistryLocked(targets, force)
	if len(picks) == 0 {
		r.mu.Unlock()
		return
	}
	r.registryBusy = true
	r.mu.Unlock()
	go func() {
		defer r.doneRegistry()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		r.EnsureLoaded(ctx)
		r.registryPass(ctx, picks)
	}()
}

func (r *Runner) doneRegistry() {
	r.mu.Lock()
	r.registryBusy = false
	r.mu.Unlock()
}

func (r *Runner) registryPass(ctx context.Context, targets []Target) {
	var events []AlertEvent
	defer func() { r.emitAlerts(events) }()
	measured := 0
	for i, target := range targets {
		host, ok := PublicTarget(target.Address)
		if !ok {
			continue
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(registryGap):
			}
		}
		res, err := registryCheck(ctx, r.client, host)
		if err != nil {
			var limited *RateLimitError
			if errors.As(err, &limited) {
				wait := 15 * time.Minute
				if limited != nil && limited.RetryAfter > 0 {
					wait = limited.RetryAfter
				}
				r.mu.Lock()
				r.registryCooldownUntil = time.Now().Add(wait)
				r.mu.Unlock()
				slog.Warn("registry check paused", "retry_in", wait.String())
				break
			}
			slog.Warn("registry check failed", "error", err)
			r.markRegistryFailed(target.Key)
			continue
		}
		r.storeRegistry(target.Key, res)
		r.mu.Lock()
		events = append(events, r.registryAlertsLocked(target, time.Now())...)
		r.mu.Unlock()
		r.persist()
		measured++
	}
	if measured > 0 {
		slog.Info("registry checks updated", "nodes", measured)
	}
}

// markRegistryFailed запоминает попытку, не трогая прежний ответ: сбой сервиса
// не должен выглядеть ни как «в реестре», ни как «чисто».
func (r *Runner) markRegistryFailed(key string) {
	r.mu.Lock()
	s := r.samples[key]
	s.RegistryTryAt = time.Now()
	s.RegistryOK = false
	r.samples[key] = s
	r.mu.Unlock()
}

func (r *Runner) storeRegistry(key string, res registryResult) {
	r.mu.Lock()
	s := r.samples[key]
	now := time.Now()
	s.RegistryTryAt, s.RegistryAt, s.RegistryOK = now, now, true
	s.RegistryBlocked = res.Blocked
	s.RegistryKind = res.Kind
	if res.Blocked {
		s.RegistryStreak++
	} else {
		s.RegistryStreak = 0
	}
	r.samples[key] = s
	r.mu.Unlock()
}

// registryAlertsLocked открывает тревогу после двух находок подряд и закрывает первым чистым ответом. Под r.mu.
func (r *Runner) registryAlertsLocked(target Target, now time.Time) []AlertEvent {
	s, ok := r.samples[target.Key]
	if !ok || !s.RegistryOK {
		return nil
	}
	if !ProbesEnabled() || !AlertRegistryEnabled() {
		s.Alerts = forgetAlert(s.Alerts, AlertRegistry)
		r.samples[target.Key] = s
		return nil
	}
	events := appendAlert(nil, &s, target, AlertRegistry, s.RegistryStreak >= alertStreak, !s.RegistryBlocked, now)
	for i := range events {
		events[i].Note = s.RegistryKind
	}
	r.samples[target.Key] = s
	return events
}

// RegistryCheck — значок «РКН» у строки: ok — не найден, bad — числится в списках.
type RegistryCheck struct {
	State string `json:"state"`
	// Kind — registry или cdn, только при bad.
	Kind string `json:"kind,omitempty"`
	// CheckedAt — когда получен последний ответ.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

func registryOf(sample *Sample) RegistryCheck {
	if sample == nil || sample.RegistryAt.IsZero() {
		return RegistryCheck{State: CheckNone}
	}
	at := sample.RegistryAt
	out := RegistryCheck{State: CheckOK, CheckedAt: &at}
	if sample.RegistryBlocked {
		out.State = CheckBad
		out.Kind = sample.RegistryKind
	}
	return out
}

// stripRegistry убирает проверку списков из замера: у строки погас внешний замер, значок серый.
func stripRegistry(sample Sample) Sample {
	sample.RegistryAt = time.Time{}
	sample.RegistryTryAt = time.Time{}
	sample.RegistryOK = false
	sample.RegistryBlocked = false
	sample.RegistryKind = ""
	sample.RegistryStreak = 0
	sample.Alerts = forgetAlert(sample.Alerts, AlertRegistry)
	return sample
}
