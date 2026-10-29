package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Referral struct {
	ID           int64     `db:"id"`
	ReferrerID   int64     `db:"referrer_id"`
	RefereeID    int64     `db:"referee_id"`
	UsedAt       time.Time `db:"used_at"`
	BonusGranted bool      `db:"bonus_granted"`
}

type RefereeSummary struct {
	TelegramID       int64
	Active           bool
	TelegramUsername *string
	Email            *string
}

type ReferralStats struct {
	Total           int
	Paid            int
	Active          int
	Conversion      int
	EarnedTotal     int
	EarnedLastMonth int
}

type ReferralRepository struct {
	pool *pgxpool.Pool
	// Журнал начислений — единственный источник правды по выданным дням.
	// Собирается здесь же, а не прокидывается снаружи: ему нужен только пул, и
	// внутренняя сборка избавляет от правки всех вызовов NewReferralRepository.
	ledger *ReferralBonusLedgerRepository
}

func NewReferralRepository(pool *pgxpool.Pool) *ReferralRepository {
	return &ReferralRepository{pool: pool, ledger: NewReferralBonusLedgerRepository(pool)}
}

// Create привязывает приглашённого к рефереру. У приглашённого одна связь
// (UNIQUE referee_id, миграция 000055): если она уже есть, возвращает nil, nil —
// первая связь остаётся, вторая молча не создаётся.
func (r *ReferralRepository) Create(ctx context.Context, referrerID, refereeID int64) (*Referral, error) {
	query := sq.Insert("referral").
		Columns("referrer_id", "referee_id", "used_at", "bonus_granted").
		Values(referrerID, refereeID, sq.Expr("NOW()"), false).
		Suffix("ON CONFLICT (referee_id) DO NOTHING RETURNING id, referrer_id, referee_id, used_at, bonus_granted").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build insert referral query: %w", err)
	}

	row := r.pool.QueryRow(ctx, sql, args...)
	var ref Referral
	if err := row.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan inserted referral: %w", err)
	}
	return &ref, nil
}

func (r *ReferralRepository) FindByReferrer(ctx context.Context, referrerID int64) ([]Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		OrderBy("used_at DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referrals by referrer query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrals by referrer: %w", err)
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted); err != nil {
			return nil, fmt.Errorf("failed to scan referral row: %w", err)
		}
		list = append(list, ref)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral rows: %w", rows.Err())
	}
	return list, nil
}

func (r *ReferralRepository) CountByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count referrals by referrer query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of referrals: %w", err)
	}
	return count, nil
}

// CountPaidReferralsByReferrer подсчитывает количество рефералов, которые оплатили подписку
func (r *ReferralRepository) CountPaidReferralsByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(DISTINCT r.referee_id)").
		From("referral r").
		Join("customer c ON c.telegram_id = r.referee_id").
		Join("purchase p ON p.customer_id = c.id").
		Where(sq.Eq{"r.referrer_id": referrerID, "p.status": PurchaseStatusPaid}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count paid referrals by referrer query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of paid referrals: %w", err)
	}
	return count, nil
}

func (r *ReferralRepository) CountActiveReferralsByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("referral r").
		Join("customer c ON c.telegram_id = r.referee_id").
		Where(sq.Eq{"r.referrer_id": referrerID}).
		Where("c.expire_at IS NOT NULL AND c.expire_at > NOW()").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count active referrals query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of active referrals: %w", err)
	}
	return count, nil
}

func (r *ReferralRepository) FindRefereeSummariesByReferrer(ctx context.Context, referrerID int64) ([]RefereeSummary, error) {
	return r.FindRefereeSummariesByReferrerPage(ctx, referrerID, 0, 0)
}

