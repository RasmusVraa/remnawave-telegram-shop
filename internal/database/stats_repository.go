package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/config"
)

// StatsRepository агрегаты для админ-экрана «Статистика».
type StatsRepository struct {
	pool *pgxpool.Pool
	// Журнал реферальных начислений. Собирается внутри — ему нужен только пул,
	// а вызовы NewStatsRepository от этого не меняются.
	referralLedger *ReferralBonusLedgerRepository
}

func NewStatsRepository(pool *pgxpool.Pool) *StatsRepository {
	return &StatsRepository{pool: pool, referralLedger: NewReferralBonusLedgerRepository(pool)}
}

const sqlSubPurchase = `p.status = 'paid' AND p.month > 0 AND p.purchase_kind IN ('subscription', 'tariff_upgrade')`

const sqlRubCurrency = `(UPPER(TRIM(COALESCE(p.currency, ''))) IN ('RUB', 'RUR', '') OR COALESCE(p.currency, '') = '')`

// Награды колеса, начисляющие дни подписки. Список нужен и вкладке «Механики»,
// и балансу колеса на дашборде: разойдись эти два определения — экраны начнут
// показывать разную «чистую раздачу» за один и тот же месяц.
const sqlFortuneDayRewards = `('days_3','days_5','days_7','days_15','days_30','days_180')`

// AdminTopReferrer строка топа рефереров.
type AdminTopReferrer struct {
	ReferrerID       int64
	CustomerID       int64
	TelegramUsername *string
	Nickname         *string
	// Всего приглашённых и сколько из них хоть раз купили подписку.
	Referees     int64
	PaidReferees int64
	// Сколько приглашённые принесли деньгами и сколько дней начислено самому
	// пригласившему — без этих двух чисел «10 оплативших» ни о чём не говорят.
	RevenueRub float64
	BonusDays  int64
}

// AdminTariffStat метрики по одному тарифу (SALES_MODE=tariffs).
type AdminTariffStat struct {
	TariffID         int64
	DisplayName      string
	SalesToday       int64
	SalesWeek        int64
	SalesMonth       int64
	SalesHalfYear    int64
	SalesYear        int64
	SubsRevenueMonth float64
	RevenueToday     float64
	RevenueWeek      float64
	RevenueHalfYear  float64
	RevenueYear      float64
	RevenueAll       float64
	ActivePaidUsers  int64
}

// AdminStatsSnapshot снимок метрик на момент запроса.
type AdminStatsSnapshot struct {
	CapturedAt time.Time

	TotalCustomers      int64
	ActiveSubscriptions int64
	NewToday            int64
	NewWeek             int64
	NewMonth            int64
	NewPrevMonth        int64
	NewHalfYear         int64
	NewYear             int64

	TrialActive    int64
	PaidActive     int64
	Inactive       int64 // InactivePaid + InactiveUnpaid
	InactivePaid   int64
	InactiveUnpaid int64 // ≡ broadcast audience inactive_trial (нет paid purchase с month > 0)

	SalesSubToday     int64
	SalesSubWeek      int64
	SalesSubMonth     int64
	SalesSubPrevMonth int64
	SalesSubHalfYear  int64
	SalesSubYear      int64

	RevenueMonthRub      float64
	RevenueTodayRub      float64
	RevenueWeekRub       float64
	RevenueHalfYearRub   float64
	RevenueYearRub       float64
	RevenueAllTimeRub    float64
	RevenueSubsMonthRub  float64
	TransactionsToday    int64
	TransactionsWeek     int64
	TransactionsMonth    int64
	TransactionsHalfYear int64
	TransactionsYear     int64
	UniquePayersDay      int64
	UniquePayersWeek     int64
	UniquePayersMonth    int64
	UniquePayersHalfYear int64
	UniquePayersYear     int64
	PaymentRubByInvoice  map[string]float64

	DistinctReferrers    int64
	ActiveReferrers      int64
	RefBonusDaysAll      int64
	RefBonusDaysToday    int64
	RefBonusDaysWeek     int64
	RefBonusDaysMonth    int64
	RefBonusDaysHalfYear int64
	RefBonusDaysYear     int64
	TopReferrers         []AdminTopReferrer

	TariffBreakdown []AdminTariffStat
}

