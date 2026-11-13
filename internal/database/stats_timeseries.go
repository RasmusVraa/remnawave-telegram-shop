package database

import (
	"context"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/config"
)

// AdminStatsTimeSeriesPoint — одна точка ряда (день / неделя / месяц).
type AdminStatsTimeSeriesPoint struct {
	Date         string
	RevenueRub   float64
	Sales        int64
	NewUsers     int64
	Transactions int64
}

// AdminTariffTimeSeriesPoint — метрики тарифа за bucket.
type AdminTariffTimeSeriesPoint struct {
	Date       string
	Sales      int64
	RevenueRub float64
}

// AdminTariffTimeSeries — ряд по одному тарифу.
type AdminTariffTimeSeries struct {
	TariffID    int64
	DisplayName string
	Points      []AdminTariffTimeSeriesPoint
}

// AdminStatsTimeSeries — ответ timeseries для админ-графиков.
type AdminStatsTimeSeries struct {
	CapturedAt   time.Time
	Period       string
	Granularity  string
	From         string
	To           string
	Points       []AdminStatsTimeSeriesPoint
	TariffSeries []AdminTariffTimeSeries
}

const (
	statsGranularityDay   = "day"
	statsGranularityWeek  = "week"
	statsGranularityMonth = "month"

	statsTimeSeriesMaxMonthlyBuckets = 36

	statsCustomGranularityDayDays  = 45
	statsCustomGranularityWeekDays = 180
)

// ErrInvalidStatsTimeSeriesRange — некорректный произвольный диапазон from/to.
var ErrInvalidStatsTimeSeriesRange = fmt.Errorf("invalid stats timeseries range")

func utcWeekStart(t time.Time) time.Time {
	t = utcDayStart(t)
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func utcMonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// ResolveStatsTimeSeriesWindow — диапазон и гранулярность ряда по периоду UI.
func ResolveStatsTimeSeriesWindow(period string, now time.Time) (from, to time.Time, granularity string) {
	now = now.UTC()
	today0 := utcDayStart(now)
	to = today0.Add(24 * time.Hour)

	switch period {
	case "day":
		from = today0.AddDate(0, 0, -6)
		return from, to, statsGranularityDay
	case "week":
		from = today0.AddDate(0, 0, -6)
		return from, to, statsGranularityDay
	case "month":
		from, _ = monthRangeUTC(now)
		return from, to, statsGranularityDay
	case "half_year":
		from = utcWeekStart(today0.AddDate(0, -6, 0))
		return from, to, statsGranularityWeek
	case "year":
		from = utcMonthStart(today0.AddDate(-1, 0, 0))
		return from, to, statsGranularityMonth
	case "all_time":
		from = utcMonthStart(today0.AddDate(0, -int(statsTimeSeriesMaxMonthlyBuckets-1), 0))
		return from, to, statsGranularityMonth
	default:
		from, _ = monthRangeUTC(now)
		return from, to, statsGranularityDay
	}
}

// ResolveStatsTimeSeriesCustomWindow — half-open [fromDay, toDay+1day) и гранулярность для произвольного диапазона.
// fromDate/toDate — календарные дни (включительно). now используется для проверки «не из будущего».
func ResolveStatsTimeSeriesCustomWindow(fromDate, toDate, now time.Time) (from, to time.Time, granularity string, err error) {
	now = now.UTC()
	from = utcDayStart(fromDate)
	toInclusive := utcDayStart(toDate)
	if toInclusive.Before(from) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("%w: from after to", ErrInvalidStatsTimeSeriesRange)
	}

	maxTo := utcDayStart(now).AddDate(0, 0, 1) // +1 день: календарное «сегодня» в TZ впереди UTC
	if toInclusive.After(maxTo) {
		toInclusive = maxTo
	}
	if from.After(maxTo) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("%w: from in the future", ErrInvalidStatsTimeSeriesRange)
	}
	if toInclusive.Before(from) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("%w: from after to", ErrInvalidStatsTimeSeriesRange)
	}

	minFrom := utcMonthStart(utcDayStart(now).AddDate(0, -int(statsTimeSeriesMaxMonthlyBuckets-1), 0))
	if from.Before(minFrom) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("%w: range exceeds %d months", ErrInvalidStatsTimeSeriesRange, statsTimeSeriesMaxMonthlyBuckets)
	}

	to = toInclusive.Add(24 * time.Hour)
	days := int(toInclusive.Sub(from).Hours()/24) + 1
	switch {
	case days <= statsCustomGranularityDayDays:
		granularity = statsGranularityDay
	case days <= statsCustomGranularityWeekDays:
		granularity = statsGranularityWeek
	default:
		granularity = statsGranularityMonth
	}
	return from, to, granularity, nil
}

