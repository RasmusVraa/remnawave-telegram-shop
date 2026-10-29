package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider — enum cabinet_identity.provider.
const (
	ProviderEmail    = "email"
	ProviderGoogle   = "google"
	ProviderYandex   = "yandex"
	ProviderVK       = "vk"
	ProviderTelegram = "telegram"
)

// Identity — модель cabinet_identity.
type Identity struct {
	ID             int64
	AccountID      int64
	Provider       string
	ProviderUserID string
	ProviderEmail  *string
	RawProfileJSON []byte // jsonb как raw-байты; парсят вызывающие
	CreatedAt      time.Time
	// UnlinkedAt — мягкая отвязка: вход по провайдеру возможен, в настройках не показываем.
	UnlinkedAt *time.Time
}

// IdentityRepo — репозиторий cabinet_identity.
type IdentityRepo struct {
	pool *pgxpool.Pool
}

// NewIdentityRepo — конструктор.
func NewIdentityRepo(pool *pgxpool.Pool) *IdentityRepo { return &IdentityRepo{pool: pool} }

const identitySelectCols = "id, account_id, provider, provider_user_id, provider_email, raw_profile_json, created_at, unlinked_at"

func scanIdentity(row pgx.Row) (*Identity, error) {
	var i Identity
	err := row.Scan(
		&i.ID,
		&i.AccountID,
		&i.Provider,
		&i.ProviderUserID,
		&i.ProviderEmail,
		&i.RawProfileJSON,
		&i.CreatedAt,
		&i.UnlinkedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan identity: %w", err)
	}
	return &i, nil
}

// Create привязывает провайдера к аккаунту. ProviderUserID для провайдера
// email — это accountID в строковом виде (самый простой способ обеспечить
// уникальность идентичности «email-логин этого аккаунта»).
func (r *IdentityRepo) Create(ctx context.Context, accountID int64, provider, providerUserID, providerEmail string, rawProfile any) (*Identity, error) {
	var rawJSON []byte
	if rawProfile != nil {
		b, err := json.Marshal(rawProfile)
		if err != nil {
			return nil, fmt.Errorf("marshal raw profile: %w", err)
		}
		rawJSON = b
	}
	var emailArg any
	if providerEmail != "" {
		emailArg = providerEmail
	}
	const q = `
		INSERT INTO cabinet_identity (account_id, provider, provider_user_id, provider_email, raw_profile_json)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + identitySelectCols
	return scanIdentity(r.pool.QueryRow(ctx, q, accountID, provider, providerUserID, emailArg, rawJSON))
}

// FindByProvider ищет identity по (provider, provider_user_id) — уникальный ключ.
func (r *IdentityRepo) FindByProvider(ctx context.Context, provider, providerUserID string) (*Identity, error) {
	const q = `SELECT ` + identitySelectCols + ` FROM cabinet_identity WHERE provider = $1 AND provider_user_id = $2`
	return scanIdentity(r.pool.QueryRow(ctx, q, provider, providerUserID))
}

// UpdateTelegramIdentityAccountID переносит строку cabinet_identity(provider=telegram)
// на другой account_id (один tg user id — один identity). Используется, когда
// «осиротевший» tg-only аккаунт нужно слить с аккаунтом, к которому уже привязан customer.
func (r *IdentityRepo) UpdateTelegramIdentityAccountID(ctx context.Context, providerUserID string, newAccountID int64) error {
	const q = `UPDATE cabinet_identity SET account_id = $1 WHERE provider = $2 AND provider_user_id = $3`
	tag, err := r.pool.Exec(ctx, q, newAccountID, ProviderTelegram, providerUserID)
	if err != nil {
		return fmt.Errorf("update telegram identity account_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteByAccountAndProvider удаляет все строки identity для пары account+provider.
// Для dev-инструментов (снятие привязки Telegram для повторных тестов).
func (r *IdentityRepo) DeleteByAccountAndProvider(ctx context.Context, accountID int64, provider string) (int64, error) {
	const q = `DELETE FROM cabinet_identity WHERE account_id = $1 AND provider = $2`
	tag, err := r.pool.Exec(ctx, q, accountID, provider)
	if err != nil {
		return 0, fmt.Errorf("delete identity: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SoftUnlinkByAccountAndProvider помечает все identity account+provider как мягко отвязанные.
func (r *IdentityRepo) SoftUnlinkByAccountAndProvider(ctx context.Context, accountID int64, provider string) (int64, error) {
	const q = `
		UPDATE cabinet_identity
		   SET unlinked_at = NOW()
		 WHERE account_id = $1 AND provider = $2 AND unlinked_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, accountID, provider)
	if err != nil {
		return 0, fmt.Errorf("soft unlink identity: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ClearUnlinkedAtForSubject снимает мягкую отвязку у конкретной строки identity
// (один provider_user_id — один OAuth-субъект; при нескольких Google на аккаунте
// не трогаем другие субъекты).
func (r *IdentityRepo) ClearUnlinkedAtForSubject(ctx context.Context, accountID int64, provider, providerUserID string) error {
	const q = `
		UPDATE cabinet_identity
		   SET unlinked_at = NULL
		 WHERE account_id = $1 AND provider = $2 AND provider_user_id = $3 AND unlinked_at IS NOT NULL`
	_, err := r.pool.Exec(ctx, q, accountID, provider, providerUserID)
	if err != nil {
		return fmt.Errorf("clear identity unlinked_at: %w", err)
	}
	return nil
}

// ListLinkedByAccount — только «видимые» в настройках привязки (без мягко отвязанных).
func (r *IdentityRepo) ListLinkedByAccount(ctx context.Context, accountID int64) ([]Identity, error) {
	const q = `SELECT ` + identitySelectCols + ` FROM cabinet_identity WHERE account_id = $1 AND unlinked_at IS NULL ORDER BY id`
	rows, err := r.pool.Query(ctx, q, accountID)
	if err != nil {
		return nil, fmt.Errorf("list linked identities: %w", err)
	}
	defer rows.Close()

	var out []Identity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list linked identities: %w", err)
	}
	return out, nil
}

// ListByAccount возвращает все identity аккаунта.
func (r *IdentityRepo) ListByAccount(ctx context.Context, accountID int64) ([]Identity, error) {
	const q = `SELECT ` + identitySelectCols + ` FROM cabinet_identity WHERE account_id = $1 ORDER BY id`
	rows, err := r.pool.Query(ctx, q, accountID)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	defer rows.Close()

	var out []Identity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	return out, nil
}
