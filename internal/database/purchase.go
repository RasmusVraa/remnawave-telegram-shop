package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type InvoiceType string

const (
	InvoiceTypeCrypto   InvoiceType = "crypto"
	InvoiceTypeYookasa  InvoiceType = "yookasa"
	InvoiceTypeTelegram InvoiceType = "telegram"
	InvoiceTypeTribute  InvoiceType = "tribute"
	// Platega (разные paymentMethod в одном шлюзе).
	InvoiceTypePlategaSBP       InvoiceType = "plt_sbp"
	InvoiceTypePlategaCards     InvoiceType = "plt_cards"
	InvoiceTypePlategaAcquiring InvoiceType = "plt_acq"
	InvoiceTypePlategaWorldwide InvoiceType = "plt_ww"
	InvoiceTypePlategaCrypto    InvoiceType = "plt_crypto"
	// Heleket (https://heleket.com) — криптоплатежи, в интерфейсе «Другая крипта».
	InvoiceTypeHeleket InvoiceType = "heleket"
)

// PlategaInvoiceTypes перечисляет все invoice_type Platega (поллинг, статистика).
func PlategaInvoiceTypes() []InvoiceType {
	return []InvoiceType{
		InvoiceTypePlategaSBP,
		InvoiceTypePlategaCards,
		InvoiceTypePlategaAcquiring,
		InvoiceTypePlategaWorldwide,
		InvoiceTypePlategaCrypto,
	}
}

// InvoiceTypeIsPlatega — true для любого метода Platega.
func InvoiceTypeIsPlatega(t InvoiceType) bool {
	switch t {
	case InvoiceTypePlategaSBP, InvoiceTypePlategaCards, InvoiceTypePlategaAcquiring,
		InvoiceTypePlategaWorldwide, InvoiceTypePlategaCrypto:
		return true
	default:
		return false
	}
}

// InvoiceTypeIsPlategaCryptoOrCryptopay — оплаты, где в публичных и налоговых формулировках нельзя указывать криптовалюту.
func InvoiceTypeIsPlategaCryptoOrCryptopay(t InvoiceType) bool {
	return t == InvoiceTypeCrypto || t == InvoiceTypePlategaCrypto
}

type PurchaseStatus string

const (
	PurchaseStatusNew     PurchaseStatus = "new"
	PurchaseStatusPending PurchaseStatus = "pending"
	PurchaseStatusPaid    PurchaseStatus = "paid"
	PurchaseStatusCancel  PurchaseStatus = "cancel"
)

// PurchaseKind вид строки покупки (тарифы / доплата и т.д.).
type PurchaseKind string

const (
	PurchaseKindSubscription  PurchaseKind = "subscription"
	PurchaseKindTariffUpgrade PurchaseKind = "tariff_upgrade"
	PurchaseKindExtraHwid     PurchaseKind = "extra_hwid"
)

