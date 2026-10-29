package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Статусы заявки на вывод (CHECK в миграции 000044).
const (
	PartnerPayoutPending  = "pending"
	PartnerPayoutApproved = "approved"
	PartnerPayoutPaid     = "paid"
	PartnerPayoutRejected = "rejected"
)

// ErrPartnerInsufficientBalance — на балансе меньше запрошенного либо партнёр
// не в статусе active. Оба случая — одна и та же ситуация «вывод сейчас
// невозможен», и различать их в ответе не нужно: статус партнёр и так видит.
var ErrPartnerInsufficientBalance = errors.New("insufficient partner balance")

type PartnerPayout struct {
	ID              int64
	PartnerID       int64
	Amount          float64
	Status          string
	Method          *string
	DetailsSnapshot *string
	AdminComment    *string
	ExternalRef     *string
	RequestedAt     time.Time
	ProcessedAt     *time.Time
	ProcessedBy     *int64
}

const partnerPayoutColumns = `id, partner_id, amount, status, method, details_snapshot,
	admin_comment, external_ref, requested_at, processed_at, processed_by`

func scanPartnerPayout(row pgx.Row) (*PartnerPayout, error) {
	var p PartnerPayout
	err := row.Scan(&p.ID, &p.PartnerID, &p.Amount, &p.Status, &p.Method, &p.DetailsSnapshot,
		&p.AdminComment, &p.ExternalRef, &p.RequestedAt, &p.ProcessedAt, &p.ProcessedBy)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePayoutDetails сохраняет реквизиты партнёра.
func (r *PartnerRepository) UpdatePayoutDetails(ctx context.Context, partnerID int64, method, details string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE partner SET payout_method = $2, payout_details = $3, updated_at = now() WHERE id = $1`,
		partnerID, method, details)
	if err != nil {
		return fmt.Errorf("failed to update partner payout details: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("partner %d not found", partnerID)
	}
	return nil
}

// CreatePayout резервирует сумму и создаёт заявку на вывод.
//
// Списание с баланса происходит СРАЗУ, в той же транзакции, что и заявка:
// иначе партнёр отправит три заявки на всю сумму, пока админ обрабатывает
// первую. Деньги не исчезают — они переезжают в reserved_balance и вернутся на
// баланс, если заявку отклонят.
//
// Проверка «хватает ли денег» встроена в UPDATE (balance >= $2), а не сделана
// отдельным SELECT: между чтением и записью помещается вторая заявка из
// параллельной вкладки.
//
// Бизнес-ограничения — минимальная сумма и кулдаун — сюда намеренно не входят:
// они зависят от настроек, а не от согласованности данных, и живут в обработчике.
func (r *PartnerRepository) CreatePayout(ctx context.Context, partnerID int64, amount float64, method, details *string) (*PartnerPayout, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("partner payout: amount must be positive, got %.2f", amount)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin partner payout tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE partner
		    SET balance = balance - $2,
		        reserved_balance = reserved_balance + $2,
		        updated_at = now()
		  WHERE id = $1 AND status = $3 AND balance >= $2`,
		partnerID, amount, PartnerStatusActive)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve partner balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrPartnerInsufficientBalance
	}

	payout, err := scanPartnerPayout(tx.QueryRow(ctx,
		`INSERT INTO partner_payout (partner_id, amount, status, method, details_snapshot)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+partnerPayoutColumns,
		partnerID, amount, PartnerPayoutPending, method, details))
	if err != nil {
		return nil, fmt.Errorf("failed to insert partner payout: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit partner payout tx: %w", err)
	}
	return payout, nil
}

// ListPayouts — история заявок партнёра, новые сверху.
func (r *PartnerRepository) ListPayouts(ctx context.Context, partnerID int64, limit, offset int) ([]PartnerPayout, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM partner_payout WHERE partner_id = $1`, partnerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count partner payouts: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+partnerPayoutColumns+`
		   FROM partner_payout WHERE partner_id = $1
		  ORDER BY requested_at DESC, id DESC LIMIT $2 OFFSET $3`, partnerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list partner payouts: %w", err)
	}
	defer rows.Close()

	var out []PartnerPayout
	for rows.Next() {
		var p PartnerPayout
		if err := rows.Scan(&p.ID, &p.PartnerID, &p.Amount, &p.Status, &p.Method, &p.DetailsSnapshot,
			&p.AdminComment, &p.ExternalRef, &p.RequestedAt, &p.ProcessedAt, &p.ProcessedBy); err != nil {
			return nil, 0, fmt.Errorf("failed to scan partner payout: %w", err)
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		return nil, 0, fmt.Errorf("error iterating partner payouts: %w", rows.Err())
	}
	return out, total, nil
}

// LastPayoutRequestAt — когда партнёр последний раз просил вывод. Отклонённые
// заявки не считаются: отказ не должен запирать партнёра на кулдаун, особенно
// если отказали из-за опечатки в реквизитах.
func (r *PartnerRepository) LastPayoutRequestAt(ctx context.Context, partnerID int64) (*time.Time, error) {
	var at *time.Time
	if err := r.pool.QueryRow(ctx,
		`SELECT MAX(requested_at) FROM partner_payout
		  WHERE partner_id = $1 AND status <> $2`, partnerID, PartnerPayoutRejected).Scan(&at); err != nil {
		return nil, fmt.Errorf("failed to load last partner payout time: %w", err)
	}
	return at, nil
}

// HasOpenPayout — есть ли необработанная заявка. Вторую подавать незачем:
// деньги под первую уже зарезервированы.
func (r *PartnerRepository) HasOpenPayout(ctx context.Context, partnerID int64) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM partner_payout
		                 WHERE partner_id = $1 AND status IN ($2, $3))`,
		partnerID, PartnerPayoutPending, PartnerPayoutApproved).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check open partner payouts: %w", err)
	}
	return exists, nil
}
