package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Статусы moynalog_receipt (CHECK в миграции 000042).
const (
	MoynalogReceiptPending   = "pending"
	MoynalogReceiptSent      = "sent"
	MoynalogReceiptFailed    = "failed"
	MoynalogReceiptCancelled = "cancelled"
)

// MoynalogReceipt — строка очереди чеков «Мой налог».
type MoynalogReceipt struct {
	ID            int64
	PurchaseID    int64
	Amount        float64
	Description   string
	OperationTime time.Time
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	LastError     *string
	ReceiptID     *string
	AlertedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MoynalogReceiptRepository struct {
	pool *pgxpool.Pool
}

func NewMoynalogReceiptRepository(pool *pgxpool.Pool) *MoynalogReceiptRepository {
	return &MoynalogReceiptRepository{pool: pool}
}

const moynalogReceiptCols = `id, purchase_id, amount, description, operation_time,
	status, attempts, next_attempt_at, last_error, receipt_id, alerted_at, created_at, updated_at`

func scanMoynalogReceipt(row pgx.Row) (*MoynalogReceipt, error) {
	var r MoynalogReceipt
	err := row.Scan(
		&r.ID, &r.PurchaseID, &r.Amount, &r.Description, &r.OperationTime,
		&r.Status, &r.Attempts, &r.NextAttemptAt, &r.LastError, &r.ReceiptID,
		&r.AlertedAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Enqueue создаёт строку чека в состоянии pending до попытки отправки (outbox).
//
// Повторный вызов для той же покупки не создаёт вторую строку и не воскрешает
// уже отправленный чек — UNIQUE(purchase_id) плюс DO NOTHING. Возвращает
// существующую строку, если она уже была.
func (r *MoynalogReceiptRepository) Enqueue(
	ctx context.Context,
	purchaseID int64,
	amount float64,
	description string,
	operationTime time.Time,
) (*MoynalogReceipt, error) {
	const q = `
		INSERT INTO moynalog_receipt (purchase_id, amount, description, operation_time)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (purchase_id) DO NOTHING
		RETURNING ` + moynalogReceiptCols

	row, err := scanMoynalogReceipt(r.pool.QueryRow(ctx, q, purchaseID, amount, description, operationTime))
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("enqueue moynalog receipt: %w", err)
	}

	// Конфликт: строка по этой покупке уже есть — отдаём её.
	existing, err := r.GetByPurchaseID(ctx, purchaseID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("enqueue moynalog receipt: conflict on purchase %d but row not found", purchaseID)
	}
	return existing, nil
}

func (r *MoynalogReceiptRepository) GetByPurchaseID(ctx context.Context, purchaseID int64) (*MoynalogReceipt, error) {
	const q = `SELECT ` + moynalogReceiptCols + ` FROM moynalog_receipt WHERE purchase_id = $1`
	row, err := scanMoynalogReceipt(r.pool.QueryRow(ctx, q, purchaseID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get moynalog receipt: %w", err)
	}
	return row, nil
}

// Claim захватывает чек на отправку до leaseUntil: переносит next_attempt_at
// в будущее, и ListDue его больше не отдаёт. Возвращает false, если чек уже
// отправлен, снят или его прямо сейчас отправляет другой (next_attempt_at ещё
// не наступил).
//
// Без захвата фоновая отправка сразу после оплаты и тик воркера брали одну
// pending-строку одновременно — и ФНС получала два чека на одну оплату. Если
// процесс упадёт посреди отправки, захват истечёт и чек подхватит воркер.
func (r *MoynalogReceiptRepository) Claim(ctx context.Context, id int64, leaseUntil time.Time) (bool, error) {
	const q = `
		UPDATE moynalog_receipt
		SET next_attempt_at = $3, updated_at = now()
		WHERE id = $1 AND status = $2 AND next_attempt_at <= now()`
	tag, err := r.pool.Exec(ctx, q, id, MoynalogReceiptPending, leaseUntil)
	if err != nil {
		return false, fmt.Errorf("claim moynalog receipt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// MarkSent закрывает чек успешно, сохраняя присвоенный ФНС идентификатор.
func (r *MoynalogReceiptRepository) MarkSent(ctx context.Context, id int64, receiptID string) error {
	const q = `
		UPDATE moynalog_receipt
		SET status = $2, receipt_id = $3, last_error = NULL, attempts = attempts + 1, updated_at = now()
		WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, MoynalogReceiptSent, receiptID); err != nil {
		return fmt.Errorf("mark moynalog receipt sent: %w", err)
	}
	return nil
}

// MarkAttemptFailed фиксирует неудачу и переносит следующую попытку.
func (r *MoynalogReceiptRepository) MarkAttemptFailed(ctx context.Context, id int64, sendErr string, nextAttemptAt time.Time) error {
	const q = `
		UPDATE moynalog_receipt
		SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3, updated_at = now()
		WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, sendErr, nextAttemptAt); err != nil {
		return fmt.Errorf("mark moynalog receipt attempt failed: %w", err)
	}
	return nil
}

// MarkFailed окончательно снимает чек с повторов (превышен предельный возраст).
func (r *MoynalogReceiptRepository) MarkFailed(ctx context.Context, id int64, sendErr string) error {
	const q = `
		UPDATE moynalog_receipt
		SET status = $2, last_error = $3, updated_at = now()
		WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, MoynalogReceiptFailed, sendErr); err != nil {
		return fmt.Errorf("mark moynalog receipt failed: %w", err)
	}
	return nil
}

// MarkAlerted помечает, что админу уже сообщили о проблеме по этой строке —
// чтобы во время многодневного простоя не слать сообщение на каждой попытке.
func (r *MoynalogReceiptRepository) MarkAlerted(ctx context.Context, id int64) error {
	const q = `UPDATE moynalog_receipt SET alerted_at = now(), updated_at = now() WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("mark moynalog receipt alerted: %w", err)
	}
	return nil
}

// ListDue возвращает чеки, которым пора на повторную отправку.
func (r *MoynalogReceiptRepository) ListDue(ctx context.Context, limit int) ([]MoynalogReceipt, error) {
	const q = `
		SELECT ` + moynalogReceiptCols + `
		FROM moynalog_receipt
		WHERE status = $1 AND next_attempt_at <= now()
		ORDER BY next_attempt_at
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, MoynalogReceiptPending, limit)
	if err != nil {
		return nil, fmt.Errorf("list due moynalog receipts: %w", err)
	}
	defer rows.Close()

	out := make([]MoynalogReceipt, 0, limit)
	for rows.Next() {
		row, err := scanMoynalogReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan due moynalog receipt: %w", err)
		}
		out = append(out, *row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due moynalog receipts: %w", err)
	}
	return out, nil
}

// CountPending — сколько чеков ждёт отправки (для сводки в уведомлении).
func (r *MoynalogReceiptRepository) CountPending(ctx context.Context) (int64, error) {
	const q = `SELECT COUNT(*) FROM moynalog_receipt WHERE status = $1`
	var n int64
	if err := r.pool.QueryRow(ctx, q, MoynalogReceiptPending).Scan(&n); err != nil {
		return 0, fmt.Errorf("count pending moynalog receipts: %w", err)
	}
	return n, nil
}