type Purchase struct {
	ID                     int64          `db:"id"`
	Amount                 float64        `db:"amount"`
	CustomerID             int64          `db:"customer_id"`
	CreatedAt              time.Time      `db:"created_at"`
	Month                  int            `db:"month"`
	PaidAt                 *time.Time     `db:"paid_at"`
	Currency               string         `db:"currency"`
	ExpireAt               *time.Time     `db:"expire_at"`
	Status                 PurchaseStatus `db:"status"`
	InvoiceType            InvoiceType    `db:"invoice_type"`
	CryptoInvoiceID        *int64         `db:"crypto_invoice_id"`
	CryptoInvoiceLink      *string        `db:"crypto_invoice_url"`
	YookasaURL             *string        `db:"yookasa_url"`
	YookasaID              *uuid.UUID     `db:"yookasa_id"`
	PlategaID              *string        `db:"platega_id"`
	PlategaURL             *string        `db:"platega_url"`
	HeleketID              *string        `db:"heleket_id"`
	HeleketURL             *string        `db:"heleket_url"`
	ExtraHwid              int            `db:"extra_hwid"`
	PromoCodeID            *int64         `db:"promo_code_id"`
	DiscountPercentApplied *int           `db:"discount_percent_applied"`
	TariffID               *int64         `db:"tariff_id"`
	PurchaseKind           PurchaseKind   `db:"purchase_kind"`
	IsEarlyDowngrade       bool           `db:"is_early_downgrade"`
	// TariffLocationID / TariffLocationName — выбор площадки при покупке тарифа
	// со списком локаций. Имя хранится снимком: строку локации могут удалить.
	TariffLocationID   *int64  `db:"tariff_location_id"`
	TariffLocationName *string `db:"tariff_location_name"`
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

// purchaseScanArgs returns pointers for scanning a full purchase row (column order must match SELECT * from purchase).
// Порядок колонок в PostgreSQL — порядок CREATE + ALTER ADD (см. миграции 000001, 000005 extra_hwid, 000007 promo, 000008 tariff, 000032 platega).
func purchaseScanArgs(p *Purchase) []interface{} {
	return []interface{}{
		&p.ID, &p.Amount, &p.CustomerID, &p.CreatedAt, &p.Month,
		&p.PaidAt, &p.Currency, &p.ExpireAt, &p.Status, &p.InvoiceType,
		&p.CryptoInvoiceID, &p.CryptoInvoiceLink, &p.YookasaURL, &p.YookasaID,
		&p.ExtraHwid,
		&p.PromoCodeID, &p.DiscountPercentApplied,
		&p.TariffID, &p.PurchaseKind, &p.IsEarlyDowngrade,
		&p.PlategaID, &p.PlategaURL,
		&p.HeleketID, &p.HeleketURL,
		&p.TariffLocationID, &p.TariffLocationName,
	}
}

// SetTariffLocation записывает выбранную клиентом площадку на уже созданный счёт.
func (pr *PurchaseRepository) SetTariffLocation(ctx context.Context, id, locationID int64, name string) error {
	_, err := pr.pool.Exec(ctx, `
		UPDATE purchase
		SET tariff_location_id = $2, tariff_location_name = $3
		WHERE id = $1`, id, locationID, name)
	if err != nil {
		return fmt.Errorf("set tariff location: %w", err)
	}
	return nil
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	if purchase.PurchaseKind == "" {
		purchase.PurchaseKind = PurchaseKindSubscription
	}
	buildInsert := sq.Insert("purchase").
		Columns("amount", "customer_id", "month", "currency", "expire_at", "status", "invoice_type", "crypto_invoice_id", "crypto_invoice_url", "yookasa_url", "yookasa_id", "platega_id", "platega_url", "heleket_id", "heleket_url", "extra_hwid", "promo_code_id", "discount_percent_applied", "tariff_id", "purchase_kind", "is_early_downgrade").
		Values(purchase.Amount, purchase.CustomerID, purchase.Month, purchase.Currency, purchase.ExpireAt, purchase.Status, purchase.InvoiceType, purchase.CryptoInvoiceID, purchase.CryptoInvoiceLink, purchase.YookasaURL, purchase.YookasaID, purchase.PlategaID, purchase.PlategaURL, purchase.HeleketID, purchase.HeleketURL, purchase.ExtraHwid, purchase.PromoCodeID, purchase.DiscountPercentApplied, purchase.TariffID, purchase.PurchaseKind, purchase.IsEarlyDowngrade).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = cr.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (cr *PurchaseRepository) FindByInvoiceTypeAndStatus(ctx context.Context, invoiceType InvoiceType, status PurchaseStatus) (*[]Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": status},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		purchase := Purchase{}
		err = rows.Scan(purchaseScanArgs(&purchase)...)
		if err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

// FindOpenByInvoiceTypeOrdered — незакрытые счета одного типа: самые старые
// первыми и порцией ограниченного размера.
//
// Отдельно от FindByInvoiceTypeAndStatus, потому что тому пользуются поллеры
// остальных касс, которым порядок и лимит не нужны. Здесь они принципиальны:
// проход поллера ограничен по времени, и без ORDER BY хвост выборки мог бы не
// проверяться вовсе — порядок строк без него не определён.
func (cr *PurchaseRepository) FindOpenByInvoiceTypeOrdered(
	ctx context.Context,
	invoiceType InvoiceType,
	statuses []PurchaseStatus,
	limit uint64,
) (*[]Purchase, error) {
	if len(statuses) == 0 {
		empty := []Purchase{}
		return &empty, nil
	}
	raw := make([]string, 0, len(statuses))
	for _, s := range statuses {
		raw = append(raw, string(s))
	}

	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": raw},
		}).
		OrderBy("created_at ASC").
		Limit(limit).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		purchase := Purchase{}
		if err := rows.Scan(purchaseScanArgs(&purchase)...); err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindById(ctx context.Context, id int64) (*Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}
	purchase := &Purchase{}

	err = cr.pool.QueryRow(ctx, sql, args...).Scan(purchaseScanArgs(purchase)...)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query purchase: %w", err)
	}

	return purchase, nil
}

