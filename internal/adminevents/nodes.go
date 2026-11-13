package adminevents

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// NodeStatus — нода панели глазами «Мониторинга».
type NodeStatus struct {
	UUID    string
	Name    string
	Country string
	Online  bool
}

// DownNode — нода, которая сейчас не в сети.
type DownNode struct {
	Name  string    `json:"name"`
	Since time.Time `json:"since"`
}

// NodeSnapshot — состояние нод на последний опрос панели.
type NodeSnapshot struct {
	Ready      bool
	Online     int
	Total      int
	Down       []DownNode
	PanelDown  bool
	PanelSince time.Time
}

// nodeDownStreak — столько опросов подряд нода должна быть не в сети, чтобы
// считаться упавшей. Один пропущенный пинг панели — ещё не сбой.
const nodeDownStreak = 2

const nodePollInterval = time.Minute

type nodeState struct {
	name, country string
	online        bool // подтверждённое состояние
	offlineStreak int
	since         time.Time
}

// NodeWatcher раз в минуту опрашивает панель и пишет «нода упала / поднялась».
// Отключённые в панели ноды сюда не попадают — список готовит вызывающий.
type NodeWatcher struct {
	list func(ctx context.Context) ([]NodeStatus, error)
	now  func() time.Time

	mu         sync.Mutex
	nodes      map[string]*nodeState
	ready      bool
	panelFails int
	panelDown  bool
	panelSince time.Time
}

// NewNodeWatcher создаёт наблюдателя. list возвращает включённые ноды панели.
func NewNodeWatcher(list func(ctx context.Context) ([]NodeStatus, error)) *NodeWatcher {
	return &NodeWatcher{list: list, now: time.Now, nodes: map[string]*nodeState{}}
}

// Run опрашивает панель, пока жив ctx.
func (w *NodeWatcher) Run(ctx context.Context) {
	w.poll(ctx)
	t := time.NewTicker(nodePollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.poll(ctx)
		}
	}
}

func (w *NodeWatcher) poll(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	list, err := w.list(pctx)
	now := w.now()

	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.panelFails++
		if w.panelFails == 1 {
			w.panelSince = now
		}
		if !w.panelDown && w.panelFails >= nodeDownStreak {
			w.panelDown = true
			slog.Warn("admin events: panel unreachable", "error", err)
		}
		return
	}
	w.panelFails = 0
	w.panelDown = false

	seen := make(map[string]bool, len(list))
	for _, n := range list {
		if n.UUID == "" {
			continue
		}
		seen[n.UUID] = true
		st, ok := w.nodes[n.UUID]
		if !ok {
			// Первая встреча — запоминаем как есть, без события: при старте бота
			// уже лежащая нода не должна выглядеть как свежий сбой.
			w.nodes[n.UUID] = &nodeState{name: n.Name, country: n.Country, online: n.Online, since: now}
			continue
		}
		st.name, st.country = n.Name, n.Country
		if n.Online {
			st.offlineStreak = 0
			if !st.online {
				down := now.Sub(st.since)
				st.online, st.since = true, now
				Emit(Event{Type: NodeUp, Source: SourcePanel, Payload: map[string]any{
					"name": n.Name, "country": n.Country, "down_minutes": int(down.Minutes()),
				}})
			}
			continue
		}
		st.offlineStreak++
		if st.online && st.offlineStreak >= nodeDownStreak {
			st.online, st.since = false, now
			Emit(Event{Type: NodeDown, Source: SourcePanel, Payload: map[string]any{
				"name": n.Name, "country": n.Country,
			}})
		}
	}
	for id := range w.nodes {
		if !seen[id] {
			delete(w.nodes, id)
		}
	}
	w.ready = true
}

// Snapshot — состояние на последний опрос.
func (w *NodeWatcher) Snapshot() NodeSnapshot {
	if w == nil {
		return NodeSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s := NodeSnapshot{Ready: w.ready, Total: len(w.nodes), PanelDown: w.panelDown, PanelSince: w.panelSince}
	for _, n := range w.nodes {
		if n.online {
			s.Online++
			continue
		}
		s.Down = append(s.Down, DownNode{Name: n.name, Since: n.since})
	}
	sort.Slice(s.Down, func(i, j int) bool { return s.Down[i].Since.Before(s.Down[j].Since) })
	return s
}

var defaultNodes atomic.Pointer[NodeWatcher]

// SetNodeWatcher делает наблюдателя общим: его читают «Требует внимания» и счётчики.
func SetNodeWatcher(w *NodeWatcher) { defaultNodes.Store(w) }

// Nodes — общий наблюдатель или nil.
func Nodes() *NodeWatcher { return defaultNodes.Load() }
