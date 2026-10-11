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
	pingBusy      bool
	listBusy      bool
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
	if ok {
		sample = limitCities(sample)
	}
	return sample, Window(r.history[key], time.Now()), ok
}

// ErrBusy — замер уже идёт, второй запуск ничего не добавит.
var ErrBusy = errors.New("probe already running")

// ErrNoTargets — нечего проверять.
var ErrNoTargets = errors.New("no targets")

// watchEvery — как часто смотреть, не пора ли мерить. Сам пинг реже:
// Kick пропускает страну, пока не вышел её интервал.
const watchEvery = 30 * time.Second

// Watch меряет включённые страны по их интервалу, даже если страницу статуса
// никто не открывает. Останавливается вместе с ctx процесса.
func (r *Runner) Watch(ctx context.Context, load func(context.Context) ([]Target, error)) {
	if r == nil || load == nil {
		return
	}
	go r.watch(ctx, watchEvery, load)
}

func (r *Runner) watch(ctx context.Context, every time.Duration, load func(context.Context) ([]Target, error)) {
	if every <= 0 {
		every = watchEvery
	}
	slog.Info("status probe scheduler started", "every", every.String())
	r.schedule(ctx, load)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.schedule(ctx, load)
		}
	}
}

func (r *Runner) schedule(ctx context.Context, load func(context.Context) ([]Target, error)) {
	if ctx.Err() != nil {
		return
	}
	loadCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r.EnsureLoaded(loadCtx)
	targets, err := load(loadCtx)
	if err != nil {
		slog.Warn("status probe schedule", "error", err)
		return
	}
	r.Kick(targets)
}

// Kick запускает замер стран, у которых снимок старше их интервала.
// Пинг и белые списки не ждут друг друга: долгий обход операторов не держит обычный пинг.
func (r *Runner) Kick(targets []Target) {
	if r == nil || len(targets) == 0 || !ProbesEnabled() {
		return
	}
	r.mu.Lock()
	pingTargets, listTargets := r.pickLocked(targets, false)
	if r.pingBusy {
		pingTargets = nil
	}
	if r.listBusy {
		listTargets = nil
	}
	if len(pingTargets) > 0 {
		r.pingBusy = true
	}
	if len(listTargets) > 0 {
		r.listBusy = true
	}
	r.mu.Unlock()
	r.launch(pingTargets, listTargets)
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
	if r.pingBusy || r.listBusy {
		r.mu.Unlock()
		return ErrBusy
	}
	r.cooldownUntil = time.Time{}
	pingTargets, listTargets := r.pickLocked(targets, true)
	if len(pingTargets) == 0 && len(listTargets) == 0 {
		r.mu.Unlock()
		return ErrNoTargets
	}
	if len(pingTargets) > 0 {
		r.pingBusy = true
	}
	if len(listTargets) > 0 {
		r.listBusy = true
	}
	r.mu.Unlock()
	r.launch(pingTargets, listTargets)
	return nil
}

func (r *Runner) launch(pingTargets, listTargets []Target) {
	if len(pingTargets) > 0 {
		go func() {
			defer r.donePing()
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
			defer cancel()
			r.EnsureLoaded(ctx)
			r.pingPass(ctx, pingTargets)
		}()
	}
	if len(listTargets) > 0 {
		go func() {
			defer r.doneList()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
			defer cancel()
			r.whitelistPass(ctx, listTargets)
		}()
	}
}

func (r *Runner) donePing() {
	r.mu.Lock()
	r.pingBusy = false
	r.lastRun = time.Now()
	r.mu.Unlock()
}

func (r *Runner) doneList() {
	r.mu.Lock()
	r.listBusy = false
	r.mu.Unlock()
}

func (r *Runner) pickLocked(targets []Target, force bool) (ping []Target, list []Target) {
	if len(targets) > maxNodes {
		targets = targets[:maxNodes]
	}
	now := time.Now()
	paused := now.Before(r.cooldownUntil)
	for _, target := range targets {
		sample, ok := r.samples[target.Key]
		if force || (!paused && pingDue(target, sample, ok, now)) {
			ping = append(ping, target)
		}
		if target.Whitelist && (force || whitelistDue(target, sample, ok, now)) {
			list = append(list, target)
		}
	}
	return ping, list
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
	if !ok || !sample.WhitelistDone {
		return true
	}
	at := sample.WhitelistAt
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
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 15*time.Second)
	r.EnsureLoaded(loadCtx)
	loadCancel()

	r.mu.Lock()
	pingTargets, listTargets := r.pickLocked(targets, force)
	r.mu.Unlock()

	// Обычный пинг и белые списки идут вместе: долгая проверка операторов
	// больше не отодвигает замер остальных стран. У белых списков свой срок,
	// чтобы ожидание лимита belspiski не обрывало список операторов.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 12*time.Minute)
	listCtx, listCancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer pingCancel()
	defer listCancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.pingPass(pingCtx, pingTargets)
	}()
	go func() {
		defer wg.Done()
		r.whitelistPass(listCtx, listTargets)
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
		partial := errors.Is(wlErr, errWhitelistPartial)
		if wlErr != nil && !partial {
			slog.Warn("whitelist probe failed", "error", wlErr)
		}
		r.finishWhitelist(target.Key, wl.PingMs, !partial)
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
	sample.WhitelistDone = prev.WhitelistDone
	r.samples[key] = sample
	RecordDay(r.history, key, sample, time.Now())
	r.mu.Unlock()
}

func (r *Runner) addWhitelistHit(key string, hit ProbeHit) {
	r.mu.Lock()
	base := r.samples[key]
	base.Probes = mergeWhitelist(base.Probes, []ProbeHit{hit})
	base.WhitelistOK, base.WhitelistTotal = whitelistCounts(base.Probes)
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
		base.WhitelistDone = true
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