// HasCabinetCheckoutForPurchase — true, если purchase_id встречается в cabinet_checkout (оплата через web-кабинет).
func (cr *PurchaseRepository) HasCabinetCheckoutForPurchase(ctx context.Context, purchaseID int64) (bool, error) {
	var exists bool
	err := cr.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cabinet_checkout WHERE purchase_id = $1)`,
		purchaseID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("cabinet_checkout exists for purchase: %w", err)
	}
	return exists, nil
}

func (p *PurchaseRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("purchase").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %d", id)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64) error {
	currentTime := time.Now()

	updates := map[string]interface{}{
		"status":  PurchaseStatusPaid,
		"paid_at": currentTime,
	}

	return pr.UpdateFields(ctx, purchaseID, updates)
}

func buildLatestActiveTributesQuery(customerIDs []int64) sq.SelectBuilder {
	return sq.
		Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": InvoiceTypeTribute},
			sq.Eq{"customer_id": customerIDs},
			sq.Expr("created_at = (SELECT MAX(created_at) FROM purchase p2 WHERE p2.customer_id = purchase.customer_id AND p2.invoice_type = ?)", InvoiceTypeTribute),
		}).
		Where(sq.NotEq{"status": PurchaseStatusCancel})
}

func (pr *PurchaseRepository) FindLatestActiveTributesByCustomerIDs(
	ctx context.Context,
	customerIDs []int64,
) (*[]Purchase, error) {
	if len(customerIDs) == 0 {
		empty := make([]Purchase, 0)
		return &empty, nil
	}

	builder := buildLatestActiveTributesQuery(customerIDs).PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query purchases: %w", err)
	}
	defer rows.Close()

	var purchases []Purchase
	for rows.Next() {
		var p Purchase
		if err := rows.Scan(purchaseScanArgs(&p)...); err != nil {
			return nil, fmt.Errorf("scan purchase: %w", err)
		}
		purchases = append(purchases, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return &purchases, nil
}

func (pr *PurchaseRepository) FindSuccessfulPaidPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Or{
				sq.Eq{"invoice_type": InvoiceTypeCrypto},
				sq.Eq{"invoice_type": InvoiceTypeYookasa},
				sq.Eq{"invoice_type": InvoiceTypePlategaSBP},
				sq.Eq{"invoice_type": InvoiceTypePlategaCards},
				sq.Eq{"invoice_type": InvoiceTypePlategaAcquiring},
				sq.Eq{"invoice_type": InvoiceTypePlategaWorldwide},
				sq.Eq{"invoice_type": InvoiceTypePlategaCrypto},
				sq.Eq{"invoice_type": InvoiceTypeHeleket},
			},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	purchase := &Purchase{}
	err = pr.pool.QueryRow(ctx, sql, args...).Scan(purchaseScanArgs(purchase)...)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Нет успешных оплат
		}
		return nil, fmt.Errorf("failed to scan purchase: %w", err)
	}

	return purchase, nil
}

func (pr *PurchaseRepository) FindPaidByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
		}).
		OrderBy("paid_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	var purchases []Purchase
	for rows.Next() {
		var purchase Purchase
		if err := rows.Scan(purchaseScanArgs(&purchase)...); err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return purchases, nil
}

// ListAllPaidForLoyaltyBackfill возвращает все успешные оплаты для полного пересчёта loyalty_xp по правилам XPRubEquivalentForPurchase.
func (pr *PurchaseRepository) ListAllPaidForLoyaltyBackfill(ctx context.Context) ([]Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.Eq{"status": PurchaseStatusPaid}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	var purchases []Purchase
	for rows.Next() {
		var purchase Purchase
		if err := rows.Scan(purchaseScanArgs(&purchase)...); err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return purchases, nil
}

// SumPaidSpendBreakdown возвращает по успешным оплатам клиента: счёт и сумму в рублях (RUB/RUR/пустая валюта,
// без строк Telegram Stars) и отдельно счёт и сумму amount по Stars (invoice_type telegram или валюта STARS/XTR).
func (pr *PurchaseRepository) SumPaidSpendBreakdown(ctx context.Context, customerID int64) (
	rubCount int64, rubSum float64, starsCount int64, starsSum float64, err error,
) {
	q := `
