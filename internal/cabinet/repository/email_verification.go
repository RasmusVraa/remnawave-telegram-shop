package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailVerification — модель cabinet_email_verification.
type EmailVerification struct {
	ID        int64
	AccountID int64
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
	// Attempts — сколько раз к коду уже приложили догадку (см. ReserveAttempt).
	Attempts int
}

// IsUsable — не истёк и не использован.
func (e *EmailVerification) IsUsable() bool {
	return e.UsedAt == nil && time.Now().Before(e.ExpiresAt)
}

// EmailVerificationRepo — cabinet_email_verification.
type EmailVerificationRepo struct {
	pool *pgxpool.Pool
}

// NewEmailVerificationRepo — конструктор.
func NewEmailVerificationRepo(pool *pgxpool.Pool) *EmailVerificationRepo {
	return &EmailVerificationRepo{pool: pool}
}

const evSelectCols = "id, account_id, token_hash, expires_at, used_at, created_at, attempts"

func scanEV(row pgx.Row) (*EmailVerification, error) {
	var e EmailVerification
	err := row.Scan(
		&e.ID,
		&e.AccountID,
		&e.TokenHash,
		&e.ExpiresAt,
		&e.UsedAt,
		&e.CreatedAt,
		&e.Attempts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan email verification: %w", err)
	}
	return &e, nil
}

// Create вставляет запись. Caller инвалидирует предыдущие токены через
// InvalidateForAccount перед созданием новой (resend).
func (r *EmailVerificationRepo) Create(ctx context.Context, accountID int64, tokenHash [32]byte, expiresAt time.Time) (*EmailVerification, error) {
	const q = `
		INSERT INTO cabinet_email_verification (account_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING ` + evSelectCols
	return scanEV(r.pool.QueryRow(ctx, q, accountID, tokenHash[:], expiresAt))
}

// FindByHash ищет токен по sha256 во всей таблице. Только для старых ссылок
// из писем (256 бит): коды из шести цифр так искать нельзя — см. FindActiveForAccount.
// ErrNotFound — нет такого токена.
func (r *EmailVerificationRepo) FindByHash(ctx context.Context, tokenHash [32]byte) (*EmailVerification, error) {
	const q = `SELECT ` + evSelectCols + ` FROM cabinet_email_verification
		WHERE token_hash = $1 ORDER BY id DESC LIMIT 1`
	return scanEV(r.pool.QueryRow(ctx, q, tokenHash[:]))
}

// MarkUsed ставит used_at = NOW().
func (r *EmailVerificationRepo) MarkUsed(ctx context.Context, id int64) error {
	const q = `UPDATE cabinet_email_verification SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark ev used: %w", err)
	}
	return nil
}

// InvalidateForAccount — перед созданием нового кода «погасить» все
// предыдущие живые (так resend гарантированно даст ровно один рабочий токен).
func (r *EmailVerificationRepo) InvalidateForAccount(ctx context.Context, accountID int64) error {
	const q = `UPDATE cabinet_email_verification SET used_at = NOW() WHERE account_id = $1 AND used_at IS NULL`
	_, err := r.pool.Exec(ctx, q, accountID)
	if err != nil {
		return fmt.Errorf("invalidate ev: %w", err)
	}
	return nil
}

// FindActiveForAccount возвращает живой (не использованный и не истёкший) код
// аккаунта. Resend гасит предыдущие, поэтому живой код у аккаунта один.
// ErrNotFound — живого кода нет.
func (r *EmailVerificationRepo) FindActiveForAccount(ctx context.Context, accountID int64) (*EmailVerification, error) {
	const q = `SELECT ` + evSelectCols + ` FROM cabinet_email_verification
		WHERE account_id = $1 AND used_at IS NULL AND expires_at > NOW()
		ORDER BY id DESC LIMIT 1`
	return scanEV(r.pool.QueryRow(ctx, q, accountID))
}

// ReserveAttempt засчитывает попытку до сравнения кода и сообщает, осталась ли
// она в лимите. Счётчик растёт атомарно в самой строке: параллельные запросы
// не могут прочитать одно и то же «попыток ещё 0» и проверить по сотне догадок.
// Исчерпавший попытки код дальше не принимается — нужен новый (resend).
func (r *EmailVerificationRepo) ReserveAttempt(ctx context.Context, id int64, maxAttempts int) (bool, error) {
	const q = `
		UPDATE cabinet_email_verification
		   SET attempts = attempts + 1
		 WHERE id = $1 AND used_at IS NULL AND expires_at > NOW() AND attempts < $2`
	tag, err := r.pool.Exec(ctx, q, id, maxAttempts)
	if err != nil {
		return false, fmt.Errorf("reserve ev attempt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Consume гасит код после верной догадки. false — код уже погашен параллельным
// запросом с той же верной догадкой.
func (r *EmailVerificationRepo) Consume(ctx context.Context, id int64) (bool, error) {
	const q = `UPDATE cabinet_email_verification SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return false, fmt.Errorf("consume ev: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
