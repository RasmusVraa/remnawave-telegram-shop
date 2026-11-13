// Package cabinetnotify — уведомления в кабинете: лента под колокольчиком и
// push в браузер.
//
// Сюда приходят два потока. Рассылки — через broadcast.Sender, который зовёт
// PublishBroadcast (интерфейс broadcast.CabinetPublisher). Личные события —
// оплата, конец подписки, ответ поддержки — через Notify из тех мест, где бот
// и так пишет человеку в Telegram.
//
// Сервис один на процесс и доступен через Default(): личные события
// рассыпаны по полудюжине пакетов, и протаскивать зависимость через все их
// конструкторы ради одной строки вызова — больше шума, чем пользы. Пока
// SetDefault не вызван, Notify ничего не делает — так живут тесты.
package cabinetnotify

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

// Service кладёт уведомления в ленту и рассылает push.
type Service struct {
	repo *database.CabinetNotificationRepository
	tm   *translation.Manager
	push *pusher
}

func New(pool *pgxpool.Pool, tm *translation.Manager) *Service {
	repo := database.NewCabinetNotificationRepository(pool)
	return &Service{repo: repo, tm: tm, push: newPusher(repo)}
}

// Repo — хранилище для HTTP-обработчиков кабинета.
func (s *Service) Repo() *database.CabinetNotificationRepository { return s.repo }

// VapidPublicKey — публичный ключ для подписки браузера на push.
func (s *Service) VapidPublicKey(ctx context.Context) (string, error) {
	return s.push.publicKey(ctx)
}

var defaultService atomic.Pointer[Service]

// SetDefault делает сервис общим для процесса.
func SetDefault(s *Service) { defaultService.Store(s) }

// Default — общий сервис или nil, если уведомления не подключены.
func Default() *Service { return defaultService.Load() }

// pushWorthy — дублировать ли уведомление push в браузер. В ленту попадает
// всё, а системное уведомление — по настройкам раздела «Уведомления» в
// админке (config.CabinetPushEnabled, там же умолчания). Важное проверяется
// своим переключателем: выключат его — важное пойдёт по правилу своего вида.
func pushWorthy(kind string, important bool) bool {
	if important && config.CabinetPushEnabled(config.CabinetPushImportant) {
		return true
	}
	return config.CabinetPushEnabled(kind)
}

// isTestBroadcast — тестовая рассылка админу: push по своему переключателю
// (по умолчанию включён), чтобы админ видел, как выглядит уведомление.
func isTestBroadcast(audiences []string) bool {
	return len(audiences) == 1 && audiences[0] == database.BroadcastAudienceTestBroadcast
}

// PublishBroadcast кладёт рассылку в ленту всем из её сегментов; push — по
// настройкам (pushWorthy, для тестовой — свой переключатель).
func (s *Service) PublishBroadcast(ctx context.Context, req broadcast.CabinetRequest) (int, error) {
	var broadcastID *int64
	if req.BroadcastID > 0 {
		id := req.BroadcastID
		broadcastID = &id
	}
	body := broadcast.CabinetBody(req.Message)
	if strings.TrimSpace(body) == "" {
		return 0, fmt.Errorf("empty cabinet notification")
	}
	n := database.CabinetNotification{
		Kind:        database.CabinetNotificationBroadcast,
		BroadcastID: broadcastID,
		Important:   req.Important,
		Body:        body,
		Buttons:     toButtons(broadcast.CabinetButtons(req.Message.Buttons)),
	}
	id, recipients, err := s.repo.CreateForAudience(ctx, n, req.Audiences, req.TariffID, req.AdminTelegramID)
	if err != nil {
		return 0, err
	}
	testOnly := isTestBroadcast(req.Audiences)
	worthy := pushWorthy(n.Kind, n.Important)
	if testOnly {
		worthy = config.CabinetPushEnabled(config.CabinetPushTestBroadcast)
	}
	if id > 0 && recipients > 0 && worthy {
		payload := pushPayload{
			Title:     brandTitle(),
			Body:      plainText(body, 180),
			Important: req.Important,
			Tag:       fmt.Sprintf("n%d", id),
		}
		utils.Go("cabinet push broadcast", func() {
			s.push.sendNotification(context.Background(), id, payload)
		})
	}
	return recipients, nil
}

