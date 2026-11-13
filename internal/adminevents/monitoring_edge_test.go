package adminevents

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Плейсхолдеры $N в WHERE должны совпадать с числом аргументов: List дописывает
// LIMIT как $len(args)+1, и любой сдвиг здесь — либо ошибка запроса, либо
// подстановка не того значения (id вместо категории).
func TestListWherePlaceholdersMatchArgs(t *testing.T) {
	re := regexp.MustCompile(`\$(\d+)`)
	filters := []ListFilter{
		{},
		{AfterID: 5},
		{BeforeID: 5},
		{AfterID: 5, BeforeID: 9},
		{AfterID: 5, BeforeID: 9, HideExpired: true, Categories: []string{CategoryMoney, CategoryUsers}},
		{AfterID: 5, BeforeID: 9, HideExpired: true, Categories: []string{CategoryMoney}, BotActions: true},
		{CustomerID: 3, AfterID: 1, BeforeID: 99, Categories: []string{CategorySupport}, HideExpired: true},
		{CustomerID: 3, Categories: []string{CategoryBot}},
		{Categories: []string{CategoryBot}, BotActions: true},
	}
	for i, f := range filters {
		where, args := listWhere(f)
		seen := map[int]bool{}
		maxN := 0
		for _, m := range re.FindAllStringSubmatch(where, -1) {
			n, _ := strconv.Atoi(m[1])
			seen[n] = true
			if n > maxN {
				maxN = n
			}
		}
		if maxN != len(args) || len(seen) != len(args) {
			t.Errorf("#%d %+v: where %q uses $1..$%d (%d distinct), args=%d", i, f, where, maxN, len(seen), len(args))
		}
	}
}

func TestListWhereMoreCases(t *testing.T) {
	cases := []struct {
		name string
		f    ListFilter
		want string
	}{
		{"load more", ListFilter{BeforeID: 100, HideExpired: true}, "e.id < $1 AND e.type <> $2 AND e.level = 1"},
		{"customer history, category filter keeps bot actions",
			ListFilter{CustomerID: 7, Categories: []string{CategoryMoney}},
			"e.customer_id = $1 AND ((e.level = 1 AND e.type = ANY($2)) OR e.level = 2)"},
		// «bot» — не категория важных: в общей ленте без BotActions нажатия не всплывают.
		{"bot category alone is ignored", ListFilter{Categories: []string{CategoryBot}}, "e.level = 1"},
	}
	for _, c := range cases {
		if got, _ := listWhere(c.f); got != c.want {
			t.Errorf("%s: where = %q, want %q", c.name, got, c.want)
		}
	}
	// Типы категории «bot» не должны попасть в ANY(...) важных.
	_, args := listWhere(ListFilter{Categories: []string{CategoryMoney, CategoryBot}})
	if len(args) != 1 {
		t.Fatalf("args = %v", args)
	}
	for _, typ := range args[0].([]string) {
		if CategoryOf(typ) != CategoryMoney {
			t.Fatalf("type %q leaked into money filter", typ)
		}
	}
}

func TestPayloadJSONBoundaryAndBadValues(t *testing.T) {
	// {"t":""} — 8 байт; строка в 2040 символов даёт ровно maxPayloadBytes.
	exact := Event{Payload: map[string]any{"t": strings.Repeat("x", maxPayloadBytes-8)}}
	if got := exact.payloadJSON(); len(got) != maxPayloadBytes || strings.Contains(string(got), "truncated") {
		t.Fatalf("payload of exactly %d bytes must pass, got %d: %.40s", maxPayloadBytes, len(got), got)
	}
	over := Event{Payload: map[string]any{"t": strings.Repeat("x", maxPayloadBytes-7)}}
	if got := string(over.payloadJSON()); got != `{"truncated":true}` {
		t.Fatalf("payload over limit = %.40s", got)
	}
	// NaN в сумме не сериализуется — строка всё равно пишется, а не роняет пачку.
	nan := Event{Payload: map[string]any{"amount": math.NaN()}}
	if got := string(nan.payloadJSON()); got != `{"truncated":true}` {
		t.Fatalf("NaN payload = %s", got)
	}
}