SELECT
  COUNT(*) FILTER (WHERE p.status = 'paid' AND NOT (
    p.invoice_type = 'telegram'
    OR UPPER(TRIM(COALESCE(p.currency, ''))) IN ('STARS', 'XTR')
  ) AND (
    UPPER(TRIM(COALESCE(p.currency, ''))) IN ('RUB', 'RUR', '')
    OR COALESCE(p.currency, '') = ''
  )),
  COALESCE(SUM(p.amount) FILTER (WHERE p.status = 'paid' AND NOT (
    p.invoice_type = 'telegram'
    OR UPPER(TRIM(COALESCE(p.currency, ''))) IN ('STARS', 'XTR')
  ) AND (
    UPPER(TRIM(COALESCE(p.currency, ''))) IN ('RUB', 'RUR', '')
    OR COALESCE(p.currency, '') = ''
  )), 0),
  COUNT(*) FILTER (WHERE p.status = 'paid' AND (
    p.invoice_type = 'telegram'
    OR UPPER(TRIM(COALESCE(p.currency, ''))) IN ('STARS', 'XTR')
  )),
  COALESCE(SUM(p.amount) FILTER (WHERE p.status = 'paid' AND (
    p.invoice_type = 'telegram'
    OR UPPER(TRIM(COALESCE(p.currency, ''))) IN ('STARS', 'XTR')
  )), 0)
