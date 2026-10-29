package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const fortunePromoCode = "__CABINET_FORTUNE__"

// FortuneRepo — лог спинов колеса фортуны.
type FortuneRepo struct {
	pool *pgxpool.Pool
}

func NewFortuneRepo(pool *pgxpool.Pool) *FortuneRepo {
	return &FortuneRepo{pool: pool}
}

func (r *FortuneRepo) Pool() *pgxpool.Pool { return r.pool }

// CountPaidSpinsBetween — платные спины (не бесплатный первый) за полуинтервал [start, end) в UTC.
func (r *FortuneRepo) CountPaidSpinsBetween(ctx context.Context, customerID int64, start, end time.Time) (int, error) {
	const q = `SELECT COUNT(*) FROM fortune_spins
WHERE customer_id = $1 AND is_free_spin = FALSE AND spin_at >= $2 AND spin_at < $3`
	var n int
	if err := r.pool.QueryRow(ctx, q, customerID, start, end).Scan(&n); err != nil {
		return 0, fmt.Errorf("fortune: count spins: %w", err)
	}
	return n, nil
}

// HasDailyFreeSpinToday — уже использован ежедневный бесплатный спин за UTC-сутки [dayStart, dayEnd).
func (r *FortuneRepo) HasDailyFreeSpinToday(ctx context.Context, customerID int64, dayStart, dayEnd time.Time) (bool, error) {
	const q = `SELECT EXISTS(
  SELECT 1 FROM fortune_spins
  WHERE customer_id = $1 AND COALESCE(is_daily_free, FALSE) = TRUE
    AND spin_at >= $2 AND spin_at < $3)`
	var ok bool
	if err := r.pool.QueryRow(ctx, q, customerID, dayStart, dayEnd).Scan(&ok); err != nil {
		return false, fmt.Errorf("fortune: has daily free today: %w", err)
	}
	return ok, nil
}

// FortuneSpinXPGain — начисление XP с колеса (reward_type xp|micro), для истории лояльности.
type FortuneSpinXPGain struct {
	ID          int64
	SpinAt      time.Time
	RewardType  string
	RewardValue int
}

// ListXPGainsByCustomer — спины с начислением XP, новые первые (без миграций: таблица fortune_spins).
func (r *FortuneRepo) ListXPGainsByCustomer(ctx context.Context, customerID int64, limit int) ([]FortuneSpinXPGain, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	const q = `SELECT id, spin_at, reward_type, reward_value
FROM fortune_spins
WHERE customer_id = $1 AND reward_type IN ('xp', 'micro') AND reward_value > 0
ORDER BY spin_at DESC, id DESC
LIMIT $2`
	rows, err := r.pool.Query(ctx, q, customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("fortune: list xp spins: %w", err)
	}
	defer rows.Close()
	var out []FortuneSpinXPGain
	for rows.Next() {
		var row FortuneSpinXPGain
		if err := rows.Scan(&row.ID, &row.SpinAt, &row.RewardType, &row.RewardValue); err != nil {
			return nil, fmt.Errorf("fortune: scan xp spin: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *FortuneRepo) InsertSpin(ctx context.Context, customerID int64, rewardType string, rewardValue, costDays int, isFree, isDailyFree bool) error {
	const q = `INSERT INTO fortune_spins (customer_id, reward_type, reward_value, cost_days, is_free_spin, is_daily_free)
VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := r.pool.Exec(ctx, q, customerID, rewardType, rewardValue, costDays, isFree, isDailyFree); err != nil {
		return fmt.Errorf("fortune: insert spin: %w", err)
	}
	return nil
}

// FortuneFeedRow — строка для публичной ленты победителей (сырой ник до маскировки на сервисе).
type FortuneFeedRow struct {
	SpinAt      time.Time
	RewardType  string
	RewardValue int
	TgUser      sql.NullString
	TgFirst     sql.NullString
	EmailLocal  sql.NullString
}

func nullStringTrimmedPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := strings.TrimSpace(ns.String)
	if s == "" {
		return nil
	}
	return &s
}

// FortuneFeedIdentityStrings — tg username, first name, email local для маскировки в ленте (cabinet service).
func FortuneFeedIdentityStrings(r FortuneFeedRow) (tgU, tgFn, emailLocal *string) {
	return nullStringTrimmedPtr(r.TgUser), nullStringTrimmedPtr(r.TgFirst), nullStringTrimmedPtr(r.EmailLocal)
}

// ListRecentFeed — последние спины с полями для отображаемого имени (tg username → first_name из identity → локальная часть email).
func (r *FortuneRepo) ListRecentFeed(ctx context.Context, limit int) ([]FortuneFeedRow, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 200 {
		limit = 200
	}
	const q = `
SELECT fs.spin_at, fs.reward_type, fs.reward_value,
  NULLIF(TRIM(BOTH '@' FROM NULLIF(TRIM(LOWER(c.telegram_username)), '')), '') AS tg_u,
  (SELECT NULLIF(TRIM(ii.raw_profile_json->>'first_name'), '')
   FROM cabinet_identity ii
   WHERE ii.account_id = l.account_id AND ii.provider = 'telegram' AND ii.unlinked_at IS NULL
   ORDER BY ii.created_at DESC
   LIMIT 1) AS tg_fn,
  NULLIF(TRIM(SPLIT_PART(LOWER(TRIM(ca.email)), '@', 1)), '') AS em_loc
FROM fortune_spins fs
INNER JOIN customer c ON c.id = fs.customer_id
LEFT JOIN cabinet_account_customer_link l ON l.customer_id = c.id AND l.link_status = 'linked'
LEFT JOIN cabinet_account ca ON ca.id = l.account_id
ORDER BY fs.spin_at DESC, fs.id DESC
LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("fortune: list recent feed: %w", err)
	}
	defer rows.Close()
	var out []FortuneFeedRow
	for rows.Next() {
		var row FortuneFeedRow
		if err := rows.Scan(&row.SpinAt, &row.RewardType, &row.RewardValue, &row.TgUser, &row.TgFirst, &row.EmailLocal); err != nil {
			return nil, fmt.Errorf("fortune: scan feed row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// FortunePromoCode — код якорного промо для pending-скидок колеса.
func FortunePromoCode() string { return fortunePromoCode }