func statsDateTruncExpr(granularity string) (string, error) {
	switch granularity {
	case statsGranularityDay:
		return "day", nil
	case statsGranularityWeek:
		return "week", nil
	case statsGranularityMonth:
		return "month", nil
	default:
		return "", fmt.Errorf("stats timeseries: unsupported granularity %q", granularity)
	}
}

func formatStatsBucketDate(t time.Time) string {
	return utcDayStart(t).Format("2006-01-02")
}

func generateStatsBuckets(from, to time.Time, granularity string) []time.Time {
	var buckets []time.Time
	switch granularity {
	case statsGranularityDay:
		for d := utcDayStart(from); d.Before(to); d = d.AddDate(0, 0, 1) {
			buckets = append(buckets, d)
		}
	case statsGranularityWeek:
		for d := utcWeekStart(from); d.Before(to); d = d.AddDate(0, 0, 7) {
			buckets = append(buckets, d)
		}
	case statsGranularityMonth:
		for d := utcMonthStart(from); d.Before(to); d = d.AddDate(0, 1, 0) {
			buckets = append(buckets, d)
		}
	}
	return buckets
}

// FetchAdminStatsTimeSeries — дневные/недельные/месячные ряды для графиков админки (пресет периода).
func (s *StatsRepository) FetchAdminStatsTimeSeries(ctx context.Context, period string) (*AdminStatsTimeSeries, error) {
	now := time.Now().UTC()
	from, to, granularity := ResolveStatsTimeSeriesWindow(period, now)

	if period == "all_time" {
		if minAt, err := s.minStatsActivityAt(ctx); err != nil {
			return nil, err
		} else if !minAt.IsZero() {
			capped := utcMonthStart(minAt)
			maxFrom := utcMonthStart(now.AddDate(0, -int(statsTimeSeriesMaxMonthlyBuckets-1), 0))
			if capped.Before(maxFrom) {
				capped = maxFrom
			}
			from = capped
		}
	}

	return s.fetchAdminStatsTimeSeriesWindow(ctx, period, from, to, granularity, now)
}

// FetchAdminStatsTimeSeriesRange — ряды за произвольный inclusive-диапазон дат (period=custom).
func (s *StatsRepository) FetchAdminStatsTimeSeriesRange(ctx context.Context, fromDate, toDate time.Time) (*AdminStatsTimeSeries, error) {
	now := time.Now().UTC()
	from, to, granularity, err := ResolveStatsTimeSeriesCustomWindow(fromDate, toDate, now)
	if err != nil {
		return nil, err
	}
	return s.fetchAdminStatsTimeSeriesWindow(ctx, "custom", from, to, granularity, now)
}