func (r *ReferralRepository) FindRefereeSummariesByReferrerPage(ctx context.Context, referrerID int64, limit, offset int) ([]RefereeSummary, error) {
	query := sq.Select("r.referee_id", "c.expire_at", "c.telegram_username", "a.email").
		From("referral r").
		Join("customer c ON c.telegram_id = r.referee_id").
		LeftJoin("cabinet_account_customer_link l ON l.customer_id = c.id").
		LeftJoin("cabinet_account a ON a.id = l.account_id").
		Where(sq.Eq{"r.referrer_id": referrerID}).
		OrderBy("r.used_at DESC").
		PlaceholderFormat(sq.Dollar)

	if limit > 0 {
		query = query.Limit(uint64(limit)).Offset(uint64(offset))
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referral list query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referral list: %w", err)
	}
	defer rows.Close()

	var list []RefereeSummary
	for rows.Next() {
		var refereeID int64
		var expireAt *time.Time
		var telegramUsername *string
		var email *string
		if err := rows.Scan(&refereeID, &expireAt, &telegramUsername, &email); err != nil {
			return nil, fmt.Errorf("failed to scan referral list row: %w", err)
		}
		active := expireAt != nil && expireAt.After(time.Now())
		list = append(list, RefereeSummary{
			TelegramID:       refereeID,
			Active:           active,
			TelegramUsername: telegramUsername,
			Email:            email,
		})
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral list rows: %w", rows.Err())
	}
	return list, nil
}

// CalculateEarnedDays возвращает фактически начисленные пригласившему дни.
//
// Раньше число выводилось из счётчика оплативших рефералов, умноженного на
// текущую настройку. Такой пересчёт задним числом менял историю при каждой
// правке настроек и в принципе не умел учитывать множитель за длину периода:
// из «реферал оплатил трижды» нельзя восстановить, на какой срок.
func (r *ReferralRepository) CalculateEarnedDays(ctx context.Context, referrerID int64) (int, error) {
	total, _, err := r.ledger.EarnedDaysByReferrer(ctx, referrerID)
	return total, err
}

func (r *ReferralRepository) GetStats(ctx context.Context, referrerID int64) (ReferralStats, error) {
	totalCount, err := r.CountByReferrer(ctx, referrerID)
	if err != nil {
		return ReferralStats{}, err
	}
	paidCount, err := r.CountPaidReferralsByReferrer(ctx, referrerID)
	if err != nil {
		return ReferralStats{}, err
	}
	activeCount, err := r.CountActiveReferralsByReferrer(ctx, referrerID)
	if err != nil {
		return ReferralStats{}, err
	}

	conversion := 0
	if totalCount > 0 {
		conversion = int(math.Round(float64(paidCount) * 100 / float64(totalCount)))
	}

	// Журнал начислений — единственный источник правды по выданным дням: и
	// режим, и множитель за длину периода уже вшиты в записанные строки.
	earnedTotal, earnedLastMonth, err := r.ledger.EarnedDaysByReferrer(ctx, referrerID)
	if err != nil {
		return ReferralStats{}, err
	}

	return ReferralStats{
		Total:           totalCount,
		Paid:            paidCount,
		Active:          activeCount,
		Conversion:      conversion,
		EarnedTotal:     earnedTotal,
		EarnedLastMonth: earnedLastMonth,
	}, nil
}

// RecentEarnedBonuses — лента «откуда взялись дни» для пригласившего.
//
// Идёт через тот же журнал, что и сумма в GetStats, поэтому лента и итог не
// могут разойтись: это один источник, а не две реализации одной формулы.
func (r *ReferralRepository) RecentEarnedBonuses(ctx context.Context, referrerID int64, limit int) ([]ReferralBonusFeedEntry, error) {
	return r.ledger.ListRecentByReferrer(ctx, referrerID, limit)
}

// EarnedDaysByReferee — сколько дней принёс каждый приглашённый.
func (r *ReferralRepository) EarnedDaysByReferee(ctx context.Context, referrerID int64) (map[int64]int, error) {
	return r.ledger.SumDaysByReferee(ctx, referrerID)
}

func (r *ReferralRepository) FindByReferee(ctx context.Context, refereeID int64) (*Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted").
		From("referral").
		Where(sq.Eq{"referee_id": refereeID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referral by referee query: %w", err)
	}

	var ref Referral
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.UsedAt, &ref.BonusGranted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query referral by referee: %w", err)
	}
	return &ref, nil
}

func (r *ReferralRepository) MarkBonusGranted(ctx context.Context, referralID int64) error {
	query := sq.Update("referral").
		Set("bonus_granted", true).
		Where(sq.Eq{"id": referralID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update bonus_granted query: %w", err)
	}

	res, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update bonus_granted: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("no referral record updated")
	}
	return nil
}
