package adminalert

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type sent struct {
	chat   int64
	thread int
	text   string
}

type fakeBot struct {
	sent []sent
	// fail — чаты, куда отправка падает с этой ошибкой.
	fail  map[int64]error
	title string
}

func (f *fakeBot) SendMessage(_ context.Context, p *bot.SendMessageParams) (*models.Message, error) {
	chat := p.ChatID.(int64)
	if err := f.fail[chat]; err != nil {
		return nil, err
	}
	f.sent = append(f.sent, sent{chat: chat, thread: p.MessageThreadID, text: p.Text})
	return &models.Message{}, nil
}

func (f *fakeBot) GetChat(_ context.Context, _ *bot.GetChatParams) (*models.ChatFullInfo, error) {
	return &models.ChatFullInfo{Title: f.title}, nil
}

func setup(t *testing.T, admin int64) *fakeBot {
	t.Helper()
	for _, key := range []string{
		"ADMIN_ALERTS_CHAT_ID",
		"ADMIN_ALERT_SYSTEM_ENABLED", "ADMIN_ALERT_SYSTEM_CHAT_ID", "ADMIN_ALERT_SYSTEM_THREAD_ID",
		"ADMIN_ALERT_SERVERS_ENABLED", "ADMIN_ALERT_SERVERS_CHAT_ID", "ADMIN_ALERT_SERVERS_THREAD_ID",
		"ADMIN_ALERT_INFRA_ENABLED", "ADMIN_ALERT_INFRA_CHAT_ID", "ADMIN_ALERT_INFRA_THREAD_ID",
	} {
		t.Setenv(key, "")
	}
	prevAdmin, prevClient := adminChat, currentClient()
	adminChat = func() int64 { return admin }
	f := &fakeBot{fail: map[int64]error{}}
	setClient(f)
	t.Cleanup(func() {
		adminChat = prevAdmin
		setClient(prevClient)
	})
	return f
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Route
		ok   bool
	}{
		{"nothing set — admin DM", nil, Route{ChatID: 7, Direct: true}, true},
		{"thread without chat is ignored", map[string]string{"ADMIN_ALERT_SYSTEM_THREAD_ID": "12"}, Route{ChatID: 7, Direct: true}, true},
		{"shared chat + kind thread", map[string]string{"ADMIN_ALERTS_CHAT_ID": "-100500", "ADMIN_ALERT_SYSTEM_THREAD_ID": "12"}, Route{ChatID: -100500, ThreadID: 12}, true},
		{"kind chat wins", map[string]string{"ADMIN_ALERTS_CHAT_ID": "-100500", "ADMIN_ALERT_SYSTEM_CHAT_ID": "-100900", "ADMIN_ALERT_SYSTEM_THREAD_ID": "3"}, Route{ChatID: -100900, ThreadID: 3}, true},
		{"disabled", map[string]string{"ADMIN_ALERT_SYSTEM_ENABLED": "false"}, Route{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setup(t, 7)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, ok := Resolve("system")
			if ok != tc.ok || got != tc.want {
				t.Fatalf("got %+v %v, want %+v %v", got, ok, tc.want, tc.ok)
			}
		})
	}
	setup(t, 0)
	if _, ok := Resolve("system"); ok {
		t.Fatal("no admin and no chat — nowhere to send")
	}
	if _, ok := Resolve("nope"); ok {
		t.Fatal("unknown kind")
	}
}

func TestSendFallsBackToAdmin(t *testing.T) {
	f := setup(t, 7)
	t.Setenv("ADMIN_ALERTS_CHAT_ID", "-100500")
	t.Setenv("ADMIN_ALERT_SYSTEM_THREAD_ID", "12")
	f.fail[-100500] = fmt.Errorf("%w, Bad Request: message thread not found", bot.ErrorBadRequest)

	if err := Send(context.Background(), "system", "почта лежит", nil); err != nil {
		t.Fatalf("delivered to DM, want nil error: %v", err)
	}
	if len(f.sent) != 1 || f.sent[0].chat != 7 || f.sent[0].thread != 0 {
		t.Fatalf("sent: %+v", f.sent)
	}
	if !strings.Contains(f.sent[0].text, "-100500, тема 12") || !strings.Contains(f.sent[0].text, "message thread not found") || !strings.HasSuffix(f.sent[0].text, "почта лежит") {
		t.Fatalf("fallback text: %q", f.sent[0].text)
	}
}

func TestSendDirectFailureIsNotDuplicated(t *testing.T) {
	f := setup(t, 7)
	f.fail[7] = fmt.Errorf("%w, Forbidden: bot was blocked by the user", bot.ErrorForbidden)
	if err := Send(context.Background(), "infra", "x", nil); err == nil {
		t.Fatal("error expected")
	}
	if len(f.sent) != 0 {
		t.Fatalf("sent: %+v", f.sent)
	}
}

func TestSendDisabledIsSilent(t *testing.T) {
	f := setup(t, 7)
	t.Setenv("ADMIN_ALERT_SERVERS_ENABLED", "false")
	if err := Send(context.Background(), "servers", "x", nil); err != nil || len(f.sent) != 0 {
		t.Fatalf("err=%v sent=%+v", err, f.sent)
	}
}

func TestTest(t *testing.T) {
	f := setup(t, 7)
	f.title = "Meows Admin"
	t.Setenv("ADMIN_ALERTS_CHAT_ID", "-100500")
	t.Setenv("ADMIN_ALERT_SERVERS_THREAD_ID", "14")
	t.Setenv("ADMIN_ALERT_SERVERS_ENABLED", "false")

	res, err := Test(context.Background(), "servers", nil)
	if err != nil || !res.OK || res.ChatTitle != "Meows Admin" || res.ThreadID != 14 || res.Direct {
		t.Fatalf("res=%+v err=%v", res, err)
	}

	f.fail[-100500] = fmt.Errorf("%w, Bad Request: message thread not found", bot.ErrorBadRequest)
	res, _ = Test(context.Background(), "servers", nil)
	if res.OK || res.Hint != HintNoThread || res.Error != "Bad Request: message thread not found" {
		t.Fatalf("thread error: %+v", res)
	}
	if len(f.sent) != 1 {
		t.Fatalf("test must not fall back to DM: %+v", f.sent)
	}

	f.sent = nil
	res, _ = Test(context.Background(), "servers", &Draft{SharedChat: -100500, Chat: -100777, Thread: 5})
	if !res.OK || res.ChatID != -100777 || res.ThreadID != 5 || len(f.sent) != 1 || f.sent[0].chat != -100777 {
		t.Fatalf("draft route: %+v sent %+v", res, f.sent)
	}
	res, _ = Test(context.Background(), "servers", &Draft{Thread: 5})
	if !res.OK || !res.Direct || f.sent[1].chat != 7 || f.sent[1].thread != 0 {
		t.Fatalf("empty draft goes to DM: %+v sent %+v", res, f.sent)
	}

	if _, err := Test(context.Background(), "nope", nil); err != ErrUnknownKind {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestHint(t *testing.T) {
	cases := map[string]error{
		HintNotMember:   fmt.Errorf("%w, Bad Request: chat not found", bot.ErrorBadRequest),
		HintTopicClosed: fmt.Errorf("%w, Bad Request: TOPIC_CLOSED", bot.ErrorBadRequest),
	}
	for want, err := range cases {
		if got := Hint(err); got != want {
			t.Errorf("%v: got %q want %q", err, got, want)
		}
	}
	if Hint(fmt.Errorf("%w, Forbidden: bot was kicked", bot.ErrorForbidden)) != HintNotMember {
		t.Error("forbidden is not_member")
	}
}
