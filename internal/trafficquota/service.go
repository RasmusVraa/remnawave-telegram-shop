// Package trafficquota считает расход трафика серверов обхода и обычных серверов,
// выключает сквад обхода при исчерпании лимита и шлёт предупреждения.
//
// Панель умеет только один общий лимит на пользователя, поэтому лимит обхода держим
// сами: сумма байт по нодам обхода из /api/bandwidth-stats/users/{id} за календарный
// месяц. Исчерпал — сквад обхода снимается (обычные серверы продолжают работать),
// начался месяц или куплен пакет — возвращается.
//
// Воркер сверяет панель «по факту»: что бы ни сделал админ с клиентом в обход нашей логики
// (вернул сквад, сменил тариф, затёр лимит), при ближайшем проходе всё приводится в порядок.
package trafficquota

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// maxAlertsPerPass — сколько предупреждений отправляется за один проход. Остальные уйдут на следующих:
// так первое включение функции не засыпает клиентов и не упирается в лимиты Telegram.
const maxAlertsPerPass = 60

// Panel — то, что воркеру нужно от клиента панели.
type Panel interface {
	GetUsers(ctx context.Context) ([]remnawave.User, error)
	GetUserByID(ctx context.Context, userID int64) (*remnawave.User, error)
	GetUserUsageByRange(ctx context.Context, userID int64, start, end time.Time) (*remnawave.UserUsageSeries, error)
	SetUserSquadsAndLimit(ctx context.Context, userID int64, squads *[]uuid.UUID, limit *int64) error
}

// Alert — что случилось с лимитом клиента.
type Alert struct {
	Kind       string // database.TrafficKindMain / TrafficKindBypass
	Level      int    // порог в процентах (80) или 100
	UsedBytes  int64
	LimitBytes int64
	ResetAt    time.Time
}

// Notifier доставляет предупреждение клиенту (кабинет + Telegram). Ошибка — предупреждение
// не сохранилось нигде, и воркер попробует снова на следующем проходе.
type Notifier interface {
	Alert(ctx context.Context, customer database.Customer, a Alert) error
}

// Service — воркер раздельного трафика.
type Service struct {
	repo      *database.TrafficSplitRepository
	customers *database.CustomerRepository
	tariffs   *database.TariffRepository
	panel     Panel
	// rwClient — тот же клиент панели в конкретном типе: нужен для поиска профиля клиента, у которого id в БД ещё не сохранён.
	rwClient *remnawave.Client
	notifier Notifier
	now      func() time.Time

	// mu защищает lastUsed, lastTick и alertBudget: Tick идёт из крона, а ReevaluateBypass и SyncMainLimit
	// зовутся из обработчиков оплаты и админки в других горутинах.
	mu sync.Mutex
	// lastUsed — расход по панели на прошлом проходе: у кого он не вырос, статистику по нодам не запрашиваем.
	lastUsed    map[int64]float64
	lastTick    time.Time
	alertBudget int
	// custLocks — по замку на клиента: воркер и обработчик оплаты не правят одного и того же клиента одновременно.
	custLocks sync.Map
}

func New(repo *database.TrafficSplitRepository, customers *database.CustomerRepository, tariffs *database.TariffRepository, panel Panel, notifier Notifier) *Service {
	s := &Service{
		repo: repo, customers: customers, tariffs: tariffs, panel: panel, notifier: notifier,
		now: time.Now, lastUsed: map[int64]float64{},
	}
	if c, ok := panel.(*remnawave.Client); ok {
		s.rwClient = c
	}
	return s
}

