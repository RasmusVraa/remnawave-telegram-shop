//go:build integration

package database

// Интеграционные тесты партнёрской программы.
//
// Проверяют то, что юнит-тестами не проверить: саму миграцию 000044, поведение
// внешних ключей и частичных индексов, а главное — денежные операции, которые
// живут в SQL, а не в Go. Ошибка знака или перепутанная колонка в UPDATE стоит
// реальных денег, и поймать её должен тест, а не партнёр.
//
// Запуск:
//
//	CABINET_INTEGRATION_PG=postgres://user:pass@localhost:5432/dbname?sslmode=disable \
//	  go test ./internal/database/... -tags=integration -count=1

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func partnerMigrationsDir(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	// .../internal/database/partner_integration_test.go → корень репозитория (2× ..)
	return filepath.Clean(filepath.Join(filepath.Dir(f), "..", "..", "db", "migrations"))
}

func partnerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CABINET_INTEGRATION_PG")
	if dsn == "" {
		t.Skip("set CABINET_INTEGRATION_PG to run integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := RunMigrations(ctx, &MigrationConfig{
		Direction:      "up",
		MigrationsPath: partnerMigrationsDir(t),
		Steps:          0,
	}, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return pool
}

// testTelegramSeq делает идентификаторы уникальными в пределах прогона:
// времени недостаточно, два клиента подряд могут попасть в одну наносекунду, а
// ON CONFLICT в FindOrCreate тихо вернул бы того же клиента и тесты начали бы
// мешать друг другу.
var testTelegramSeq atomic.Int64

// uniqueTelegramID — идентификатор, не пересекающийся ни с реальными данными
// тестовой базы, ни с соседними прогонами. Отрицательный диапазон реальным
// пользователям Telegram не принадлежит.
func uniqueTelegramID() int64 {
	return -(time.Now().Unix()*1_000_000 + testTelegramSeq.Add(1))
}

// makeCustomer заводит клиента и убирает его вместе со всеми хвостами.
func makeCustomer(t *testing.T, pool *pgxpool.Pool) *Customer {
	t.Helper()
	ctx := context.Background()
	repo := NewCustomerRepository(pool)
	c, err := repo.Create(ctx, &Customer{TelegramID: uniqueTelegramID(), Language: "ru"})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		// Покупки не каскадятся от клиента — удаляем сами, иначе останется
		// висящий FK и следующий прогон будет падать на мусоре.
		_, _ = pool.Exec(bg, `DELETE FROM purchase WHERE customer_id = $1`, c.ID)
		_, _ = pool.Exec(bg, `DELETE FROM referral WHERE referee_id = $1 OR referrer_id = $1`, c.TelegramID)
		_, _ = pool.Exec(bg, `DELETE FROM customer WHERE id = $1`, c.ID)
	})
	return c
}

// makePartner заводит клиента, партнёра и его основную ссылку.
func makePartner(t *testing.T, pool *pgxpool.Pool, status string) (*PartnerRepository, *Partner, *PartnerLink) {
	t.Helper()
	ctx := context.Background()
	repo := NewPartnerRepository(pool)
	owner := makeCustomer(t, pool)

	p, err := repo.Create(ctx, &Partner{CustomerID: owner.ID, Status: status}, "Основная ссылка")
	if err != nil {
		t.Fatalf("create partner: %v", err)
	}
	link, err := repo.FindDefaultLink(ctx, p.ID)
	if err != nil || link == nil {
		t.Fatalf("default link: %v (link=%v)", err, link)
	}
	return repo, p, link
}

