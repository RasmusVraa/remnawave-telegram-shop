package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmailMergeVerification struct {
	ID           int64
	AccountID    int64
	PeerAccountID int64
	CodeHash     string
	MaskedEmail  string
	AttemptsLeft int
	ExpiresAt    time.Time
}

type EmailMergeVerificationRepo struct {
	pool *pgxpool.Pool
}

func NewEmailMergeVerificationRepo(pool *pgxpool.Pool) *EmailMergeVerificationRepo {
	return &EmailMergeVerificationRepo{pool: pool}
}

func (r *EmailMergeVerificationRepo) Upsert(ctx context.Context, accountID, peerAccountID int64, codeHash, maskedEmail string, attempts int, expiresAt time.Time) error {
	const q = `
		INSERT INTO cabinet_email_merge_verification
			(account_id, peer_account_id, code_hash, masked_email, attempts_left, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (account_id) DO UPDATE SET
			peer_account_id = EXCLUDED.peer_account_id,
			code_hash = EXCLUDED.code_hash,
			masked_email = EXCLUDED.masked_email,
			attempts_left = EXCLUDED.attempts_left,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()`
	_, err := r.pool.Exec(ctx, q, accountID, peerAccountID, codeHash, maskedEmail, attempts, expiresAt)
	if err != nil {
		return fmt.Errorf("upsert email merge verification: %w", err)
	}
	return nil
}

func (r *EmailMergeVerificationRepo) Get(ctx context.Context, accountID int64) (*EmailMergeVerification, error) {
	const q = `
		SELECT id, account_id, peer_account_id, code_hash, masked_email, attempts_left, expires_at
		  FROM cabinet_email_merge_verification
		 WHERE account_id = $1`
	var v EmailMergeVerification
	err := r.pool.QueryRow(ctx, q, accountID).Scan(
		&v.ID, &v.AccountID, &v.PeerAccountID, &v.CodeHash, &v.MaskedEmail, &v.AttemptsLeft, &v.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get email merge verification: %w", err)
	}
	return &v, nil
}

func (r *EmailMergeVerificationRepo) Delete(ctx context.Context, accountID int64) error {
	const q = `DELETE FROM cabinet_email_merge_verification WHERE account_id = $1`
	if _, err := r.pool.Exec(ctx, q, accountID); err != nil {
		return fmt.Errorf("delete email merge verification: %w", err)
	}
	return nil
}

func (r *EmailMergeVerificationRepo) DecrementAttempts(ctx context.Context, accountID int64) (int, error) {
	const q = `
		UPDATE cabinet_email_merge_verification
		   SET attempts_left = attempts_left - 1, updated_at = NOW()
		 WHERE account_id = $1
		RETURNING attempts_left`
	var left int
	if err := r.pool.QueryRow(ctx, q, accountID).Scan(&left); err != nil {
		return 0, fmt.Errorf("decrement attempts: %w", err)
	}
	return left, nil
}
