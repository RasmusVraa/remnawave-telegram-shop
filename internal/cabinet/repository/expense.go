package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Категории расходов — фиксированный набор, совпадает с фронтом.
var ExpenseCategories = map[string]struct{}{
	"servers":  {},
	"traffic":  {},
	"domains":  {},
	"ads":      {},
	"software": {},
	"taxes":    {},
	"salary":   {},
	"other":    {},
}

const (
	ExpenseTitleMax = 200
	ExpenseNoteMax  = 2000
	ExpenseIconMax  = 32
)

var ErrExpenseValidation = errors.New("repository: invalid expense")

type Expense struct {
	ID          int64
	OccurredOn  time.Time
	AmountRub   float64
	Category    string
	Title       string
	Note        string
	Icon        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ExpenseInput struct {
	OccurredOn time.Time
	AmountRub  float64
	Category   string
	Title      string
	Note       string
	Icon       string
}

type ExpenseCategorySum struct {
	Category  string
	AmountRub float64
	Count     int64
}

type ExpenseRepo struct {
	pool *pgxpool.Pool
}

func NewExpenseRepo(pool *pgxpool.Pool) *ExpenseRepo {
	return &ExpenseRepo{pool: pool}
}

func NormalizeExpenseInput(in ExpenseInput) (ExpenseInput, error) {
	in.Category = strings.TrimSpace(strings.ToLower(in.Category))
	if _, ok := ExpenseCategories[in.Category]; !ok {
		return in, fmt.Errorf("%w: category", ErrExpenseValidation)
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > ExpenseTitleMax {
		return in, fmt.Errorf("%w: title", ErrExpenseValidation)
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > ExpenseNoteMax {
		return in, fmt.Errorf("%w: note", ErrExpenseValidation)
	}
	in.Icon = strings.TrimSpace(in.Icon)
	if utf8.RuneCountInString(in.Icon) > ExpenseIconMax {
		return in, fmt.Errorf("%w: icon", ErrExpenseValidation)
	}
	if in.AmountRub <= 0 || in.AmountRub > 1_000_000_000 {
		return in, fmt.Errorf("%w: amount", ErrExpenseValidation)
	}
	if in.OccurredOn.IsZero() {
		return in, fmt.Errorf("%w: date", ErrExpenseValidation)
	}
	// Только календарный день UTC.
	d := in.OccurredOn.UTC()
	in.OccurredOn = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	return in, nil
}

func scanExpense(row pgx.Row) (*Expense, error) {
	var e Expense
	err := row.Scan(
		&e.ID, &e.OccurredOn, &e.AmountRub, &e.Category, &e.Title, &e.Note, &e.Icon, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

const expenseCols = `id, occurred_on, amount_rub::float8, category, title, note, icon, created_at, updated_at`

func (r *ExpenseRepo) Create(ctx context.Context, in ExpenseInput) (*Expense, error) {
	in, err := NormalizeExpenseInput(in)
	if err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO admin_expense (occurred_on, amount_rub, category, title, note, icon)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+expenseCols, in.OccurredOn, in.AmountRub, in.Category, in.Title, in.Note, in.Icon)
	return scanExpense(row)
}

func (r *ExpenseRepo) Update(ctx context.Context, id int64, in ExpenseInput) (*Expense, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id", ErrExpenseValidation)
	}
	in, err := NormalizeExpenseInput(in)
	if err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE admin_expense
		SET occurred_on = $2, amount_rub = $3, category = $4, title = $5, note = $6, icon = $7, updated_at = NOW()
		WHERE id = $1
		RETURNING `+expenseCols, id, in.OccurredOn, in.AmountRub, in.Category, in.Title, in.Note, in.Icon)
	e, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return e, err
}

func (r *ExpenseRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM admin_expense WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ExpenseRepo) ListMonth(ctx context.Context, year int, month time.Month) ([]Expense, error) {
	from, to := monthBounds(year, month)
	rows, err := r.pool.Query(ctx, `
		SELECT `+expenseCols+`
		FROM admin_expense
		WHERE occurred_on >= $1 AND occurred_on < $2
		ORDER BY occurred_on DESC, id DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Expense, 0, 32)
	for rows.Next() {
		var e Expense
		if err := rows.Scan(
			&e.ID, &e.OccurredOn, &e.AmountRub, &e.Category, &e.Title, &e.Note, &e.Icon, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *ExpenseRepo) SumMonth(ctx context.Context, year int, month time.Month) (total float64, byCat []ExpenseCategorySum, err error) {
	from, to := monthBounds(year, month)
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_rub), 0)::float8
		FROM admin_expense
		WHERE occurred_on >= $1 AND occurred_on < $2`, from, to).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT category, COALESCE(SUM(amount_rub), 0)::float8, COUNT(*)::bigint
		FROM admin_expense
		WHERE occurred_on >= $1 AND occurred_on < $2
		GROUP BY category
		ORDER BY SUM(amount_rub) DESC`, from, to)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ExpenseCategorySum
		if err := rows.Scan(&c.Category, &c.AmountRub, &c.Count); err != nil {
			return 0, nil, err
		}
		byCat = append(byCat, c)
	}
	return total, byCat, rows.Err()
}

// RevenueMonthRub — оплаченные покупки в RUB за календарный месяц (как в админ-статистике).
func (r *ExpenseRepo) RevenueMonthRub(ctx context.Context, year int, month time.Month) (float64, error) {
	from, to := monthBounds(year, month)
	var sum float64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(p.amount), 0)::float8
		FROM purchase p
		WHERE p.status = 'paid'
		  AND p.paid_at IS NOT NULL
		  AND p.paid_at >= $1 AND p.paid_at < $2
		  AND (UPPER(TRIM(COALESCE(p.currency, ''))) IN ('RUB', 'RUR', '') OR COALESCE(p.currency, '') = '')`,
		from, to).Scan(&sum)
	return sum, err
}

func monthBounds(year int, month time.Month) (time.Time, time.Time) {
	from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	return from, to
}
