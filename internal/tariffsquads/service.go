// Package tariffsquads применяет состав internal squads тарифа к тем, кто уже
// на этом тарифе.
//
// Зачем: правка тарифа меняет только строку в БД, а профиль в панели
// перезаписывается лишь при следующей оплате/продлении (см.
// payment.ProcessPurchaseById -> CreateOrUpdateUserWithTariffProfile).
// Из-за этого админ, случайно снявший сквад и вернувший его обратно, не мог
// вернуть доступ действующим подписчикам — приходилось ждать продления.
package tariffsquads

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// Ошибки валидации запроса — обработчик переводит их в 400/409.
var (
	ErrNoChanges        = errors.New("no squads to apply")
	ErrUnknownSquad     = errors.New("squad not found in panel")
	ErrAddNotInTariff   = errors.New("squad is not part of the tariff")
	ErrRemoveInTariff   = errors.New("squad is still part of the tariff")
	ErrAlreadyRunning   = errors.New("apply already running for this tariff")
	ErrPanelUnavailable = errors.New("remnawave panel is not configured")
)

// applyConcurrency — сколько профилей патчим параллельно. Панель отвечает
// быстро, но заваливать её сотней одновременных PATCH ради фоновой операции
// незачем: при 4 потоках тысяча клиентов проходит за минуты и не мешает оплатам.
const applyConcurrency = 4

// applyTimeout ограничивает прогон целиком: запуск идёт в фоне, и повисший
// HTTP к панели не должен держать «running» вечно — иначе кнопка залипнет.
const applyTimeout = 30 * time.Minute

// maxFailureSamples — сколько текстов ошибок держим для показа в админке.
const maxFailureSamples = 20

