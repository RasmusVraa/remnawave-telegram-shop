// Package channelgate — обязательная подписка на Telegram-канал.
//
// Сервис отвечает на один вопрос: подписан ли пользователь на канал,
// который задал админ. Он ничего не блокирует сам — решение «показать экран
// подписки» принимают бот (internal/handler) и кабинет (internal/cabinet/http).
//
// Принципы:
//   - сбой Telegram никогда не запирает пользователя: любая ошибка даёт Unknown,
//     а вызывающий код трактует Unknown как «пропустить»;
//   - ответ «подписан» кэшируется на 2 минуты, «не подписан» — на 30 секунд:
//     кабинет при загрузке шлёт около десятка запросов разом, и каждый из них
//     иначе ушёл бы в Telegram;
//   - одновременные проверки одного пользователя склеиваются в один запрос.
package channelgate

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/sync/singleflight"

	"remnawave-tg-shop-bot/internal/config"
)

// State — результат проверки подписки.
type State int

const (
	// Unknown — проверить не удалось (сбой Telegram, бот не в канале и т.п.). Пропускаем.
	Unknown State = iota
	Subscribed
	NotSubscribed
)

// ErrTooFast — кнопку «Да, я подписался» нажали чаще, чем раз в 3 секунды.
var ErrTooFast = errors.New("channelgate: check requested too often")

const (
	subscribedTTL       = 2 * time.Minute
	notSubscribedTTL    = 30 * time.Second
	minCheckInterval    = 3 * time.Second
	apiTimeout          = 4 * time.Second
	failurePause        = 30 * time.Second
	failuresBeforeAlert = 3
	alertRepeat         = 6 * time.Hour
	cachePruneAbove     = 10000
)

// Коды ошибок проверки канала в админке (поле error ответа).
const (
	ProbeBadFormat   = "bad_format"
	ProbeNotFound    = "not_found"
	ProbePrivate     = "private"
	ProbeBotNotAdmin = "bot_not_admin"
	ProbeUnavailable = "unavailable"
)

// API — часть Bot API, которая нужна сервису. *bot.Bot её реализует.
type API interface {
	GetMe(ctx context.Context) (*models.User, error)
	GetChat(ctx context.Context, params *bot.GetChatParams) (*models.ChatFullInfo, error)
	GetChatMember(ctx context.Context, params *bot.GetChatMemberParams) (*models.ChatMember, error)
	GetChatMemberCount(ctx context.Context, params *bot.GetChatMemberCountParams) (int, error)
}

type cacheEntry struct {
	state   State
	channel string
	until   time.Time
}

// Service — проверка подписки с кэшем.
type Service struct {
	api      API
	now      func() time.Time
	onHealth func(down bool, reason string)
	// onSubscribed (необязателен) зовётся, когда Telegram только что подтвердил подписку.
	onSubscribed func(telegramID int64)
	// exempt — кого не проверяем; по умолчанию админы бота.
	exempt func(telegramID int64) bool

	sf singleflight.Group

	mu          sync.Mutex
	cache       map[int64]cacheEntry
	lastNow     map[int64]time.Time
	pausedUntil time.Time
	botID       int64

	failures  int
	alerted   bool
	alertedAt time.Time
}

// New создаёт сервис. onHealth (может быть nil) зовётся, когда проверка
// сломалась (down=true, повторно не чаще раза в 6 часов) и когда заработала
// снова (down=false). Он должен быть быстрым или сам уходить в горутину.
func New(api API, onHealth func(down bool, reason string)) *Service {
	return &Service{
		api:      api,
		now:      time.Now,
		onHealth: onHealth,
		cache:    make(map[int64]cacheEntry),
		lastNow:  make(map[int64]time.Time),
		exempt:   config.IsAdminTelegramID,
	}
}

// OnSubscribed задаёт обработчик «Telegram подтвердил подписку» — он нужен статистике:
// человек мог подписаться и вернуться не кнопкой «Да, я подписался», а повторным /start
// или обновлением страницы. Вызывается синхронно, обработчик должен быть быстрым.
func (s *Service) OnSubscribed(fn func(telegramID int64)) { s.onSubscribed = fn }

// SetExemptFunc заменяет правило «кого не проверяем» (по умолчанию — админы бота). Для тестов.
func (s *Service) SetExemptFunc(fn func(telegramID int64) bool) { s.exempt = fn }

// Active — требование действует: включено и канал задан.
func (s *Service) Active() bool {
	return s != nil && config.ChannelGateActive()
}

// Exempt — пользователя не проверяем: админ бота.
func (s *Service) Exempt(telegramID int64) bool {
	return s.exempt != nil && s.exempt(telegramID)
}

