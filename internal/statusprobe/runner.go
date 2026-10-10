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
	r.start(due, false)
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
	r.start(targets, true)
	return nil
}

func (r *Runner) start(targets []Target, force bool) {
	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.lastRun = time.Now()
			r.mu.Unlock()
		}()
		r.execute(targets, force)
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

func targetInterval(target Target) time.Duration {
	interval := target.Interval
	if interval <= 0 {
		interval = time.Duration(ProbeIntervalMinutes()) * time.Minute
	}
	if interval < 5*time.Minute {
		return 5 * time.Minute
	}
	return interval
}

func pingDue(target Target, sample Sample, ok bool, now time.Time) bool {
	if !ok || sample.MeasuredAt.IsZero() {
		return true
	}
	return now.Sub(sample.MeasuredAt) >= targetInterval(target)
}

func whitelistDue(target Target, sample Sample, ok bool, now time.Time) bool {
	if !target.Whitelist {
		return false
	}
	if !ok {
		return true
	}
	// Старые снимки без отдельной метки белых списков считаем по времени пинга,
	// иначе каждый заход на страницу заново гонял бы проверку.
	at := sample.WhitelistAt
	if at.IsZero() {
		at = sample.MeasuredAt
	}
	if at.IsZero() {
		return true
	}
	return now.Sub(at) >= targetInterval(target)
}

func dueTargets(targets []Target, samples map[string]Sample, now time.Time) []Target {
	out := make([]Target, 0, len(targets))
	for _, target := range targets {
		sample, ok := samples[target.Key]
		if pingDue(target, sample, ok, now) || whitelistDue(target, sample, ok, now) {
			out = append(out, target)
		}
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
	r.execute(targets, false)
}

func (r *Runner) execute(targets []Target, force bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	r.EnsureLoaded(ctx)

	if len(targets) > maxNodes {
		targets = targets[:maxNodes]
	}
	now := time.Now()
	r.mu.Lock()
	pingTargets := make([]Target, 0, len(targets))
	listTargets := make([]Target, 0, len(targets))
	for _, target := range targets {
		sample, ok := r.samples[target.Key]
		if force || pingDue(target, sample, ok, now) {
			pingTargets = append(pingTargets, target)
		}
		if target.Whitelist && (force || whitelistDue(target, sample, ok, now)) {
			listTargets = append(listTargets, target)
		}
	}
	r.mu.Unlock()

	// Обычный пинг и белые списки идут вместе: долгая проверка операторов
	// больше не отодвигает замер остальных стран.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.pingPass(ctx, pingTargets)
	}()
	go func() {
		defer wg.Done()
		r.whitelistPass(ctx, listTargets)
	}()
	wg.Wait()
}

func (r *Runner) pingPass(ctx context.Context, targets []Target) {
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
	if measured > 0 {
		slog.Info("status probes updated", "nodes", measured)
	}
}

func (r *Runner) whitelistPass(ctx context.Context, targets []Target) {
	measured := 0
	for _, target := range targets {
		host, ok := PublicTarget(target.Address)
		if !ok {
			continue
		}
		r.mu.Lock()
		base := r.samples[target.Key]
		base.Probes = dropProxyErrorHits(base.Probes)
		r.samples[target.Key] = base
		r.mu.Unlock()
		wl, wlErr := listTarget(ctx, r.client, host, func(hit ProbeHit) {
			r.addWhitelistHit(target.Key, hit)
		})
		if wlErr != nil {
			slog.Warn("whitelist probe failed", "error", wlErr)
		}
		r.finishWhitelist(target.Key, wl.PingMs, true)
		if wl.Total == 0 && wlErr != nil {
			continue
		}
		r.persist()
		measured++
		slog.Info("whitelist probe", "operators", wl.Total)
	}
	if measured > 0 {
		r.persist()
	}
}

func (r *Runner) storeSample(key string, sample Sample) {
	r.mu.Lock()
	prev := r.samples[key]
	sample.Probes = append(withoutWhitelist(sample.Probes), whitelistOnly(prev.Probes)...)
	sample.WhitelistOK, sample.WhitelistTotal = whitelistCounts(sample.Probes)
	sample.WhitelistPingMs = prev.WhitelistPingMs
	sample.WhitelistAt = prev.WhitelistAt
	r.samples[key] = sample
	RecordDay(r.history, key, sample, time.Now())
	r.mu.Unlock()
}

func (r *Runner) addWhitelistHit(key string, hit ProbeHit) {
	r.mu.Lock()
	base := r.samples[key]
	base.Probes = mergeWhitelist(base.Probes, []ProbeHit{hit})
	base.WhitelistOK, base.WhitelistTotal = whitelistCounts(base.Probes)
	if base.WhitelistAt.IsZero() {
		base.WhitelistAt = time.Now()
	}
	r.samples[key] = base
	r.mu.Unlock()
}

func (r *Runner) finishWhitelist(key string, pingMs *int, stamp bool) {
	r.mu.Lock()
	base := r.samples[key]
	if pingMs != nil {
		base.WhitelistPingMs = pingMs
	}
	base.WhitelistOK, base.WhitelistTotal = whitelistCounts(base.Probes)
	if stamp {
		base.WhitelistAt = time.Now()
	}
	r.samples[key] = base
	r.mu.Unlock()
}

func whitelistOnly(hits []ProbeHit) []ProbeHit {
	out := make([]ProbeHit, 0)
	for _, hit := range hits {
		if hit.Kind == "whitelist" {
			out = append(out, hit)
		}
	}
	return out
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
