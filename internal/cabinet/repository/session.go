package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Session — модель cabinet_session.
type Session struct {
	ID                   int64
	AccountID            int64
	RefreshTokenHash     []byte // BYTEA, sha256
	RefreshTokenFamilyID uuid.UUID
	UserAgent            *string
	IP                   *net.IP
	CreatedAt            time.Time
	ExpiresAt            time.Time
	RevokedAt            *time.Time
	RotatedToSessionID   *int64
}

// IsActive — живая ли сессия сейчас.
func (s *Session) IsActive() bool {
	return s.RevokedAt == nil && time.Now().Before(s.ExpiresAt)
}

// IsRotated — была ли заменена на новую. Отличается от просто RevokedAt тем,
// что логаут тоже ставит revoked_at, но rotated_to_session_id там NULL.
func (s *Session) IsRotated() bool {
	return s.RotatedToSessionID != nil
}

// SessionRepo — cabinet_session.
type SessionRepo struct {
	pool *pgxpool.Pool
}

// NewSessionRepo — конструктор.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo { return &SessionRepo{pool: pool} }

const sessionSelectCols = "id, account_id, refresh_token_hash, refresh_token_family_id, user_agent, ip, created_at, expires_at, revoked_at, rotated_to_session_id"

func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	var pgIP *netip.Prefix // INET; NULL — nil
	err := row.Scan(
		&s.ID,
		&s.AccountID,
		&s.RefreshTokenHash,
		&s.RefreshTokenFamilyID,
		&s.UserAgent,
		&pgIP,
		&s.CreatedAt,
		&s.ExpiresAt,
		&s.RevokedAt,
		&s.RotatedToSessionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan session: %w", err)
	}
	if pgIP != nil && pgIP.Addr().IsValid() {
		ip := net.IP(pgIP.Addr().AsSlice())
		s.IP = &ip
	}
	return &s, nil
}

// CreateInput — параметры создания новой сессии (при логине или после ротации).
type CreateInput struct {
	AccountID int64
	TokenHash [32]byte
	FamilyID  uuid.UUID // при первом входе генерируется новый; при ротации наследуется
	UserAgent string
	IP        string
	ExpiresAt time.Time
}

// Create вставляет новую строку cabinet_session и возвращает её.
func (r *SessionRepo) Create(ctx context.Context, in CreateInput) (*Session, error) {
	return r.createIn(ctx, r.pool, in)
}

// createIn работает в любом QueryableEx (pool или tx).
func (r *SessionRepo) createIn(ctx context.Context, q queryable, in CreateInput) (*Session, error) {
	var uaArg any
	if in.UserAgent != "" {
		uaArg = in.UserAgent
	}
	var ipArg any
	if in.IP != "" {
		ipArg = in.IP
	}
	const sqlStmt = `
		INSERT INTO cabinet_session (account_id, refresh_token_hash, refresh_token_family_id, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + sessionSelectCols
	return scanSession(q.QueryRow(ctx, sqlStmt,
		in.AccountID, in.TokenHash[:], in.FamilyID, uaArg, ipArg, in.ExpiresAt))
}

// FindByRefreshHash ищет сессию по sha256-хешу refresh-токена.
// ErrNotFound, если такого хеша нет (например, refresh подделан).
func (r *SessionRepo) FindByRefreshHash(ctx context.Context, hash [32]byte) (*Session, error) {
	const q = `SELECT ` + sessionSelectCols + ` FROM cabinet_session WHERE refresh_token_hash = $1`
	return scanSession(r.pool.QueryRow(ctx, q, hash[:]))
}

// Rotate выполняет атомарную ротацию: revoke старой сессии и создаёт новую
// с тем же family_id. Возвращает новую. Гарантии:
//   - если старая уже revoked (reuse!) — возвращаем ErrReused, не создаём новую;
//   - всё в одной транзакции, чтобы никогда не остаться в промежуточном состоянии.
func (r *SessionRepo) Rotate(ctx context.Context, oldSessionID int64, in CreateInput) (*Session, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin rotate tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Лочим старую строку.
	var revokedAt *time.Time
	var familyID uuid.UUID
	var accountID int64
	err = tx.QueryRow(ctx,
		`SELECT revoked_at, refresh_token_family_id, account_id
		   FROM cabinet_session
		  WHERE id = $1
		  FOR UPDATE`,
		oldSessionID,
	).Scan(&revokedAt, &familyID, &accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock old session: %w", err)
	}
	if revokedAt != nil {
		return nil, ErrReused
	}
	if accountID != in.AccountID {
		return nil, fmt.Errorf("session account mismatch")
	}

	in.FamilyID = familyID
	newSess, err := r.createIn(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE cabinet_session
		    SET revoked_at = NOW(), rotated_to_session_id = $2
		  WHERE id = $1`,
		oldSessionID, newSess.ID,
	); err != nil {
		return nil, fmt.Errorf("mark old session rotated: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit rotate: %w", err)
	}
	return newSess, nil
}

// ErrReused сигнализирует, что refresh-токен уже был ротирован ранее. Это
// признак компрометации: вызывающий обязан revoke всю family.
var ErrReused = errors.New("repository: refresh token reused")

// Revoke помечает одну сессию как отозванную (logout).
func (r *SessionRepo) Revoke(ctx context.Context, id int64) error {
	const q = `UPDATE cabinet_session SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeFamily отзывает все «живые» сессии из family. Вызывается при
// обнаружении reuse старого refresh-токена.
func (r *SessionRepo) RevokeFamily(ctx context.Context, familyID uuid.UUID) (int64, error) {
	const q = `UPDATE cabinet_session SET revoked_at = NOW() WHERE refresh_token_family_id = $1 AND revoked_at IS NULL`
	ct, err := r.pool.Exec(ctx, q, familyID)
	if err != nil {
		return 0, fmt.Errorf("revoke family: %w", err)
	}
	return ct.RowsAffected(), nil
}

// RevokeAllForAccount — logout со всех устройств (смена пароля, «выйти отовсюду»).
func (r *SessionRepo) RevokeAllForAccount(ctx context.Context, accountID int64) (int64, error) {
	const q = `UPDATE cabinet_session SET revoked_at = NOW() WHERE account_id = $1 AND revoked_at IS NULL`
	ct, err := r.pool.Exec(ctx, q, accountID)
	if err != nil {
		return 0, fmt.Errorf("revoke all: %w", err)
	}
	return ct.RowsAffected(), nil
}

// queryable — общий интерфейс для pool и tx.
type queryable interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}
