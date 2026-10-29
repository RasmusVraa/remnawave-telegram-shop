package database

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoyaltyTier строка уровня лояльности (порог XP и скидка %).
type LoyaltyTier struct {
	ID               int64   `db:"id"`
	SortOrder        int     `db:"sort_order"`
	XpMin            int64   `db:"xp_min"`
	DiscountPercent  int     `db:"discount_percent"`
	DisplayName      *string `db:"display_name"`
}

type LoyaltyTierRepository struct {
	pool *pgxpool.Pool
}

func NewLoyaltyTierRepository(pool *pgxpool.Pool) *LoyaltyTierRepository {
	return &LoyaltyTierRepository{pool: pool}
}

const loyaltyTierSelectColumns = "id, sort_order, xp_min, discount_percent, display_name"

// ListAllOrderedByXpMinAsc возвращает уровни по возрастанию xp_min.
func (r *LoyaltyTierRepository) ListAllOrderedByXpMinAsc(ctx context.Context) ([]LoyaltyTier, error) {
	q, args, err := sq.Select(loyaltyTierSelectColumns).
		From("loyalty_tier").
		OrderBy("xp_min ASC", "sort_order ASC").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LoyaltyTier
	for rows.Next() {
		var t LoyaltyTier
		if err := rows.Scan(&t.ID, &t.SortOrder, &t.XpMin, &t.DiscountPercent, &t.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DiscountPercentForXP возвращает процент скидки для текущего накопленного XP (до оплаты): последний уровень по возрастанию xp_min, для которого XP достаточно.
func (r *LoyaltyTierRepository) DiscountPercentForXP(ctx context.Context, loyaltyXP int64) (int, error) {
	tiers, err := r.ListAllOrderedByXpMinAsc(ctx)
	if err != nil {
		return 0, err
	}
	var discount int
	for _, t := range tiers {
		if loyaltyXP >= t.XpMin {
			discount = t.DiscountPercent
		}
	}
	return discount, nil
}

// LoyaltyProgress описывает текущий и следующий порог для UI «Мой VPN» / экран лояльности.
type LoyaltyProgress struct {
	CurrentTier LoyaltyTier
	NextTier    *LoyaltyTier // следующий порог xp_min выше текущего XP; nil если выше некуда
}

// ProgressForXP возвращает последний достигнутый по xp_min уровень и следующий порог (первый tier с xp_min > loyaltyXP).
func (r *LoyaltyTierRepository) ProgressForXP(ctx context.Context, loyaltyXP int64) (LoyaltyProgress, error) {
	tiers, err := r.ListAllOrderedByXpMinAsc(ctx)
	if err != nil {
		return LoyaltyProgress{}, err
	}
	var cur LoyaltyTier
	for _, t := range tiers {
		if loyaltyXP >= t.XpMin {
			cur = t
		}
	}
	var next *LoyaltyTier
	for i := range tiers {
		if tiers[i].XpMin > loyaltyXP {
			next = &tiers[i]
			break
		}
	}
	return LoyaltyProgress{CurrentTier: cur, NextTier: next}, nil
}

// MinXpMinWithPositiveDiscount — минимальный порог xp_min среди уровней со скидкой > 0 (для статистики «выше нулевого уровня»).
func (r *LoyaltyTierRepository) MinXpMinWithPositiveDiscount(ctx context.Context) (xpMin int64, ok bool, err error) {
	tiers, err := r.ListAllOrderedByXpMinAsc(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, t := range tiers {
		if t.DiscountPercent > 0 {
			return t.XpMin, true, nil
		}
	}
	return 0, false, nil
}

// GetByID возвращает уровень по id.
func (r *LoyaltyTierRepository) GetByID(ctx context.Context, id int64) (*LoyaltyTier, error) {
	q, args, err := sq.Select(loyaltyTierSelectColumns).
		From("loyalty_tier").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, err
	}
	var t LoyaltyTier
	err = r.pool.QueryRow(ctx, q, args...).Scan(&t.ID, &t.SortOrder, &t.XpMin, &t.DiscountPercent, &t.DisplayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// MaxSortOrder возвращает максимальный sort_order среди уровней (0 если таблица пуста).
func (r *LoyaltyTierRepository) MaxSortOrder(ctx context.Context) (int, error) {
	var m int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM loyalty_tier`).Scan(&m)
	return m, err
}

// Insert добавляет уровень (sort_order должен быть уникален по смыслу админки).
func (r *LoyaltyTierRepository) Insert(ctx context.Context, sortOrder int, xpMin int64, discountPct int, displayName *string) (int64, error) {
	q, args, err := sq.Insert("loyalty_tier").
		Columns("sort_order", "xp_min", "discount_percent", "display_name").
		Values(sortOrder, xpMin, discountPct, displayName).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.pool.QueryRow(ctx, q, args...).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Update обновляет порог и процент уровня (sort_order можно менять раздельно позже — MVP без смены порядка кроме явного поля).
func (r *LoyaltyTierRepository) Update(ctx context.Context, id int64, sortOrder int, xpMin int64, discountPct int, displayName *string) error {
	q, args, err := sq.Update("loyalty_tier").
		Set("sort_order", sortOrder).
		Set("xp_min", xpMin).
		Set("discount_percent", discountPct).
		Set("display_name", displayName).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("loyalty tier not found: %d", id)
	}
	return nil
}

// Delete удаляет уровень; уровень с sort_order = 0 (базовый) удалять нельзя.
func (r *LoyaltyTierRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.pool.Exec(ctx,
		`DELETE FROM loyalty_tier WHERE id = $1 AND sort_order <> 0`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("cannot delete tier %d (missing or base tier)", id)
	}
	return nil
}