func (s *Service) lockCustomer(id int64) func() {
	v, _ := s.custLocks.LoadOrStore(id, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

func (s *Service) getLastUsed(panelID int64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.lastUsed[panelID]
	return v, ok
}

func (s *Service) setLastUsed(panelID int64, v float64) {
	s.mu.Lock()
	s.lastUsed[panelID] = v
	s.mu.Unlock()
}

// resolveUser возвращает профиль клиента в панели. Идентификатор профиля у клиента сохраняется лениво
// (при покупке тарифа, заходе в кабинет), поэтому при его отсутствии ищем по общим правилам проекта
// (telegram_id, имя профиля, ссылка подписки) и запоминаем найденное. nil, nil — профиля в панели нет.
func (s *Service) resolveUser(ctx context.Context, c *database.Customer) (*remnawave.User, error) {
	if c == nil {
		return nil, nil
	}
	if c.RemnawaveUserID != nil {
		return s.panel.GetUserByID(ctx, *c.RemnawaveUserID)
	}
	if s.rwClient == nil {
		return nil, nil
	}
	u, err := cabsvc.ResolveRemnawaveCustomerUser(ctx, s.rwClient, s.customers, c)
	if errors.Is(err, remnawave.ErrUserNotFound) {
		return nil, nil
	}
	return u, err
}

// matchCustomers сопоставляет профили панели с клиентами бота: по сохранённому id, а для остальных
// (клиенты, у которых id ещё не сохранён) — по telegram_id; найденное запоминается.
func (s *Service) matchCustomers(ctx context.Context, users []remnawave.User) (map[int64]database.Customer, error) {
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	out, err := s.repo.CustomersByPanelUserIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var tgIDs []int64
	byTG := map[int64]remnawave.User{}
	for _, u := range users {
		if _, ok := out[u.ID]; ok || u.TelegramID == nil || utils.IsSyntheticTelegramID(*u.TelegramID) {
			continue
		}
		tgIDs = append(tgIDs, *u.TelegramID)
		byTG[*u.TelegramID] = u
	}
	if len(tgIDs) == 0 {
		return out, nil
	}
	found, err := s.customers.FindByTelegramIds(ctx, tgIDs)
	if err != nil {
		return out, nil // запасной путь не должен ломать основной
	}
	for _, c := range found {
		u, ok := byTG[c.TelegramID]
		if !ok || c.RemnawaveUserID != nil {
			continue
		}
		if err := s.customers.SetRemnawaveIdentity(ctx, c.ID, u.ID, u.ShortUUID); err != nil {
			slog.Warn("trafficquota: remember panel identity", "customer_id", c.ID, "error", err)
		}
		id := u.ID
		c.RemnawaveUserID = &id
		out[u.ID] = c
	}
	return out, nil
}

// MainLimitForTariff — лимит обычного трафика тарифа в байтах (как при покупке: у тарифа 0 — берём общий).
func MainLimitForTariff(t *database.Tariff) int64 {
	var tl int64
	if t != nil && t.TrafficLimitBytes > 0 {
		tl = t.TrafficLimitBytes
	} else {
		tl = int64(config.TrafficLimit())
	}
	if tl < 0 {
		tl = 0
	}
	return tl
}

func parseNodeSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out[p] = struct{}{}
		}
	}
	return out
}

func percent(used, limit int64) int {
	if limit <= 0 {
		return 0
	}
	return int(used * 100 / limit)
}

func hasAnySquad(have, want []uuid.UUID) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

// RunDue запускает проход, если с прошлого прошло не меньше TRAFFIC_POLL_MINUTES.
// Крон дёргает его раз в минуту, а период настраивается в админке без перезапуска.
func (s *Service) RunDue(ctx context.Context) {
	s.mu.Lock()
	due := s.lastTick.IsZero() || s.now().Sub(s.lastTick) >= time.Duration(config.TrafficPollMinutes())*time.Minute-5*time.Second
	if due {
		s.lastTick = s.now()
	}
	s.mu.Unlock()
	if !due {
		return
	}
	if err := s.Tick(ctx); err != nil {
		slog.Error("trafficquota: tick", "error", err)
	}
}

// Tick — один проход воркера.
//
// Работает всегда: истёкшие пакеты (и возврат лимита) нужны и при выключенном раздельном трафике,
// ведь пакеты обычного трафика продаются независимо от него. Всё, что про обход, — только при включённом.
func (s *Service) Tick(ctx context.Context) error {
	now := s.now().UTC()
	s.mu.Lock()
	s.alertBudget = maxAlertsPerPass
	s.mu.Unlock()

	forced := s.expireExtras(ctx, now)
	s.reconcileMainLimits(ctx, now)

	if !config.TrafficSplitEnabled() {
		// Функцию выключили: клиентов со снятым сквадом обхода возвращаем, иначе они останутся без него насовсем.
		s.restoreRemoved(ctx)
		return nil
	}

	users, err := s.panel.GetUsers(ctx)
	if err != nil {
		return fmt.Errorf("trafficquota: panel users: %w", err)
	}
	byID := make(map[int64]remnawave.User, len(users))
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		byID[u.ID] = u
		ids = append(ids, u.ID)
	}

	s.reconcileTariffs(ctx, now)
	if err := s.processBypass(ctx, now, byID, forced); err != nil {
		slog.Error("trafficquota: bypass", "error", err)
	}
	if err := s.processMainAlerts(ctx, now, byID, ids); err != nil {
		slog.Error("trafficquota: main alerts", "error", err)
	}
	return nil
}

