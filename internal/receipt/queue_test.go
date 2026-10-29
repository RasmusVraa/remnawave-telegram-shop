package receipt

import (
	"context"
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// --- заглушки ---------------------------------------------------------------

type fakeSender struct {
	err error
	// gotOperationTime — с каким временем операции ушёл чек.
	gotOperationTime time.Time
	calls            int
}

func (f *fakeSender) CreateIncome(_ context.Context, _ float64, _ string, operationTime time.Time) (string, error) {
	f.calls++
	f.gotOperationTime = operationTime
	if f.err != nil {
		return "", f.err
	}
	return "receipt-1", nil
}

type fakeStore struct {
	row *database.MoynalogReceipt
	// taken — строку уже захватил кто-то другой.
	taken bool

	sentReceiptID string
	markedSent    bool
	markedFailed  bool
	markedAlerted bool
	attemptFailed bool
	nextAttemptAt time.Time
}

func (s *fakeStore) Enqueue(_ context.Context, purchaseID int64, amount float64, description string, operationTime time.Time) (*database.MoynalogReceipt, error) {
	if s.row != nil {
		return s.row, nil
	}
	s.row = &database.MoynalogReceipt{
		ID: 1, PurchaseID: purchaseID, Amount: amount, Description: description,
		OperationTime: operationTime, Status: database.MoynalogReceiptPending,
	}
	return s.row, nil
}

func (s *fakeStore) Claim(_ context.Context, _ int64, _ time.Time) (bool, error) {
	return !s.taken, nil
}

func (s *fakeStore) MarkSent(_ context.Context, _ int64, receiptID string) error {
	s.markedSent = true
	s.sentReceiptID = receiptID
	return nil
}

func (s *fakeStore) MarkAttemptFailed(_ context.Context, _ int64, _ string, nextAttemptAt time.Time) error {
	s.attemptFailed = true
	s.nextAttemptAt = nextAttemptAt
	return nil
}

func (s *fakeStore) MarkFailed(_ context.Context, _ int64, _ string) error {
	s.markedFailed = true
	return nil
}

func (s *fakeStore) MarkAlerted(_ context.Context, _ int64) error {
	s.markedAlerted = true
	return nil
}

func (s *fakeStore) ListDue(_ context.Context, _ int) ([]database.MoynalogReceipt, error) {
	if s.row == nil {
		return nil, nil
	}
	return []database.MoynalogReceipt{*s.row}, nil
}

func (s *fakeStore) CountPending(_ context.Context) (int64, error) { return 1, nil }

func newTestService(store receiptStore, sender incomeSender, maxAge time.Duration) *Service {
	// bot=nil, adminID=0 — уведомления в тестах просто не уходят.
	return NewService(store, sender, nil, 0,
		func() bool { return true },
		func() time.Duration { return maxAge })
}

// --- тесты ------------------------------------------------------------------

// Главное свойство: у ФНС доход должен регистрироваться ВРЕМЕНЕМ ОПЛАТЫ, а не
// временем отправки. Иначе чек, отправленный после многодневного простоя,
// уедет не в тот день, а на стыке месяцев — не в тот налоговый период.
func TestSubmitUsesPaidAtAsOperationTime(t *testing.T) {
	paidAt := time.Date(2026, 8, 29, 12, 30, 0, 0, time.UTC)
	purchase := &database.Purchase{ID: 1192, Amount: 147, PaidAt: &paidAt}

	sender := &fakeSender{}
	store := &fakeStore{}
	svc := newTestService(store, sender, 720*time.Hour)
	svc.Submit(context.Background(), purchase, "Подписка на 1 месяц")
	svc.waitInflight()

	if !sender.gotOperationTime.Equal(paidAt) {
		t.Fatalf("operationTime = %v, ожидалось время оплаты %v", sender.gotOperationTime, paidAt)
	}
	if !store.markedSent {
		t.Fatal("успешная отправка должна закрывать строку очереди")
	}
	if store.sentReceiptID != "receipt-1" {
		t.Fatalf("receipt_id = %q, ожидался ID от ФНС", store.sentReceiptID)
	}
}

// Провал отправки не должен терять чек: строка остаётся в очереди с датой
// следующей попытки, а админу уходит ровно один сигнал.
func TestSubmitFailureSchedulesRetry(t *testing.T) {
	paidAt := time.Now().Add(-time.Hour)
	purchase := &database.Purchase{ID: 1192, Amount: 147, PaidAt: &paidAt}

	sender := &fakeSender{err: errors.New("entity.not.found")}
	store := &fakeStore{}
	svc := newTestService(store, sender, 720*time.Hour)
	svc.Submit(context.Background(), purchase, "Подписка")
	svc.waitInflight()

	if store.markedSent {
		t.Fatal("неуспешный чек не должен помечаться отправленным")
	}
	if !store.attemptFailed {
		t.Fatal("неудача должна фиксироваться с переносом следующей попытки")
	}
	if !store.nextAttemptAt.After(time.Now()) {
		t.Fatalf("next_attempt_at = %v, ожидалось будущее время", store.nextAttemptAt)
	}
	if !store.markedAlerted {
		t.Fatal("по первой неудаче админу должен уйти сигнал")
	}
	if store.markedFailed {
		t.Fatal("чек не исчерпал предельный срок — снимать с повторов рано")
	}
}

// Уже закрытый чек повторно не отправляем: у API «Мой налог» нет ключа
// идемпотентности, так что вторая отправка создала бы дубль дохода.
func TestSubmitSkipsAlreadySettledReceipt(t *testing.T) {
	paidAt := time.Now()
	purchase := &database.Purchase{ID: 1192, Amount: 147, PaidAt: &paidAt}

	sender := &fakeSender{}
	store := &fakeStore{row: &database.MoynalogReceipt{
		ID: 1, PurchaseID: 1192, Status: database.MoynalogReceiptSent,
	}}
	svc := newTestService(store, sender, 720*time.Hour)
	svc.Submit(context.Background(), purchase, "Подписка")
	svc.waitInflight()

	if sender.calls != 0 {
		t.Fatalf("отправок = %d, ожидалось 0 для уже закрытого чека", sender.calls)
	}
}

// За предельным сроком повторы прекращаются — доход придётся внести вручную.
func TestTrySendGivesUpAfterMaxAge(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	store := &fakeStore{}
	sender := &fakeSender{err: errors.New("ещё лежит")}

	svc := newTestService(store, sender, 24*time.Hour)
	svc.trySend(context.Background(), database.MoynalogReceipt{
		ID: 1, PurchaseID: 1192, Amount: 147, OperationTime: old, Status: database.MoynalogReceiptPending,
	})

	if !store.markedFailed {
		t.Fatal("чек старше предельного срока должен сниматься с повторов")
	}
	if store.attemptFailed {
		t.Fatal("снятый чек не должен получать новую дату попытки")
	}
}

// Простой ФНС длится днями, поэтому пауза растёт, но упирается в потолок —
// иначе повторы разъехались бы на недели и чек завис бы дольше простоя.
func TestBackoffGrowsAndCaps(t *testing.T) {
	if got := backoffDelay(0); got != time.Minute {
		t.Fatalf("первая пауза = %v, ожидалась 1м", got)
	}
	if got := backoffDelay(3); got != 8*time.Minute {
		t.Fatalf("четвёртая пауза = %v, ожидалось 8м", got)
	}
	for _, attempts := range []int{6, 7, 50, 1000} {
		if got := backoffDelay(attempts); got != time.Hour {
			t.Fatalf("backoffDelay(%d) = %v, ожидался потолок в 1ч", attempts, got)
		}
	}
}

// ProcessDue должен переваривать пустую очередь без обращения к API.
func TestProcessDueEmptyQueue(t *testing.T) {
	sender := &fakeSender{}
	svc := newTestService(&fakeStore{}, sender, time.Hour)
	if err := svc.ProcessDue(context.Background()); err != nil {
		t.Fatalf("ProcessDue вернул ошибку на пустой очереди: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("отправок = %d, ожидалось 0", sender.calls)
	}
}

// Выключение «Мой налог» из админки должно действовать сразу, без рестарта:
// сервис спрашивает настройку на каждом обращении, а не запоминает при старте.
func TestEnabledFollowsRuntimeSetting(t *testing.T) {
	on := true
	store := &fakeStore{}
	sender := &fakeSender{}
	svc := NewService(store, sender, nil, 0,
		func() bool { return on },
		func() time.Duration { return time.Hour })

	if !svc.Enabled() {
		t.Fatal("при включённой настройке очередь должна быть активна")
	}

	on = false
	if svc.Enabled() {
		t.Fatal("выключение настройки должно действовать без пересоздания сервиса")
	}

	paidAt := time.Now()
	svc.Submit(context.Background(), &database.Purchase{ID: 1, Amount: 10, PaidAt: &paidAt}, "Подписка")
	svc.waitInflight()
	if sender.calls != 0 {
		t.Fatalf("отправок = %d, при выключенной интеграции ожидалось 0", sender.calls)
	}
}

// Выключенная интеграция не должна ронять обработку оплаты.
func TestDisabledServiceIsSafe(t *testing.T) {
	var svc *Service
	if svc.Enabled() {
		t.Fatal("nil-сервис не может быть Enabled")
	}
	paidAt := time.Now()
	svc.Submit(context.Background(), &database.Purchase{ID: 1, PaidAt: &paidAt}, "x")
	svc.waitInflight()
	if err := svc.ProcessDue(context.Background()); err != nil {
		t.Fatalf("ProcessDue на nil-сервисе вернул ошибку: %v", err)
	}
}

// Чек, который уже отправляет другая попытка, второй раз в ФНС не уходит.
func TestTrySendSkipsReceiptClaimedElsewhere(t *testing.T) {
	sender := &fakeSender{}
	store := &fakeStore{
		taken: true,
		row: &database.MoynalogReceipt{
			ID: 1, PurchaseID: 1192, Amount: 147, Status: database.MoynalogReceiptPending,
			OperationTime: time.Now(),
		},
	}
	svc := newTestService(store, sender, 720*time.Hour)
	if err := svc.ProcessDue(context.Background()); err != nil {
		t.Fatalf("ProcessDue: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("CreateIncome called %d times for a claimed receipt, want 0", sender.calls)
	}
	if store.markedSent || store.attemptFailed {
		t.Fatal("claimed receipt must not be touched")
	}
}
