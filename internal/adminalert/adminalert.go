// Package adminalert — единая точка, через которую бот пишет админу о сбоях
// и событиях: почта не отправляется, сервер недоступен, пора платить за сервер.
//
// Куда уходит сообщение, решают настройки «Оповещения админу» (config.AdminAlertSettings):
// чат вида, иначе общий чат, иначе личка ADMIN_TELEGRAM_ID. Если в группу или тему
// отправить не удалось (бота выгнали, тему удалили), сообщение уходит в личку
// с пометкой: оповещение о поломке не должно теряться так же тихо, как сама поломка.
package adminalert

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

// Route — куда уйдёт сообщение.
type Route struct {
	ChatID   int64
	ThreadID int
	// Direct — личка админу: общий чат и чат вида не заданы.
	Direct bool
}

// messenger — то, что нужно от бота. В тестах подменяется.
type messenger interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	GetChat(ctx context.Context, params *bot.GetChatParams) (*models.ChatFullInfo, error)
}

var (
	mu     sync.RWMutex
	client messenger
)

// ErrNoBot — бот ещё не передан в Init.
var ErrNoBot = errors.New("adminalert: bot is not set")

// Init запоминает бота. Вызывается один раз при старте.
func Init(b *bot.Bot) {
	if b == nil {
		return
	}
	setClient(b)
}

func setClient(c messenger) {
	mu.Lock()
	client = c
	mu.Unlock()
}

func currentClient() messenger {
	mu.RLock()
	defer mu.RUnlock()
	return client
}

// adminChat — личка админа. Когда админов станет несколько, меняется только здесь.
// Переменная — чтобы подменить в тесте.
var adminChat = func() int64 {
	return config.GetAdminTelegramId()
}

// RouteFor — маршрут вида без учёта выключателя. ok=false — вида нет или слать некому.
func RouteFor(kind string) (Route, bool) {
	_, chat, thread, ok := config.AdminAlertSettings(kind)
	if !ok {
		return Route{}, false
	}
	return resolve(config.AdminAlertsChatID(), chat, thread)
}

// resolve: чат вида, иначе общий чат, иначе личка админа.
func resolve(shared, chat int64, thread int) (Route, bool) {
	if chat == 0 {
		chat = shared
	}
	if chat == 0 {
		admin := adminChat()
		if admin == 0 {
			return Route{}, false
		}
		return Route{ChatID: admin, Direct: true}, true
	}
	if thread < 0 {
		thread = 0
	}
	return Route{ChatID: chat, ThreadID: thread}, true
}

// Resolve — маршрут включённого вида. ok=false — вид выключен, неизвестен или слать некому.
func Resolve(kind string) (Route, bool) {
	enabled, _, _, ok := config.AdminAlertSettings(kind)
	if !ok || !enabled {
		return Route{}, false
	}
	return RouteFor(kind)
}

// Send отправляет HTML-сообщение по маршруту вида. Выключенный вид — тихо nil.
// Сбой в группе или теме — повтор в личку с пометкой; ошибка возвращается,
// только если не дошло никуда.
func Send(ctx context.Context, kind, text string, markup models.ReplyMarkup) error {
	r, ok := Resolve(kind)
	if !ok {
		return nil
	}
	c := currentClient()
	if c == nil {
		return ErrNoBot
	}
	err := send(ctx, c, r, text, markup)
	if err == nil || r.Direct {
		return err
	}
	admin := adminChat()
	if admin == 0 {
		return err
	}
	slog.Warn("admin alert: route failed, sending to admin DM", "kind", kind, "chat_id", r.ChatID, "thread_id", r.ThreadID, "error", err)
	note := fmt.Sprintf(fallbackNote(), html.EscapeString(describe(r)), html.EscapeString(Reason(err)))
	if ferr := send(ctx, c, Route{ChatID: admin, Direct: true}, note+"\n\n"+text, markup); ferr != nil {
		return errors.Join(err, ferr)
	}
	return nil
}

func send(ctx context.Context, c messenger, r Route, text string, markup models.ReplyMarkup) error {
	params := &bot.SendMessageParams{
		ChatID:             r.ChatID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	}
	if r.ThreadID > 0 && !r.Direct {
		params.MessageThreadID = r.ThreadID
	}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	_, err := c.SendMessage(ctx, params)
	return err
}

func describe(r Route) string {
	s := strconv.FormatInt(r.ChatID, 10)
	if r.ThreadID > 0 {
		s += ", " + topicWord() + " " + strconv.Itoa(r.ThreadID)
	}
	return s
}