func (s *StatsRepository) fetchAdminStatsTimeSeriesWindow(
	ctx context.Context,
	period string,
	from, to time.Time,
	granularity string,
	now time.Time,
) (*AdminStatsTimeSeries, error) {
	trunc, err := statsDateTruncExpr(granularity)
	if err != nil {
		return nil, err
	}

	buckets := generateStatsBuckets(from, to, granularity)
	if len(buckets) == 0 {
		buckets = []time.Time{utcDayStart(from)}
	}

	revenue, err := s.statsTimeSeriesRevenue(ctx, from, to, trunc)
	if err != nil {
		return nil, err
	}
	sales, err := s.statsTimeSeriesSales(ctx, from, to, trunc)
	if err != nil {
		return nil, err
	}
	newUsers, err := s.statsTimeSeriesNewUsers(ctx, from, to, trunc)
	if err != nil {
		return nil, err
	}
	transactions, err := s.statsTimeSeriesTransactions(ctx, from, to, trunc)
	if err != nil {
		return nil, err
	}

	points := make([]AdminStatsTimeSeriesPoint, 0, len(buckets))
	for _, b := range buckets {
		key := formatStatsBucketDate(b)
		points = append(points, AdminStatsTimeSeriesPoint{
			Date:         key,
			RevenueRub:   revenue[key],
			Sales:        sales[key],
			NewUsers:     newUsers[key],
			Transactions: transactions[key],
		})
	}

	out := &AdminStatsTimeSeries{
		CapturedAt:  now,
		Period:      period,
		Granularity: granularity,
		From:        formatStatsBucketDate(from),
		To:          formatStatsBucketDate(to.Add(-24 * time.Hour)),
		Points:      points,
	}

	if config.SalesMode() == "tariffs" {
		series, err := s.statsTimeSeriesTariffs(ctx, from, to, trunc, buckets)
		if err != nil {
			return nil, err
		}
		out.TariffSeries = series
	}

	return out, nil
}

func (s *StatsRepository) minStatsActivityAt(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx, `
SELECT MIN(ts) FROM (
  SELECT MIN(created_at) AS ts FROM customer
  UNION ALL
  SELECT MIN(paid_at) AS ts FROM purchase WHERE paid_at IS NOT NULL
) combined WHERE ts IS NOT NULL`).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("stats timeseries min activity: %w", err)
	}
	return t.UTC(), nil
}

func (s *StatsRepository) statsTimeSeriesRevenue(ctx context.Context, from, to time.Time, trunc string) (map[string]float64, error) {
	q := fmt.Sprintf(`
SELECT (date_trunc('%s', p.paid_at AT TIME ZONE 'UTC'))::date AS bucket,
       COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
  AND p.paid_at >= $1 AND p.paid_at < $2
  AND %s
GROUP BY 1`, trunc, sqlRubCurrency)
	return scanFloatBucketMap(ctx, s, q, from, to)
}

func (s *StatsRepository) statsTimeSeriesSales(ctx context.Context, from, to time.Time, trunc string) (map[string]int64, error) {
	q := fmt.Sprintf(`
SELECT (date_trunc('%s', p.paid_at AT TIME ZONE 'UTC'))::date AS bucket,
       COUNT(*)::bigint
FROM purchase p
WHERE %s AND p.paid_at >= $1 AND p.paid_at < $2
GROUP BY 1`, trunc, sqlSubPurchase)
	return scanIntBucketMap(ctx, s, q, from, to)
}

func (s *StatsRepository) statsTimeSeriesNewUsers(ctx context.Context, from, to time.Time, trunc string) (map[string]int64, error) {
	q := fmt.Sprintf(`
SELECT (date_trunc('%s', c.created_at AT TIME ZONE 'UTC'))::date AS bucket,
       COUNT(*)::bigint
FROM customer c
WHERE c.created_at >= $1 AND c.created_at < $2
GROUP BY 1`, trunc)
	return scanIntBucketMap(ctx, s, q, from, to)
}

func (s *StatsRepository) statsTimeSeriesTransactions(ctx context.Context, from, to time.Time, trunc string) (map[string]int64, error) {
	q := fmt.Sprintf(`
SELECT (date_trunc('%s', p.paid_at AT TIME ZONE 'UTC'))::date AS bucket,
       COUNT(*)::bigint
FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
  AND p.paid_at >= $1 AND p.paid_at < $2
GROUP BY 1`, trunc)
	return scanIntBucketMap(ctx, s, q, from, to)
}

func scanIntBucketMap(ctx context.Context, s *StatsRepository, q string, from, to time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, q, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var bucket time.Time
		var v int64
		if err := rows.Scan(&bucket, &v); err != nil {
			return nil, err
		}
		out[formatStatsBucketDate(bucket)] = v
	}
	return out, rows.Err()
}

