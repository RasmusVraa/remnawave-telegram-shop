package adminevents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNodeWatcherTransitions(t *testing.T) {
	prev := botActionsEnabled
	botActionsEnabled = func() bool { return false }
	t.Cleanup(func() { botActionsEnabled = prev; SetDefault(nil) })

	store := &fakeInserter{}
	w := newWriter(store)
	SetDefault(w)

	online := true
	var listErr error
	clock := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	nw := NewNodeWatcher(func(context.Context) ([]NodeStatus, error) {
		if listErr != nil {
			return nil, listErr
		}
		return []NodeStatus{
			{UUID: "a", Name: "NL-1", Online: online},
			{UUID: "b", Name: "DE-1", Online: true},
		}, nil
	})
	nw.now = func() time.Time { return clock }
	ctx := context.Background()

	nw.poll(ctx) // базовая линия — без событий
	online = false
	nw.poll(ctx) // один пропуск — ещё не сбой
	if s := nw.Snapshot(); s.Online != 2 {
		t.Fatalf("single offline poll must not count, online=%d", s.Online)
	}
	clock = clock.Add(time.Minute)
	nw.poll(ctx) // второй подряд — упала
	s := nw.Snapshot()
	if s.Online != 1 || len(s.Down) != 1 || s.Down[0].Name != "NL-1" {
		t.Fatalf("snapshot after down: %+v", s)
	}
	clock = clock.Add(5 * time.Minute)
	online = true
	nw.poll(ctx) // поднялась

	listErr = errors.New("panel down")
	nw.poll(ctx)
	if nw.Snapshot().PanelDown {
		t.Fatal("single failed poll must not raise panel_down")
	}
	nw.poll(ctx)
	if !nw.Snapshot().PanelDown {
		t.Fatal("two failed polls in a row must raise panel_down")
	}

	if len(w.ch) != 2 {
		t.Fatalf("queued %d events, want down+up", len(w.ch))
	}
	if e := <-w.ch; e.Type != NodeDown {
		t.Fatalf("first event %s", e.Type)
	}
	if e := <-w.ch; e.Type != NodeUp || e.Payload["down_minutes"] != 5 {
		t.Fatalf("second event %+v", e)
	}
}
