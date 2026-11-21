package channelgate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

type fakeAPI struct {
	memberCalls atomic.Int32
	member      func(userID int64) (*models.ChatMember, error)
	chat        *models.ChatFullInfo
	chatErr     error
	botMember   *models.ChatMember
	botErr      error
	count       int
	delay       time.Duration
}

func (f *fakeAPI) GetMe(context.Context) (*models.User, error) {
	return &models.User{ID: 999}, nil
}

func (f *fakeAPI) GetChat(context.Context, *bot.GetChatParams) (*models.ChatFullInfo, error) {
	return f.chat, f.chatErr
}

func (f *fakeAPI) GetChatMember(_ context.Context, p *bot.GetChatMemberParams) (*models.ChatMember, error) {
	f.memberCalls.Add(1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if p.UserID == 999 {
		return f.botMember, f.botErr
	}
	return f.member(p.UserID)
}

func (f *fakeAPI) GetChatMemberCount(context.Context, *bot.GetChatMemberCountParams) (int, error) {
	return f.count, nil
}

func member(t models.ChatMemberType) *models.ChatMember { return &models.ChatMember{Type: t} }

// activate включает требование через реестр настроек, как это делает админка.
func activate(t *testing.T) {
	t.Helper()
	if _, err := config.ApplyRuntimePatch(map[string]string{
		"CHANNEL_GATE_CHANNEL": "@meows_news", "CHANNEL_GATE_ENABLED": "true",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	})
}

func newSvc(api API, health func(bool, string)) (*Service, *time.Time) {
	s := New(api, health)
	clock := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return s, &clock
}

func TestStateOf(t *testing.T) {
	cases := []struct {
		name string
		in   *models.ChatMember
		want State
	}{
		{"creator", member(models.ChatMemberTypeOwner), Subscribed},
		{"administrator", member(models.ChatMemberTypeAdministrator), Subscribed},
		{"member", member(models.ChatMemberTypeMember), Subscribed},
		{"left", member(models.ChatMemberTypeLeft), NotSubscribed},
		{"kicked", member(models.ChatMemberTypeBanned), NotSubscribed},
		{"restricted still in channel", &models.ChatMember{Type: models.ChatMemberTypeRestricted, Restricted: &models.ChatMemberRestricted{IsMember: true}}, Subscribed},
		{"restricted already left", &models.ChatMember{Type: models.ChatMemberTypeRestricted, Restricted: &models.ChatMemberRestricted{IsMember: false}}, NotSubscribed},
		{"restricted without details", &models.ChatMember{Type: models.ChatMemberTypeRestricted}, NotSubscribed},
		{"nil", nil, Unknown},
		{"unknown status", &models.ChatMember{Type: "weird"}, Unknown},
	}
	for _, c := range cases {
		if got := stateOf(c.in); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDisabledNeverBlocks(t *testing.T) {
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s, _ := newSvc(api, nil)
	if s.Blocks(context.Background(), 1) {
		t.Fatal("выключенная функция не должна блокировать")
	}
	if api.memberCalls.Load() != 0 {
		t.Fatal("при выключенной функции в Telegram ходить не нужно")
	}
}

func TestEnabledWithoutChannelNeverBlocks(t *testing.T) {
	if _, err := config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "true"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false"}) })
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s, _ := newSvc(api, nil)
	if s.Blocks(context.Background(), 1) {
		t.Fatal("включено, но канала нет — запирать нельзя")
	}
}

func TestBlocksAndCaches(t *testing.T) {
	activate(t)
	left := true
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) {
		if left {
			return member(models.ChatMemberTypeLeft), nil
		}
		return member(models.ChatMemberTypeMember), nil
	}}
	s, clock := newSvc(api, nil)
	ctx := context.Background()

	if !s.Blocks(ctx, 5) {
		t.Fatal("не подписанного нужно блокировать")
	}
	s.Blocks(ctx, 5)
	if api.memberCalls.Load() != 1 {
		t.Fatalf("второй вызов должен идти из кэша, запросов: %d", api.memberCalls.Load())
	}
	// «не подписан» живёт 30 секунд.
	*clock = clock.Add(31 * time.Second)
	left = false
	if s.Blocks(ctx, 5) {
		t.Fatal("после подписки блокировки быть не должно")
	}
	if api.memberCalls.Load() != 2 {
		t.Fatalf("после истечения TTL нужен свежий запрос, запросов: %d", api.memberCalls.Load())
	}
	// «подписан» живёт 2 минуты.
	s.Blocks(ctx, 5)
	if api.memberCalls.Load() != 2 {
		t.Fatal("«подписан» должен кэшироваться")
	}
	*clock = clock.Add(3 * time.Minute)
	s.Blocks(ctx, 5)
	if api.memberCalls.Load() != 3 {
		t.Fatal("кэш «подписан» должен истечь")
	}
}

func TestAdminIsExempt(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s, _ := newSvc(api, nil)
	// id вне списка админов блокируется; админ из конфига — нет (проверяем через Exempt).
	if s.Exempt(123456) {
		t.Fatal("посторонний не админ")
	}
}

func TestTelegramErrorLetsUserThrough(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return nil, errors.New("network down") }}
	s, _ := newSvc(api, nil)
	if s.Blocks(context.Background(), 7) {
		t.Fatal("сбой Telegram не должен запирать пользователя")
	}
	if got := s.Check(context.Background(), 7); got != Unknown {
		t.Fatalf("при сбое ждём Unknown, получили %v", got)
	}
}