// Reason — описание ошибки от Telegram без префикса библиотеки.
func Reason(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.Index(msg, ", "); i >= 0 && (errors.Is(err, bot.ErrorBadRequest) || errors.Is(err, bot.ErrorForbidden)) {
		msg = msg[i+2:]
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// Подсказки к ошибкам теста: фронт переводит их сам.
const (
	HintNotMember   = "not_member"
	HintNoThread    = "no_thread"
	HintTopicClosed = "topic_closed"
)

// Hint — понятная причина ошибки Telegram, пусто если причина не узнана.
func Hint(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "message thread not found"):
		return HintNoThread
	case strings.Contains(msg, "topic_closed"):
		return HintTopicClosed
	case errors.Is(err, bot.ErrorForbidden),
		strings.Contains(msg, "chat not found"),
		strings.Contains(msg, "not enough rights"),
		strings.Contains(msg, "bot is not a member"),
		strings.Contains(msg, "bot was kicked"):
		return HintNotMember
	}
	return ""
}

// TestResult — ответ кнопки «Тест».
type TestResult struct {
	OK        bool   `json:"ok"`
	Direct    bool   `json:"direct"`
	ChatID    int64  `json:"chat_id,omitempty"`
	ThreadID  int    `json:"thread_id,omitempty"`
	ChatTitle string `json:"chat_title,omitempty"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// ErrUnknownKind — такого вида нет.
var ErrUnknownKind = errors.New("adminalert: unknown kind")

// Draft — ещё не сохранённые чат и тема из формы: «Тест» проверяет то, что
// админ ввёл, а не то, что лежит в настройках.
type Draft struct {
	SharedChat int64
	Chat       int64
	Thread     int
}

// Test шлёт пробное сообщение по маршруту вида — даже если вид выключен, чтобы
// проверить маршрут до включения. draft == nil — маршрут из настроек.
// Запасного пути нет: админ должен увидеть ошибку.
func Test(ctx context.Context, kind string, draft *Draft) (TestResult, error) {
	if _, _, _, ok := config.AdminAlertSettings(kind); !ok {
		return TestResult{}, ErrUnknownKind
	}
	r, ok := RouteFor(kind)
	if draft != nil {
		r, ok = resolve(draft.SharedChat, draft.Chat, draft.Thread)
	}
	if !ok {
		return TestResult{Error: "ADMIN_TELEGRAM_ID is not set"}, nil
	}
	c := currentClient()
	if c == nil {
		return TestResult{}, ErrNoBot
	}
	res := TestResult{Direct: r.Direct, ChatID: r.ChatID, ThreadID: r.ThreadID}
	if err := send(ctx, c, r, fmt.Sprintf(testText(), html.EscapeString(kindLabel(kind))), nil); err != nil {
		res.Error = Reason(err)
		res.Hint = Hint(err)
		return res, nil
	}
	res.OK = true
	if !r.Direct {
		if chat, err := c.GetChat(ctx, &bot.GetChatParams{ChatID: r.ChatID}); err == nil && chat != nil {
			res.ChatTitle = chat.Title
		}
	}
	return res, nil
}

func english() bool {
	return strings.HasPrefix(strings.ToLower(config.DefaultLanguage()), "en")
}

func fallbackNote() string {
	if english() {
		return "⚠️ <b>Could not deliver to %s:</b> %s"
	}
	return "⚠️ <b>Не дошло в %s:</b> %s"
}

func topicWord() string {
	if english() {
		return "topic"
	}
	return "тема"
}

func testText() string {
	if english() {
		return "🔔 Test alert: %s\n\nAlerts of this kind will arrive here."
	}
	return "🔔 Тестовое оповещение: %s\n\nОповещения этого вида будут приходить сюда."
}

func kindLabel(kind string) string {
	en := english()
	switch kind {
	case config.AdminAlertSystem:
		if en {
			return "System (cabinet mail)"
		}
		return "Система (почта кабинета)"
	case config.AdminAlertServers:
		if en {
			return "Server status"
		}
		return "Статус серверов"
	case config.AdminAlertInfra:
		if en {
			return "Server billing"
		}
		return "Оплата серверов"
	case config.AdminAlertPayments:
		if en {
			return "Client payments"
		}
		return "Оплаты клиентов"
	case config.AdminAlertPartner:
		if en {
			return "Partner program"
		}
		return "Партнёрка"
	}
	return kind
}
