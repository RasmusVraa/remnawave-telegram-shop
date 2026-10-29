package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"remnawave-tg-shop-bot/internal/pgerr"
)

// LinkStatus — enum cabinet_account_customer_link.link_status.
const (
	LinkStatusLinked       = "linked"
	LinkStatusPendingMerge = "pending_merge"
	LinkStatusRejected     = "rejected"
)

// AccountCustomerLink — связь аккаунта кабинета с customer.
type AccountCustomerLink struct {
	ID         int64
	AccountID  int64
	CustomerID int64
	LinkStatus string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AccountCustomerLinkRepo — cabinet_account_customer_link.
type AccountCustomerLinkRepo struct {
	pool *pgxpool.Pool
}

// NewAccountCustomerLinkRepo — конструктор.
func NewAccountCustomerLinkRepo(pool *pgxpool.Pool) *AccountCustomerLinkRepo {
	return &AccountCustomerLinkRepo{pool: pool}
}

const linkSelectCols = "id, account_id, customer_id, link_status, created_at, updated_at"

func scanLink(row pgx.Row) (*AccountCustomerLink, error) {
	var l AccountCustomerLink
	err := row.Scan(
		&l.ID,
		&l.AccountID,
		&l.CustomerID,
		&l.LinkStatus,
		&l.CreatedAt,
		&l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan link: %w", err)
	}
	return &l, nil
}

// Create вставляет новую связь. UNIQUE (account_id) гарантирует, что у одного
// аккаунта не может быть двух customer'ов. При конфликте — ErrConflict.
//
// Вставка идёт через SELECT ... WHERE EXISTS, а не VALUES: если аккаунт уже
// удалён (живой access-JWT удалённого аккаунта), строк не вставится и мы вернём
// ErrAccountMissing вместо того, чтобы уронить FK и засорить лог PostgreSQL
// ошибками cabinet_account_customer_link_account_id_fkey.
func (r *AccountCustomerLinkRepo) Create(ctx context.Context, accountID, customerID int64, status string) (*AccountCustomerLink, error) {
	if status == "" {
		status = LinkStatusLinked
	}
	const q = `
		INSERT INTO cabinet_account_customer_link (account_id, customer_id, link_status)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM cabinet_account WHERE id = $1)
		RETURNING ` + linkSelectCols
	link, err := scanLink(r.pool.QueryRow(ctx, q, accountID, customerID, status))
	if err != nil {
		// Нет вставленных строк = EXISTS не прошёл, аккаунта нет.
		if errors.Is(err, ErrNotFound) {
			return nil, ErrAccountMissing
		}
		// Гонка: аккаунт удалён между снимком EXISTS и коммитом вставки.
		if isAccountFKViolation(err) {
			return nil, ErrAccountMissing
		}
		return nil, err
	}
	return link, nil
}

// isAccountFKViolation — нарушение FK именно по account_id (23503).
func isAccountFKViolation(err error) bool {
	return pgerr.IsForeignKeyViolation(err, "cabinet_account_customer_link_account_id_fkey")
}

// FindByAccountID — основной lookup. ErrNotFound, если связи ещё нет (аккаунт
// только что создан и bootstrap не отработал).
func (r *AccountCustomerLinkRepo) FindByAccountID(ctx context.Context, accountID int64) (*AccountCustomerLink, error) {
	const q = `SELECT ` + linkSelectCols + ` FROM cabinet_account_customer_link WHERE account_id = $1`
	return scanLink(r.pool.QueryRow(ctx, q, accountID))
}

// FindByCustomerID — обратный lookup. Полезен, когда в бот приходит апдейт и
// хочется понять, есть ли у этого customer web-кабинет.
func (r *AccountCustomerLinkRepo) FindByCustomerID(ctx context.Context, customerID int64) (*AccountCustomerLink, error) {
	const q = `SELECT ` + linkSelectCols + ` FROM cabinet_account_customer_link WHERE customer_id = $1`
	return scanLink(r.pool.QueryRow(ctx, q, customerID))
}

// UpdateCustomerID меняет customer_id в link'е на newCustomerID и выставляет
// статус 'linked'. Используется в merge-транзакции для перевода аккаунта
// с web-only customer на Telegram customer.
func (r *AccountCustomerLinkRepo) UpdateCustomerID(ctx context.Context, accountID, newCustomerID int64) error {
	const q = `UPDATE cabinet_account_customer_link
		SET customer_id = $2, link_status = 'linked', updated_at = NOW()
		WHERE account_id = $1`
	_, err := r.pool.Exec(ctx, q, accountID, newCustomerID)
	if err != nil {
		return fmt.Errorf("update link customer_id: %w", err)
	}
	return nil
}

// UpdateStatus переводит линк между статусами ('linked' ↔ 'pending_merge' ↔ 'rejected').
func (r *AccountCustomerLinkRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	const q = `UPDATE cabinet_account_customer_link SET link_status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("update link status: %w", err)
	}
	return nil
}
