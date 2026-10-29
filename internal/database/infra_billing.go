package database

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InfraBillingSettings — напоминания админу о сроке оплаты нод (за N календарных дней).
type InfraBillingSettings struct {
	NotifyBefore1  bool
	NotifyBefore3  bool
	NotifyBefore7  bool
	NotifyBefore14 bool
}

type InfraBillingRepository struct {
	pool *pgxpool.Pool
}

func NewInfraBillingRepository(pool *pgxpool.Pool) *InfraBillingRepository {
	return &InfraBillingRepository{pool: pool}
}

func (r *InfraBillingRepository) GetSettings(ctx context.Context) (InfraBillingSettings, error) {
	var s InfraBillingSettings
	err := r.pool.QueryRow(ctx, `
SELECT notify_before_1, notify_before_3, notify_before_7, notify_before_14
FROM admin_infra_billing_settings WHERE id = 1`).
		Scan(&s.NotifyBefore1, &s.NotifyBefore3, &s.NotifyBefore7, &s.NotifyBefore14)
	return s, err
}

func (r *InfraBillingRepository) SetNotifyBefore(ctx context.Context, days int, enabled bool) error {
	var col string
	switch days {
	case 1:
		col = "notify_before_1"
	case 3:
		col = "notify_before_3"
	case 7:
		col = "notify_before_7"
	case 14:
		col = "notify_before_14"
	default:
		return nil
	}
	q := "UPDATE admin_infra_billing_settings SET " + col + " = $1, updated_at = NOW() WHERE id = 1"
	_, err := r.pool.Exec(ctx, q, enabled)
	return err
}

func (r *InfraBillingRepository) WasNotifySent(ctx context.Context, billingUUID uuid.UUID, nextBillingAt time.Time, thresholdDays int) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
SELECT 1 FROM admin_infra_billing_notify_sent
WHERE billing_uuid = $1 AND next_billing_at = $2 AND threshold_days = $3
LIMIT 1`, billingUUID, nextBillingAt, thresholdDays).Scan(&n)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MarkNotifySent записывает факт отправки (один раз на порог на ноду за конкретный next_billing_at).
func (r *InfraBillingRepository) MarkNotifySent(ctx context.Context, billingUUID uuid.UUID, nextBillingAt time.Time, thresholdDays int) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO admin_infra_billing_notify_sent (billing_uuid, next_billing_at, threshold_days)
VALUES ($1, $2, $3)
ON CONFLICT (billing_uuid, next_billing_at, threshold_days) DO NOTHING`,
		billingUUID, nextBillingAt, thresholdDays)
	return err
}