func scanFloatBucketMap(ctx context.Context, s *StatsRepository, q string, from, to time.Time) (map[string]float64, error) {
	rows, err := s.pool.Query(ctx, q, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]float64)
	for rows.Next() {
		var bucket time.Time
		var v float64
		if err := rows.Scan(&bucket, &v); err != nil {
			return nil, err
		}
		out[formatStatsBucketDate(bucket)] = v
	}
	return out, rows.Err()
}

func (s *StatsRepository) statsTimeSeriesTariffs(ctx context.Context, from, to time.Time, trunc string, buckets []time.Time) ([]AdminTariffTimeSeries, error) {
	qTariffs := `
SELECT id, COALESCE(NULLIF(TRIM(name), ''), slug) AS disp
FROM tariff
ORDER BY sort_order ASC, id ASC`
	trows, err := s.pool.Query(ctx, qTariffs)
	if err != nil {
		return nil, fmt.Errorf("stats timeseries tariffs list: %w", err)
	}
	type tarMeta struct {
		id   int64
		name string
	}
	var order []tarMeta
	for trows.Next() {
		var m tarMeta
		if err := trows.Scan(&m.id, &m.name); err != nil {
			trows.Close()
			return nil, err
		}
		order = append(order, m)
	}
	trows.Close()
	if err := trows.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, nil
	}

	salesQ := fmt.Sprintf(`
SELECT p.tariff_id,
       (date_trunc('%s', p.paid_at AT TIME ZONE 'UTC'))::date AS bucket,
       COUNT(*)::bigint
FROM purchase p
WHERE (%s) AND p.tariff_id IS NOT NULL
  AND p.paid_at >= $1 AND p.paid_at < $2
GROUP BY 1, 2`, trunc, sqlSubPurchase)
	srows, err := s.pool.Query(ctx, salesQ, from, to)
	if err != nil {
		return nil, fmt.Errorf("stats timeseries tariff sales: %w", err)
	}
	salesMap := make(map[int64]map[string]int64)
	for srows.Next() {
		var tid int64
		var bucket time.Time
		var n int64
		if err := srows.Scan(&tid, &bucket, &n); err != nil {
			srows.Close()
			return nil, err
		}
		key := formatStatsBucketDate(bucket)
		if salesMap[tid] == nil {
			salesMap[tid] = make(map[string]int64)
		}
		salesMap[tid][key] = n
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}

	revQ := fmt.Sprintf(`
SELECT p.tariff_id,
       (date_trunc('%s', p.paid_at AT TIME ZONE 'UTC'))::date AS bucket,
       COALESCE(SUM(p.amount), 0)::float8
FROM purchase p
WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
  AND p.tariff_id IS NOT NULL
  AND p.paid_at >= $1 AND p.paid_at < $2
  AND %s
GROUP BY 1, 2`, trunc, sqlRubCurrency)
	rrows, err := s.pool.Query(ctx, revQ, from, to)
	if err != nil {
		return nil, fmt.Errorf("stats timeseries tariff revenue: %w", err)
	}
	revMap := make(map[int64]map[string]float64)
	for rrows.Next() {
		var tid int64
		var bucket time.Time
		var sum float64
		if err := rrows.Scan(&tid, &bucket, &sum); err != nil {
			rrows.Close()
			return nil, err
		}
		key := formatStatsBucketDate(bucket)
		if revMap[tid] == nil {
			revMap[tid] = make(map[string]float64)
		}
		revMap[tid][key] = sum
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, err
	}

	if len(buckets) == 0 {
		buckets = []time.Time{utcDayStart(from)}
	}

	out := make([]AdminTariffTimeSeries, 0, len(order))
	for _, tr := range order {
		pts := make([]AdminTariffTimeSeriesPoint, 0, len(buckets))
		for _, b := range buckets {
			key := formatStatsBucketDate(b)
			pts = append(pts, AdminTariffTimeSeriesPoint{
				Date:       key,
				Sales:      salesMap[tr.id][key],
				RevenueRub: revMap[tr.id][key],
			})
		}
		out = append(out, AdminTariffTimeSeries{
			TariffID:    tr.id,
			DisplayName: tr.name,
			Points:      pts,
		})
	}
	return out, nil
}