FROM purchase p
WHERE p.customer_id = $1
`
	err = pr.pool.QueryRow(ctx, q, customerID).Scan(&rubCount, &rubSum, &starsCount, &starsSum)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("sum paid spend breakdown for customer: %w", err)
	}
	return rubCount, rubSum, starsCount, starsSum, nil
}

func (pr *PurchaseRepository) CountPaidByCustomer(ctx context.Context, customerID int64) (int, error) {
	buildSelect := sq.Select("COUNT(*)").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	var count int
	if err := pr.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to query count: %w", err)
	}
	return count, nil
}

func (pr *PurchaseRepository) CountPaidSubscriptionsByCustomer(ctx context.Context, customerID int64) (int, error) {
	buildSelect := sq.Select("COUNT(*)").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Gt{"month": 0},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	var count int
	err = pr.pool.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count paid purchases: %w", err)
	}

	return count, nil
}

func (pr *PurchaseRepository) HasPaidSubscription(ctx context.Context, customerID int64) (bool, error) {
	buildSelect := sq.Select("1").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Gt{"month": 0},
		}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build query: %w", err)
	}

	var dummy int
	err = pr.pool.QueryRow(ctx, sql, args...).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check paid subscription: %w", err)
	}
	return true, nil
}

// --- Admin listing (раздел «Платежи» в веб-админке) -------------------------

// AdminPurchaseFilter — фильтр списка платежей в админке.
type AdminPurchaseFilter struct {
	Status PurchaseStatus // "" = любой статус
	Search string         // ID платежа / ID клиента / telegram_id / @username (подстрока)
}

// AdminPurchaseRow — Purchase + краткие данные клиента одним JOIN-запросом
// (без N+1) для списка и карточки платежа в админке.
type AdminPurchaseRow struct {
	Purchase
	CustomerTelegramID       int64
	CustomerTelegramUsername *string
	CustomerIsWebOnly        bool
}

// adminPurchaseSelectCols — порядок должен совпадать с purchaseScanArgs + 3 доп. поля клиента.
const adminPurchaseSelectCols = `p.id, p.amount, p.customer_id, p.created_at, p.month,
	p.paid_at, p.currency, p.expire_at, p.status, p.invoice_type,
	p.crypto_invoice_id, p.crypto_invoice_url, p.yookasa_url, p.yookasa_id,
	p.extra_hwid,
	p.promo_code_id, p.discount_percent_applied,
	p.tariff_id, p.purchase_kind, p.is_early_downgrade,
	p.platega_id, p.platega_url,
	p.heleket_id, p.heleket_url,
	p.tariff_location_id, p.tariff_location_name,
	c.telegram_id, c.telegram_username, c.is_web_only`

func scanAdminPurchaseRow(row pgx.Row) (*AdminPurchaseRow, error) {
	var r AdminPurchaseRow
	args := append(purchaseScanArgs(&r.Purchase), &r.CustomerTelegramID, &r.CustomerTelegramUsername, &r.CustomerIsWebOnly)
	if err := row.Scan(args...); err != nil {
		return nil, err
	}
	return &r, nil
}

func adminPurchaseWhereClause(filter AdminPurchaseFilter) sq.Sqlizer {
	var and sq.And
	if filter.Status != "" {
		and = append(and, sq.Eq{"p.status": filter.Status})
	}
	if needle := strings.TrimSpace(strings.TrimPrefix(filter.Search, "@")); needle != "" {
		pattern := "%" + escapeSQLLikePattern(needle) + "%"
		and = append(and, sq.Or{
			sq.Expr("c.telegram_username ILIKE ?", pattern),
			sq.Expr("CAST(p.id AS TEXT) LIKE ?", pattern),
			sq.Expr("CAST(p.customer_id AS TEXT) LIKE ?", pattern),
			sq.Expr("CAST(c.telegram_id AS TEXT) LIKE ?", pattern),
		})
	}
	if len(and) == 0 {
		return sq.Expr("TRUE")
	}
	return and
}

// ListForAdmin — страница платежей для админки, отсортирована по дате создания (новые сверху).
// limit также используется для CSV-экспорта (caller передаёт большой limit и offset=0).
func (pr *PurchaseRepository) ListForAdmin(ctx context.Context, filter AdminPurchaseFilter, limit, offset int) ([]AdminPurchaseRow, error) {
	sqlStr, args, err := sq.Select(adminPurchaseSelectCols).
		From("purchase p").
		Join("customer c ON c.id = p.customer_id").
		Where(adminPurchaseWhereClause(filter)).
		OrderBy("p.created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build admin purchases query: %w", err)
	}

	rows, err := pr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin purchases: %w", err)
	}
	defer rows.Close()

	out := make([]AdminPurchaseRow, 0, limit)
	for rows.Next() {
		row, err := scanAdminPurchaseRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admin purchase: %w", err)
		}
		out = append(out, *row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin purchases: %w", err)
	}
	return out, nil
}

// CountForAdmin — общее число платежей под тем же фильтром, что ListForAdmin (для пагинации).
func (pr *PurchaseRepository) CountForAdmin(ctx context.Context, filter AdminPurchaseFilter) (int64, error) {
	sqlStr, args, err := sq.Select("COUNT(*)").
		From("purchase p").
		Join("customer c ON c.id = p.customer_id").
		Where(adminPurchaseWhereClause(filter)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build admin purchases count: %w", err)
	}
	var n int64
	if err := pr.pool.QueryRow(ctx, sqlStr, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count admin purchases: %w", err)
	}
	return n, nil
}

// GetForAdmin — один платёж с данными клиента, для модалки «Платёж #ID» в админке.
func (pr *PurchaseRepository) GetForAdmin(ctx context.Context, id int64) (*AdminPurchaseRow, error) {
	sqlStr, args, err := sq.Select(adminPurchaseSelectCols).
		From("purchase p").
		Join("customer c ON c.id = p.customer_id").
		Where(sq.Eq{"p.id": id}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build admin purchase query: %w", err)
	}
	row, err := scanAdminPurchaseRow(pr.pool.QueryRow(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query admin purchase: %w", err)
	}
	return row, nil
}

func (pr *PurchaseRepository) FindByCustomerIDAndInvoiceTypeLast(
	ctx context.Context,
	customerID int64,
	invoiceType InvoiceType,
) (*Purchase, error) {

	query := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"invoice_type": invoiceType},
		}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	p := &Purchase{}
	err = pr.pool.QueryRow(ctx, sql, args...).Scan(purchaseScanArgs(p)...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query purchase: %w", err)
	}

	return p, nil
}
