package adminevents

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"remnawave-tg-shop-bot/utils"
)

const (
	queueSize     = 4096
	batchSize     = 200
	flushInterval = time.Second
	// botShedLevel — с какого заполнения очереди нажатия в боте отбрасываются
	// первыми: важным событиям нужнее место в пиковую минуту рассылки.
	botShedLevel = queueSize / 2
)

// inserter — то, что писатель делает с пачкой. В тестах подменяется.
type inserter interface {
	Insert(ctx context.Context, batch []Event) error
}

// Writer — очередь событий и фоновая запись пачками.
type Writer struct {
	store    inserter
	ch       chan Event
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
	dropped  atomic.Int64
	lastWarn atomic.Int64
}

// NewWriter создаёт писателя. Запись начинается после Start.
func NewWriter(store *Store) *Writer {
	return newWriter(store)
}

func newWriter(store inserter) *Writer {
	return &Writer{
		store: store,
		ch:    make(chan Event, queueSize),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Emit кладёт событие в очередь. Никогда не блокирует.
func (w *Writer) Emit(e Event) {
	if e.Type == "" {
		return
	}
	if e.Level == 0 {
		e.Level = LevelImportant
	}
	if e.Level == LevelBot && !botActionsEnabled() {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if e.Source == "" {
		e.Source = SourceSystem
	}
	if e.Level == LevelBot && len(w.ch) >= botShedLevel {
		w.drop(e)
		return
	}
	select {
	case w.ch <- e:
	default:
		w.drop(e)
	}
}

// drop считает потерянное и пишет в лог не чаще раза в минуту.
func (w *Writer) drop(e Event) {
	n := w.dropped.Add(1)
	now := time.Now().Unix()
	last := w.lastWarn.Load()
	if now-last < 60 || !w.lastWarn.CompareAndSwap(last, now) {
		return
	}
	slog.Warn("admin events: queue full, events dropped", "dropped_total", n, "last_type", e.Type)
}

// Start запускает фоновую запись.
func (w *Writer) Start() {
	utils.Go("admin-events-writer", w.loop)
}

// Stop дописывает то, что осталось в очереди, и останавливает запись.
// Ждёт не дольше, чем живёт ctx.
func (w *Writer) Stop(ctx context.Context) {
	w.stopOnce.Do(func() { close(w.stop) })
	select {
	case <-w.done:
	case <-ctx.Done():
		slog.Warn("admin events: stop timeout, queue not flushed", "queued", len(w.ch))
	}
}

func (w *Writer) loop() {
	defer close(w.done)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	batch := make([]Event, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := w.store.Insert(ctx, batch); err != nil {
			slog.Error("admin events: insert batch", "error", err, "size", len(batch))
			// Одна плохая строка не должна утянуть остальные: пишем по одной.
			if ctx.Err() == nil {
				for i := range batch {
					if err := w.store.Insert(ctx, batch[i:i+1]); err != nil {
						slog.Error("admin events: insert event", "error", err, "type", batch[i].Type)
					}
				}
			}
		}
		cancel()
		batch = batch[:0]
	}
	for {
		select {
		case e := <-w.ch:
			batch = append(batch, e)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.stop:
			for {
				select {
				case e := <-w.ch:
					batch = append(batch, e)
					if len(batch) >= batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}
