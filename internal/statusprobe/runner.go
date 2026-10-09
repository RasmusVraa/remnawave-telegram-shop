package statusprobe

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const maxNodes = 24

// Target — узел, который можно пинговать. Address не покидает этот пакет.
// Interval 0 — общий интервал из настроек. World и Russia 0 — общие лимиты зондов.
type Target struct {
	Key       string
	Address   string
	Interval  time.Duration
	World     int
	Russia    int
	Whitelist bool
}

// Runner держит последний замер и журнал. Страница статуса не ждёт зонды:
// она забирает уже готовый снимок, а обновление идёт в фоне.
type Runner struct {
	store  HistoryStore
	client *http.Client

	mu            sync.Mutex
	samples       map[string]Sample
	history       map[string]map[string]DayRecord
	lastRun       time.Time
	cooldownUntil time.Time
	running       bool
	loaded        bool
}

func NewRunner(store HistoryStore) *Runner {
	return &Runner{
		store:   store,
		client:  &http.Client{Timeout: 30 * time.Second},
		samples: map[string]Sample{},
		history: map[string]map[string]DayRecord{},
	}
}

// Snapshot возвращает последний удачный замер и окно истории.
func (r *Runner) Snapshot(key string) (Sample, []*float64, bool) {
	if r == nil {
		return Sample{}, nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sample, ok := r.samples[key]
	return sample, Window(r.history[key], time.Now()), ok
}

// ErrBusy — замер уже идёт, второй запуск ничего не добавит.
var ErrBusy = errors.New("probe already running")

// ErrNoTargets — нечего проверять.
var ErrNoTargets = errors.New("no targets")

// Kick запускает замер стран, у которых снимок старше их интервала.
// Повторный вызов, пока замер идёт, ничего не делает.
func (r *Runner) Kick(targets []Target) {
	if r == nil || len(targets) == 0 || !ProbesEnabled() {
		return
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	due := dueTargets(targets, r.samples, time.Now())
	if time.Now().Before(r.cooldownUntil) {
		only := due[:0]
		for _, target := range due {
			if target.Whitelist {
				only = append(only, target)
			}
		}
		due = only
	}
	if len(due) == 0 {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()
	r.start(due)
}

// Force измеряет все переданные страны сразу, не дожидаясь интервала.
func (r *Runner) Force(targets []Target) error {
	if r == nil || !ProbesEnabled() {
		return ErrNoTargets
	}
	if len(targets) == 0 {
		return ErrNoTargets
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return ErrBusy
	}
	r.running = true
	r.cooldownUntil = time.Time{}
	r.mu.Unlock()
	r.start(targets)
	return nil
}

func (r *Runner) start(targets []Target) {
	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.lastRun = time.Now()
			r.mu.Unlock()
		}()
		r.run(targets)
	}()
}

func mergeWhitelist(prev, next []ProbeHit) []ProbeHit {
	out := withoutWhitelist(prev)
	index := map[string]int{}
	for _, hit := range prev {
		if hit.Kind != "whitelist" {
			continue
		}
		key := hit.City + "\n" + hit.Network
		if _, ok := index[key]; ok {
			continue
		}
		index[key] = len(out)
		out = append(out, hit)
	}
	for _, hit := range next {
		key := hit.City + "\n" + hit.Network
		if i, ok := index[key]; ok {
			out[i] = hit
			continue
		}
		index[key] = len(out)
		out = append(out, hit)
	}
	start := 0
	for start < len(out) && out[start].Kind != "whitelist" {
		start++
	}
	sortWhitelistHits(out[start:])
	return out
}

func whitelistCounts(hits []ProbeHit) (ok, total int) {
	for _, hit := range hits {
		if hit.Kind != "whitelist" {
			continue
		}
		total++
		if hit.OK {
			ok++
		}
	}
	return ok, total
}

func dropProxyErrorHits(hits []ProbeHit) []ProbeHit {
	out := make([]ProbeHit, 0, len(hits))
	for _, hit := range hits {
		if hitProxyError(hit) {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func hasProxyError(hits []ProbeHit) bool {
	for _, hit := range hits {
		if hitProxyError(hit) {
			return true
		}
	}
	return false
}

func withoutWhitelist(hits []ProbeHit) []ProbeHit {
	out := make([]ProbeHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Kind == "whitelist" {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func dueTargets(targets []Target, samples map[string]Sample, now time.Time) []Target {
	out := make([]Target, 0, len(targets))
	for _, target := range targets {
		interval := target.Interval
		if interval <= 0 {
			interval = time.Duration(ProbeIntervalMinutes()) * time.Minute
		}
		if interval < 5*time.Minute {
			interval = 5 * time.Minute
		}
		if sample, ok := samples[target.Key]; ok && !sample.MeasuredAt.IsZero() && now.Sub(sample.MeasuredAt) < interval {
			incomplete := sample.WhitelistTotal < len(belspiskiPicks) || hasProxyError(sample.Probes)
			if target.Whitelist && incomplete && (sample.WhitelistTotal == 0 || now.Sub(sample.MeasuredAt) >= 90*time.Second) {
				out = append(out, target)
			}
			continue
		}
		out = append(out, target)
	}
	return out
}

// pingTarget и listTarget подменяются в тесте. В бою это Globalping и belspiski.
var pingTarget = MeasurePingCounts
var listTarget = measureWhitelist

func probeCounts(target Target) (world, russia int) {
	world, russia = ProbeLimits()
	if target.World > 0 {
		world = target.World
	}
	if target.Russia > 0 {
		russia = target.Russia
	}
	return world, russia
}

func (r *Runner) run(targets []Target) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	r.EnsureLoaded(ctx)

	if len(targets) > maxNodes {
		targets = targets[:maxNodes]
	}
	// Сначала пинг всех стран. Белые списки долгие и иначе держат очередь:
	// первая страна с галочкой измеряется, остальные висят на «Ещё считаем».
	measured := 0
	for _, target := range targets {
		host, ok := PublicTarget(target.Address)
		if !ok {
			continue
		}
		r.mu.Lock()
		paused := time.Now().Before(r.cooldownUntil)
		r.mu.Unlock()
		if paused {
			break
		}
		world, russia := probeCounts(target)
		sample, err := pingTarget(ctx, r.client, host, world, russia)
		if err != nil {
			var limited *RateLimitError
			if errors.As(err, &limited) {
				wait := 15 * time.Minute
				if limited != nil && limited.RetryAfter > 0 {
					wait = limited.RetryAfter
				}
				r.mu.Lock()
				r.cooldownUntil = time.Now().Add(wait)
				r.mu.Unlock()
				slog.Warn("status probe paused", "error", err, "retry_in", wait.String())
				break
			}
			slog.Warn("status probe failed", "error", err)
			continue
		}
		if sample.WorldTotal == 0 && sample.RussiaTotal == 0 {
			continue
		}
		if sample.MeasuredAt.IsZero() {
			sample.MeasuredAt = time.Now()
		}
		r.storeSample(target.Key, sample)
		r.persist()
		measured++
	}

	for _, target := range targets {
		if !target.Whitelist {
			continue
		}
		host, ok := PublicTarget(target.Address)
		if !ok {
			continue
		}
		r.mu.Lock()
		base := r.samples[target.Key]
		r.mu.Unlock()
		base.Probes = dropProxyErrorHits(base.Probes)
		base.WhitelistOK, base.WhitelistTotal = whitelistCounts(base.Probes)
		wl, wlErr := listTarget(ctx, r.client, host, func(hit ProbeHit) {
			base.Probes = mergeWhitelist(base.Probes, []ProbeHit{hit})
			base.WhitelistOK, base.WhitelistTotal = whitelistCounts(base.Probes)
			if base.MeasuredAt.IsZero() {
				base.MeasuredAt = time.Now()
			}
			r.mu.Lock()
			r.samples[target.Key] = base
			r.mu.Unlock()
		})
		if wlErr != nil {
			slog.Warn("whitelist probe failed", "error", wlErr)
		}
		if wl.Total == 0 {
			continue
		}
		base.WhitelistPingMs = wl.PingMs
		if base.MeasuredAt.IsZero() {
			base.MeasuredAt = time.Now()
		}
		r.mu.Lock()
		r.samples[target.Key] = base
		r.mu.Unlock()
		r.persist()
		measured++
		slog.Info("whitelist probe", "operators", base.WhitelistTotal)
	}
	if measured == 0 {
		return
	}
	r.persist()
	slog.Info("status probes updated", "nodes", measured)
}

func (r *Runner) storeSample(key string, sample Sample) {
	r.mu.Lock()
	r.samples[key] = sample
	RecordDay(r.history, key, sample, time.Now())
	r.mu.Unlock()
}

func (r *Runner) persist() {
	if r.store == nil {
		return
	}
	r.mu.Lock()
	if !r.loaded {
		r.mu.Unlock()
		return
	}
	snapshot := cloneHistory(r.history)
	samples := make(map[string]Sample, len(r.samples))
	for key, sample := range r.samples {
		samples[key] = sample
	}
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.store.Save(ctx, snapshot); err != nil {
		slog.Warn("status probe history save", "error", err)
	}
	latest, ok := r.store.(interface {
		SaveLatest(context.Context, map[string]Sample) error
	})
	if !ok {
		return
	}
	if err := latest.SaveLatest(ctx, samples); err != nil {
		slog.Warn("status probe latest save", "error", err)
	}
}

// EnsureLoaded поднимает журнал из базы до того, как страница статуса его прочитает.
// Пока чтение не удалось, запись не включается: пустой снимок не сотрёт старые дни.
func (r *Runner) EnsureLoaded(ctx context.Context) {
	r.mu.Lock()
	if r.loaded || r.store == nil {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	history, histErr := r.store.Load(ctx)
	var latest map[string]Sample
	var latestErr error
	if store, ok := r.store.(interface {
		LoadLatest(context.Context) (map[string]Sample, error)
	}); ok {
		latest, latestErr = store.LoadLatest(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded {
		return
	}
	if histErr != nil {
		slog.Warn("status probe history load", "error", histErr)
		return
	}
	if latestErr != nil {
		slog.Warn("status probe latest load", "error", latestErr)
		return
	}
	r.loaded = true
	if history != nil {
		r.history = history
	}
	for key, sample := range latest {
		if _, exists := r.samples[key]; exists {
			continue
		}
		r.samples[key] = sample
	}
}

func cloneHistory(src map[string]map[string]DayRecord) map[string]map[string]DayRecord {
	out := make(map[string]map[string]DayRecord, len(src))
	for key, days := range src {
		cp := make(map[string]DayRecord, len(days))
		for day, rec := range days {
			cp[day] = rec
		}
		out[key] = cp
	}
	return out
}
