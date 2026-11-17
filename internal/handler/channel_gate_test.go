package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/channelgate"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
)

// fakeChannelAPI отвечает на getChatMember по таблице статусов.
type fakeChannelAPI struct{ status models.ChatMemberType }

func (f fakeChannelAPI) GetMe(context.Context) (*models.User, error) { return &models.User{ID: 1}, nil }
func (f fakeChannelAPI) GetChat(context.Context, *bot.GetChatParams) (*models.ChatFullInfo, error) {
	return &models.ChatFullInfo{Type: models.ChatTypeChannel, Username: "meows_news"}, nil
}
func (f fakeChannelAPI) GetChatMember(context.Context, *bot.GetChatMemberParams) (*models.ChatMember, error) {
	return &models.ChatMember{Type: f.status}, nil
}
func (f fakeChannelAPI) GetChatMemberCount(context.Context, *bot.GetChatMemberCountParams) (int, error) {
	return 1, nil
}

// telegramStub — поддельный Bot API: запоминает вызовы методов.
type telegramStub struct {
	mu    sync.Mutex
	calls []stubCall
}

type stubCall struct {
	Method string
	Body   map[string]any
}

func (s *telegramStub) handler(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(1 << 20)
		if r.MultipartForm != nil {
			for k, v := range r.MultipartForm.Value {
				body[k] = v[0]
			}
		}
	} else {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
	}
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	s.mu.Lock()
	s.calls = append(s.calls, stubCall{Method: method, Body: body})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch method {
	case "sendMessage", "editMessageText":
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
	default:
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}
}

func (s *telegramStub) find(method string) *stubCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.calls {
		if s.calls[i].Method == method {
			return &s.calls[i]
		}
	}
	return nil
}

func newGateTestEnv(t *testing.T, status models.ChatMemberType, active bool) (Handler, *bot.Bot, *telegramStub) {
	t.Helper()
	stub := &telegramStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	t.Cleanup(srv.Close)
	b, err := bot.New("123:test", bot.WithServerURL(srv.URL), bot.WithSkipGetMe(), bot.WithHTTPClient(0, srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	tm := translation.GetInstance()
	if err := tm.InitTranslations("../../translations", "ru"); err != nil {
		t.Fatal(err)
	}
	patch := map[string]string{"CHANNEL_GATE_CHANNEL": "@meows_news", "CHANNEL_GATE_ENABLED": "false"}
	if active {
		patch["CHANNEL_GATE_ENABLED"] = "true"
	}
	if _, err := config.ApplyRuntimePatch(patch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	})
	h := Handler{translation: tm}
	h.SetChannelGate(channelgate.New(fakeChannelAPI{status: status}, nil))
	return h, b, stub
}

func runMiddleware(h Handler, b *bot.Bot, update *models.Update) (nextCalled bool) {
	h.RequireChannelSubscriptionMiddleware(func(context.Context, *bot.Bot, *models.Update) { nextCalled = true })(context.Background(), b, update)
	return nextCalled
}

func messageUpdate(userID int64) *models.Update {
	return &models.Update{Message: &models.Message{
		Chat: models.Chat{ID: userID, Type: models.ChatTypePrivate},
		From: &models.User{ID: userID, LanguageCode: "ru"},
		Text: "/connect",
	}}
}

func callbackUpdate(userID int64, data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb1", Data: data, From: models.User{ID: userID, LanguageCode: "ru"}}}
}

func TestChannelGateMiddleware_Disabled_PassesThrough(t *testing.T) {
	h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, false)
	if !runMiddleware(h, b, messageUpdate(77)) {
		t.Fatal("выключенная подписка не должна останавливать запрос")
	}
	if len(stub.calls) != 0 {
		t.Fatalf("при выключенной функции бот ничего не отправляет, вызовов: %d", len(stub.calls))
	}
}