func TestUserNotFoundLetsThrough(t *testing.T) {
	activate(t)
	for _, msg := range []string{"bad request, Bad Request: user not found", "bad request, Bad Request: invalid user identifier specified"} {
		api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return nil, errors.New(msg) }}
		s, _ := newSvc(api, nil)
		if s.Blocks(context.Background(), 7) {
			t.Fatalf("%q: ответ про несуществующего пользователя не должен запирать", msg)
		}
	}
}

func TestOnSubscribedHook(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeMember), nil }}
	s, clock := newSvc(api, nil)
	var got []int64
	s.OnSubscribed(func(id int64) { got = append(got, id) })
	s.Check(context.Background(), 5)
	s.Check(context.Background(), 5) // из кэша — хук не зовётся
	if len(got) != 1 || got[0] != 5 {
		t.Fatalf("хук должен сработать один раз на свежую проверку, получили %v", got)
	}
	*clock = clock.Add(3 * time.Minute)
	s.Check(context.Background(), 5)
	if len(got) != 2 {
		t.Fatalf("после истечения кэша — новая проверка и новый вызов, получили %v", got)
	}
	left := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s2, _ := newSvc(left, nil)
	called := false
	s2.OnSubscribed(func(int64) { called = true })
	s2.Check(context.Background(), 6)
	if called {
		t.Fatal("для неподписанного хук не зовётся")
	}
}

func TestRepeatedFailuresPauseRequests(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return nil, errors.New("timeout") }}
	s, clock := newSvc(api, nil)
	ctx := context.Background()
	for i := int64(1); i <= 3; i++ {
		s.Check(ctx, i)
	}
	calls := api.memberCalls.Load()
	for i := int64(10); i < 20; i++ {
		if s.Blocks(ctx, i) {
			t.Fatal("во время паузы пользователей пропускаем")
		}
	}
	if api.memberCalls.Load() != calls {
		t.Fatalf("после трёх сбоев Telegram не должен дёргаться полминуты: было %d вызовов, стало %d", calls, api.memberCalls.Load())
	}
	*clock = clock.Add(31 * time.Second)
	s.Check(ctx, 30)
	if api.memberCalls.Load() != calls+1 {
		t.Fatal("после паузы проверка пробует снова")
	}
}

func TestExemptFunc(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s, _ := newSvc(api, nil)
	s.SetExemptFunc(func(id int64) bool { return id == 42 })
	if s.Blocks(context.Background(), 42) {
		t.Fatal("исключённого не блокируем")
	}
	if !s.Blocks(context.Background(), 43) {
		t.Fatal("остальных блокируем")
	}
	if api.memberCalls.Load() != 1 {
		t.Fatal("исключённого не проверяем в Telegram вовсе")
	}
}

