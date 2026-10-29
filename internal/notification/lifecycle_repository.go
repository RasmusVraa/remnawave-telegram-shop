package notification

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LifecycleRepository struct {
	pool *pgxpool.Pool
}

func NewLifecycleRepository(pool *pgxpool.Pool) *LifecycleRepository {
	return &LifecycleRepository{pool: pool}
}

// WasNotifySent проверяет, было ли уже отправлено уведомление данного типа с данным ключом.
func (r *LifecycleRepository) WasNotifySent(ctx context.Context, customerID int64, kind, referenceKey string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM customer_lifecycle_notify_sent 
			WHERE customer_id = $1 AND kind = $2 AND reference_key = $3
		)
	`, customerID, kind, referenceKey).Scan(&exists)
	return exists, err
}

// MarkNotifySent записывает факт отправки уведомления (dedup).
func (r *LifecycleRepository) MarkNotifySent(ctx context.Context, customerID int64, kind, referenceKey string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO customer_lifecycle_notify_sent (customer_id, kind, reference_key, sent_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (customer_id, kind, reference_key) DO NOTHING
	`, customerID, kind, referenceKey)
	return err
}

// FindNoConnectPaidCandidates находит пользователей, оплативших первую подписку, но не подключившихся.
// Критерии: CountPaidSubscriptions == 1, прошло >= delayHours с MIN(paid_at), <= maxAgeHours, expire_at > now.
func (r *LifecycleRepository) FindNoConnectPaidCandidates(ctx context.Context, delayHours, maxAgeHours int) ([]int64, error) {
	query := `
		WITH first_paid AS (
			SELECT 
				p.customer_id,
				MIN(p.paid_at) as first_paid_at,
				COUNT(*) as paid_count
			FROM purchase p
			WHERE p.status = 'paid' AND p.month > 0
			GROUP BY p.customer_id
			HAVING COUNT(*) = 1
		)
		SELECT DISTINCT c.id
		FROM customer c
		INNER JOIN first_paid fp ON fp.customer_id = c.id
		WHERE 
			c.expire_at > NOW()
			AND fp.first_paid_at <= NOW() - INTERVAL '1 hour' * $1
			AND fp.first_paid_at >= NOW() - INTERVAL '1 hour' * $2
			AND NOT c.is_web_only
			AND c.telegram_id > 0
			AND NOT EXISTS (
				SELECT 1 FROM customer_lifecycle_notify_sent ln
				WHERE ln.customer_id = c.id 
				  AND ln.kind = 'no_connect_paid'
				  AND ln.reference_key = 'once'
			)
	`
	rows, err := r.pool.Query(ctx, query, delayHours, maxAgeHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// FindNoConnectTrialCandidates находит триальных пользователей, не подключившихся.
// Критерии: subscription_link IS NOT NULL, CountPaidSubscriptions == 0, прошло >= delayHours с subscription_period_start.
func (r *LifecycleRepository) FindNoConnectTrialCandidates(ctx context.Context, delayHours, maxAgeHours int) ([]int64, error) {
	query := `
		WITH paid_counts AS (
			SELECT customer_id, COUNT(*) as cnt
			FROM purchase
			WHERE status = 'paid' AND month > 0
			GROUP BY customer_id
		)
		SELECT DISTINCT c.id
		FROM customer c
		LEFT JOIN paid_counts pc ON pc.customer_id = c.id
		WHERE
			c.subscription_link IS NOT NULL
			AND c.expire_at > NOW()
			AND c.subscription_period_start IS NOT NULL
			AND c.subscription_period_start <= NOW() - INTERVAL '1 hour' * $1
			AND c.subscription_period_start >= NOW() - INTERVAL '1 hour' * $2
			AND COALESCE(pc.cnt, 0) = 0
			AND NOT c.is_web_only
			AND c.telegram_id > 0
			AND NOT EXISTS (
				SELECT 1 FROM customer_lifecycle_notify_sent ln
				WHERE ln.customer_id = c.id 
				  AND ln.kind = 'no_connect_trial'
				  AND ln.reference_key = 'once'
			)
	`
	rows, err := r.pool.Query(ctx, query, delayHours, maxAgeHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// FindWinbackCandidates находит пользователей с истекшей подпиской ровно N дней назад.
// Критерии: expire_at ровно daysAfterExpiry календарных дней назад, был хотя бы один paid (month > 0, без триалов), не продлил после истечения.
func (r *LifecycleRepository) FindWinbackCandidates(ctx context.Context, daysAfterExpiry int) ([]WinbackCandidate, error) {
	query := `
		WITH target_expires AS (
			SELECT 
				c.id,
				c.telegram_id,
				c.language,
				c.expire_at,
				DATE_PART('day', NOW() - c.expire_at)::int as days_expired
			FROM customer c
			WHERE 
				c.expire_at <= NOW()
				AND c.subscription_link IS NOT NULL
				AND NOT c.is_web_only
				AND c.telegram_id > 0
				AND DATE_PART('day', NOW() - c.expire_at)::int = $1
		),
		has_paid AS (
			SELECT DISTINCT customer_id 
			FROM purchase 
			WHERE status = 'paid' AND month > 0
		),
		renewed_after_expiry AS (
			SELECT DISTINCT p.customer_id
			FROM purchase p
			INNER JOIN target_expires te ON te.id = p.customer_id
			WHERE p.status = 'paid' 
			  AND p.month > 0 
			  AND p.paid_at > te.expire_at
		)
		SELECT 
			te.id,
			te.telegram_id,
			te.language,
			te.expire_at
		FROM target_expires te
		INNER JOIN has_paid hp ON hp.customer_id = te.id
		WHERE NOT EXISTS (
			SELECT 1 FROM renewed_after_expiry ra WHERE ra.customer_id = te.id
		)
		AND NOT EXISTS (
			SELECT 1 FROM customer_lifecycle_notify_sent ln
			WHERE ln.customer_id = te.id 
			  AND ln.kind = 'winback'
			  AND ln.reference_key = TO_CHAR(te.expire_at, 'YYYY-MM-DD')
		)
	`

	rows, err := r.pool.Query(ctx, query, daysAfterExpiry)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []WinbackCandidate
	for rows.Next() {
		var c WinbackCandidate
		if err := rows.Scan(&c.CustomerID, &c.TelegramID, &c.Language, &c.ExpireAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

type WinbackCandidate struct {
	CustomerID int64
	TelegramID int64
	Language   string
	ExpireAt   time.Time
}