// Event — личное событие для ленты одного человека.
type Event struct {
	Kind      string
	Important bool
	// TitleKey и BodyKey — ключи переводов бота; Args подставляются в тело.
	TitleKey string
	BodyKey  string
	// Title — готовый заголовок с подставленными значениями; если задан, TitleKey не нужен.
	Title string
	Args  []any
	// BodyHTML — готовый текст в Telegram HTML (тот же, что ушёл в бот);
	// если задан, BodyKey и Args не нужны.
	BodyHTML string
	// Buttons — ключи разделов кабинета (tariffs, connections…).
	Buttons []string
	// DedupeKey — ключ события; без него уведомление заводится каждый раз.
	DedupeKey string
}

// Notify кладёт личное событие в ленту человека и шлёт push, если событие того
// стоит (pushWorthy). Ошибки только
// логируются: уведомление в кабинете не должно ломать оплату или крон.
func (s *Service) Notify(ctx context.Context, customerID int64, lang string, ev Event) {
	if s == nil || customerID <= 0 {
		return
	}
	title := strings.TrimSpace(ev.Title)
	if title == "" {
		title = strings.TrimSpace(s.text(lang, ev.TitleKey))
	}
	var body string
	if h := strings.TrimSpace(ev.BodyHTML); h != "" {
		body = h
	} else {
		body = strings.TrimSpace(s.text(lang, ev.BodyKey))
		if len(ev.Args) > 0 && body != "" {
			body = fmt.Sprintf(body, ev.Args...)
		}
		// Переводы — простой текст: экранируем, лента рендерит Telegram HTML.
		body = html.EscapeString(body)
	}
	if title == "" && body == "" {
		slog.Warn("cabinet notify: empty texts", "kind", ev.Kind, "title_key", ev.TitleKey)
		return
	}
	n := database.CabinetNotification{
		Kind:      ev.Kind,
		Important: ev.Important,
		Title:     title,
		Body:      body,
		Buttons:   toButtons(ev.Buttons),
		DedupeKey: ev.DedupeKey,
	}
	id, created, err := s.repo.CreatePersonal(ctx, customerID, n)
	if err != nil {
		slog.Error("cabinet notify: create", "kind", ev.Kind, "error", err)
		return
	}
	if !created || !pushWorthy(ev.Kind, ev.Important) {
		return
	}
	payload := pushPayload{Title: title, Body: plainText(n.Body, 180), Important: ev.Important, Tag: fmt.Sprintf("n%d", id)}
	if payload.Title == "" {
		payload.Title = brandTitle()
	}
	utils.Go("cabinet push personal", func() {
		s.push.sendNotification(context.Background(), id, payload)
	})
}

// Notify — то же через общий сервис; без него ничего не делает.
func Notify(ctx context.Context, customerID int64, lang string, ev Event) {
	Default().Notify(ctx, customerID, lang, ev)
}

func (s *Service) text(lang, key string) string {
	if key == "" || s.tm == nil {
		return ""
	}
	return s.tm.GetText(lang, key)
}

func toButtons(keys []string) []database.CabinetNotificationButton {
	out := make([]database.CabinetNotificationButton, 0, len(keys))
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, database.CabinetNotificationButton{Key: k})
		}
	}
	return out
}

var (
	reBreak = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|blockquote|pre)>`)
	reTag   = regexp.MustCompile(`<[^>]+>`)
	reSpace = regexp.MustCompile(`[ \t]+`)
)

// plainText — текст уведомления без разметки для push: системное уведомление
// тегов не понимает. Длинное обрезается — шторка всё равно покажет пару строк.
func plainText(htmlText string, max int) string {
	s := reBreak.ReplaceAllString(htmlText, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max-1])) + "…"
	}
	return s
}

// Date — дата для текста уведомления: «05.11.2026».
func Date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02.01.2006")
}
