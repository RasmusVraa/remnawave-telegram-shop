package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tariff строка тарифа (SALES_MODE=tariffs).
type Tariff struct {
	ID                         int64      `db:"id"`
	Slug                       string     `db:"slug"`
	Name                       *string    `db:"name"`
	SortOrder                  int        `db:"sort_order"`
	IsActive                   bool       `db:"is_active"`
	DeviceLimit                int        `db:"device_limit"`
	TrafficLimitBytes          int64      `db:"traffic_limit_bytes"`
	TrafficLimitResetStrategy  string     `db:"traffic_limit_reset_strategy"`
	ActiveInternalSquadUUIDs   string     `db:"active_internal_squad_uuids"`
	ExternalSquadUUID          *uuid.UUID `db:"external_squad_uuid"`
	RemnawaveTag               *string    `db:"remnawave_tag"`
	TierLevel                  *int       `db:"tier_level"`
	Description                *string    `db:"description"`
	DescriptionDetail          *string    `db:"description_detail"`
}

// TariffPrice цена тарифа за период (месяцы).
type TariffPrice struct {
	TariffID    int64 `db:"tariff_id"`
	Months      int   `db:"months"`
	AmountRub   int   `db:"amount_rub"`
	AmountStars *int  `db:"amount_stars"`
}

type TariffRepository struct {
	pool *pgxpool.Pool
}