// expireExtras возвращает лимит у клиентов с истёкшими пакетами и только после этого удаляет пакеты:
// если панель недоступна, пакет остаётся и попытка повторится на следующем проходе.
// Возвращает клиентов, у которых истёк пакет обхода: их обход пересчитывается принудительно.
func (s *Service) expireExtras(ctx context.Context, now time.Time) map[int64]bool {
	forced := map[int64]bool{}
	expired, err := s.repo.ListExpiredExtras(ctx, now)
	if err != nil {
		slog.Error("trafficquota: list expired extras", "error", err)
		return forced
	}
	for _, e := range expired {
		if e.Kind == database.TrafficKindMain {
			if err := s.SyncMainLimit(ctx, e.CustomerID); err != nil {
				slog.Error("trafficquota: restore main limit", "customer_id", e.CustomerID, "error", err)
				continue
			}
		} else {
			forced[e.CustomerID] = true
		}
		if err := s.repo.DeleteExpiredExtrasFor(ctx, e.CustomerID, e.Kind, now); err != nil {
			slog.Error("trafficquota: delete expired extras", "customer_id", e.CustomerID, "error", err)
		}
	}
	return forced
}

// SyncMainLimit выставляет в панели лимит обычного трафика: лимит тарифа плюс действующие пакеты.
// Безлимит (0) не трогаем.
func (s *Service) SyncMainLimit(ctx context.Context, customerID int64) error {
	defer s.lockCustomer(customerID)()
	c, err := s.customers.FindById(ctx, customerID)
	if err != nil || c == nil {
		return err
	}
	pu, err := s.resolveUser(ctx, c)
	if err != nil || pu == nil {
		return err
	}
	base, err := s.baseMainLimit(ctx, c)
	if err != nil || base <= 0 {
		return err
	}
	extra, err := s.repo.ActiveExtraBytes(ctx, customerID, database.TrafficKindMain, s.now())
	if err != nil {
		return err
	}
	limit := base + extra
	return s.panel.SetUserSquadsAndLimit(ctx, pu.ID, nil, &limit)
}

func (s *Service) baseMainLimit(ctx context.Context, c *database.Customer) (int64, error) {
	var tariff *database.Tariff
	if c.CurrentTariffID != nil {
		var err error
		if tariff, err = s.tariffs.GetByID(ctx, *c.CurrentTariffID); err != nil {
			return 0, err
		}
	}
	return MainLimitForTariff(tariff), nil
}

// reconcileMainLimits возвращает пакеты обычного трафика, если лимит в панели затёрт чужой записью
// (админ сменил тариф, промокод, продление): у клиента с действующим пакетом лимит должен быть «тариф + пакеты».
// Любое другое значение считаем намеренной ручной правкой админа и не трогаем.
func (s *Service) reconcileMainLimits(ctx context.Context, now time.Time) {
	sums, err := s.repo.ActiveMainExtraSums(ctx, now)
	if err != nil {
		slog.Error("trafficquota: active extras", "error", err)
		return
	}
	for customerID, extra := range sums {
		c, err := s.customers.FindById(ctx, customerID)
		if err != nil || c == nil {
			continue
		}
		base, err := s.baseMainLimit(ctx, c)
		if err != nil || base <= 0 {
			continue
		}
		u, err := s.resolveUser(ctx, c)
		if err != nil || u == nil {
			continue
		}
		if u.TrafficLimitBytes == base && extra > 0 {
			if err := s.SyncMainLimit(ctx, customerID); err != nil {
				slog.Error("trafficquota: reconcile main limit", "customer_id", customerID, "error", err)
			} else {
				slog.Info("trafficquota: main limit restored after foreign write", "customer_id", customerID)
			}
		}
	}
}