// AdminFortunePeriodAgg — спины колеса фортуны за полуинтервал времени [start, end).
type AdminFortunePeriodAgg struct {
	DistinctUsers   int64
	TotalSpins      int64
	FreeSpins       int64
	PaidSpins       int64
	PaidCostDaysSum int64
	// Суммы по reward_value из лога (для сверки «отдали» vs списали cost_days).
	WonSubsDaysSum    int64 // призы days_* — начисленные дни подписки
	WonLoyaltyXPSum   int64 // xp + micro
	WonDiscountPctSum int64 // discount_* — сумма процентных пунктов (не «скидка в ₽»)
	ByReward          map[string]int64
}

// AdminFortuneStatsSnapshot — агрегаты по таблице fortune_spins для админки.
type AdminFortuneStatsSnapshot struct {
	CapturedAt time.Time
	Month      AdminFortunePeriodAgg
	Today      AdminFortunePeriodAgg
	AllTime    AdminFortunePeriodAgg
}

func utcDayStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func monthRangeUTC(t time.Time) (start, end time.Time) {
	t = t.UTC()
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	return start, end
}

func prevMonthRangeUTC(t time.Time) (start, end time.Time) {
	t = t.UTC()
	firstThis := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = firstThis
	start = firstThis.AddDate(0, -1, 0)
	return start, end
}

