package mail

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type healthEvent struct {
	down   bool
	reason string
}

func newTestHealth() (*health, *[]healthEvent, *time.Time) {
	var events []healthEvent
	clock := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	h := &health{
		notify: func(down bool, reason string) { events = append(events, healthEvent{down, reason}) },
		now:    func() time.Time { return clock },
	}
	return h, &events, &clock
}

func TestHealth_downOnceThenRepeatAfterInterval(t *testing.T) {
	h, events, clock := newTestHealth()
	timeout := errors.New("dial tcp: i/o timeout")

	h.report(timeout)
	*clock = clock.Add(time.Hour)
	h.report(timeout)
	if len(*events) != 1 || !(*events)[0].down || (*events)[0].reason != timeout.Error() {
		t.Fatalf("ждали одно «лежит», получили %+v", *events)
	}

	*clock = clock.Add(alertRepeat)
	h.report(timeout)
	if len(*events) != 2 || !(*events)[1].down {
		t.Fatalf("ждали напоминание через %v, получили %+v", alertRepeat, *events)
	}
}

func TestHealth_recoveryOnlyAfterAnnouncedDown(t *testing.T) {
	h, events, clock := newTestHealth()

	h.report(nil)
	if len(*events) != 0 {
		t.Fatalf("успех без падения не должен оповещать: %+v", *events)
	}

	h.report(errors.New("boom"))
	*clock = clock.Add(time.Minute)
	h.report(nil)
	h.report(nil)
	if len(*events) != 2 || (*events)[1].down {
		t.Fatalf("ждали «лежит» и одно «поднялась», получили %+v", *events)
	}
}

func TestHealth_flappingIsQuiet(t *testing.T) {
	h, events, clock := newTestHealth()
	boom := errors.New("boom")

	h.report(boom) // «лежит»
	*clock = clock.Add(time.Minute)
	h.report(nil) // «поднялась»
	*clock = clock.Add(time.Minute)
	h.report(boom) // моргнула — молчим
	*clock = clock.Add(time.Minute)
	h.report(nil) // о молчаливом падении и «поднялась» не шлём
	*clock = clock.Add(alertQuiet)
	h.report(boom) // пауза прошла — снова «лежит»
	if len(*events) != 3 || !(*events)[0].down || (*events)[1].down || !(*events)[2].down {
		t.Fatalf("ждали лежит/поднялась/лежит, получили %+v", *events)
	}
}

func TestHealth_ignoresCanceledContext(t *testing.T) {
	h, events, _ := newTestHealth()
	h.report(fmt.Errorf("dial: %w", context.Canceled))
	if len(*events) != 0 {
		t.Fatalf("отменённый запрос не должен оповещать: %+v", *events)
	}
}

func TestHealth_noSubscriber(t *testing.T) {
	var h health
	h.report(errors.New("boom")) // не паникует без обработчика
}
