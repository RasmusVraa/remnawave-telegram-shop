//go:build integration

package database

import (
	"context"
	"testing"
	"time"
)

func trafficTestFixture(t *testing.T) (*TrafficSplitRepository, int64, int64, func()) {
	t.Helper()
	pool := partnerTestPool(t)
	ctx := context.Background()
	customers := NewCustomerRepository(pool)
	c, err := customers.FindOrCreate(ctx, &Customer{TelegramID: uniqueTelegramID(), Language: "ru"})
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	var tariffID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO tariff (slug, name, device_limit, traffic_limit_bytes, traffic_limit_reset_strategy)
		 VALUES ($1, 'trafficsplit-test', 3, 0, 'month') RETURNING id`, "ts-test-"+time.Now().Format("150405.000000000")).Scan(&tariffID); err != nil {
		t.Fatalf("tariff: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, c.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM tariff WHERE id = $1`, tariffID)
	}
	t.Cleanup(cleanup)
	return NewTrafficSplitRepository(pool), c.ID, tariffID, cleanup
}

// Один платёж не должен выдать пакет дважды: повтор вебхука или поллера — тот же purchase_id.
func TestTrafficExtra_sameOnePurchaseGrantsOnce(t *testing.T) {
	repo, customerID, _, _ := trafficTestFixture(t)
	ctx := context.Background()
	pool := partnerTestPool(t)
	var purchaseID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, purchase_kind)
		 VALUES (199, $1, 0, 'RUB', 'pending', 'yookasa', 'extra_traffic') RETURNING id`, customerID).Scan(&purchaseID); err != nil {
		t.Fatalf("purchase: %v", err)
	}
	until := TrafficPeriodEnd(time.Now())
	e := TrafficExtra{CustomerID: customerID, Kind: TrafficKindBypass, Bytes: 20 << 30, PurchaseID: &purchaseID, ValidUntil: until}

	first, err := repo.AddExtra(ctx, e)
	if err != nil || !first {
		t.Fatalf("first AddExtra = %v, %v; want true, nil", first, err)
	}
	second, err := repo.AddExtra(ctx, e)
	if err != nil || second {
		t.Fatalf("repeat AddExtra = %v, %v; want false, nil (пакет выдан дважды)", second, err)
	}
	sum, err := repo.ActiveExtraBytes(ctx, customerID, TrafficKindBypass, time.Now())
	if err != nil || sum != 20<<30 {
		t.Fatalf("ActiveExtraBytes = %d, %v; want %d", sum, err, int64(20<<30))
	}
	// Пакет обхода не засчитывается в обычный трафик.
	if main, _ := repo.ActiveExtraBytes(ctx, customerID, TrafficKindMain, time.Now()); main != 0 {
		t.Fatalf("main extra = %d, want 0", main)
	}
}

// Истёкший пакет не считается и удаляется воркером с указанием, у кого и какого вида.
func TestTrafficExtra_expiredIsDroppedAndReported(t *testing.T) {
	repo, customerID, _, _ := trafficTestFixture(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	if _, err := repo.AddExtra(ctx, TrafficExtra{CustomerID: customerID, Kind: TrafficKindMain, Bytes: 10 << 30, ValidUntil: past}); err != nil {
		t.Fatal(err)
	}
	if sum, _ := repo.ActiveExtraBytes(ctx, customerID, TrafficKindMain, time.Now()); sum != 0 {
		t.Fatalf("истёкший пакет посчитан: %d", sum)
	}
	expired, err := repo.ListExpiredExtras(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range expired {
		if e.CustomerID == customerID && e.Kind == TrafficKindMain {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListExpiredExtras не вернул клиента %d: %+v", customerID, expired)
	}
	// Удаляется только по явной команде (после того, как лимит в панели приведён в порядок).
	if again, _ := repo.ListExpiredExtras(ctx, time.Now()); len(again) == 0 {
		t.Fatal("истёкший пакет пропал до удаления")
	}
	if err := repo.DeleteExpiredExtrasFor(ctx, customerID, TrafficKindMain, time.Now()); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.ListExpiredExtras(ctx, time.Now()); len(again) != 0 {
		t.Fatalf("после удаления остались: %+v", again)
	}
}

// Пересохранение пакетов тарифа не меняет id существующих (открытая у клиента страница оплаты остаётся рабочей).
func TestReplacePacks_keepsIDsOfExistingSizes(t *testing.T) {
	repo, _, tariffID, _ := trafficTestFixture(t)
	ctx := context.Background()
	packs := []TrafficPack{
		{Kind: TrafficKindBypass, Bytes: 5 << 30, AmountRub: 100, IsActive: true},
		{Kind: TrafficKindBypass, Bytes: 10 << 30, AmountRub: 180, IsActive: true},
	}
	if err := repo.ReplacePacks(ctx, tariffID, packs); err != nil {
		t.Fatal(err)
	}
	before, _ := repo.ListPacks(ctx, tariffID, "", false)
	packs[0].AmountRub = 120
	packs[1] = TrafficPack{Kind: TrafficKindBypass, Bytes: 20 << 30, AmountRub: 300, IsActive: true}
	if err := repo.ReplacePacks(ctx, tariffID, packs); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListPacks(ctx, tariffID, "", false)
	if len(before) != 2 || len(after) != 2 {
		t.Fatalf("before=%d after=%d", len(before), len(after))
	}
	if after[0].ID != before[0].ID || after[0].AmountRub != 120 {
		t.Fatalf("пакет 5 ГБ потерял id или не обновил цену: %+v / %+v", before[0], after[0])
	}
	if after[1].Bytes != 20<<30 {
		t.Fatalf("второй пакет не заменён: %+v", after[1])
	}
	dup := []TrafficPack{packs[0], packs[0]}
	if err := repo.ReplacePacks(ctx, tariffID, dup); err == nil {
		t.Fatal("два пакета одного размера должны отклоняться")
	}
}

// Снимок и пакеты тарифа сохраняются одной транзакцией: ошибка в пакетах не оставляет половину.
func TestSaveTariffTraffic_isAtomic(t *testing.T) {
	repo, _, tariffID, _ := trafficTestFixture(t)
	ctx := context.Background()
	bad := []TrafficPack{{Kind: "weird", Bytes: 1, AmountRub: 1}}
	if err := repo.SaveTariffTraffic(ctx, tariffID, TariffBypass{LimitBytes: 7 << 30, SquadUUIDs: "x", NodeUUIDs: "y"}, bad); err == nil {
		t.Fatal("ожидалась ошибка")
	}
	b, _ := repo.GetTariffBypass(ctx, tariffID)
	if b.LimitBytes == 7<<30 {
		t.Fatal("настройки обхода сохранились, хотя пакеты отклонены")
	}
}

// Сброс предупреждений после докупки: тот же уровень снова может уйти.
func TestResetAlerts_allowsResend(t *testing.T) {
	repo, customerID, _, _ := trafficTestFixture(t)
	ctx := context.Background()
	period := TrafficPeriodStart(time.Now())
	_, _ = repo.TryRecordAlert(ctx, customerID, TrafficKindMain, period, 80)
	if err := repo.ResetAlerts(ctx, customerID, TrafficKindMain); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.TryRecordAlert(ctx, customerID, TrafficKindMain, period, 80); !ok {
		t.Fatal("после сброса уровень должен уходить снова")
	}
}

// Предупреждение каждого уровня уходит один раз за месяц.
func TestTrafficAlert_oncePerLevelPerPeriod(t *testing.T) {
	repo, customerID, _, _ := trafficTestFixture(t)
	ctx := context.Background()
	period := TrafficPeriodStart(time.Now())
	if ok, err := repo.TryRecordAlert(ctx, customerID, TrafficKindBypass, period, 80); err != nil || !ok {
		t.Fatalf("first = %v, %v", ok, err)
	}
	if ok, _ := repo.TryRecordAlert(ctx, customerID, TrafficKindBypass, period, 80); ok {
		t.Fatal("повторное предупреждение 80% в том же месяце")
	}
	if ok, _ := repo.TryRecordAlert(ctx, customerID, TrafficKindBypass, period, 100); !ok {
		t.Fatal("уровень 100% — отдельное предупреждение")
	}
	if ok, _ := repo.TryRecordAlert(ctx, customerID, TrafficKindMain, period, 80); !ok {
		t.Fatal("обычный трафик считается отдельно от обхода")
	}
	if ok, _ := repo.TryRecordAlert(ctx, customerID, TrafficKindBypass, period.AddDate(0, 1, 0), 80); !ok {
		t.Fatal("в новом месяце предупреждение снова доступно")
	}
}

// Снимок настроек обхода: смена тарифа в админке не трогает клиента, а «сквад снят» переживает
// продление на тариф с обходом и сбрасывается на тарифе без него.
func TestBypassSnapshot_keepsRemovedFlagOnlyWhileLimited(t *testing.T) {
	repo, customerID, tariffID, _ := trafficTestFixture(t)
	ctx := context.Background()
	now := time.Now()
	with := TariffBypass{LimitBytes: 30 << 30, SquadUUIDs: "9fb130a6-751c-4469-af27-929a0b97b072", NodeUUIDs: "73434c18-1b66-4253-982c-95c4e41fce60"}

	if err := repo.SaveSnapshot(ctx, customerID, &tariffID, with, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveUsage(ctx, customerID, TrafficPeriodStart(now), 31<<30, true, now); err != nil {
		t.Fatal(err)
	}
	// Админ потом поменял лимит тарифа — снимок клиента прежний, пока он не продлит.
	if err := repo.SetTariffBypass(ctx, tariffID, TariffBypass{LimitBytes: 100 << 30, SquadUUIDs: with.SquadUUIDs, NodeUUIDs: with.NodeUUIDs}); err != nil {
		t.Fatal(err)
	}
	st, err := repo.GetState(ctx, customerID)
	if err != nil || st == nil {
		t.Fatalf("state: %v %v", st, err)
	}
	if st.LimitBytes != 30<<30 {
		t.Fatalf("снимок поменялся без покупки: %d", st.LimitBytes)
	}
	// Продление: новый лимит из тарифа, расход и «сквад снят» сохраняются.
	tb, _ := repo.GetTariffBypass(ctx, tariffID)
	if err := repo.SaveSnapshot(ctx, customerID, &tariffID, tb, now); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.GetState(ctx, customerID)
	if st.LimitBytes != 100<<30 || !st.SquadRemoved || st.UsedBytes != 31<<30 {
		t.Fatalf("после продления: limit=%d removed=%v used=%d", st.LimitBytes, st.SquadRemoved, st.UsedBytes)
	}
	// Тариф без обхода — лимит 0, признак снят: профиль тарифа выдаст сквады заново.
	if err := repo.SaveSnapshot(ctx, customerID, &tariffID, TariffBypass{}, now); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.GetState(ctx, customerID)
	if st.LimitBytes != 0 || st.SquadRemoved {
		t.Fatalf("тариф без обхода: limit=%d removed=%v", st.LimitBytes, st.SquadRemoved)
	}
}

func TestTariffPacks_replaceKeepsKindsAndOrder(t *testing.T) {
	repo, _, tariffID, _ := trafficTestFixture(t)
	ctx := context.Background()
	stars := 150
	in := []TrafficPack{
		{Kind: TrafficKindBypass, Bytes: 5 << 30, AmountRub: 59, IsActive: true},
		{Kind: TrafficKindBypass, Bytes: 20 << 30, AmountRub: 199, AmountStars: &stars, IsActive: true},
		{Kind: TrafficKindMain, Bytes: 10 << 30, AmountRub: 49, IsActive: false},
	}
	if err := repo.ReplacePacks(ctx, tariffID, in); err != nil {
		t.Fatal(err)
	}
	all, _ := repo.ListPacks(ctx, tariffID, "", false)
	if len(all) != 3 {
		t.Fatalf("packs = %d, want 3", len(all))
	}
	active, _ := repo.ListPacks(ctx, tariffID, "", true)
	if len(active) != 2 {
		t.Fatalf("active packs = %d, want 2 (выключенный пакет не показывается клиенту)", len(active))
	}
	byp, _ := repo.ListPacks(ctx, tariffID, TrafficKindBypass, true)
	if len(byp) != 2 || byp[0].Bytes != 5<<30 || byp[1].AmountStars == nil || *byp[1].AmountStars != 150 {
		t.Fatalf("bypass packs = %+v", byp)
	}
	// Некорректный пакет отклоняется целиком, старый набор остаётся.
	if err := repo.ReplacePacks(ctx, tariffID, []TrafficPack{{Kind: "weird", Bytes: 1, AmountRub: 1}}); err == nil {
		t.Fatal("пакет с неизвестным видом принят")
	}
	if left, _ := repo.ListPacks(ctx, tariffID, "", false); len(left) != 3 {
		t.Fatalf("после отклонённой замены пакетов %d, want 3", len(left))
	}
}