// FetchAdminStatsSnapshot собирает метрики для админ-статистики.
func (s *StatsRepository) FetchAdminStatsSnapshot(ctx context.Context) (*AdminStatsSnapshot, error) {
	now := time.Now().UTC()
	today0 := utcDayStart(now)
	weekAgo := today0.AddDate(0, 0, -7)
	halfYearAgo := today0.AddDate(0, -6, 0)
	yearAgo := today0.AddDate(-1, 0, 0)
	monthStart, monthEnd := monthRangeUTC(now)
	prevStart, prevEnd := prevMonthRangeUTC(now)

	out := &AdminStatsSnapshot{
		CapturedAt:          now,
		PaymentRubByInvoice: make(map[string]float64),
	}

	q := `SELECT COUNT(*) FROM customer`
	if err := s.pool.QueryRow(ctx, q).Scan(&out.TotalCustomers); err != nil {
		return nil, fmt.Errorf("stats total customers: %w", err)
	}

	q = `SELECT COUNT(*) FROM customer WHERE expire_at IS NOT NULL AND expire_at > NOW()`
	if err := s.pool.QueryRow(ctx, q).Scan(&out.ActiveSubscriptions); err != nil {
		return nil, fmt.Errorf("stats active subscriptions: %w", err)
	}

	q = `SELECT COUNT(*) FROM customer WHERE created_at >= $1`
	if err := s.pool.QueryRow(ctx, q, today0).Scan(&out.NewToday); err != nil {
		return nil, fmt.Errorf("stats new today: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, weekAgo).Scan(&out.NewWeek); err != nil {
		return nil, fmt.Errorf("stats new week: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, monthStart).Scan(&out.NewMonth); err != nil {
		return nil, fmt.Errorf("stats new month: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, halfYearAgo).Scan(&out.NewHalfYear); err != nil {
		return nil, fmt.Errorf("stats new half year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, yearAgo).Scan(&out.NewYear); err != nil {
		return nil, fmt.Errorf("stats new year: %w", err)
	}

	q = `SELECT COUNT(*) FROM customer WHERE created_at >= $1 AND created_at < $2`
	if err := s.pool.QueryRow(ctx, q, prevStart, prevEnd).Scan(&out.NewPrevMonth); err != nil {
		return nil, fmt.Errorf("stats new in prev calendar month: %w", err)
	}

	q = `
SELECT
  COUNT(*) FILTER (WHERE c.expire_at IS NOT NULL AND c.expire_at > NOW() AND NOT EXISTS (
    SELECT 1 FROM purchase p WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
  )) AS trial,
  COUNT(*) FILTER (WHERE c.expire_at IS NOT NULL AND c.expire_at > NOW() AND EXISTS (
    SELECT 1 FROM purchase p WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
  )) AS paid,
  COUNT(*) FILTER (WHERE NOT (c.expire_at IS NOT NULL AND c.expire_at > NOW()) AND EXISTS (
    SELECT 1 FROM purchase p WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
  )) AS inactive_paid,
  COUNT(*) FILTER (WHERE NOT (c.expire_at IS NOT NULL AND c.expire_at > NOW()) AND NOT EXISTS (
    SELECT 1 FROM purchase p WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
  )) AS inactive_unpaid
FROM customer c`
	if err := s.pool.QueryRow(ctx, q).Scan(
		&out.TrialActive, &out.PaidActive, &out.InactivePaid, &out.InactiveUnpaid,
	); err != nil {
		return nil, fmt.Errorf("stats subscription buckets: %w", err)
	}
	out.Inactive = out.InactivePaid + out.InactiveUnpaid

	q = fmt.Sprintf(`SELECT COUNT(*) FROM purchase p WHERE %s AND p.paid_at >= $1 AND p.paid_at < $2`, sqlSubPurchase)
	if err := s.pool.QueryRow(ctx, q, today0, now).Scan(&out.SalesSubToday); err != nil {
		return nil, fmt.Errorf("stats sales today: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, weekAgo, now).Scan(&out.SalesSubWeek); err != nil {
		return nil, fmt.Errorf("stats sales week: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, monthStart, monthEnd).Scan(&out.SalesSubMonth); err != nil {
		return nil, fmt.Errorf("stats sales month: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, halfYearAgo, now).Scan(&out.SalesSubHalfYear); err != nil {
		return nil, fmt.Errorf("stats sales half year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, yearAgo, now).Scan(&out.SalesSubYear); err != nil {
		return nil, fmt.Errorf("stats sales year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, prevStart, prevEnd).Scan(&out.SalesSubPrevMonth); err != nil {
		return nil, fmt.Errorf("stats sales prev month: %w", err)
	}

	q = fmt.Sprintf(`
SELECT COALESCE(SUM(p.amount), 0)::float8 FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL AND p.paid_at >= $1 AND p.paid_at < $2 AND %s`, sqlRubCurrency)
	if err := s.pool.QueryRow(ctx, q, monthStart, monthEnd).Scan(&out.RevenueMonthRub); err != nil {
		return nil, fmt.Errorf("stats revenue month: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, weekAgo, now).Scan(&out.RevenueWeekRub); err != nil {
		return nil, fmt.Errorf("stats revenue week: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, halfYearAgo, now).Scan(&out.RevenueHalfYearRub); err != nil {
		return nil, fmt.Errorf("stats revenue half year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, yearAgo, now).Scan(&out.RevenueYearRub); err != nil {
		return nil, fmt.Errorf("stats revenue year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, today0, now).Scan(&out.RevenueTodayRub); err != nil {
		return nil, fmt.Errorf("stats revenue today: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(p.amount), 0)::float8 FROM purchase p WHERE p.status = 'paid' AND p.paid_at IS NOT NULL AND `+sqlRubCurrency).Scan(&out.RevenueAllTimeRub); err != nil {
		return nil, fmt.Errorf("stats revenue all time: %w", err)
	}

	q = fmt.Sprintf(`
SELECT COALESCE(SUM(p.amount), 0)::float8 FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL AND p.paid_at >= $1 AND p.paid_at < $2 AND %s AND %s`, sqlSubPurchase, sqlRubCurrency)
	if err := s.pool.QueryRow(ctx, q, monthStart, monthEnd).Scan(&out.RevenueSubsMonthRub); err != nil {
		return nil, fmt.Errorf("stats revenue subs month: %w", err)
	}

	q = `SELECT COUNT(*) FROM purchase p WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2`
	if err := s.pool.QueryRow(ctx, q, today0, now).Scan(&out.TransactionsToday); err != nil {
		return nil, fmt.Errorf("stats tx today: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, weekAgo, now).Scan(&out.TransactionsWeek); err != nil {
		return nil, fmt.Errorf("stats tx week: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, monthStart, monthEnd).Scan(&out.TransactionsMonth); err != nil {
		return nil, fmt.Errorf("stats tx month: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, halfYearAgo, now).Scan(&out.TransactionsHalfYear); err != nil {
		return nil, fmt.Errorf("stats tx half year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, yearAgo, now).Scan(&out.TransactionsYear); err != nil {
		return nil, fmt.Errorf("stats tx year: %w", err)
	}

	q = fmt.Sprintf(`
SELECT COUNT(DISTINCT p.customer_id) FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL AND p.paid_at >= $1 AND p.paid_at < $2 AND %s`, sqlRubCurrency)
	if err := s.pool.QueryRow(ctx, q, today0, now).Scan(&out.UniquePayersDay); err != nil {
		return nil, fmt.Errorf("stats unique payers day: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, weekAgo, now).Scan(&out.UniquePayersWeek); err != nil {
		return nil, fmt.Errorf("stats unique payers week: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, monthStart, monthEnd).Scan(&out.UniquePayersMonth); err != nil {
		return nil, fmt.Errorf("stats unique payers month: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, halfYearAgo, now).Scan(&out.UniquePayersHalfYear); err != nil {
		return nil, fmt.Errorf("stats unique payers half year: %w", err)
	}
	if err := s.pool.QueryRow(ctx, q, yearAgo, now).Scan(&out.UniquePayersYear); err != nil {
		return nil, fmt.Errorf("stats unique payers year: %w", err)
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
SELECT p.invoice_type::text, COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL AND %s
GROUP BY p.invoice_type`, sqlRubCurrency))
	if err != nil {
		return nil, fmt.Errorf("stats payment breakdown: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var inv string
		var sum float64
		if err := rows.Scan(&inv, &sum); err != nil {
			return nil, err
		}
		out.PaymentRubByInvoice[inv] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT referrer_id) FROM referral`).Scan(&out.DistinctReferrers); err != nil {
		return nil, fmt.Errorf("stats distinct referrers: %w", err)
	}

	q = `
SELECT COUNT(DISTINCT r.referrer_id) FROM referral r
JOIN customer c ON c.telegram_id = r.referee_id
WHERE EXISTS (
  SELECT 1 FROM purchase p WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
)`
	if err := s.pool.QueryRow(ctx, q).Scan(&out.ActiveReferrers); err != nil {
		return nil, fmt.Errorf("stats active referrers: %w", err)
	}

	refToday, refWeek, refMonth, refAll, err := s.referralBonusDaysReferrer(ctx, today0, weekAgo, monthStart, monthEnd, now)
	if err != nil {
		return nil, err
	}
	out.RefBonusDaysToday = refToday
	out.RefBonusDaysWeek = refWeek
	out.RefBonusDaysMonth = refMonth
	out.RefBonusDaysAll = refAll
	if out.RefBonusDaysHalfYear, err = s.referralBonusDaysRange(ctx, halfYearAgo, now); err != nil {
		return nil, err
	}
	if out.RefBonusDaysYear, err = s.referralBonusDaysRange(ctx, yearAgo, now); err != nil {
		return nil, err
	}

	top, err := s.topReferrers(ctx, 10)
	if err != nil {
		return nil, err
	}
	out.TopReferrers = top

	if config.SalesMode() == "tariffs" {
		tb, err := s.loadTariffBreakdown(ctx, now, today0, weekAgo, halfYearAgo, yearAgo, monthStart, monthEnd)
		if err != nil {
			return nil, err
		}
		out.TariffBreakdown = tb
	}

	return out, nil
}

// referralBonusDaysRange — дни, начисленные пригласившим за интервал.
//
// Раньше здесь жила отдельная реконструкция формулы из таблицы purchase, своя
// для progressive и своя для default, — третья и четвёртая копии одного расчёта
// в кодовой базе. Журнал делает вопрос режима неважным: в нём лежит выданное.
func (s *StatsRepository) referralBonusDaysRange(ctx context.Context, from, to time.Time) (int64, error) {
	return s.referralLedger.SumReferrerDaysRange(ctx, from, to)
}

func (s *StatsRepository) loadTariffBreakdown(ctx context.Context, now, today0, weekAgo, halfYearAgo, yearAgo, monthStart, monthEnd time.Time) ([]AdminTariffStat, error) {
	qTariffs := `
SELECT id, COALESCE(NULLIF(TRIM(name), ''), slug) AS disp, sort_order
FROM tariff
ORDER BY sort_order ASC, id ASC`
	rows, err := s.pool.Query(ctx, qTariffs)
	if err != nil {
		return nil, fmt.Errorf("stats list tariffs: %w", err)
	}
	type tarRow struct {
		id    int64
		name  string
		order int
	}
	var order []tarRow
	for rows.Next() {
		var r tarRow
		if err := rows.Scan(&r.id, &r.name, &r.order); err != nil {
			rows.Close()
			return nil, err
		}
		order = append(order, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, nil
	}

	salesWhere := fmt.Sprintf(`(%s) AND p.tariff_id IS NOT NULL`, sqlSubPurchase)
	revCond := fmt.Sprintf(`p.status = 'paid' AND p.paid_at IS NOT NULL AND %s AND p.tariff_id IS NOT NULL`, sqlRubCurrency)

	salesQ := fmt.Sprintf(`
SELECT p.tariff_id,
  COUNT(*) FILTER (WHERE p.paid_at >= $1 AND p.paid_at < $2) AS d,
  COUNT(*) FILTER (WHERE p.paid_at >= $3 AND p.paid_at < $2) AS w,
  COUNT(*) FILTER (WHERE p.paid_at >= $4 AND p.paid_at < $5) AS m,
  COUNT(*) FILTER (WHERE p.paid_at >= $6 AND p.paid_at < $2) AS hy,
  COUNT(*) FILTER (WHERE p.paid_at >= $7 AND p.paid_at < $2) AS y
FROM purchase p
WHERE %s
GROUP BY p.tariff_id`, salesWhere)

	type saleAgg struct{ d, w, m, hy, y int64 }
	salesMap := make(map[int64]saleAgg)
	srows, err := s.pool.Query(ctx, salesQ, today0, now, weekAgo, monthStart, monthEnd, halfYearAgo, yearAgo)
	if err != nil {
		return nil, fmt.Errorf("stats tariff sales: %w", err)
	}
	for srows.Next() {
		var tid int64
		var a saleAgg
		if err := srows.Scan(&tid, &a.d, &a.w, &a.m, &a.hy, &a.y); err != nil {
			srows.Close()
			return nil, err
		}
		salesMap[tid] = a
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}

	revTodayQ := fmt.Sprintf(`
SELECT p.tariff_id, COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.paid_at >= $1 AND p.paid_at < $2 AND %s
GROUP BY p.tariff_id`, revCond)
	revToday := map[int64]float64{}
	rtRows, err := s.pool.Query(ctx, revTodayQ, today0, now)
	if err != nil {
		return nil, fmt.Errorf("stats tariff rev today: %w", err)
	}
	for rtRows.Next() {
		var tid int64
		var sum float64
		if err := rtRows.Scan(&tid, &sum); err != nil {
			rtRows.Close()
			return nil, err
		}
		revToday[tid] = sum
	}
	rtRows.Close()
	if err := rtRows.Err(); err != nil {
		return nil, err
	}

	revRange := func(from, to time.Time) (map[int64]float64, error) {
		q := fmt.Sprintf(`
SELECT p.tariff_id, COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.paid_at >= $1 AND p.paid_at < $2 AND %s
GROUP BY p.tariff_id`, revCond)
		rows, err := s.pool.Query(ctx, q, from, to)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[int64]float64{}
		for rows.Next() {
			var tid int64
			var sum float64
			if err := rows.Scan(&tid, &sum); err != nil {
				return nil, err
			}
			out[tid] = sum
		}
		return out, rows.Err()
	}

	revWeek, err := revRange(weekAgo, now)
	if err != nil {
		return nil, fmt.Errorf("stats tariff rev week: %w", err)
	}
	revHalfYear, err := revRange(halfYearAgo, now)
	if err != nil {
		return nil, fmt.Errorf("stats tariff rev half year: %w", err)
	}
	revYear, err := revRange(yearAgo, now)
	if err != nil {
		return nil, fmt.Errorf("stats tariff rev year: %w", err)
	}

	revAllQ := fmt.Sprintf(`
SELECT p.tariff_id, COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE %s
GROUP BY p.tariff_id`, revCond)
	revAll := map[int64]float64{}
	raRows, err := s.pool.Query(ctx, revAllQ)
	if err != nil {
		return nil, fmt.Errorf("stats tariff rev all: %w", err)
	}
	for raRows.Next() {
		var tid int64
		var sum float64
		if err := raRows.Scan(&tid, &sum); err != nil {
			raRows.Close()
			return nil, err
		}
		revAll[tid] = sum
	}
	raRows.Close()
	if err := raRows.Err(); err != nil {
		return nil, err
	}

	subsMonthQ := fmt.Sprintf(`
SELECT p.tariff_id, COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.paid_at >= $1 AND p.paid_at < $2
  AND (%s)
  AND (%s)
  AND p.tariff_id IS NOT NULL
GROUP BY p.tariff_id`, sqlSubPurchase, sqlRubCurrency)
	subsMonth := map[int64]float64{}
	smRows, err := s.pool.Query(ctx, subsMonthQ, monthStart, monthEnd)
	if err != nil {
		return nil, fmt.Errorf("stats tariff subs month rev: %w", err)
	}
	for smRows.Next() {
		var tid int64
		var sum float64
		if err := smRows.Scan(&tid, &sum); err != nil {
			smRows.Close()
			return nil, err
		}
		subsMonth[tid] = sum
	}
	smRows.Close()
	if err := smRows.Err(); err != nil {
		return nil, err
	}

	activeQ := `
SELECT c.current_tariff_id, COUNT(*)::bigint
FROM customer c
WHERE c.expire_at IS NOT NULL AND c.expire_at > NOW()
  AND c.current_tariff_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM purchase p
    WHERE p.customer_id = c.id AND p.status = 'paid' AND p.month > 0
  )
GROUP BY c.current_tariff_id`
	activeMap := map[int64]int64{}
	arows, err := s.pool.Query(ctx, activeQ)
	if err != nil {
		return nil, fmt.Errorf("stats tariff active paid: %w", err)
	}
	for arows.Next() {
		var tid, n int64
		if err := arows.Scan(&tid, &n); err != nil {
			arows.Close()
			return nil, err
		}
		activeMap[tid] = n
	}
	arows.Close()
	if err := arows.Err(); err != nil {
		return nil, err
	}

	out := make([]AdminTariffStat, 0, len(order))
	for _, tr := range order {
		sa := salesMap[tr.id]
		out = append(out, AdminTariffStat{
			TariffID:         tr.id,
			DisplayName:      tr.name,
			SalesToday:       sa.d,
			SalesWeek:        sa.w,
			SalesMonth:       sa.m,
			SalesHalfYear:    sa.hy,
			SalesYear:        sa.y,
			SubsRevenueMonth: subsMonth[tr.id],
			RevenueToday:     revToday[tr.id],
			RevenueWeek:      revWeek[tr.id],
			RevenueHalfYear:  revHalfYear[tr.id],
			RevenueYear:      revYear[tr.id],
			RevenueAll:       revAll[tr.id],
			ActivePaidUsers:  activeMap[tr.id],
		})
	}
	return out, nil
}

func (s *StatsRepository) referralBonusDaysReferrer(ctx context.Context, today0, weekAgo, monthStart, monthEnd, now time.Time) (today, week, month, all int64, err error) {
	if all, err = s.referralLedger.SumReferrerDaysRange(ctx, time.Time{}, now); err != nil {
		return 0, 0, 0, 0, err
	}
	if today, err = s.referralLedger.SumReferrerDaysRange(ctx, today0, now); err != nil {
		return 0, 0, 0, 0, err
	}
	if week, err = s.referralLedger.SumReferrerDaysRange(ctx, weekAgo, now); err != nil {
		return 0, 0, 0, 0, err
	}
	if month, err = s.referralLedger.SumReferrerDaysRange(ctx, monthStart, monthEnd); err != nil {
		return 0, 0, 0, 0, err
	}
	return today, week, month, all, nil
}

// topReferrers — топ пригласивших вместе с тем, что они принесли.
//
// Раньше здесь считалось COUNT(DISTINCT referee_id) под именем PaidReferees, а
// отбор шёл по тому, платил ли сам пригласивший. То есть в колонке «оплативших»
// стояло число всех приглашённых, а из таблицы выпадали активные рефереры,
// которые сами ничего не покупали. Теперь оплативших считают среди
// приглашённых, а фильтр по покупкам самого реферера снят.
func (s *StatsRepository) topReferrers(ctx context.Context, limit int) ([]AdminTopReferrer, error) {
	// Считается в один проход по агрегатам, а не подзапросом на каждого
	// реферера: коррелированные подзапросы выполнились бы для всей таблицы
	// рефереров ещё до LIMIT. Подпись (ник) добирается уже после отбора топа —
	// в отдельном шаге, где строк ровно limit.
	q := fmt.Sprintf(`
WITH pairs AS (
  SELECT DISTINCT r.referrer_id, rc.id AS referee_customer_id
  FROM referral r
  JOIN customer rc ON rc.telegram_id = r.referee_id
),
referee_money AS (
  SELECT p.customer_id,
         COALESCE(SUM(p.amount) FILTER (WHERE %s), 0)::float8 AS revenue,
         BOOL_OR(p.month > 0) AS has_paid_sub
  FROM purchase p
  WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
  GROUP BY p.customer_id
),
agg AS (
  SELECT pairs.referrer_id,
         COUNT(*)                                      AS referees,
         COUNT(*) FILTER (WHERE rm.has_paid_sub)       AS paid_referees,
         COALESCE(SUM(rm.revenue), 0)::float8          AS revenue_rub
  FROM pairs
  LEFT JOIN referee_money rm ON rm.customer_id = pairs.referee_customer_id
  GROUP BY pairs.referrer_id
),
top AS (
  SELECT agg.*, COALESCE(bl.bonus_days, 0) AS bonus_days
  FROM agg
  LEFT JOIN (
    SELECT recipient_telegram_id, SUM(days)::bigint AS bonus_days
    FROM referral_bonus_ledger
    GROUP BY recipient_telegram_id
  ) bl ON bl.recipient_telegram_id = agg.referrer_id
  ORDER BY agg.paid_referees DESC, agg.revenue_rub DESC, agg.referees DESC
  LIMIT $1
)
SELECT
  top.referrer_id,
  c.id,
  c.telegram_username,
  (SELECT NULLIF(TRIM(ii.raw_profile_json->>'first_name'), '')
   FROM cabinet_account_customer_link l
   JOIN cabinet_identity ii ON ii.account_id = l.account_id AND ii.unlinked_at IS NULL
   WHERE l.customer_id = c.id AND l.link_status = 'linked'
   ORDER BY ii.created_at DESC
   LIMIT 1) AS nickname,
  top.referees,
  top.paid_referees,
  top.revenue_rub,
  top.bonus_days
FROM top
JOIN customer c ON c.telegram_id = top.referrer_id
ORDER BY top.paid_referees DESC, top.revenue_rub DESC, top.referees DESC`, sqlRubCurrency)
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("stats top referrers: %w", err)
	}
	defer rows.Close()
	var list []AdminTopReferrer
	for rows.Next() {
		var tr AdminTopReferrer
		if err := rows.Scan(
			&tr.ReferrerID, &tr.CustomerID, &tr.TelegramUsername, &tr.Nickname,
			&tr.Referees, &tr.PaidReferees, &tr.RevenueRub, &tr.BonusDays,
		); err != nil {
			return nil, err
		}
		list = append(list, tr)
	}
	return list, rows.Err()
}

func newFortunePeriodAgg() AdminFortunePeriodAgg {
	return AdminFortunePeriodAgg{ByReward: make(map[string]int64)}
}

func (s *StatsRepository) loadFortuneWindow(ctx context.Context, start, end time.Time) (AdminFortunePeriodAgg, error) {
	agg := newFortunePeriodAgg()
	q := `
SELECT
  COUNT(DISTINCT customer_id)::bigint,
  COUNT(*)::bigint,
  COUNT(*) FILTER (WHERE is_free_spin)::bigint,
  COUNT(*) FILTER (WHERE NOT is_free_spin)::bigint,
  COALESCE(SUM(cost_days) FILTER (WHERE NOT is_free_spin), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ` + sqlFortuneDayRewards + `), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ('xp','micro')), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ('discount_3','discount_5')), 0)::bigint
FROM fortune_spins
WHERE spin_at >= $1 AND spin_at < $2`
	if err := s.pool.QueryRow(ctx, q, start, end).Scan(
		&agg.DistinctUsers, &agg.TotalSpins, &agg.FreeSpins, &agg.PaidSpins, &agg.PaidCostDaysSum,
		&agg.WonSubsDaysSum, &agg.WonLoyaltyXPSum, &agg.WonDiscountPctSum,
	); err != nil {
		return agg, fmt.Errorf("stats fortune window agg: %w", err)
	}
	qr := `SELECT reward_type, COUNT(*)::bigint FROM fortune_spins WHERE spin_at >= $1 AND spin_at < $2 GROUP BY reward_type`
	rows, err := s.pool.Query(ctx, qr, start, end)
	if err != nil {
		return agg, fmt.Errorf("stats fortune window by reward: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rt string
		var n int64
		if err := rows.Scan(&rt, &n); err != nil {
			return agg, err
		}
		agg.ByReward[rt] = n
	}
	return agg, rows.Err()
}

func (s *StatsRepository) loadFortuneAllTime(ctx context.Context) (AdminFortunePeriodAgg, error) {
	agg := newFortunePeriodAgg()
	q := `
SELECT
  COUNT(DISTINCT customer_id)::bigint,
  COUNT(*)::bigint,
  COUNT(*) FILTER (WHERE is_free_spin)::bigint,
  COUNT(*) FILTER (WHERE NOT is_free_spin)::bigint,
  COALESCE(SUM(cost_days) FILTER (WHERE NOT is_free_spin), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ('days_3','days_5','days_7','days_15','days_30','days_180')), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ('xp','micro')), 0)::bigint,
  COALESCE(SUM(reward_value) FILTER (WHERE reward_type IN ('discount_3','discount_5')), 0)::bigint
FROM fortune_spins`
	if err := s.pool.QueryRow(ctx, q).Scan(
		&agg.DistinctUsers, &agg.TotalSpins, &agg.FreeSpins, &agg.PaidSpins, &agg.PaidCostDaysSum,
		&agg.WonSubsDaysSum, &agg.WonLoyaltyXPSum, &agg.WonDiscountPctSum,
	); err != nil {
		return agg, fmt.Errorf("stats fortune all agg: %w", err)
	}
	qr := `SELECT reward_type, COUNT(*)::bigint FROM fortune_spins GROUP BY reward_type`
	rows, err := s.pool.Query(ctx, qr)
	if err != nil {
		return agg, fmt.Errorf("stats fortune all by reward: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rt string
		var n int64
		if err := rows.Scan(&rt, &n); err != nil {
			return agg, err
		}
		agg.ByReward[rt] = n
	}
	return agg, rows.Err()
}

// FetchAdminFortuneStats собирает статистику спинов колеса фортуны (кабинет).
func (s *StatsRepository) FetchAdminFortuneStats(ctx context.Context) (*AdminFortuneStatsSnapshot, error) {
	now := time.Now().UTC()
	today0 := utcDayStart(now)
	todayEnd := today0.Add(24 * time.Hour)
	monthStart, monthEnd := monthRangeUTC(now)
	out := &AdminFortuneStatsSnapshot{CapturedAt: now}
	var err error
	out.Month, err = s.loadFortuneWindow(ctx, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	out.Today, err = s.loadFortuneWindow(ctx, today0, todayEnd)
	if err != nil {
		return nil, err
	}
	out.AllTime, err = s.loadFortuneAllTime(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