// makePaidPurchase кладёт оплаченную покупку — база для начисления.
func makePaidPurchase(t *testing.T, pool *pgxpool.Pool, customerID int64, amount float64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, paid_at)
VALUES ($1, $2, 1, 'RUB', 'paid', 'yookasa', now())
RETURNING id`, amount, customerID).Scan(&id)
	if err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	return id
}

func partnerBalances(t *testing.T, repo *PartnerRepository, partnerID int64) *Partner {
	t.Helper()
	p, err := repo.FindByID(context.Background(), partnerID)
	if err != nil || p == nil {
		t.Fatalf("reload partner: %v", err)
	}
	return p
}

func assertMoney(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Fatalf("%s = %.2f, want %.2f", name, got, want)
	}
}

// Закрепление держит три правила программы, и держит их SQL, а не вызывающий
// код: first touch, только клиенты без оплат, никакого пересечения с рефералкой.
func TestPartnerAttribution_guards(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)

	t.Run("first touch закрепляет один раз", func(t *testing.T) {
		client := makeCustomer(t, pool)
		attached, err := repo.AttachAttribution(ctx, client.ID, partner.ID, &link.ID, PartnerAttributionSourceTelegram)
		if err != nil || !attached {
			t.Fatalf("attach: attached=%v err=%v", attached, err)
		}
		// Повторный переход по чужой ссылке ничего не меняет.
		_, other, otherLink := makePartner(t, pool, PartnerStatusActive)
		again, err := repo.AttachAttribution(ctx, client.ID, other.ID, &otherLink.ID, PartnerAttributionSourceTelegram)
		if err != nil {
			t.Fatalf("second attach: %v", err)
		}
		if again {
			t.Fatal("клиент перезакреплён за другим партнёром — first touch нарушен")
		}
		got, err := repo.AttributionByCustomer(ctx, client.ID)
		if err != nil || got == nil {
			t.Fatalf("attribution: %v", err)
		}
		if got.PartnerID != partner.ID {
			t.Fatalf("attribution.partner_id = %d, want %d", got.PartnerID, partner.ID)
		}
	})

	t.Run("клиент с оплатой не закрепляется", func(t *testing.T) {
		client := makeCustomer(t, pool)
		makePaidPurchase(t, pool, client.ID, 1000)

		attached, err := repo.AttachAttribution(ctx, client.ID, partner.ID, &link.ID, PartnerAttributionSourceTelegram)
		if err != nil {
			t.Fatalf("attach: %v", err)
		}
		if attached {
			t.Fatal("закреплён клиент с историей оплат — партнёр получил бы процент с чужой базы")
		}
	})

	t.Run("реферал не закрепляется", func(t *testing.T) {
		referrer := makeCustomer(t, pool)
		client := makeCustomer(t, pool)
		if _, err := NewReferralRepository(pool).Create(ctx, referrer.TelegramID, client.TelegramID); err != nil {
			t.Fatalf("create referral: %v", err)
		}

		attached, err := repo.AttachAttribution(ctx, client.ID, partner.ID, &link.ID, PartnerAttributionSourceWeb)
		if err != nil {
			t.Fatalf("attach: %v", err)
		}
		if attached {
			t.Fatal("реферал закреплён за партнёром — за одну оплату заплатили бы дважды")
		}
	})
}

// Повторная обработка платежа не должна удваивать деньги: защита структурная,
// частичным уникальным индексом по purchase_id.
func TestPartnerEarning_idempotentByPurchase(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)
	client := makeCustomer(t, pool)
	purchaseID := makePaidPurchase(t, pool, client.ID, 2990)

	entry := PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &purchaseID, LinkID: &link.ID,
		BaseAmount: 2990, BaseCurrency: "RUB", BaseAmountRub: 2990,
		Percent: 40, Amount: 1196,
		Kind: PartnerEarningKindFirst, Status: PartnerEarningAvailable,
	}

	inserted, err := repo.InsertEarning(ctx, entry)
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	inserted, err = repo.InsertEarning(ctx, entry)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if inserted {
		t.Fatal("начисление за ту же покупку прошло дважды")
	}

	got := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance", got.Balance, 1196)
	assertMoney(t, "total_earned", got.TotalEarned, 1196)
}

// Раскрытие холда — перекладывание денег между двумя колонками одной строки.
// Здесь легче всего перепутать направление, поэтому проверяем обе половины.
func TestReleaseDueHolds_movesMoneyOutOfHold(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)
	client := makeCustomer(t, pool)

	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)

	dueID := makePaidPurchase(t, pool, client.ID, 2990)
	notDueID := makePaidPurchase(t, pool, client.ID, 1790)

	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &dueID, LinkID: &link.ID,
		Amount: 1196, Percent: 40, Kind: PartnerEarningKindFirst,
		Status: PartnerEarningHold, HoldUntil: &past,
	}); err != nil {
		t.Fatalf("insert due earning: %v", err)
	}
	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &notDueID, LinkID: &link.ID,
		Amount: 358, Percent: 20, Kind: PartnerEarningKindRenewal,
		Status: PartnerEarningHold, HoldUntil: &future,
	}); err != nil {
		t.Fatalf("insert pending earning: %v", err)
	}

	before := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "hold_balance до раскрытия", before.HoldBalance, 1554)
	assertMoney(t, "balance до раскрытия", before.Balance, 0)

	released, err := repo.ReleaseDueHolds(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	var releasedForPartner float64
	for _, r := range released {
		if r.PartnerID == partner.ID {
			releasedForPartner = r.Amount
			if r.Count != 1 {
				t.Fatalf("released count = %d, want 1", r.Count)
			}
		}
	}
	assertMoney(t, "сумма раскрытия", releasedForPartner, 1196)

	after := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance после раскрытия", after.Balance, 1196)
	assertMoney(t, "hold_balance после раскрытия", after.HoldBalance, 358)

	// Второй прогон ничего не должен трогать: выборка идёт по состоянию строк.
	againReleased, err := repo.ReleaseDueHolds(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	for _, r := range againReleased {
		if r.PartnerID == partner.ID {
			t.Fatalf("повторное раскрытие выдало ещё %.2f", r.Amount)
		}
	}
}

// Полный цикл выплаты: резерв при подаче, возврат при отказе, списание при
// выплате. Каждый переход двигает деньги, и ни один не должен их потерять.
func TestPartnerPayout_lifecycle(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)
	client := makeCustomer(t, pool)
	purchaseID := makePaidPurchase(t, pool, client.ID, 5000)

	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &purchaseID, LinkID: &link.ID,
		Amount: 2000, Percent: 40, Kind: PartnerEarningKindFirst, Status: PartnerEarningAvailable,
	}); err != nil {
		t.Fatalf("insert earning: %v", err)
	}

	method, details := "sbp", "+7 999 000 00 00"
	payout, err := repo.CreatePayout(ctx, partner.ID, 1500, &method, &details)
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}

	reserved := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance после резерва", reserved.Balance, 500)
	assertMoney(t, "reserved_balance", reserved.ReservedBalance, 1500)

	// Вторая заявка на остаток сверх баланса не проходит.
	if _, err := repo.CreatePayout(ctx, partner.ID, 1000, &method, &details); !errors.Is(err, ErrPartnerInsufficientBalance) {
		t.Fatalf("вторая заявка сверх баланса: want ErrPartnerInsufficientBalance, got %v", err)
	}

	rejected, err := repo.RejectPayout(ctx, payout.ID, "неверные реквизиты", 1)
	if err != nil {
		t.Fatalf("reject payout: %v", err)
	}
	// Заявку возвращают вызывающей стороне ради уведомления партнёру: без неё
	// пришлось бы читать её повторно, уже после смены статуса.
	if rejected == nil || rejected.Status != PartnerPayoutRejected || rejected.PartnerID != partner.ID {
		t.Fatalf("reject payout вернул %+v", rejected)
	}
	returned := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance после отказа", returned.Balance, 2000)
	assertMoney(t, "reserved_balance после отказа", returned.ReservedBalance, 0)
	assertMoney(t, "total_paid после отказа", returned.TotalPaid, 0)

	// Отклонённая заявка не запирает партнёра на кулдаун.
	last, err := repo.LastPayoutRequestAt(ctx, partner.ID)
	if err != nil {
		t.Fatalf("last payout: %v", err)
	}
	if last != nil {
		t.Fatal("отклонённая заявка учтена как последняя — кулдаун сработал бы после отказа")
	}

	second, err := repo.CreatePayout(ctx, partner.ID, 2000, &method, &details)
	if err != nil {
		t.Fatalf("second payout: %v", err)
	}
	closed, err := repo.MarkPayoutPaid(ctx, second.ID, "чек #4471", "", 1)
	if err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if closed == nil || closed.ExternalRef == nil || *closed.ExternalRef != "чек #4471" {
		t.Fatalf("mark paid вернул %+v", closed)
	}
	paid := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance после выплаты", paid.Balance, 0)
	assertMoney(t, "reserved_balance после выплаты", paid.ReservedBalance, 0)
	assertMoney(t, "total_paid после выплаты", paid.TotalPaid, 2000)

	// Повторная обработка закрытой заявки не должна списать деньги ещё раз.
	if _, err := repo.MarkPayoutPaid(ctx, second.ID, "", "", 1); !errors.Is(err, ErrPartnerPayoutClosed) {
		t.Fatalf("повтор выплаты: want ErrPartnerPayoutClosed, got %v", err)
	}
}

// Заявка на вывод от замороженного партнёра не проходит: заморозка блокирует
// именно вывод.
func TestPartnerPayout_suspendedCannotWithdraw(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusSuspended)
	client := makeCustomer(t, pool)
	purchaseID := makePaidPurchase(t, pool, client.ID, 5000)

	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &purchaseID, LinkID: &link.ID,
		Amount: 2000, Kind: PartnerEarningKindFirst, Status: PartnerEarningAvailable,
	}); err != nil {
		t.Fatalf("insert earning: %v", err)
	}

	if _, err := repo.CreatePayout(ctx, partner.ID, 1000, nil, nil); !errors.Is(err, ErrPartnerInsufficientBalance) {
		t.Fatalf("вывод у замороженного: want ErrPartnerInsufficientBalance, got %v", err)
	}
}

// Ручная правка баланса пишется в тот же журнал и не даёт уйти в минус.
func TestAdjustBalance(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, _ := makePartner(t, pool, PartnerStatusActive)

	if err := repo.AdjustBalance(ctx, partner.ID, 500, "компенсация", 1); err != nil {
		t.Fatalf("positive adjust: %v", err)
	}
	assertMoney(t, "balance после начисления", partnerBalances(t, repo, partner.ID).Balance, 500)

	if err := repo.AdjustBalance(ctx, partner.ID, -200, "возврат клиенту", 1); err != nil {
		t.Fatalf("negative adjust: %v", err)
	}
	assertMoney(t, "balance после списания", partnerBalances(t, repo, partner.ID).Balance, 300)

	if err := repo.AdjustBalance(ctx, partner.ID, -1000, "слишком много", 1); !errors.Is(err, ErrPartnerBalanceTooLow) {
		t.Fatalf("списание больше остатка: want ErrPartnerBalanceTooLow, got %v", err)
	}
	assertMoney(t, "balance не изменился", partnerBalances(t, repo, partner.ID).Balance, 300)

	ops, total, err := repo.ListOperations(ctx, partner.ID, 10, 0)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("в журнале %d операций, want 2 — правки должны попадать в ленту", len(ops))
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 — счётчик вкладки берётся из него", total)
	}

	// Вторая страница: смещение отрезает первую строку, total от него не зависит.
	page2, total2, err := repo.ListOperations(ctx, partner.ID, 10, 1)
	if err != nil {
		t.Fatalf("list operations offset: %v", err)
	}
	if len(page2) != 1 || total2 != 2 {
		t.Fatalf("страница со смещением: %d строк при total %d, want 1 при 2", len(page2), total2)
	}
	if !page2[0].At.Equal(ops[1].At) {
		t.Fatalf("смещение сдвинуло не на ту строку: %v, want %v", page2[0].At, ops[1].At)
	}
}

// Отмена начисления снимает деньги оттуда, куда они попали.
func TestCancelEarning_revertsFromRightBucket(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)
	client := makeCustomer(t, pool)

	holdPurchase := makePaidPurchase(t, pool, client.ID, 2990)
	availablePurchase := makePaidPurchase(t, pool, client.ID, 1790)
	future := time.Now().UTC().Add(72 * time.Hour)

	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &holdPurchase, LinkID: &link.ID,
		Amount: 1196, Kind: PartnerEarningKindFirst, Status: PartnerEarningHold, HoldUntil: &future,
	}); err != nil {
		t.Fatalf("insert hold earning: %v", err)
	}
	if _, err := repo.InsertEarning(ctx, PartnerEarning{
		PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &availablePurchase, LinkID: &link.ID,
		Amount: 358, Kind: PartnerEarningKindRenewal, Status: PartnerEarningAvailable,
	}); err != nil {
		t.Fatalf("insert available earning: %v", err)
	}

	rows, _, err := repo.ListEarnings(ctx, partner.ID, 10, 0)
	if err != nil {
		t.Fatalf("list earnings: %v", err)
	}
	var holdID, availableID int64
	for _, row := range rows {
		if row.Status == PartnerEarningHold {
			holdID = row.ID
		} else {
			availableID = row.ID
		}
	}

	if err := repo.CancelEarning(ctx, holdID, "возврат платежа"); err != nil {
		t.Fatalf("cancel hold earning: %v", err)
	}
	afterHold := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "hold_balance после отмены холда", afterHold.HoldBalance, 0)
	assertMoney(t, "balance не тронут", afterHold.Balance, 358)

	if err := repo.CancelEarning(ctx, availableID, "спорный платёж"); err != nil {
		t.Fatalf("cancel available earning: %v", err)
	}
	afterAvailable := partnerBalances(t, repo, partner.ID)
	assertMoney(t, "balance после отмены доступного", afterAvailable.Balance, 0)

	// Повтор отмены безопасен.
	if err := repo.CancelEarning(ctx, availableID, ""); err != nil {
		t.Fatalf("repeat cancel: %v", err)
	}
	assertMoney(t, "balance после повтора", partnerBalances(t, repo, partner.ID).Balance, 0)
}

// Пустой поток удаляется, поток с историей — только в архив. Правило держат и
// код, и внешние ключи.
func TestPartnerLinks_deleteAndArchiveRules(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, defaultLink := makePartner(t, pool, PartnerStatusActive)

	empty, err := repo.CreateLink(ctx, partner.ID, "Пустой поток")
	if err != nil {
		t.Fatalf("create empty link: %v", err)
	}
	used, err := repo.CreateLink(ctx, partner.ID, "Рабочий поток")
	if err != nil {
		t.Fatalf("create used link: %v", err)
	}

	client := makeCustomer(t, pool)
	if attached, err := repo.AttachAttribution(ctx, client.ID, partner.ID, &used.ID, PartnerAttributionSourceTelegram); err != nil || !attached {
		t.Fatalf("attach to used link: attached=%v err=%v", attached, err)
	}

	if err := repo.DeleteLink(ctx, partner.ID, empty.ID); err != nil {
		t.Fatalf("delete empty link: %v", err)
	}
	if err := repo.DeleteLink(ctx, partner.ID, used.ID); !errors.Is(err, ErrPartnerLinkHasHistory) {
		t.Fatalf("delete used link: want ErrPartnerLinkHasHistory, got %v", err)
	}
	if err := repo.DeleteLink(ctx, partner.ID, defaultLink.ID); !errors.Is(err, ErrPartnerLinkIsDefault) {
		t.Fatalf("delete default link: want ErrPartnerLinkIsDefault, got %v", err)
	}

	// Архив закрывает ссылку для новых, но не отвязывает уже пришедших.
	if err := repo.SetLinkArchived(ctx, partner.ID, used.ID, true, 10); err != nil {
		t.Fatalf("archive link: %v", err)
	}
	if resolved, err := repo.ResolveLinkCode(ctx, used.Code); err != nil || resolved != nil {
		t.Fatalf("архивная ссылка резолвится: resolved=%v err=%v", resolved, err)
	}
	attribution, err := repo.AttributionByCustomer(ctx, client.ID)
	if err != nil || attribution == nil {
		t.Fatalf("клиент отвязался при архивации: %v", err)
	}

	// Архивные потоки не занимают лимит.
	count, err := repo.CountLinks(ctx, partner.ID)
	if err != nil {
		t.Fatalf("count links: %v", err)
	}
	if count != 1 {
		t.Fatalf("рабочих потоков %d, want 1 (архивный не должен считаться)", count)
	}
}

// Возврат потока из архива не должен обходить лимит: архивные потоки его не
// занимают, поэтому цикл «заархивировать → создать новый → вернуть» иначе
// позволял бы завести сколько угодно рабочих ссылок.
func TestPartnerLinks_restoreRespectsLimit(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, _ := makePartner(t, pool, PartnerStatusActive)

	// Лимит 2: основная ссылка плюс один поток.
	const limit = 2
	first, err := repo.CreateLink(ctx, partner.ID, "Поток 1")
	if err != nil {
		t.Fatalf("create first link: %v", err)
	}
	if err := repo.SetLinkArchived(ctx, partner.ID, first.ID, true, limit); err != nil {
		t.Fatalf("archive first link: %v", err)
	}

	// Место освободилось — заводим второй поток.
	if _, err := repo.CreateLink(ctx, partner.ID, "Поток 2"); err != nil {
		t.Fatalf("create second link: %v", err)
	}
	count, err := repo.CountLinks(ctx, partner.ID)
	if err != nil {
		t.Fatalf("count links: %v", err)
	}
	if count != limit {
		t.Fatalf("рабочих потоков %d, want %d", count, limit)
	}

	// Возврат первого из архива превысил бы лимит.
	if err := repo.SetLinkArchived(ctx, partner.ID, first.ID, false, limit); !errors.Is(err, ErrPartnerLinkLimitReached) {
		t.Fatalf("возврат из архива сверх лимита: want ErrPartnerLinkLimitReached, got %v", err)
	}
	if count, _ := repo.CountLinks(ctx, partner.ID); count != limit {
		t.Fatalf("лимит обойдён: рабочих потоков %d", count)
	}
}

// Чужой поток недоступен: проверка владения встроена в запросы.
func TestPartnerLinks_ownershipIsEnforced(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, alice, _ := makePartner(t, pool, PartnerStatusActive)
	_, bob, bobLink := makePartner(t, pool, PartnerStatusActive)

	if err := repo.RenameLink(ctx, alice.ID, bobLink.ID, "чужое"); !errors.Is(err, ErrPartnerLinkNotFound) {
		t.Fatalf("переименование чужого потока: want ErrPartnerLinkNotFound, got %v", err)
	}
	if err := repo.DeleteLink(ctx, alice.ID, bobLink.ID); !errors.Is(err, ErrPartnerLinkNotFound) {
		t.Fatalf("удаление чужого потока: want ErrPartnerLinkNotFound, got %v", err)
	}
	link, err := repo.FindDefaultLink(ctx, bob.ID)
	if err != nil || link == nil {
		t.Fatalf("поток Боба пропал: %v", err)
	}
}

// Заявка: подать можно один раз, повторно — только после отказа.
func TestSubmitApplication_transitions(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo := NewPartnerRepository(pool)
	customer := makeCustomer(t, pool)

	first, err := repo.SubmitApplication(ctx, customer.ID, "о себе", "@channel", "30", false)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if first.Status != PartnerStatusPending {
		t.Fatalf("status = %s, want pending", first.Status)
	}
	if link, err := repo.FindDefaultLink(ctx, first.ID); err != nil || link == nil {
		t.Fatalf("основная ссылка не создана вместе с заявкой: %v", err)
	}

	if _, err := repo.SubmitApplication(ctx, customer.ID, "ещё раз", "", "", false); !errors.Is(err, ErrPartnerApplicationPending) {
		t.Fatalf("повторная подача: want ErrPartnerApplicationPending, got %v", err)
	}

	if _, err := repo.RejectApplication(ctx, first.ID, "не подходит", 1); err != nil {
		t.Fatalf("reject: %v", err)
	}
	again, err := repo.SubmitApplication(ctx, customer.ID, "исправился", "@channel2", "50", false)
	if err != nil {
		t.Fatalf("resubmit after reject: %v", err)
	}
	if again.Status != PartnerStatusPending {
		t.Fatalf("status после переподачи = %s, want pending", again.Status)
	}
	if again.AdminNote != nil {
		t.Fatal("комментарий к отклонённой заявке остался после переподачи")
	}

	approved, err := repo.ApproveApplication(ctx, again.ID, nil, nil, "ок", 1)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != PartnerStatusActive || approved.ApprovedAt == nil {
		t.Fatalf("после одобрения status=%s approved_at=%v", approved.Status, approved.ApprovedAt)
	}
	if _, err := repo.SubmitApplication(ctx, customer.ID, "снова", "", "", false); !errors.Is(err, ErrPartnerAlreadyActive) {
		t.Fatalf("подача действующим партнёром: want ErrPartnerAlreadyActive, got %v", err)
	}
}

// Балансы в partner — производные от журнала. После полного цикла операций
// расхождения быть не должно: именно это делает цифры в кабинете доверенными.
func TestCheckBalances_noDriftAfterFullCycle(t *testing.T) {
	ctx := context.Background()
	pool := partnerTestPool(t)
	repo, partner, link := makePartner(t, pool, PartnerStatusActive)
	client := makeCustomer(t, pool)

	past := time.Now().UTC().Add(-time.Hour)
	for i, amount := range []float64{1196, 358, 598} {
		purchaseID := makePaidPurchase(t, pool, client.ID, amount*2)
		status, hold := PartnerEarningHold, &past
		if i == 2 {
			status, hold = PartnerEarningAvailable, nil
		}
		if _, err := repo.InsertEarning(ctx, PartnerEarning{
			PartnerID: partner.ID, CustomerID: &client.ID, PurchaseID: &purchaseID, LinkID: &link.ID,
			Amount: amount, Kind: PartnerEarningKindRenewal, Status: status, HoldUntil: hold,
		}); err != nil {
			t.Fatalf("insert earning %d: %v", i, err)
		}
	}
	if _, err := repo.ReleaseDueHolds(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("release: %v", err)
	}
	method := "sbp"
	payout, err := repo.CreatePayout(ctx, partner.ID, 1000, &method, &method)
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}
	if _, err := repo.MarkPayoutPaid(ctx, payout.ID, "ref", "", 1); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if err := repo.AdjustBalance(ctx, partner.ID, -100, "возврат", 1); err != nil {
		t.Fatalf("adjust: %v", err)
	}

	checks, err := repo.CheckBalances(ctx)
	if err != nil {
		t.Fatalf("check balances: %v", err)
	}
	found := false
	for _, c := range checks {
		if c.PartnerID != partner.ID {
			continue
		}
		found = true
		if math.Abs(c.Drift) > 0.005 {
			t.Fatalf("расхождение баланса с журналом: %+v", c)
		}
	}
	if !found {
		t.Fatalf("партнёр %d не попал в сверку балансов", partner.ID)
	}
}