// Blocks — true, если пользователя нужно остановить на экране подписки.
// При выключенной функции, админе и любом сбое проверки — false.
func (s *Service) Blocks(ctx context.Context, telegramID int64) bool {
	if !s.Active() || telegramID <= 0 || s.Exempt(telegramID) {
		return false
	}
	return s.Check(ctx, telegramID) == NotSubscribed
}

// Check возвращает состояние подписки, при возможности из кэша.
func (s *Service) Check(ctx context.Context, telegramID int64) State {
	if !s.Active() || telegramID <= 0 {
		return Subscribed
	}
	channel := config.ChannelGateChannel()
	if st, ok := s.cached(telegramID, channel); ok {
		return st
	}
	return s.lookup(ctx, telegramID, channel)
}

// CheckNow — проверка по кнопке «Да, я подписался»: кэш сбрасывается, но не
// чаще раза в 3 секунды на пользователя (иначе ErrTooFast).
func (s *Service) CheckNow(ctx context.Context, telegramID int64) (State, error) {
	if !s.Active() || telegramID <= 0 {
		return Subscribed, nil
	}
	channel := config.ChannelGateChannel()
	now := s.now()
	s.mu.Lock()
	if last, ok := s.lastNow[telegramID]; ok && now.Sub(last) < minCheckInterval {
		s.mu.Unlock()
		return Unknown, ErrTooFast
	}
	s.lastNow[telegramID] = now
	delete(s.cache, telegramID)
	s.mu.Unlock()
	return s.lookup(ctx, telegramID, channel), nil
}

// Reset сбрасывает кэш и счётчик сбоев: вызывается, когда админ сменил канал
// или включил/выключил требование.
func (s *Service) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache = make(map[int64]cacheEntry)
	s.lastNow = make(map[int64]time.Time)
	s.pausedUntil = time.Time{}
	s.botID = 0
	s.failures = 0
	s.alerted = false
	s.mu.Unlock()
}

func (s *Service) cached(id int64, channel string) (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[id]
	if !ok || e.channel != channel || !s.now().Before(e.until) {
		return Unknown, false
	}
	return e.state, true
}

func (s *Service) lookup(ctx context.Context, id int64, channel string) State {
	v, _, _ := s.sf.Do(strconv.FormatInt(id, 10), func() (any, error) {
		return s.fetch(context.WithoutCancel(ctx), id, channel), nil
	})
	st, _ := v.(State)
	return st
}

func (s *Service) fetch(ctx context.Context, id int64, channel string) State {
	s.mu.Lock()
	paused := s.now().Before(s.pausedUntil)
	s.mu.Unlock()
	if paused {
		return Unknown
	}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	m, err := s.api.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: channel, UserID: id})
	if err != nil {
		return s.onError(id, err)
	}
	st := stateOf(m)
	if st == Unknown {
		slog.Warn("channel gate: unexpected member status", "channel", channel)
		return Unknown
	}
	s.recordOK()
	ttl := notSubscribedTTL
	if st == Subscribed {
		ttl = subscribedTTL
		if s.onSubscribed != nil {
			s.onSubscribed(id)
		}
	}
	s.mu.Lock()
	if len(s.cache) > cachePruneAbove {
		s.pruneLocked()
	}
	s.cache[id] = cacheEntry{state: st, channel: channel, until: s.now().Add(ttl)}
	s.mu.Unlock()
	return st
}

func (s *Service) pruneLocked() {
	now := s.now()
	for k, e := range s.cache {
		if !now.Before(e.until) {
			delete(s.cache, k)
		}
	}
	for k, t := range s.lastNow {
		if now.Sub(t) >= minCheckInterval {
			delete(s.lastNow, k)
		}
	}
}

// stateOf переводит статус участника канала в State.
func stateOf(m *models.ChatMember) State {
	if m == nil {
		return Unknown
	}
	switch m.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return Subscribed
	case models.ChatMemberTypeRestricted:
		// Ограниченный участник может и состоять в канале, и уже не состоять.
		if m.Restricted != nil && m.Restricted.IsMember {
			return Subscribed
		}
		return NotSubscribed
	case models.ChatMemberTypeLeft, models.ChatMemberTypeBanned:
		return NotSubscribed
	}
	return Unknown
}

func (s *Service) onError(id int64, err error) State {
	var tooMany *bot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		wait := time.Duration(tooMany.RetryAfter) * time.Second
		if wait <= 0 || wait > time.Minute {
			wait = 5 * time.Second
		}
		s.mu.Lock()
		s.pausedUntil = s.now().Add(wait)
		s.mu.Unlock()
		slog.Warn("channel gate: Telegram rate limit, checks paused", "retry_after_sec", tooMany.RetryAfter)
		return Unknown
	}
	msg := strings.ToLower(err.Error())
	// Telegram не знает такого пользователя (удалённый аккаунт, выдуманный id):
	// это не поломка проверки, но и не повод запирать — пропускаем, не считая сбоем.
	if strings.Contains(msg, "user not found") || strings.Contains(msg, "participant_id_invalid") ||
		strings.Contains(msg, "invalid user identifier") {
		return Unknown
	}
	slog.Warn("channel gate: member check failed, letting the user through", "error", err)
	s.recordFailure(err)
	return Unknown
}