func TestRateLimitPausesRequests(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) {
		return nil, &bot.TooManyRequestsError{Message: "slow down", RetryAfter: 10}
	}}
	s, clock := newSvc(api, nil)
	ctx := context.Background()
	if s.Blocks(ctx, 1) {
		t.Fatal("429 должен пропускать пользователя")
	}
	s.Blocks(ctx, 2)
	s.Blocks(ctx, 3)
	if api.memberCalls.Load() != 1 {
		t.Fatalf("во время паузы в Telegram ходить нельзя, запросов: %d", api.memberCalls.Load())
	}
	*clock = clock.Add(11 * time.Second)
	s.Blocks(ctx, 4)
	if api.memberCalls.Load() != 2 {
		t.Fatal("после паузы запросы должны возобновиться")
	}
}

func TestCheckNowRateLimitAndCacheReset(t *testing.T) {
	activate(t)
	subscribed := false
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) {
		if subscribed {
			return member(models.ChatMemberTypeMember), nil
		}
		return member(models.ChatMemberTypeLeft), nil
	}}
	s, clock := newSvc(api, nil)
	ctx := context.Background()

	st, err := s.CheckNow(ctx, 9)
	if err != nil || st != NotSubscribed {
		t.Fatalf("первая проверка: %v %v", st, err)
	}
	if _, err := s.CheckNow(ctx, 9); !errors.Is(err, ErrTooFast) {
		t.Fatalf("повтор в пределах 3 секунд должен дать ErrTooFast, получили %v", err)
	}
	subscribed = true
	*clock = clock.Add(4 * time.Second)
	// В кэше ещё «не подписан» (30 с), но кнопка его сбрасывает.
	st, err = s.CheckNow(ctx, 9)
	if err != nil || st != Subscribed {
		t.Fatalf("после подписки: %v %v", st, err)
	}
}

func TestConcurrentChecksAreCoalesced(t *testing.T) {
	activate(t)
	api := &fakeAPI{delay: 80 * time.Millisecond, member: func(int64) (*models.ChatMember, error) {
		return member(models.ChatMemberTypeLeft), nil
	}}
	s := New(api, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Check(context.Background(), 42)
		}()
	}
	wg.Wait()
	if n := api.memberCalls.Load(); n != 1 {
		t.Fatalf("10 одновременных проверок одного юзера должны дать 1 запрос, дали %d", n)
	}
}

func TestChannelChangeInvalidatesCache(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeMember), nil }}
	s, _ := newSvc(api, nil)
	s.Check(context.Background(), 3)
	if _, err := config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_CHANNEL": "@another_channel"}); err != nil {
		t.Fatal(err)
	}
	s.Check(context.Background(), 3)
	if api.memberCalls.Load() != 2 {
		t.Fatal("смена канала должна обесценить кэш прежнего канала")
	}
}

func TestHealthAlertAfterRepeatedFailuresAndRecovery(t *testing.T) {
	activate(t)
	var downs, ups int
	healthy := false
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) {
		if healthy {
			return member(models.ChatMemberTypeMember), nil
		}
		return nil, errors.New("chat not found")
	}}
	s, clock := newSvc(api, func(down bool, _ string) {
		if down {
			downs++
		} else {
			ups++
		}
	})
	ctx := context.Background()
	s.Check(ctx, 1)
	s.Check(ctx, 2)
	if downs != 0 {
		t.Fatal("два сбоя подряд — ещё не повод будить админа")
	}
	s.Check(ctx, 3)
	if downs != 1 {
		t.Fatalf("после трёх сбоев нужно одно оповещение, получили %d", downs)
	}
	s.Check(ctx, 4)
	if downs != 1 {
		t.Fatal("повторное оповещение — не чаще раза в 6 часов")
	}
	*clock = clock.Add(7 * time.Hour)
	s.Check(ctx, 5)
	if downs != 2 {
		t.Fatalf("через 6 часов напоминание, получили %d", downs)
	}
	healthy = true
	*clock = clock.Add(31 * time.Second) // пауза после сбоев (30 с) прошла
	s.Check(ctx, 6)
	if ups != 1 {
		t.Fatalf("после починки — одно «снова работает», получили %d", ups)
	}
}