// Status — состояние прогона.
type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Run — снимок хода применения. Живёт в памяти: операция идемпотентна и
// перезапускаема кнопкой, поэтому переживать рестарт процесса ей не нужно.
type Run struct {
	TariffID       int64       `json:"tariff_id"`
	Add            []uuid.UUID `json:"add"`
	Remove         []uuid.UUID `json:"remove"`
	Status         Status      `json:"status"`
	Total          int         `json:"total"`
	Processed      int         `json:"processed"`
	Changed        int         `json:"changed"`
	AlreadyOK      int         `json:"already_ok"`
	NotFound       int         `json:"not_found"`
	Failed         int         `json:"failed"`
	Error          string      `json:"error,omitempty"`
	Failures       []string    `json:"failures,omitempty"`
	AdminAccountID int64       `json:"admin_account_id"`
	StartedAt      time.Time   `json:"started_at"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
}

func (r *Run) clone() *Run {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Add = append([]uuid.UUID(nil), r.Add...)
	cp.Remove = append([]uuid.UUID(nil), r.Remove...)
	cp.Failures = append([]string(nil), r.Failures...)
	if r.FinishedAt != nil {
		t := *r.FinishedAt
		cp.FinishedAt = &t
	}
	return &cp
}

// Service применяет сквады тарифа к действующим подписчикам.
type Service struct {
	customers *database.CustomerRepository
	tariffs   *database.TariffRepository
	rw        *remnawave.Client

	mu   sync.Mutex
	runs map[int64]*Run
}

func New(customers *database.CustomerRepository, tariffs *database.TariffRepository, rw *remnawave.Client) *Service {
	return &Service{customers: customers, tariffs: tariffs, rw: rw, runs: make(map[int64]*Run)}
}

// Preview — что увидит админ до запуска: состав тарифа и сколько людей на нём.
type Preview struct {
	TariffSquads []uuid.UUID
	ActiveCount  int
	Run          *Run
}

// ResolveTariffSquads возвращает фактический состав тарифа.
//
// Пустая строка в active_internal_squad_uuids означает «все сквады панели»
// (см. filterSquadsByUUIDList) — здесь эта семантика разворачивается в явный
// список, чтобы дальше по коду не было второго места с этим правилом.
func (s *Service) ResolveTariffSquads(ctx context.Context, t *database.Tariff) ([]uuid.UUID, error) {
	if s.rw == nil {
		return nil, ErrPanelUnavailable
	}
	panelSquads, err := s.rw.ListInternalSquads(ctx)
	if err != nil {
		return nil, err
	}
	return resolveAgainstPanel(t, panelSquads)
}

func resolveAgainstPanel(t *database.Tariff, panelSquads []remnawave.InternalSquad) ([]uuid.UUID, error) {
	if t == nil {
		return nil, errors.New("nil tariff")
	}
	want, err := database.ParseSquadUUIDList(t.ActiveInternalSquadUUIDs)
	if err != nil {
		return nil, err
	}
	if len(want) == 0 {
		out := make([]uuid.UUID, 0, len(panelSquads))
		for _, sq := range panelSquads {
			out = append(out, sq.UUID)
		}
		return out, nil
	}
	known := make(map[uuid.UUID]struct{}, len(panelSquads))
	for _, sq := range panelSquads {
		known[sq.UUID] = struct{}{}
	}
	out := make([]uuid.UUID, 0, len(want))
	for _, u := range want {
		if _, ok := known[u]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// Preview собирает данные для диалога подтверждения.
func (s *Service) Preview(ctx context.Context, tariffID int64) (*Preview, error) {
	t, err := s.tariffs.GetByID(ctx, tariffID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("tariff %d not found", tariffID)
	}
	squads, err := s.ResolveTariffSquads(ctx, t)
	if err != nil {
		return nil, err
	}
	customers, err := s.customers.FindActiveByCurrentTariffID(ctx, tariffID)
	if err != nil {
		return nil, err
	}
	return &Preview{TariffSquads: squads, ActiveCount: len(customers), Run: s.RunStatus(tariffID)}, nil
}

// RunStatus — состояние последнего прогона по тарифу (или nil).
func (s *Service) RunStatus(tariffID int64) *Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[tariffID].clone()
}

// Start валидирует запрос и запускает применение в фоне.
//
// add/remove задаются явно, а не выводятся из тарифа: так одна ручка
// обслуживает и «сохранили тариф с изменённым составом» (add/remove = дифф),
// и «применить состав ко всем» (add = состав тарифа, remove пуст или
// «всё остальное»). Инварианты ниже не дают выдать сквад, которого в тарифе
// нет, и снять тот, который в нём есть.
func (s *Service) Start(ctx context.Context, tariffID int64, add, remove []uuid.UUID, adminAccountID int64) (*Run, error) {
	if s.rw == nil {
		return nil, ErrPanelUnavailable
	}
	add = dedupe(add)
	remove = dedupe(remove)
	if len(add) == 0 && len(remove) == 0 {
		return nil, ErrNoChanges
	}
	for _, u := range add {
		for _, r := range remove {
			if u == r {
				return nil, fmt.Errorf("%w: %s", ErrNoChanges, u)
			}
		}
	}

	t, err := s.tariffs.GetByID(ctx, tariffID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("tariff %d not found", tariffID)
	}

	panelSquads, err := s.rw.ListInternalSquads(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[uuid.UUID]struct{}, len(panelSquads))
	for _, sq := range panelSquads {
		known[sq.UUID] = struct{}{}
	}
	for _, u := range append(append([]uuid.UUID(nil), add...), remove...) {
		if _, ok := known[u]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSquad, u)
		}
	}

	tariffSquads, err := resolveAgainstPanel(t, panelSquads)
	if err != nil {
		return nil, err
	}
	inTariff := make(map[uuid.UUID]struct{}, len(tariffSquads))
	for _, u := range tariffSquads {
		inTariff[u] = struct{}{}
	}
	for _, u := range add {
		if _, ok := inTariff[u]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrAddNotInTariff, u)
		}
	}
	for _, u := range remove {
		if _, ok := inTariff[u]; ok {
			return nil, fmt.Errorf("%w: %s", ErrRemoveInTariff, u)
		}
	}

	customers, err := s.customers.FindActiveByCurrentTariffID(ctx, tariffID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if prev := s.runs[tariffID]; prev != nil && prev.Status == StatusRunning {
		s.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	run := &Run{
		TariffID:       tariffID,
		Add:            add,
		Remove:         remove,
		Status:         StatusRunning,
		Total:          len(customers),
		AdminAccountID: adminAccountID,
		StartedAt:      time.Now().UTC(),
	}
	s.runs[tariffID] = run
	s.mu.Unlock()

	slog.Info("tariff squads apply started",
		"tariff_id", tariffID, "add", uuidsToStrings(add), "remove", uuidsToStrings(remove),
		"active_customers", len(customers), "admin_account_id", adminAccountID)

	utils.Go("tariff squads apply", func() { s.process(run, customers) })

	return run.clone(), nil
}

func (s *Service) process(run *Run, customers []database.Customer) {
	// Свой контекст: HTTP-запрос, который запустил прогон, уже завершён.
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()

	defer func() {
		s.mu.Lock()
		now := time.Now().UTC()
		run.FinishedAt = &now
		if run.Status == StatusRunning {
			run.Status = StatusDone
		}
		s.mu.Unlock()
		slog.Info("tariff squads apply finished",
			"tariff_id", run.TariffID, "status", string(run.Status),
			"total", run.Total, "changed", run.Changed, "already_ok", run.AlreadyOK,
			"not_found", run.NotFound, "failed", run.Failed)
	}()

	if len(customers) == 0 {
		return
	}

	index, err := s.rw.BuildUserIndex(ctx)
	if err != nil {
		s.mu.Lock()
		run.Status = StatusFailed
		run.Error = err.Error()
		s.mu.Unlock()
		slog.Error("tariff squads apply: build panel index", "tariff_id", run.TariffID, "error", err.Error())
		return
	}

	jobs := make(chan database.Customer)
	var wg sync.WaitGroup
	for i := 0; i < applyConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cust := range jobs {
				// Паника на одном клиенте не должна останавливать воркер:
				// иначе отправитель заданий навсегда встал бы на канале.
				func() {
					defer utils.LogPanic("tariff squads apply customer")
					s.applyOne(ctx, run, index, cust)
				}()
			}
		}()
	}
	for _, cust := range customers {
		if ctx.Err() != nil {
			break
		}
		jobs <- cust
	}
	close(jobs)
	wg.Wait()

	if ctx.Err() != nil {
		s.mu.Lock()
		if run.Error == "" {
			run.Status = StatusFailed
			run.Error = ctx.Err().Error()
		}
		s.mu.Unlock()
	}
}

func (s *Service) applyOne(ctx context.Context, run *Run, index *remnawave.UserIndex, cust database.Customer) {
	rwUser := index.Find(cust.ID, cust.TelegramID, cust.SubscriptionLink, cust.IsWebOnly)
	if rwUser == nil {
		s.bump(run, func(r *Run) {
			r.Processed++
			r.NotFound++
		})
		slog.Warn("tariff squads apply: panel user not found", "tariff_id", run.TariffID, "customer_id", cust.ID)
		return
	}

	current := remnawave.SquadUUIDsOf(rwUser)
	next := remnawave.MergeSquads(current, run.Add, run.Remove)
	if remnawave.SameSquadSet(current, next) {
		s.bump(run, func(r *Run) {
			r.Processed++
			r.AlreadyOK++
		})
		return
	}

	if _, err := s.rw.SetUserSquads(ctx, rwUser.ID, next); err != nil {
		s.bump(run, func(r *Run) {
			r.Processed++
			r.Failed++
			if len(r.Failures) < maxFailureSamples {
				r.Failures = append(r.Failures, fmt.Sprintf("customer %d: %s", cust.ID, err.Error()))
			}
		})
		slog.Error("tariff squads apply: patch failed",
			"tariff_id", run.TariffID, "customer_id", cust.ID, "rw_user_id", rwUser.ID, "error", err.Error())
		return
	}

	s.bump(run, func(r *Run) {
		r.Processed++
		r.Changed++
	})
	slog.Info("tariff squads apply: patched",
		"tariff_id", run.TariffID, "customer_id", cust.ID, "rw_user_id", rwUser.ID,
		"before", uuidsToStrings(current), "after", uuidsToStrings(next))
}

func (s *Service) bump(run *Run, fn func(*Run)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(run)
}

func dedupe(in []uuid.UUID) []uuid.UUID {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(in))
	out := make([]uuid.UUID, 0, len(in))
	for _, u := range in {
		if u == uuid.Nil {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func uuidsToStrings(in []uuid.UUID) []string {
	out := make([]string, 0, len(in))
	for _, u := range in {
		out = append(out, u.String())
	}
	return out
}

// ValidateSquadsExist проверяет, что каждый UUID есть в панели.
//
// Возвращает nil, если панель недоступна: правка тарифа не должна падать
// из-за временной недоступности Remnawave — сверка лишь ловит опечатки
// и сквады, пересозданные в панели с новым UUID.
func (s *Service) ValidateSquadsExist(ctx context.Context, list []uuid.UUID) error {
	if s == nil || s.rw == nil || len(list) == 0 {
		return nil
	}
	panelSquads, err := s.rw.ListInternalSquads(ctx)
	if err != nil {
		slog.Warn("tariff squads validate: panel unavailable, skipping check", "error", err.Error())
		return nil
	}
	known := make(map[uuid.UUID]struct{}, len(panelSquads))
	for _, sq := range panelSquads {
		known[sq.UUID] = struct{}{}
	}
	for _, u := range list {
		if _, ok := known[u]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownSquad, u)
		}
	}
	return nil
}
