package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PasswordReset — модель cabinet_password_reset_token.
type PasswordReset struct {
	ID        int64
	AccountID int64
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsUsable — не истёк и не использован.
func (p *PasswordReset) IsUsable() bool {
	return p.UsedAt == nil && time.Now().Before(p.ExpiresAt)
}

// PasswordResetRepo — cabinet_password_reset_token.
type PasswordResetRepo struct {
	pool *pgxpool.Pool
}

// NewPasswordResetRepo — конструктор.
func NewPasswordResetRepo(pool *pgxpool.Pool) *PasswordResetRepo {
	return &PasswordResetRepo{pool: pool}
}

const prSelectCols = "id, account_id, token_hash, expires_at, used_at, created_at"

func scanPR(row pgx.Row) (*PasswordReset, error) {
	var p PasswordReset
	err := row.Scan(
		&p.ID,
		&p.AccountID,
		&p.TokenHash,
		&p.ExpiresAt,
		&p.UsedAt,
		&p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan password reset: %w", err)
	}
	return &p, nil
}

// Create вставляет новый токен сброса пароля.
func (r *PasswordResetRepo) Create(ctx context.Context, accountID int64, tokenHash [32]byte, expiresAt time.Time) (*PasswordReset, error) {
	const q = `
		INSERT INTO cabinet_password_reset_token (account_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING ` + prSelectCols
	return scanPR(r.pool.QueryRow(ctx, q, accountID, tokenHash[:], expiresAt))
}

// FindByHash — поиск по sha256.
func (r *PasswordResetRepo) FindByHash(ctx context.Context, tokenHash [32]byte) (*PasswordReset, error) {
	const q = `SELECT ` + prSelectCols + ` FROM cabinet_password_reset_token WHERE token_hash = $1`
	return scanPR(r.pool.QueryRow(ctx, q, tokenHash[:]))
}

// MarkUsed — used_at = NOW().
func (r *PasswordResetRepo) MarkUsed(ctx context.Context, id int64) error {
	const q = `UPDATE cabinet_password_reset_token SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark pr used: %w", err)
	}
	return nil
}

// InvalidateForAccount гасит все предыдущие живые reset-токены, чтобы резкое
// нажатие «Forgot password» два раза подряд не оставляло два рабочих токена.
func (r *PasswordResetRepo) InvalidateForAccount(ctx context.Context, accountID int64) error {
	const q = `UPDATE cabinet_password_reset_token SET used_at = NOW() WHERE account_id = $1 AND used_at IS NULL`
	_, err := r.pool.Exec(ctx, q, accountID)
	if err != nil {
		return fmt.Errorf("invalidate pr: %w", err)
	}
	return nil
}