func TestProbe(t *testing.T) {
	good := &models.ChatFullInfo{Type: models.ChatTypeChannel, Title: "Мяу-ус ВПН — новости", Username: "meows_news"}
	cases := []struct {
		name    string
		api     *fakeAPI
		raw     string
		wantErr string
		wantOK  bool
	}{
		{"ok", &fakeAPI{chat: good, botMember: member(models.ChatMemberTypeAdministrator), count: 2418}, "t.me/meows_news", "", true},
		{"bad format", &fakeAPI{}, "https://t.me/+AbCdEf", ProbeBadFormat, false},
		{"empty", &fakeAPI{}, "  ", ProbeBadFormat, false},
		{"not found", &fakeAPI{chatErr: errors.New("bad request, Bad Request: chat not found")}, "@meows_news", ProbeNotFound, false},
		{"telegram down", &fakeAPI{chatErr: errors.New("timeout")}, "@meows_news", ProbeUnavailable, false},
		{"private", &fakeAPI{chat: &models.ChatFullInfo{Type: models.ChatTypeChannel, Title: "x"}}, "@meows_news", ProbePrivate, false},
		{"not a channel", &fakeAPI{chat: &models.ChatFullInfo{Type: models.ChatTypePrivate, Username: "someone"}}, "@someone1", ProbeNotFound, false},
		{"bot is plain member", &fakeAPI{chat: good, botMember: member(models.ChatMemberTypeMember)}, "@meows_news", ProbeBotNotAdmin, false},
		{"bot not in channel", &fakeAPI{chat: good, botErr: errors.New("Bad Request: user not found")}, "@meows_news", ProbeBotNotAdmin, false},
		{"member list is inaccessible (живой ответ Telegram)", &fakeAPI{chat: good, botErr: errors.New("bad request, Bad Request: member list is inaccessible")}, "@meows_news", ProbeBotNotAdmin, false},
	}
	for _, c := range cases {
		s := New(c.api, nil)
		res := s.Probe(context.Background(), c.raw)
		if res.OK != c.wantOK || res.Error != c.wantErr {
			t.Errorf("%s: ok=%v error=%q, want ok=%v error=%q", c.name, res.OK, res.Error, c.wantOK, c.wantErr)
		}
		if c.wantOK && (res.Members != 2418 || res.PublicURL != "https://t.me/meows_news" || res.Title == "") {
			t.Errorf("%s: неполный результат %+v", c.name, res)
		}
	}
}

func TestTrialOnlyScope(t *testing.T) {
	activate(t)
	api := &fakeAPI{member: func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }}
	s, _ := newSvc(api, nil)
	ctx := context.Background()

	// «Для всего»: блокирует меню, а выдачу пробного отдельно не проверяет.
	if !s.Blocks(ctx, 5) {
		t.Fatal("в режиме «для всего» неподписанного нужно блокировать")
	}
	if s.BlocksTrial(ctx, 5) {
		t.Fatal("в режиме «для всего» пробный отдельно не проверяется: его закрывает общий гейт")
	}

	if _, err := config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": "trial"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": "all"}) })

	if s.Blocks(ctx, 5) {
		t.Fatal("в режиме «только для пробного» меню и оплата открыты")
	}
	if !s.BlocksTrial(ctx, 5) {
		t.Fatal("в режиме «только для пробного» неподписанного нужно остановить на выдаче пробного")
	}
	// Кнопка «Да, я подписался» работает и в этом режиме.
	if st, err := s.CheckNow(ctx, 6); err != nil || st != NotSubscribed {
		t.Fatalf("CheckNow: state=%v err=%v", st, err)
	}

	api.member = func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeMember), nil }
	if s.BlocksTrial(ctx, 7) {
		t.Fatal("подписанного останавливать не нужно")
	}
	s.SetExemptFunc(func(id int64) bool { return id == 8 })
	api.member = func(int64) (*models.ChatMember, error) { return member(models.ChatMemberTypeLeft), nil }
	if s.BlocksTrial(ctx, 8) {
		t.Fatal("админ не блокируется")
	}
}