// reconcileTariffs обновляет снимок обхода у клиентов, чей тариф сменили мимо покупки (админ):
// для них это новая выдача тарифа, настройки берутся у нового тарифа.
func (s *Service) reconcileTariffs(ctx context.Context, now time.Time) {
	list, err := s.repo.ListTariffMismatches(ctx)
	if err != nil {
		slog.Error("trafficquota: tariff mismatches", "error", err)
		return
	}
	for _, m := range list {
		b, err := s.repo.GetTariffBypass(ctx, m.TariffID)
		if err != nil {
			continue
		}
		tid := m.TariffID
		if err := s.repo.SaveSnapshot(ctx, m.CustomerID, &tid, b, now); err != nil {
			slog.Error("trafficquota: resnapshot", "customer_id", m.CustomerID, "error", err)
			continue
		}
		slog.Info("trafficquota: snapshot refreshed after tariff change", "customer_id", m.CustomerID, "tariff_id", m.TariffID)
	}
}

// restoreRemoved возвращает сквад обхода всем, у кого его снял воркер (при выключении функции).
func (s *Service) restoreRemoved(ctx context.Context) {
	states, err := s.repo.ListRemovedStates(ctx)
	if err != nil || len(states) == 0 {
		return
	}
	for _, st := range states {
		if err := s.restoreOne(ctx, st); err != nil {
			slog.Error("trafficquota: restore squad on disable", "customer_id", st.CustomerID, "error", err)
		}
	}
}

func (s *Service) restoreOne(ctx context.Context, st database.BypassState) error {
	defer s.lockCustomer(st.CustomerID)()
	c, err := s.customers.FindById(ctx, st.CustomerID)
	if err != nil || c == nil {
		return err
	}
	u, err := s.resolveUser(ctx, c)
	if err != nil {
		return err
	}
	if u != nil {
		if squads, _ := database.ParseSquadUUIDList(st.SquadUUIDs); len(squads) > 0 {
			cur := remnawave.WithSquads(remnawave.UserSquadUUIDs(u), squads)
			if err := s.panel.SetUserSquadsAndLimit(ctx, u.ID, &cur, nil); err != nil {
				return err
			}
		}
	}
	return s.repo.SaveUsage(ctx, st.CustomerID, st.PeriodStart, st.UsedBytes, false, s.now().UTC())
}

// processBypass считает расход обхода у клиентов с отдельным лимитом.
func (s *Service) processBypass(ctx context.Context, now time.Time, users map[int64]remnawave.User, forced map[int64]bool) error {
	states, err := s.repo.ListTrackedStates(ctx)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}
	slog.Info("trafficquota: pass", "tracked", len(states), "panel_users", len(users))
	list := make([]remnawave.User, 0, len(users))
	for _, u := range users {
		list = append(list, u)
	}
	custByPanel, err := s.matchCustomers(ctx, list)
	if err != nil {
		return err
	}
	panelByCustomer := make(map[int64]int64, len(custByPanel))
	for pid, c := range custByPanel {
		panelByCustomer[c.ID] = pid
	}
	for _, st := range states {
		pid, ok := panelByCustomer[st.CustomerID]
		if !ok {
			// Не нашли по id и telegram_id (web-only, id ещё не сохранён): ищем профиль по общим правилам.
			c, err := s.customers.FindById(ctx, st.CustomerID)
			if err != nil || c == nil {
				continue
			}
			ru, err := s.resolveUser(ctx, c)
			if err != nil || ru == nil {
				continue
			}
			if _, inPanel := users[ru.ID]; !inPanel {
				continue
			}
			pid = ru.ID
			custByPanel[pid] = *c
		}
		u := users[pid]
		if u.Status == "DISABLED" || u.Status == "EXPIRED" {
			continue
		}
		if err := s.processOneBypass(ctx, now, st, u, custByPanel[pid], forced[st.CustomerID]); err != nil {
			slog.Error("trafficquota: bypass customer", "customer_id", st.CustomerID, "error", err)
		}
	}
	return nil
}

// ReevaluateBypass пересчитывает обход одного клиента сразу (после покупки пакета),
// не дожидаясь воркера.
func (s *Service) ReevaluateBypass(ctx context.Context, customerID int64) error {
	st, err := s.repo.GetState(ctx, customerID)
	if err != nil || st == nil || st.LimitBytes <= 0 {
		return err
	}
	c, err := s.customers.FindById(ctx, customerID)
	if err != nil || c == nil {
		return err
	}
	u, err := s.resolveUser(ctx, c)
	if err != nil || u == nil {
		return err
	}
	return s.processOneBypass(ctx, s.now().UTC(), *st, *u, *c, true)
}

