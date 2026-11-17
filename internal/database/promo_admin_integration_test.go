//go:build integration

package database

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// promoTestToken — уникальная метка прогона: попадает в код и заметку каждого
// тестового промокода, поиск по ней отсекает чужие строки общей тестовой базы.
func promoTestToken() string {
	return fmt.Sprintf("ZQ%d", time.Now().UnixNano()%1_000_000_000_000)
}

// makePromo создаёт промокод и удаляет его после теста (активации и
// невыкупленные скидки уходят каскадом).
func makePromo(t *testing.T, pool *pgxpool.Pool, p PromoCode) int64 {
	t.Helper()
	ctx := context.Background()
	repo := NewPromoRepository(pool)
	if p.DiscountMaxSubscriptionPaymentsPerCustomer == 0 {
		p.DiscountMaxSubscriptionPaymentsPerCustomer = 1
	}
	id, err := repo.Create(ctx, &p)
	if err != nil {
		t.Fatalf("create promo %s: %v", p.Code, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM promo_code WHERE id = $1`, id) })
	return id
}

func promoCodes(list []PromoCode) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Code)
	}
	return out
}

func sortedCodes(list []PromoCode) []string {
	out := promoCodes(list)
	sort.Strings(out)
	return out
}

func codesWith(token string, suffixes ...string) []string {
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, token+s)
	}
	sort.Strings(out)
	return out
}

func TestPromoListFiltered_statusTypeCounts(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPromoRepository(pool)
	ctx := context.Background()
	tok := promoTestToken()
	ip := func(v int) *int { return &v }
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	type spec struct {
		suffix, typ string
		active      bool
		maxUses     *int
		uses        int
		validUntil  *time.Time
	}
	specs := []spec{
		{"A", PromoTypeSubscriptionDays, true, nil, 4, nil},     // active, без лимита
		{"Z0", PromoTypeDiscount, true, ip(0), 5, nil},          // max_uses 0 = безлимит → active
		{"FUT", PromoTypeDiscount, true, ip(10), 2, &future},    // active со сроком
		{"EXP", PromoTypeTrial, true, nil, 0, &past},            // expired → ended
		{"EXPOFF", PromoTypeExtraHwid, false, nil, 0, &past},    // истёк важнее выключен → ended
		{"EXH", PromoTypeSubscriptionDays, true, ip(3), 3, nil}, // exhausted → ended
		{"OFF", PromoTypeDiscount, false, nil, 0, &future},      // off
		{"EXHOFF", PromoTypeTrial, false, ip(1), 1, nil},        // выключен важнее исчерпан → off
	}
	for _, s := range specs {
		makePromo(t, pool, PromoCode{
			Code: tok + s.suffix, Type: s.typ, Active: s.active, MaxUses: s.maxUses,
			UsesCount: s.uses, ValidUntil: s.validUntil,
		})
	}
	// Служебный код с той же меткой в выдачу не попадает.
	makePromo(t, pool, PromoCode{Code: "__" + tok + "TECH", Type: PromoTypeDiscount, Active: true})

	list := func(f PromoListFilter) ([]PromoCode, PromoListCounts) {
		t.Helper()
		if f.Query == "" {
			f.Query = tok
		}
		items, counts, err := repo.ListFiltered(ctx, f, 0, 100)
		if err != nil {
			t.Fatalf("ListFiltered %+v: %v", f, err)
		}
		return items, counts
	}

	items, counts := list(PromoListFilter{Status: PromoStatusAll})
	if got := len(items); got != 8 {
		t.Fatalf("all: %d items %v", got, promoCodes(items))
	}
	if want := (PromoStatusCounts{All: 8, Active: 3, Ended: 3, Off: 2}); counts.Status != want {
		t.Fatalf("status counts %+v, want %+v", counts.Status, want)
	}
	if want := (PromoTypeCounts{All: 8, SubscriptionDays: 2, Discount: 3, Trial: 2, ExtraHwid: 1}); counts.Type != want {
		t.Fatalf("type counts %+v, want %+v", counts.Type, want)
	}

	byStatus := map[string][]string{
		PromoStatusActive: codesWith(tok, "A", "Z0", "FUT"),
		PromoStatusEnded:  codesWith(tok, "EXP", "EXPOFF", "EXH"),
		PromoStatusOff:    codesWith(tok, "OFF", "EXHOFF"),
	}
	for status, want := range byStatus {
		items, counts := list(PromoListFilter{Status: status})
		if got := sortedCodes(items); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("status %s: %v, want %v", status, got, want)
		}
		// Счётчики статуса не зависят от выбранного статуса.
		if counts.Status.All != 8 {
			t.Errorf("status %s: counts.status.all = %d", status, counts.Status.All)
		}
	}

	// counts.type учитывает статус, но не тип.
	_, counts = list(PromoListFilter{Status: PromoStatusActive, Type: PromoTypeTrial})
	if want := (PromoTypeCounts{All: 3, SubscriptionDays: 1, Discount: 2}); counts.Type != want {
		t.Fatalf("active type counts %+v, want %+v", counts.Type, want)
	}
	// counts.status учитывает тип, но не статус.
	items, counts = list(PromoListFilter{Status: PromoStatusOff, Type: PromoTypeDiscount})
	if want := (PromoStatusCounts{All: 3, Active: 2, Ended: 0, Off: 1}); counts.Status != want {
		t.Fatalf("discount status counts %+v, want %+v", counts.Status, want)
	}
	if got := promoCodes(items); len(got) != 1 || got[0] != tok+"OFF" {
		t.Fatalf("off+discount: %v", got)
	}

	// Поиск без учёта регистра.
	items, _ = list(PromoListFilter{Query: strings.ToLower(tok) + "exh"})
	if got := sortedCodes(items); strings.Join(got, ",") != strings.Join(codesWith(tok, "EXH", "EXHOFF"), ",") {
		t.Fatalf("lowercase search: %v", got)
	}

	// Пустой результат.
	items, counts = list(PromoListFilter{Query: tok + "NOPE"})
	if len(items) != 0 || counts.Status != (PromoStatusCounts{}) || counts.Type != (PromoTypeCounts{}) {
		t.Fatalf("empty: %v %+v", promoCodes(items), counts)
	}

	// Пагинация: offset/limit поверх фильтра.
	page, _, err := repo.ListFiltered(ctx, PromoListFilter{Query: tok, Sort: PromoSortNew}, 2, 3)
	if err != nil || len(page) != 3 {
		t.Fatalf("page: %v %v", promoCodes(page), err)
	}
}

func TestPromoListFiltered_searchNoteAndEscaping(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPromoRepository(pool)
	ctx := context.Background()
	tok := promoTestToken()
	note := func(s string) *string { return &s }

	makePromo(t, pool, PromoCode{Code: tok + "N1", Type: PromoTypeTrial, Active: true, Note: note("blogger Vasya " + tok + " 50%off")})
	makePromo(t, pool, PromoCode{Code: tok + "N2", Type: PromoTypeTrial, Active: true, Note: note("channel " + tok + " 50xoff")})
	makePromo(t, pool, PromoCode{Code: tok + "N3", Type: PromoTypeTrial, Active: true, Note: note("misc " + tok + " a_b back\\slash")})
	makePromo(t, pool, PromoCode{Code: tok + "N4", Type: PromoTypeTrial, Active: true, Note: note("misc " + tok + " axb")})
	makePromo(t, pool, PromoCode{Code: tok + "N5", Type: PromoTypeTrial, Active: true})

	search := func(q string) []string {
		t.Helper()
		items, counts, err := repo.ListFiltered(ctx, PromoListFilter{Query: q}, 0, 100)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if counts.Status.All != len(items) {
			t.Fatalf("search %q: counts %d vs items %d", q, counts.Status.All, len(items))
		}
		return sortedCodes(items)
	}

	if got := search("VASYA " + strings.ToLower(tok)); strings.Join(got, ",") != tok+"N1" {
		t.Errorf("note search: %v", got)
	}
	if got := search(tok + " 50%off"); strings.Join(got, ",") != tok+"N1" {
		t.Errorf("%% must be literal: %v", got)
	}
	if got := search(tok + " a_b"); strings.Join(got, ",") != tok+"N3" {
		t.Errorf("_ must be literal: %v", got)
	}
	if got := search(`back\slash`); len(got) != 1 || got[0] != tok+"N3" {
		t.Errorf(`\ must be literal: %v`, got)
	}
	// Метка есть во всех кодах — по коду находятся все пять, NULL-заметка не мешает.
	if got := search(tok); len(got) != 5 {
		t.Errorf("code search: %v", got)
	}
	if p, err := repo.FindByCode(ctx, tok+"N1"); err != nil || p == nil || p.Note == nil || !strings.HasPrefix(*p.Note, "blogger") {
		t.Errorf("FindByCode note: %+v %v", p, err)
	}
	if p, err := repo.FindByCode(ctx, tok+"N5"); err != nil || p == nil || p.Note != nil {
		t.Errorf("FindByCode nil note: %+v %v", p, err)
	}
}

func TestPromoListFiltered_sorts(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPromoRepository(pool)
	ctx := context.Background()
	tok := promoTestToken()
	at := func(d time.Duration) *time.Time { v := time.Now().Add(d); return &v }

	// Порядок создания = порядок id.
	makePromo(t, pool, PromoCode{Code: tok + "S1", Type: PromoTypeTrial, Active: true, UsesCount: 5, ValidUntil: at(-72 * time.Hour)})
	makePromo(t, pool, PromoCode{Code: tok + "S2", Type: PromoTypeTrial, Active: true, UsesCount: 50, ValidUntil: nil})
	makePromo(t, pool, PromoCode{Code: tok + "S3", Type: PromoTypeTrial, Active: true, UsesCount: 5, ValidUntil: at(240 * time.Hour)})
	makePromo(t, pool, PromoCode{Code: tok + "S4", Type: PromoTypeTrial, Active: true, UsesCount: 1, ValidUntil: at(2 * time.Hour)})
	makePromo(t, pool, PromoCode{Code: tok + "S5", Type: PromoTypeTrial, Active: true, UsesCount: 0, ValidUntil: at(-1 * time.Hour)})
	makePromo(t, pool, PromoCode{Code: tok + "S6", Type: PromoTypeTrial, Active: true, UsesCount: 0, ValidUntil: nil})

	order := func(sortBy string) string {
		t.Helper()
		items, _, err := repo.ListFiltered(ctx, PromoListFilter{Query: tok, Sort: sortBy}, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(strings.Join(promoCodes(items), ","), tok, "")
	}

	if got, want := order(PromoSortNew), "S6,S5,S4,S3,S2,S1"; got != want {
		t.Errorf("new: %s, want %s", got, want)
	}
	if got, want := order(""), "S6,S5,S4,S3,S2,S1"; got != want {
		t.Errorf("default: %s, want %s", got, want)
	}
	// uses_count DESC, при равенстве новые выше.
	if got, want := order(PromoSortUses), "S2,S3,S1,S4,S6,S5"; got != want {
		t.Errorf("uses: %s, want %s", got, want)
	}
	// Будущие по возрастанию срока, потом бессрочные (новые выше), потом истёкшие (давние раньше).
	if got, want := order(PromoSortExpiring), "S4,S3,S6,S2,S1,S5"; got != want {
		t.Errorf("expiring: %s, want %s", got, want)
	}
}

// insertRedemption — активация с заданным временем (без увеличения uses_count:
// здесь проверяются только связанные с активацией оплаты).
func insertRedemption(t *testing.T, pool *pgxpool.Pool, promoID, customerID int64, usedAgo time.Duration) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO promo_redemption (promo_code_id, customer_id, used_at) VALUES ($1, $2, now() - $3::interval)`,
		promoID, customerID, fmt.Sprintf("%d seconds", int(usedAgo.Seconds()))); err != nil {
		t.Fatalf("insert redemption: %v", err)
	}
}

type testPurchase struct {
	customerID int64
	amount     float64
	currency   string
	invoice    string
	status     string
	promoID    *int64
	paidAgo    *time.Duration // nil — paid_at NULL
	createdAgo time.Duration
}

func insertTestPurchase(t *testing.T, pool *pgxpool.Pool, p testPurchase) {
	t.Helper()
	var paidAgo *string
	if p.paidAgo != nil {
		s := fmt.Sprintf("%d seconds", int(p.paidAgo.Seconds()))
		paidAgo = &s
	}
	if _, err := pool.Exec(context.Background(), `
INSERT INTO purchase (amount, customer_id, month, currency, status, invoice_type, promo_code_id, paid_at, created_at)
VALUES ($1, $2, 1, $3, $4, $5, $6, now() - $7::interval, now() - $8::interval)`,
		p.amount, p.customerID, p.currency, p.status, p.invoice, p.promoID, paidAgo,
		fmt.Sprintf("%d seconds", int(p.createdAgo.Seconds()))); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPromoDetailStats_paybackAndPending(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPromoRepository(pool)
	ctx := context.Background()
	tok := promoTestToken()
	dur := func(d time.Duration) *time.Duration { return &d }

	before, err := repo.RedemptionsSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}

	pct := 20
	days := 7
	discID := makePromo(t, pool, PromoCode{Code: tok + "DISC", Type: PromoTypeDiscount, Active: true, DiscountPercent: &pct})
	daysID := makePromo(t, pool, PromoCode{Code: tok + "DAYS", Type: PromoTypeSubscriptionDays, Active: true, SubscriptionDays: &days})
	techID := makePromo(t, pool, PromoCode{Code: "__" + tok, Type: PromoTypeDiscount, Active: true, DiscountPercent: &pct})

	c1, c2, c3, c4, c5, c6 := makeCustomer(t, pool), makeCustomer(t, pool), makeCustomer(t, pool),
		makeCustomer(t, pool), makeCustomer(t, pool), makeCustomer(t, pool)

	// Скидка: c1 активировал и оплатил со скидкой рублями и Stars; c2 оплатил со
	// скидкой без строки активации (считается — связь точная, через покупку).
	insertRedemption(t, pool, discID, c1.ID, time.Hour)
	insertTestPurchase(t, pool, testPurchase{customerID: c1.ID, amount: 100, currency: "RUB", invoice: "yookasa", status: "paid", promoID: &discID, paidAgo: dur(30 * time.Minute)})
	insertTestPurchase(t, pool, testPurchase{customerID: c1.ID, amount: 50, currency: "STARS", invoice: "telegram", status: "paid", promoID: &discID, paidAgo: dur(20 * time.Minute)})
	insertTestPurchase(t, pool, testPurchase{customerID: c1.ID, amount: 70, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: dur(10 * time.Minute)}) // без скидки
	insertTestPurchase(t, pool, testPurchase{customerID: c1.ID, amount: 90, currency: "RUB", invoice: "yookasa", status: "new", promoID: &discID})                // не оплачена
	insertTestPurchase(t, pool, testPurchase{customerID: c2.ID, amount: 30, currency: "", invoice: "yookasa", status: "paid", promoID: &discID, paidAgo: dur(5 * time.Minute)})
	for _, c := range []int64{c1.ID, c2.ID} {
		if _, err := pool.Exec(ctx, `INSERT INTO customer_pending_discount (customer_id, promo_code_id, percent) VALUES ($1, $2, 20)`, c, discID); err != nil {
			t.Fatalf("pending: %v", err)
		}
	}

	// Дни: оплаты после ввода есть (c3 — рублями и Stars, c4 — без paid_at), но у
	// кода без скидки отдачи нет — ни в карточке, ни в строках. c5 платил, но код
	// не вводил; c6 ввёл код 30 ч назад и не платил.
	insertRedemption(t, pool, daysID, c3.ID, time.Hour)
	insertRedemption(t, pool, daysID, c4.ID, time.Hour)
	insertRedemption(t, pool, daysID, c6.ID, 30*time.Hour)
	insertTestPurchase(t, pool, testPurchase{customerID: c3.ID, amount: 999, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: dur(2 * time.Hour), createdAgo: 2 * time.Hour})
	insertTestPurchase(t, pool, testPurchase{customerID: c3.ID, amount: 200, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: dur(0)})
	insertTestPurchase(t, pool, testPurchase{customerID: c3.ID, amount: 15, currency: "XTR", invoice: "telegram", status: "paid", paidAgo: dur(0)})
	insertTestPurchase(t, pool, testPurchase{customerID: c3.ID, amount: 5, currency: "USD", invoice: "crypto", status: "paid", paidAgo: dur(0)}) // ни рубли, ни Stars
	insertTestPurchase(t, pool, testPurchase{customerID: c4.ID, amount: 40, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: nil, createdAgo: 10 * time.Minute})
	insertTestPurchase(t, pool, testPurchase{customerID: c4.ID, amount: 41, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: nil, createdAgo: 3 * time.Hour})
	insertTestPurchase(t, pool, testPurchase{customerID: c5.ID, amount: 500, currency: "RUB", invoice: "yookasa", status: "paid", paidAgo: dur(0)})

	// Служебный код не входит в сводку.
	insertRedemption(t, pool, techID, c5.ID, time.Minute)

	disc, _ := repo.FindByID(ctx, discID)
	ds, err := repo.DetailStats(ctx, disc)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Redemptions24h != 1 || ds.PendingDiscounts != 2 {
		t.Errorf("discount counters: %+v", ds)
	}
	if pb := ds.Payback; pb.PaidCustomers != 2 || pb.Payments != 3 || !almost(pb.RevenueRub, 130) || !almost(pb.RevenueStars, 50) {
		t.Errorf("discount payback: %+v", pb)
	}

	daysPromo, _ := repo.FindByID(ctx, daysID)
	ss, err := repo.DetailStats(ctx, daysPromo)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Redemptions24h != 2 || ss.PendingDiscounts != 0 {
		t.Errorf("days counters: %+v", ss)
	}
	if ss.Payback != (PromoPayback{}) {
		t.Errorf("days payback must be empty: %+v", ss.Payback)
	}

	// Оплаты в строках «Кто использовал».
	rows, err := repo.ListRedemptionsPage(ctx, daysID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	byCustomer := map[int64]PromoRedemptionDetailRow{}
	for _, r := range rows {
		byCustomer[r.CustomerID] = r
	}
	if len(rows) != 3 {
		t.Fatalf("days redemptions: %d rows", len(rows))
	}
	for _, c := range []int64{c3.ID, c4.ID, c6.ID} {
		if r := byCustomer[c]; r.PaidCount != 0 || r.PaidRub != 0 || r.PaidStars != 0 {
			t.Errorf("days row %d must have no payments: %+v", c, r)
		}
	}

	drows, err := repo.ListRedemptionsPage(ctx, discID, 0, 20)
	if err != nil || len(drows) != 1 {
		t.Fatalf("discount redemptions: %v %v", drows, err)
	}
	if r := drows[0]; r.CustomerID != c1.ID || r.PaidCount != 2 || !almost(r.PaidRub, 100) || !almost(r.PaidStars, 50) {
		t.Errorf("c1 discount row: %+v", r)
	}

	after, err := repo.RedemptionsSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d := after.Total - before.Total; d != 4 {
		t.Errorf("summary total delta %d, want 4", d)
	}
	if d := after.Last24h - before.Last24h; d != 3 {
		t.Errorf("summary 24h delta %d, want 3", d)
	}
}

// Отдача без оплат и без активаций — нули, а не ошибка.
func TestPromoDetailStats_empty(t *testing.T) {
	pool := partnerTestPool(t)
	repo := NewPromoRepository(pool)
	ctx := context.Background()
	tok := promoTestToken()
	for _, typ := range []string{PromoTypeDiscount, PromoTypeTrial} {
		id := makePromo(t, pool, PromoCode{Code: tok + strings.ToUpper(typ[:3]), Type: typ, Active: true})
		p, _ := repo.FindByID(ctx, id)
		s, err := repo.DetailStats(ctx, p)
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if s != (PromoDetailStats{}) {
			t.Errorf("%s: %+v", typ, s)
		}
	}
}