func TestPreviewRunesAndEdges(t *testing.T) {
	if got := Preview("", 10); got != "" {
		t.Fatalf("empty = %q", got)
	}
	// Обрезка по рунам, а не по байтам: кириллица не должна ломаться посередине символа.
	got := Preview(strings.Repeat("я", 150), 100)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != 101 {
		t.Fatalf("cyrillic preview: %d runes", len([]rune(got)))
	}
}

// --- Writer ---

type flakyInserter struct {
	mu    sync.Mutex
	calls int
	fail  int // столько первых вызовов вернут ошибку
	rows  []Event
	block chan struct{}
}

func (f *flakyInserter) Insert(ctx context.Context, batch []Event) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.fail {
		return errors.New("db down")
	}
	f.rows = append(f.rows, batch...)
	return nil
}

func (f *flakyInserter) snapshot() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, len(f.rows)
}

func withBotActions(t *testing.T, on bool) {
	t.Helper()
	prev := botActionsEnabled
	botActionsEnabled = func() bool { return on }
	t.Cleanup(func() { botActionsEnabled = prev })
}

// Сбой пачки не теряет её: писатель повторяет запись по одной строке и дальше работает.
func TestWriterSurvivesInsertError(t *testing.T) {
	withBotActions(t, false)
	store := &flakyInserter{fail: 1}
	w := newWriter(store)
	w.Start()
	w.Emit(Event{Type: PaymentFailed})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if calls, _ := store.snapshot(); calls >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first batch was never flushed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.Emit(Event{Type: PaymentSubscriptionNew})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.Stop(ctx)
	if _, rows := store.snapshot(); rows != 2 {
		t.Fatalf("failed batch must be retried row by row and writer keep writing, rows=%d", rows)
	}
}

// Stop повторно и Emit после Stop не блокируют и не паникуют.
func TestWriterStopIdempotentAndEmitAfterStop(t *testing.T) {
	withBotActions(t, false)
	w := newWriter(&fakeInserter{})
	w.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.Stop(ctx)
	w.Stop(ctx)
	done := make(chan struct{})
	go func() {
		for i := 0; i < queueSize+10; i++ {
			w.Emit(Event{Type: PaymentFailed})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Emit after Stop blocked")
	}
}

// Stop не ждёт дольше ctx, даже если база повисла.
func TestWriterStopRespectsTimeout(t *testing.T) {
	withBotActions(t, false)
	store := &flakyInserter{block: make(chan struct{})}
	defer close(store.block)
	w := newWriter(store)
	w.Start()
	w.Emit(Event{Type: PaymentFailed})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	w.Stop(ctx)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop waited %v with a hung store", d)
	}
}

// Полная очередь отбрасывает и важные события, но Emit не блокирует.
func TestWriterFullQueueDropsWithoutBlocking(t *testing.T) {
	withBotActions(t, true)
	w := newWriter(&fakeInserter{}) // не запущен
	for i := 0; i < queueSize+10; i++ {
		w.Emit(Event{Type: PaymentFailed})
	}
	if len(w.ch) != queueSize || w.dropped.Load() != 10 {
		t.Fatalf("len=%d dropped=%d", len(w.ch), w.dropped.Load())
	}
	// Нажатие при полной очереди тоже теряется, без паники.
	w.Emit(Event{Type: BotButton, Level: LevelBot})
	if w.dropped.Load() != 11 {
		t.Fatalf("dropped=%d", w.dropped.Load())
	}
}

// --- NodeWatcher ---

type nodeHarness struct {
	t     *testing.T
	w     *Writer
	nw    *NodeWatcher
	list  []NodeStatus
	err   error
	clock time.Time
}