// OnPackGranted применяет выданный пакет сразу: забывает отправленные предупреждения этого вида
// (лимит вырос — приближение к нему снова заслуживает уведомления) и приводит панель в порядок.
func (s *Service) OnPackGranted(ctx context.Context, customerID int64, kind string) error {
	if err := s.repo.ResetAlerts(ctx, customerID, kind); err != nil {
		slog.Warn("trafficquota: reset alerts", "customer_id", customerID, "error", err)
	}
	if kind == database.TrafficKindMain {
		return s.SyncMainLimit(ctx, customerID)
	}
	return s.ReevaluateBypass(ctx, customerID)
}

func (s *Service) processOneBypass(ctx context.Context, now time.Time, st database.BypassState, u remnawave.User, c database.Customer, force bool) error {
	defer s.lockCustomer(st.CustomerID)()
	// Состояние могло измениться, пока ждали замок (оплата пакета vs проход воркера).
	if fresh, err := s.repo.GetState(ctx, st.CustomerID); err == nil && fresh != nil {
		st = *fresh
	}
	if st.LimitBytes <= 0 {
		return nil
	}
	period := database.TrafficPeriodStart(now)
	rollover := !st.PeriodStart.UTC().Equal(period)
	if rollover {
		st.PeriodStart = period
		st.UsedBytes = 0
	}
	// Расход по панели не вырос — статистику по нодам не запрашиваем. Клиентов со снятым
	// сквадом проверяем всегда: у них расход не растёт, а сквад должен вернуться, как только
	// появятся пакет или новый месяц (в том числе выданный админом, мимо оплаты).
	if last, seen := s.getLastUsed(u.ID); !force && !rollover && !st.SquadRemoved && seen && last == u.UserTraffic.UsedTrafficBytes && st.CheckedAt != nil {
		return nil
	}
	used := st.UsedBytes
	nodes := parseNodeSet(st.NodeUUIDs)
	if len(nodes) > 0 {
		series, err := s.panel.GetUserUsageByRange(ctx, u.ID, period, now)
		if err != nil {
			return err
		}
		used = series.NodesTotal(nodes)
	}
	extras, err := s.repo.ActiveExtraBytes(ctx, st.CustomerID, database.TrafficKindBypass, now)
	if err != nil {
		return err
	}
	limit := st.LimitBytes + extras
	squads, _ := database.ParseSquadUUIDList(st.SquadUUIDs)
	exhausted := used >= limit && len(squads) > 0
	present := hasAnySquad(remnawave.UserSquadUUIDs(&u), squads)

	switch {
	case exhausted && present:
		// Перечитываем профиль прямо перед записью: список сквадов из общего снимка мог устареть
		// (клиент продлил тариф посреди прохода), и мы бы затёрли свежие сквады.
		cur, err := s.freshSquads(ctx, u)
		if err != nil {
			return err
		}
		cur = remnawave.WithoutSquads(cur, squads)
		if err := s.panel.SetUserSquadsAndLimit(ctx, u.ID, &cur, nil); err != nil {
			return err
		}
		slog.Info("trafficquota: bypass squad removed", "customer_id", st.CustomerID, "used_bytes", used, "limit_bytes", limit, "was_marked", st.SquadRemoved)
	case !exhausted && st.SquadRemoved && len(squads) > 0 && !present:
		cur, err := s.freshSquads(ctx, u)
		if err != nil {
			return err
		}
		cur = remnawave.WithSquads(cur, squads)
		if err := s.panel.SetUserSquadsAndLimit(ctx, u.ID, &cur, nil); err != nil {
			return err
		}
		slog.Info("trafficquota: bypass squad restored", "customer_id", st.CustomerID, "used_bytes", used, "limit_bytes", limit)
	}

	if err := s.repo.SaveUsage(ctx, st.CustomerID, period, used, exhausted, now); err != nil {
		return err
	}
	s.setLastUsed(u.ID, u.UserTraffic.UsedTrafficBytes)
	s.alert(ctx, c, database.TrafficKindBypass, period, used, limit, now)
	return nil
}

// freshSquads — актуальный список внутренних сквадов клиента прямо из панели.
func (s *Service) freshSquads(ctx context.Context, u remnawave.User) ([]uuid.UUID, error) {
	fresh, err := s.panel.GetUserByID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if fresh != nil {
		u = *fresh
	}
	return remnawave.UserSquadUUIDs(&u), nil
}

