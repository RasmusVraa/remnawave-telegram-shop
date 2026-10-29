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

// CheckoutProvider — enum cabinet_checkout.provider (CHECK в миграции 000017).
const (
	CheckoutProviderYookassa         = "yookassa"
	CheckoutProviderCryptoPay        = "cryptopay"
	CheckoutProviderTelegram         = "telegram"
	CheckoutProviderPlategaSBP       = "platega_sbp"
	CheckoutProviderPlategaCards     = "platega_cards"
	CheckoutProviderPlategaAcquiring = "platega_acquiring"
	CheckoutProviderPlategaWorldwide = "platega_worldwide"
	CheckoutProviderPlategaCrypto    = "platega_crypto"
	CheckoutProviderHeleket          = "heleket"
)

// CheckoutStatus — enum cabinet_checkout.status.
const (
	CheckoutStatusNew     = "new"
	CheckoutStatusPending = "pending"
	CheckoutStatusPaid    = "paid"
	CheckoutStatusFailed  = "failed"
	CheckoutStatusExpired = "expired"
)

// ErrCheckoutConflict — idempotency_key уже используется другим платежом
// или purchase_id уже привязан к существующему checkout'у.
var ErrCheckoutConflict = errors.New("repository: checkout conflict")

// Checkout — строка cabinet_checkout.
//
// Привязка purchase_id появляется не сразу: сначала создаётся row со статусом
// 'new' и idempotency_key (UNIQUE). После того как у провайдера получена
// payment_url, row обновляется до 'pending' с проставленным purchase_id.
type Checkout struct {
	ID             int64
	AccountID      int64
	IdempotencyKey string
	PurchaseID     *int64
	Provider       string
	ReturnURL      string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CheckoutRepo — cabinet_checkout.
type CheckoutRepo struct {
	pool *pgxpool.Pool
}

// NewCheckoutRepo — конструктор.
func NewCheckoutRepo(pool *pgxpool.Pool) *CheckoutRepo {
	return &CheckoutRepo{pool: pool}
}

const checkoutSelectCols = "id, account_id, idempotency_key, purchase_id, provider, COALESCE(return_url, ''), status, created_at, updated_at"

func scanCheckout(row pgx.Row) (*Checkout, error) {
	var c Checkout
	err := row.Scan(
		&c.ID,
		&c.AccountID,
		&c.IdempotencyKey,
		&c.PurchaseID,
		&c.Provider,
		&c.ReturnURL,
		&c.Status,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan checkout: %w", err)
	}
	return &c, nil
}

// Create вставляет новый checkout со status='new'. При конфликте по
// idempotency_key возвращает ErrCheckoutConflict — caller должен перечитать
// существующую строку через FindByIdempotencyKey и вернуть её клиенту.
func (r *CheckoutRepo) Create(ctx context.Context, accountID int64, idempotencyKey, provider string) (*Checkout, error) {
	const q = `
		INSERT INTO cabinet_checkout (account_id, idempotency_key, provider, status)
		VALUES ($1, $2, $3, 'new')
		RETURNING ` + checkoutSelectCols
	c, err := scanCheckout(r.pool.QueryRow(ctx, q, accountID, idempotencyKey, provider))
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return nil, ErrCheckoutConflict
		}
		return nil, fmt.Errorf("create checkout: %w", err)
	}
	return c, nil
}

// FindByIdempotencyKey ищет уже существующий checkout по (account_id, idempotency_key).
// UNIQUE индекс в БД стоит только на idempotency_key, но account_id в условии нужен,
// чтобы клиент A не мог прочитать checkout клиента B повторным POST с тем же ключом.
func (r *CheckoutRepo) FindByIdempotencyKey(ctx context.Context, accountID int64, idempotencyKey string) (*Checkout, error) {
	const q = `SELECT ` + checkoutSelectCols + ` FROM cabinet_checkout WHERE account_id = $1 AND idempotency_key = $2`
	return scanCheckout(r.pool.QueryRow(ctx, q, accountID, idempotencyKey))
}

// FindByID — для GET /cabinet/api/payments/:id/status. Владение проверяется в сервисе
// через сравнение AccountID.
func (r *CheckoutRepo) FindByID(ctx context.Context, id int64) (*Checkout, error) {
	const q = `SELECT ` + checkoutSelectCols + ` FROM cabinet_checkout WHERE id = $1`
	return scanCheckout(r.pool.QueryRow(ctx, q, id))
}

// FindByPurchaseID ищет checkout, привязанный к данному purchase_id — используется
// в админ-модалке платежа, чтобы показать провайдера/ключ идемпотентности оплаты,
// созданной через web-кабинет. Возвращает ErrNotFound, если платёж создан
// не через кабинет (например, из Telegram-бота) — это штатный случай, а не ошибка.
func (r *CheckoutRepo) FindByPurchaseID(ctx context.Context, purchaseID int64) (*Checkout, error) {
	const q = `SELECT ` + checkoutSelectCols + ` FROM cabinet_checkout WHERE purchase_id = $1 ORDER BY created_at DESC LIMIT 1`
	return scanCheckout(r.pool.QueryRow(ctx, q, purchaseID))
}

// AttachPurchase привязывает созданный в PaymentService purchase_id к checkout'у
// и переводит статус 'new' → 'pending'. Также сохраняет return_url, чтобы
// последующие поллы /status могли отдать пользователю корректную ссылку.
func (r *CheckoutRepo) AttachPurchase(ctx context.Context, id, purchaseID int64, returnURL string) error {
	const q = `
		UPDATE cabinet_checkout
		SET purchase_id = $2,
		    return_url  = $3,
		    status      = 'pending',
		    updated_at  = NOW()
		WHERE id = $1
		  AND status = 'new'`
	tag, err := r.pool.Exec(ctx, q, id, purchaseID, returnURL)
	if err != nil {
		return fmt.Errorf("attach purchase: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("attach purchase: checkout %d not in 'new' status", id)
	}
	return nil
}

// UpdateStatus — ленивая синхронизация статуса cabinet_checkout со статусом purchase.
// Полл UI вызывает GET /payments/:id/status, хендлер читает purchase и, если статус
// изменился, обновляет checkout через этот метод. Идемпотентно.
func (r *CheckoutRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	const q = `
		UPDATE cabinet_checkout
		SET status     = $2,
		    updated_at = NOW()
		WHERE id = $1
		  AND status <> $2`
	_, err := r.pool.Exec(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("update checkout status: %w", err)
	}
	return nil
}