func NewTariffRepository(pool *pgxpool.Pool) *TariffRepository {
	return &TariffRepository{pool: pool}
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanTariff(row rowScanner) (*Tariff, error) {
	var t Tariff
	err := row.Scan(
		&t.ID, &t.Slug, &t.Name, &t.SortOrder, &t.IsActive,
		&t.DeviceLimit, &t.TrafficLimitBytes, &t.TrafficLimitResetStrategy,
		&t.ActiveInternalSquadUUIDs, &t.ExternalSquadUUID, &t.RemnawaveTag, &t.TierLevel,
		&t.Description, &t.DescriptionDetail,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListActive возвращает активные тарифы по sort_order.
func (r *TariffRepository) ListActive(ctx context.Context) ([]Tariff, error) {
	q := sq.Select(
		"id", "slug", "name", "sort_order", "is_active",
		"device_limit", "traffic_limit_bytes", "traffic_limit_reset_strategy",
		"active_internal_squad_uuids", "external_squad_uuid", "remnawave_tag", "tier_level",
		"description", "description_detail",
	).From("tariff").Where(sq.Eq{"is_active": true}).OrderBy("sort_order ASC", "id ASC").PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list tariffs: %w", err)
	}
	defer rows.Close()
	var out []Tariff
	for rows.Next() {
		t, err := scanTariff(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tariff: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetByID загружает тариф по id.
func (r *TariffRepository) GetByID(ctx context.Context, id int64) (*Tariff, error) {
	q := sq.Select(
		"id", "slug", "name", "sort_order", "is_active",
		"device_limit", "traffic_limit_bytes", "traffic_limit_reset_strategy",
		"active_internal_squad_uuids", "external_squad_uuid", "remnawave_tag", "tier_level",
		"description", "description_detail",
	).From("tariff").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	t, err := scanTariff(r.pool.QueryRow(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// ListPricesForTariff возвращает все ценовые строки тарифа.
func (r *TariffRepository) ListPricesForTariff(ctx context.Context, tariffID int64) ([]TariffPrice, error) {
	q := sq.Select("tariff_id", "months", "amount_rub", "amount_stars").
		From("tariff_price").
		Where(sq.Eq{"tariff_id": tariffID}).
		OrderBy("months ASC").
		PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list tariff prices: %w", err)
	}
	defer rows.Close()
	var out []TariffPrice
	for rows.Next() {
		var p TariffPrice
		if err := rows.Scan(&p.TariffID, &p.Months, &p.AmountRub, &p.AmountStars); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPrice возвращает цену для (tariff_id, months) или nil.
func (r *TariffRepository) GetPrice(ctx context.Context, tariffID int64, months int) (*TariffPrice, error) {
	q := sq.Select("tariff_id", "months", "amount_rub", "amount_stars").
		From("tariff_price").
		Where(sq.Eq{"tariff_id": tariffID, "months": months}).
		PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	var p TariffPrice
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&p.TariffID, &p.Months, &p.AmountRub, &p.AmountStars)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ParseSquadUUIDList парсит колонку active_internal_squad_uuids (UUID через запятую, как SQUAD_UUIDS).
func ParseSquadUUIDList(s string) ([]uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]uuid.UUID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		u, err := uuid.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("invalid squad uuid %q: %w", p, err)
		}
		out = append(out, u)
	}
	return out, nil
}

// ListAll возвращает все тарифы (включая неактивные) для админки.
func (r *TariffRepository) ListAll(ctx context.Context) ([]Tariff, error) {
	q := sq.Select(
		"id", "slug", "name", "sort_order", "is_active",
		"device_limit", "traffic_limit_bytes", "traffic_limit_reset_strategy",
		"active_internal_squad_uuids", "external_squad_uuid", "remnawave_tag", "tier_level",
		"description", "description_detail",
	).From("tariff").OrderBy("sort_order ASC", "id ASC").PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list all tariffs: %w", err)
	}
	defer rows.Close()
	var out []Tariff
	for rows.Next() {
		t, err := scanTariff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// MinAmountRubForTariff минимальная положительная цена в ₽ среди периодов тарифа.
func (r *TariffRepository) MinAmountRubForTariff(ctx context.Context, tariffID int64) (int, bool, error) {
	var n sql.NullInt64
	err := r.pool.QueryRow(ctx,
		`SELECT MIN(amount_rub) FROM tariff_price WHERE tariff_id = $1 AND amount_rub > 0`,
		tariffID,
	).Scan(&n)
	if err != nil {
		return 0, false, err
	}
	if !n.Valid {
		return 0, false, nil
	}
	return int(n.Int64), true, nil
}

// MinAmountStarsForTariff минимальное положительное значение Stars среди периодов тарифа.
func (r *TariffRepository) MinAmountStarsForTariff(ctx context.Context, tariffID int64) (int, bool, error) {
	var n sql.NullInt64
	err := r.pool.QueryRow(ctx,
		`SELECT MIN(amount_stars) FROM tariff_price WHERE tariff_id = $1 AND amount_stars IS NOT NULL AND amount_stars > 0`,
		tariffID,
	).Scan(&n)
	if err != nil {
		return 0, false, err
	}
	if !n.Valid {
		return 0, false, nil
	}
	return int(n.Int64), true, nil
}

// CountAll число строк в tariff.
func (r *TariffRepository) CountAll(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tariff`).Scan(&n)
	return n, err
}

// CountActive число активных тарифов.
func (r *TariffRepository) CountActive(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tariff WHERE is_active = true`).Scan(&n)
	return n, err
}

// CountPaidPurchasesWithTariff число оплаченных покупок с любым tariff_id.
func (r *TariffRepository) CountPaidPurchasesWithTariff(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM purchase WHERE tariff_id IS NOT NULL AND status = $1`, PurchaseStatusPaid).Scan(&n)
	return n, err
}

// SlugExists проверяет занятость slug.
func (r *TariffRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tariff WHERE slug = $1`, slug).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateWithPrices создаёт тариф и четыре ценовые строки (месяцы 1,3,6,12).
func (r *TariffRepository) CreateWithPrices(ctx context.Context, t *Tariff, rub [4]int, stars [4]*int) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	q := `INSERT INTO tariff (slug, name, sort_order, is_active, device_limit, traffic_limit_bytes,
		traffic_limit_reset_strategy, active_internal_squad_uuids, external_squad_uuid, remnawave_tag, tier_level, description, description_detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`
	var id int64
	err = tx.QueryRow(ctx, q,
		t.Slug, t.Name, t.SortOrder, t.IsActive, t.DeviceLimit, t.TrafficLimitBytes,
		t.TrafficLimitResetStrategy, t.ActiveInternalSquadUUIDs, t.ExternalSquadUUID, t.RemnawaveTag, t.TierLevel,
		t.Description, t.DescriptionDetail,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert tariff: %w", err)
	}
	months := []int{1, 3, 6, 12}
	for i, m := range months {
		_, err = tx.Exec(ctx,
			`INSERT INTO tariff_price (tariff_id, months, amount_rub, amount_stars) VALUES ($1,$2,$3,$4)`,
			id, m, rub[i], stars[i],
		)
		if err != nil {
			return 0, fmt.Errorf("insert tariff_price: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateTariff обновляет поля тарифа.
func (r *TariffRepository) UpdateTariff(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	b := sq.Update("tariff").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar)
	for k, v := range updates {
		b = b.Set(k, v)
	}
	sqlStr, args, err := b.ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

// ReplaceAllPrices заменяет все четыре цены.
func (r *TariffRepository) ReplaceAllPrices(ctx context.Context, tariffID int64, rub [4]int, stars [4]*int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tariff_price WHERE tariff_id = $1`, tariffID); err != nil {
		return err
	}
	months := []int{1, 3, 6, 12}
	for i, m := range months {
		if _, err := tx.Exec(ctx,
			`INSERT INTO tariff_price (tariff_id, months, amount_rub, amount_stars) VALUES ($1,$2,$3,$4)`,
			tariffID, m, rub[i], stars[i],
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DeleteTariff удаляет цены и тариф.
func (r *TariffRepository) DeleteTariff(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tariff_price WHERE tariff_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tariff WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CountPurchasesForTariff число оплаченных покупок с данным tariff_id.
func (r *TariffRepository) CountPurchasesForTariff(ctx context.Context, tariffID int64) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM purchase WHERE tariff_id = $1 AND status = $2`,
		tariffID, PurchaseStatusPaid,
	).Scan(&n)
	return n, err
}