// processMainAlerts шлёт предупреждения по лимиту обычных серверов (лимит держит панель).
//
// Только тем, кто может докупить трафик: у кого есть тариф с пакетами обычного трафика. Клиентам без тарифа
// (пробный период, классический режим) и тарифам без пакетов предупреждений не шлём: как и раньше, они
// узнают о лимите по самой подписке, а кнопка «Докупить» вела бы в никуда.
func (s *Service) processMainAlerts(ctx context.Context, now time.Time, users map[int64]remnawave.User, ids []int64) error {
	var withLimit []remnawave.User
	for _, id := range ids {
		if u := users[id]; u.TrafficLimitBytes > 0 && u.Status != "DISABLED" && u.Status != "EXPIRED" {
			withLimit = append(withLimit, u)
		}
	}
	if len(withLimit) == 0 {
		return nil
	}
	custByPanel, err := s.matchCustomers(ctx, withLimit)
	if err != nil {
		return err
	}
	canBuy := map[int64]bool{}
	for pid, c := range custByPanel {
		if c.CurrentTariffID == nil || c.ExpireAt == nil || !c.ExpireAt.After(now) {
			continue
		}
		ok, cached := canBuy[*c.CurrentTariffID]
		if !cached {
			packs, err := s.repo.ListPacks(ctx, *c.CurrentTariffID, database.TrafficKindMain, true)
			ok = err == nil && len(packs) > 0
			canBuy[*c.CurrentTariffID] = ok
		}
		if !ok {
			continue
		}
		u := users[pid]
		period := database.TrafficPeriodStart(now)
		if u.LastTrafficResetAt != nil {
			t := u.LastTrafficResetAt.UTC()
			period = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}
		s.alert(ctx, c, database.TrafficKindMain, period, int64(u.UserTraffic.UsedTrafficBytes), u.TrafficLimitBytes, now)
	}
	return nil
}

func (s *Service) takeAlertBudget() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alertBudget <= 0 {
		return false
	}
	s.alertBudget--
	return true
}

// alert отправляет уведомление о пороге или исчерпании, один раз на период.
func (s *Service) alert(ctx context.Context, c database.Customer, kind string, period time.Time, used, limit int64, now time.Time) {
	if s.notifier == nil || limit <= 0 {
		return
	}
	pct := percent(used, limit)
	threshold := config.TrafficAlertPercent()
	level := 0
	switch {
	case pct >= 100:
		level = 100
	case pct >= threshold:
		level = threshold
	default:
		return
	}
	fresh, err := s.repo.TryRecordAlert(ctx, c.ID, kind, period, level)
	if err != nil {
		slog.Error("trafficquota: record alert", "customer_id", c.ID, "error", err)
		return
	}
	if !fresh {
		return
	}
	// Бюджет тратится только на новое предупреждение. Раньше его списывали до проверки «уже отправляли»,
	// и клиенты, давно получившие своё предупреждение, каждый проход съедали все 60 мест: при их числе
	// больше бюджета новые предупреждения не уходили никогда (порядок обхода стабилен).
	if !s.takeAlertBudget() {
		// Не в этот проход: снимаем отметку, предупреждение уйдёт на одном из следующих.
		if ferr := s.repo.ForgetAlert(ctx, c.ID, kind, period, level); ferr != nil {
			slog.Error("trafficquota: forget alert", "customer_id", c.ID, "error", ferr)
		}
		return
	}
	if level == 100 {
		// Порог предупреждения при прыжке сразу к 100% уже не нужен.
		_, _ = s.repo.TryRecordAlert(ctx, c.ID, kind, period, threshold)
	}
	if err := s.notifier.Alert(ctx, c, Alert{Kind: kind, Level: level, UsedBytes: used, LimitBytes: limit, ResetAt: database.TrafficPeriodEnd(now)}); err != nil {
		slog.Warn("trafficquota: alert not delivered, will retry", "customer_id", c.ID, "kind", kind, "level", level, "error", err)
		if ferr := s.repo.ForgetAlert(ctx, c.ID, kind, period, level); ferr != nil {
			slog.Error("trafficquota: forget alert", "customer_id", c.ID, "error", ferr)
		}
	}
}