func TestChannelGateMiddleware_NotSubscribed_ShowsScreen(t *testing.T) {
	h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
	if runMiddleware(h, b, messageUpdate(77)) {
		t.Fatal("неподписанный не должен доходить до обработчика")
	}
	call := stub.find("sendMessage")
	if call == nil {
		t.Fatal("ожидали экран подписки (sendMessage)")
	}
	text, _ := call.Body["text"].(string)
	if !strings.Contains(text, "Почти готово") {
		t.Fatalf("текст экрана: %q", text)
	}
	markup, _ := call.Body["reply_markup"].(string)
	for _, want := range []string{`"url":"https://t.me/meows_news"`, `"callback_data":"channel_gate_check"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("в кнопках нет %s: %s", want, markup)
		}
	}
}

func TestChannelGateMiddleware_Subscribed_PassesThrough(t *testing.T) {
	h, b, stub := newGateTestEnv(t, models.ChatMemberTypeMember, true)
	if !runMiddleware(h, b, messageUpdate(77)) {
		t.Fatal("подписанный должен проходить")
	}
	if stub.find("sendMessage") != nil {
		t.Fatal("подписанному экран не показывают")
	}
}

func TestChannelGateMiddleware_CheckAndLegalCallbacksPass(t *testing.T) {
	h, b, _ := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
	for _, data := range []string{CallbackChannelGateCheck, CallbackLegalAccept, CallbackLegalDecline} {
		if !runMiddleware(h, b, callbackUpdate(77, data)) {
			t.Errorf("callback %q должен проходить мимо экрана подписки", data)
		}
	}
}

func TestChannelGateMiddleware_PreCheckoutNeverBlocked(t *testing.T) {
	h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
	upd := &models.Update{PreCheckoutQuery: &models.PreCheckoutQuery{ID: "pq", From: &models.User{ID: 77}}}
	if !runMiddleware(h, b, upd) {
		t.Fatal("оплата уже выставленного счёта Stars не должна блокироваться")
	}
	if len(stub.calls) != 0 {
		t.Fatal("для PreCheckoutQuery бот не должен ничего отправлять")
	}
}

func TestChannelGateMiddleware_BlockedCallbackIsAnswered(t *testing.T) {
	h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
	if runMiddleware(h, b, callbackUpdate(77, CallbackBuy)) {
		t.Fatal("кнопка «Купить» без подписки должна остановиться")
	}
	if stub.find("answerCallbackQuery") == nil {
		t.Fatal("на callback нужно ответить, иначе у кнопки крутится часик")
	}
	if stub.find("sendMessage") == nil {
		t.Fatal("после ответа нужен экран подписки")
	}
}

func TestChannelGateMiddleware_TelegramFailureLetsThrough(t *testing.T) {
	h, b, _ := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
	h.SetChannelGate(channelgate.New(brokenChannelAPI{}, nil))
	if !runMiddleware(h, b, messageUpdate(77)) {
		t.Fatal("сбой проверки не должен запирать пользователя")
	}
}

type brokenChannelAPI struct{ fakeChannelAPI }

func (brokenChannelAPI) GetChatMember(context.Context, *bot.GetChatMemberParams) (*models.ChatMember, error) {
	return nil, io.ErrUnexpectedEOF
}

// withLegalDocs включает экран согласия: он действует, когда заданы политика и оферта.
func withLegalDocs(t *testing.T) {
	t.Helper()
	if _, err := config.ApplyRuntimePatch(map[string]string{
		"PRIVACY_POLICY_URL": "https://example.com/privacy", "PUBLIC_OFFER_URL": "https://example.com/offer",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = config.ApplyRuntimePatch(map[string]string{"PRIVACY_POLICY_URL": "", "PUBLIC_OFFER_URL": ""})
	})
}

func sentText(stub *telegramStub) string {
	c := stub.find("sendMessage")
	if c == nil {
		return ""
	}
	s, _ := c.Body["text"].(string)
	return s
}

// Порядок экранов перед меню: согласие → подписка → меню.
func TestGateBeforeMenu_Order(t *testing.T) {
	now := time.Now()
	accepted := &database.Customer{LegalAcceptedAt: &now}
	update := messageUpdate(77)

	t.Run("нужно согласие — показываем его, а не канал", func(t *testing.T) {
		withLegalDocs(t)
		h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
		if !h.gateBeforeMenu(context.Background(), b, update, &database.Customer{}, 77, "ru") {
			t.Fatal("меню рисовать нельзя: нужно согласие")
		}
		txt := sentText(stub)
		if !strings.Contains(txt, "принять документы") || strings.Contains(txt, "Почти готово") {
			t.Fatalf("ожидали экран согласия, получили %q", txt)
		}
	})
	t.Run("согласие есть, не подписан — экран канала", func(t *testing.T) {
		withLegalDocs(t)
		h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
		if !h.gateBeforeMenu(context.Background(), b, update, accepted, 77, "ru") {
			t.Fatal("неподписанного нужно остановить")
		}
		if !strings.Contains(sentText(stub), "Почти готово") {
			t.Fatalf("ожидали экран подписки, получили %q", sentText(stub))
		}
	})
	t.Run("согласие есть и подписан — меню", func(t *testing.T) {
		withLegalDocs(t)
		h, b, stub := newGateTestEnv(t, models.ChatMemberTypeMember, true)
		if h.gateBeforeMenu(context.Background(), b, update, accepted, 77, "ru") {
			t.Fatal("подписанному экраны не нужны")
		}
		if len(stub.calls) != 0 {
			t.Fatal("ничего отправлять не нужно")
		}
	})
	t.Run("согласие не нужно, подписка выключена — меню", func(t *testing.T) {
		h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, false)
		if h.gateBeforeMenu(context.Background(), b, update, &database.Customer{}, 77, "ru") {
			t.Fatal("без обязательных экранов сразу меню")
		}
		if len(stub.calls) != 0 {
			t.Fatal("ничего отправлять не нужно")
		}
	})
	t.Run("после «Принимаю» неподписанный попадает на канал: callback-вход", func(t *testing.T) {
		h, b, stub := newGateTestEnv(t, models.ChatMemberTypeLeft, true)
		if !h.gateBeforeMenu(context.Background(), b, callbackUpdate(77, CallbackStart), accepted, 77, "ru") {
			t.Fatal("нужен экран подписки")
		}
		if stub.find("sendMessage") == nil && stub.find("editMessageText") == nil {
			t.Fatal("экран не показан")
		}
	})
}
