package adminevents

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPreview(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"привет", 10, "привет"},
		{"  много \n\n пробелов\tтут ", 50, "много пробелов тут"},
		{"не подключается на айфоне", 15, "не подключается…"},
	}
	for _, c := range cases {
		if got := Preview(c.in, c.n); got != c.want {
			t.Errorf("Preview(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestPayloadJSONCapped(t *testing.T) {
	e := Event{Payload: map[string]any{"text": strings.Repeat("x", 5000)}}
	if got := string(e.payloadJSON()); got != `{"truncated":true}` {
		t.Fatalf("big payload = %s", got)
	}
	if got := string((Event{}).payloadJSON()); got != "{}" {
		t.Fatalf("empty payload = %s", got)
	}
}

func TestEveryCategoryHasTypes(t *testing.T) {
	for _, c := range append(append([]string{}, Categories...), CategoryBot) {
		if len(TypesOf(c)) == 0 {
			t.Errorf("category %q has no types", c)
		}
	}
	if CategoryOf("something.new") != CategoryAdmin {
		t.Fatal("unknown type must fall back to admin category")
	}
	for _, typ := range PaidTypes {
		if CategoryOf(typ) != CategoryMoney {
			t.Errorf("paid type %q is not in money", typ)
		}
	}
}

func TestListWhere(t *testing.T) {
	cases := []struct {
		name string
		f    ListFilter
		want string
		args int
	}{
		{"all important", ListFilter{}, "e.level = 1", 0},
		{"with bot actions", ListFilter{BotActions: true}, "(e.level = 1 OR e.level = 2)", 0},
		{"category", ListFilter{Categories: []string{CategoryMoney}}, "(e.level = 1 AND e.type = ANY($1))", 1},
		{"category and bot", ListFilter{Categories: []string{CategoryMoney}, BotActions: true},
			"((e.level = 1 AND e.type = ANY($1)) OR e.level = 2)", 1},
		{"poll", ListFilter{AfterID: 10, HideExpired: true}, "e.id > $1 AND e.type <> $2 AND e.level = 1", 2},
		{"customer history shows all levels", ListFilter{CustomerID: 7, HideExpired: true}, "e.customer_id = $1", 1},
	}
	for _, c := range cases {
		got, args := listWhere(c.f)
		if got != c.want || len(args) != c.args {
			t.Errorf("%s: where = %q (%d args), want %q (%d args)", c.name, got, len(args), c.want, c.args)
		}
	}
}

type fakeInserter struct {
	mu   sync.Mutex
	rows []Event
}

func (f *fakeInserter) Insert(_ context.Context, batch []Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, batch...)
	return nil
}

func (f *fakeInserter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

func TestWriterFlushesOnStop(t *testing.T) {
	prev := botActionsEnabled
	botActionsEnabled = func() bool { return false }
	t.Cleanup(func() { botActionsEnabled = prev })

	store := &fakeInserter{}
	w := newWriter(store)
	w.Start()
	for i := 0; i < 250; i++ {
		w.Emit(Event{Type: PaymentSubscriptionNew, CustomerID: int64(i + 1)})
	}
	w.Emit(Event{Type: BotButton, Level: LevelBot}) // выключено — не пишется
	w.Emit(Event{})                                 // без типа — не пишется
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.Stop(ctx)
	if got := store.count(); got != 250 {
		t.Fatalf("written %d events, want 250", got)
	}
	if store.rows[0].Source != SourceSystem || store.rows[0].Level != LevelImportant || store.rows[0].At.IsZero() {
		t.Fatalf("defaults not applied: %+v", store.rows[0])
	}
}

func TestWriterShedsBotActionsFirst(t *testing.T) {
	prev := botActionsEnabled
	botActionsEnabled = func() bool { return true }
	t.Cleanup(func() { botActionsEnabled = prev })

	w := newWriter(&fakeInserter{}) // не запущен: очередь только копится
	for i := 0; i < botShedLevel; i++ {
		w.Emit(Event{Type: PaymentFailed})
	}
	w.Emit(Event{Type: BotButton, Level: LevelBot})
	if len(w.ch) != botShedLevel {
		t.Fatalf("bot action must be dropped at half-full queue, len=%d", len(w.ch))
	}
	w.Emit(Event{Type: PaymentFailed})
	if len(w.ch) != botShedLevel+1 {
		t.Fatalf("important event must still fit, len=%d", len(w.ch))
	}
	if w.dropped.Load() != 1 {
		t.Fatalf("dropped = %d, want 1", w.dropped.Load())
	}
}

func TestSourceFrom(t *testing.T) {
	ctx := context.Background()
	if SourceFrom(ctx, SourceBot) != SourceBot {
		t.Fatal("default source")
	}
	if SourceFrom(WithSource(ctx, SourceCabinet), SourceBot) != SourceCabinet {
		t.Fatal("source from context")
	}
}