func (s *Service) recordOK() {
	s.mu.Lock()
	wasAlerted := s.alerted
	s.failures = 0
	s.alerted = false
	s.mu.Unlock()
	if wasAlerted && s.onHealth != nil {
		s.onHealth(false, "")
	}
}

func (s *Service) recordFailure(err error) {
	now := s.now()
	s.mu.Lock()
	s.failures++
	fire := s.failures >= failuresBeforeAlert && (!s.alerted || now.Sub(s.alertedAt) >= alertRepeat)
	if s.failures >= failuresBeforeAlert {
		// Telegram явно недоступен или бот не годится для проверки: полминуты не ходим
		// туда вовсе, иначе каждая страница кабинета ждала бы таймаута.
		s.pausedUntil = now.Add(failurePause)
	}
	if fire {
		s.alerted = true
		s.alertedAt = now
	}
	s.mu.Unlock()
	if fire && s.onHealth != nil {
		s.onHealth(true, err.Error())
	}
}

// ProbeResult — итог проверки канала из админки.
type ProbeResult struct {
	OK         bool   `json:"ok"`
	Title      string `json:"title,omitempty"`
	Username   string `json:"username,omitempty"`
	Members    int    `json:"members,omitempty"`
	BotIsAdmin bool   `json:"bot_is_admin"`
	PublicURL  string `json:"public_url,omitempty"`
	// Error — один из Probe*; пусто при OK.
	Error string `json:"error,omitempty"`
}

// Probe проверяет канал, введённый админом: он существует, публичный и бот
// в нём администратор. Не меняет настроек.
func (s *Service) Probe(ctx context.Context, raw string) ProbeResult {
	ch, err := config.NormalizeChannelUsername(raw)
	if err != nil || ch == "" {
		return ProbeResult{Error: ProbeBadFormat}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*apiTimeout)
	defer cancel()

	chat, err := s.api.GetChat(ctx, &bot.GetChatParams{ChatID: ch})
	if err != nil {
		if isNotFound(err) {
			return ProbeResult{Error: ProbeNotFound}
		}
		slog.Warn("channel gate: probe getChat failed", "error", err)
		return ProbeResult{Error: ProbeUnavailable}
	}
	switch chat.Type {
	case models.ChatTypeChannel, models.ChatTypeSupergroup, models.ChatTypeGroup:
	default:
		return ProbeResult{Error: ProbeNotFound}
	}
	if chat.Username == "" {
		return ProbeResult{Error: ProbePrivate}
	}
	res := ProbeResult{Title: chat.Title, Username: "@" + chat.Username, PublicURL: "https://t.me/" + chat.Username}

	botID, err := s.selfID(ctx)
	if err != nil {
		slog.Warn("channel gate: probe getMe failed", "error", err)
		res.Error = ProbeUnavailable
		return res
	}
	member, err := s.api.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: ch, UserID: botID})
	if err != nil {
		// «member list is inaccessible» — настоящий ответ Telegram, когда бота нет в
		// канале или он не админ: список участников ему недоступен.
		msg := strings.ToLower(err.Error())
		if isNotFound(err) || strings.Contains(msg, "user not found") || strings.Contains(msg, "member list is inaccessible") {
			res.Error = ProbeBotNotAdmin
			return res
		}
		slog.Warn("channel gate: probe getChatMember failed", "error", err)
		res.Error = ProbeUnavailable
		return res
	}
	res.BotIsAdmin = member != nil && (member.Type == models.ChatMemberTypeOwner || member.Type == models.ChatMemberTypeAdministrator)
	if !res.BotIsAdmin {
		res.Error = ProbeBotNotAdmin
		return res
	}
	if n, err := s.api.GetChatMemberCount(ctx, &bot.GetChatMemberCountParams{ChatID: ch}); err == nil {
		res.Members = n
	}
	res.OK = true
	return res
}

func (s *Service) selfID(ctx context.Context) (int64, error) {
	s.mu.Lock()
	id := s.botID
	s.mu.Unlock()
	if id != 0 {
		return id, nil
	}
	me, err := s.api.GetMe(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.botID = me.ID
	s.mu.Unlock()
	return me.ID, nil
}

func isNotFound(err error) bool {
	if errors.Is(err, bot.ErrorNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "chat not found") || strings.Contains(msg, "bot is not a member") ||
		strings.Contains(msg, "bot was kicked") || strings.Contains(msg, "channel_private")
}
