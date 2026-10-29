package tgclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type step struct {
	status int
	body   string
}

// fakeDoer отвечает по сценарию и запоминает тела запросов.
type fakeDoer struct {
	mu     sync.Mutex
	steps  []step
	bodies []string
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, string(b))
	s := step{http.StatusOK, `{"ok":true,"result":true}`}
	if len(f.steps) > 0 {
		s, f.steps = f.steps[0], f.steps[1:]
	}
	return &http.Response{StatusCode: s.status, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}

func newReq(t *testing.T, method, body string) *http.Request {
	t.Helper()
	// Тело — поток, как у go-telegram/bot (io.Pipe): повторно не читается.
	pr, pw := io.Pipe()
	go func() { _, _ = pw.Write([]byte(body)); _ = pw.Close() }()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.telegram.org/bot1:x/"+method, pr)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func doClose(t *testing.T, c *Client, method string) {
	t.Helper()
	resp, err := c.Do(newReq(t, method, "x"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

const tooMany = `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 2","parameters":{"retry_after":2}}`

// 429 с retry_after: ждём столько, сколько сказал Telegram, и повторяем с тем же телом.
func TestDo_retriesAfter429WithSameBody(t *testing.T) {
	base := &fakeDoer{steps: []step{{http.StatusTooManyRequests, tooMany}}}
	c := New(base, 1000)
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }

	resp, err := c.Do(newReq(t, "sendMessage", "chat_id=1&text=hi"))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("resp=%v err=%v, want 200", resp, err)
	}
	if len(base.bodies) != 2 || base.bodies[0] != base.bodies[1] || base.bodies[1] != "chat_id=1&text=hi" {
		t.Fatalf("bodies = %q, want the same body twice", base.bodies)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Fatalf("slept %v, want [2s]", slept)
	}
}

// Повторы ограничены: после maxRetries вызывающий получает исходный 429
// с телом, которое библиотека разберёт в TooManyRequestsError.
func TestDo_givesUpAfterMaxRetries(t *testing.T) {
	var steps []step
	for range maxRetries + 1 {
		steps = append(steps, step{http.StatusTooManyRequests, tooMany})
	}
	base := &fakeDoer{steps: steps}
	c := New(base, 1000)
	c.sleep = func(context.Context, time.Duration) error { return nil }

	resp, err := c.Do(newReq(t, "sendMessage", "x"))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("resp=%v err=%v, want 429", resp, err)
	}
	if len(base.bodies) != maxRetries+1 {
		t.Fatalf("attempts = %d, want %d", len(base.bodies), maxRetries+1)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != tooMany {
		t.Fatalf("body = %q, want original 429 body", b)
	}
}

// Слишком долгий retry_after не ждём — возвращаем 429 сразу.
func TestDo_doesNotWaitTooLong(t *testing.T) {
	long := `{"ok":false,"error_code":429,"parameters":{"retry_after":3600}}`
	base := &fakeDoer{steps: []step{{http.StatusTooManyRequests, long}}}
	c := New(base, 1000)
	c.sleep = func(context.Context, time.Duration) error { t.Fatal("must not sleep"); return nil }

	resp, err := c.Do(newReq(t, "sendMessage", "x"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || len(base.bodies) != 1 {
		t.Fatalf("status=%d attempts=%d, want 429 after one attempt", resp.StatusCode, len(base.bodies))
	}
}

// Отправки делят один лимит, а ответы на кнопки им не тормозятся.
func TestDo_rateLimitsOnlySends(t *testing.T) {
	c := New(&fakeDoer{}, 5)
	// Опустошаем запас: burst = 5.
	for range 5 {
		doClose(t, c, "sendMessage")
	}
	start := time.Now()
	doClose(t, c, "answerCallbackQuery")
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("answerCallbackQuery waited %v for the send limit", d)
	}
	start = time.Now()
	doClose(t, c, "sendPhoto")
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Fatalf("sixth send went after %v, want ~200ms wait at 5/s", d)
	}
}