func newNodeHarness(t *testing.T) *nodeHarness {
	t.Helper()
	withBotActions(t, false)
	h := &nodeHarness{t: t, w: newWriter(&fakeInserter{}), clock: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	SetDefault(h.w)
	t.Cleanup(func() { SetDefault(nil) })
	h.nw = NewNodeWatcher(func(context.Context) ([]NodeStatus, error) {
		if h.err != nil {
			return nil, h.err
		}
		return append([]NodeStatus(nil), h.list...), nil
	})
	h.nw.now = func() time.Time { return h.clock }
	return h
}

func (h *nodeHarness) poll(advance time.Duration) {
	h.clock = h.clock.Add(advance)
	h.nw.poll(context.Background())
}

func (h *nodeHarness) events() []Event {
	var out []Event
	for {
		select {
		case e := <-h.w.ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

// Один пропущенный пинг между «в сети» не даёт ни события, ни инцидента.
func TestNodeWatcherFlappingSinglePollsIgnored(t *testing.T) {
	h := newNodeHarness(t)
	h.list = []NodeStatus{{UUID: "a", Name: "NL-1", Online: true}}
	h.poll(0)
	for i := 0; i < 3; i++ {
		h.list[0].Online = false
		h.poll(time.Minute)
		h.list[0].Online = true
		h.poll(time.Minute)
	}
	if ev := h.events(); len(ev) != 0 {
		t.Fatalf("flapping produced events: %+v", ev)
	}
	if s := h.nw.Snapshot(); s.Online != 1 || len(s.Down) != 0 {
		t.Fatalf("snapshot: %+v", s)
	}
}

// Нода, лежащая уже при старте: без «упала» в ленте, но видна как недоступная;
// когда поднимется — «поднялась».
func TestNodeWatcherDownAtStartup(t *testing.T) {
	h := newNodeHarness(t)
	h.list = []NodeStatus{{UUID: "a", Name: "NL-1", Online: false}}
	h.poll(0)
	h.poll(time.Minute)
	h.poll(time.Minute)
	if ev := h.events(); len(ev) != 0 {
		t.Fatalf("node down at startup must not emit node.down: %+v", ev)
	}
	s := h.nw.Snapshot()
	if !s.Ready || s.Online != 0 || len(s.Down) != 1 {
		t.Fatalf("snapshot: %+v", s)
	}
	h.list[0].Online = true
	h.poll(10 * time.Minute)
	ev := h.events()
	if len(ev) != 1 || ev[0].Type != NodeUp {
		t.Fatalf("events after recovery: %+v", ev)
	}
}

// Нода, убранная из панели (или выключенная), пропадает из счётчиков и инцидентов.
func TestNodeWatcherRemovedNodeForgotten(t *testing.T) {
	h := newNodeHarness(t)
	h.list = []NodeStatus{{UUID: "a", Name: "NL-1", Online: true}, {UUID: "b", Name: "DE-1", Online: true}}
	h.poll(0)
	h.list[1].Online = false
	h.poll(time.Minute)
	h.poll(time.Minute)
	if s := h.nw.Snapshot(); len(s.Down) != 1 || s.Total != 2 {
		t.Fatalf("before removal: %+v", s)
	}
	h.list = h.list[:1]
	h.poll(time.Minute)
	if s := h.nw.Snapshot(); len(s.Down) != 0 || s.Total != 1 || s.Online != 1 {
		t.Fatalf("after removal: %+v", s)
	}
}

func TestNodeWatcherPanelDownAndRecovery(t *testing.T) {
	h := newNodeHarness(t)
	h.err = errors.New("timeout")
	h.poll(0)
	first := h.clock
	h.poll(time.Minute)
	s := h.nw.Snapshot()
	if !s.PanelDown || !s.PanelSince.Equal(first) || s.Ready {
		t.Fatalf("panel down snapshot: %+v (since must stay at first failure)", s)
	}
	h.err = nil
	h.list = []NodeStatus{{UUID: "a", Name: "NL-1", Online: true}, {UUID: "", Name: "no uuid", Online: false}}
	h.poll(time.Minute)
	s = h.nw.Snapshot()
	if s.PanelDown || !s.Ready || s.Total != 1 || s.Online != 1 {
		t.Fatalf("after recovery: %+v (node without uuid must be skipped)", s)
	}
}

// Ошибка из-за остановки бота — не «панель недоступна».
func TestNodeWatcherCanceledContextIsNotPanelDown(t *testing.T) {
	h := newNodeHarness(t)
	h.err = context.Canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.nw.poll(ctx)
	if h.nw.Snapshot().PanelDown {
		t.Fatal("shutdown must not raise panel_down")
	}
}

func TestNodeSnapshotNilWatcher(t *testing.T) {
	var w *NodeWatcher
	if s := w.Snapshot(); s.Ready || s.PanelDown || s.Total != 0 {
		t.Fatalf("nil watcher snapshot: %+v", s)
	}
}
