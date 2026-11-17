package repository

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ReviewVisibilityAnonymous  = "anonymous"
	ReviewVisibilityName       = "name"
	ReviewVisibilityNameAvatar = "name_avatar"
)

var ErrReviewValidation = errors.New("review validation")

type Review struct {
	ID           int64
	AccountID    int64
	CustomerID   *int64
	Rating       float64
	Body         string
	Visibility   string
	DisplayName  string
	AvatarURL    string
	ClientSince  *time.Time
	TariffID     *int64
	TariffName   string
	OriginalBody string
	EditedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ReviewUpsertInput struct {
	AccountID   int64
	CustomerID  *int64
	Rating      float64
	Body        string
	Visibility  string
	DisplayName string
	AvatarURL   string
	ClientSince *time.Time
	TariffID    *int64
	TariffName  string
}

type ReviewRepo struct {
	pool *pgxpool.Pool
}

func NewReviewRepo(pool *pgxpool.Pool) *ReviewRepo {
	return &ReviewRepo{pool: pool}
}

const reviewSelectCols = `
	id, account_id, customer_id, rating, body, visibility, display_name, avatar_url,
	client_since, tariff_id, tariff_name, original_body, edited_at, created_at, updated_at`

func (r *ReviewRepo) FindByID(ctx context.Context, id int64) (*Review, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+reviewSelectCols+` FROM cabinet_review WHERE id = $1`, id)
	return scanReviewOrNotFound(row)
}

func (r *ReviewRepo) FindByAccountID(ctx context.Context, accountID int64) (*Review, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+reviewSelectCols+` FROM cabinet_review WHERE account_id = $1`, accountID)
	return scanReviewOrNotFound(row)
}

func (r *ReviewRepo) FindByCustomerID(ctx context.Context, customerID int64) (*Review, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+reviewSelectCols+`
		FROM cabinet_review
		WHERE customer_id = $1
		ORDER BY updated_at DESC, id DESC
		LIMIT 1`, customerID)
	return scanReviewOrNotFound(row)
}

func (r *ReviewRepo) Upsert(ctx context.Context, in ReviewUpsertInput) (*Review, error) {
	if err := validateReviewInput(in); err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO cabinet_review (
			account_id, customer_id, rating, body, visibility, display_name, avatar_url,
			client_since, tariff_id, tariff_name, original_body, edited_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $4, NULL, NOW(), NOW()
		)
		ON CONFLICT (account_id) DO UPDATE SET
			customer_id = EXCLUDED.customer_id,
			rating = EXCLUDED.rating,
			original_body = CASE
				WHEN cabinet_review.original_body = '' THEN cabinet_review.body
				ELSE cabinet_review.original_body
			END,
			edited_at = CASE
				WHEN EXCLUDED.body IS DISTINCT FROM cabinet_review.body THEN NOW()
				ELSE cabinet_review.edited_at
			END,
			body = EXCLUDED.body,
			visibility = EXCLUDED.visibility,
			display_name = EXCLUDED.display_name,
			avatar_url = EXCLUDED.avatar_url,
			client_since = EXCLUDED.client_since,
			tariff_id = EXCLUDED.tariff_id,
			tariff_name = EXCLUDED.tariff_name,
			updated_at = NOW()
		RETURNING `+reviewSelectCols,
		in.AccountID, in.CustomerID, in.Rating, in.Body, in.Visibility, in.DisplayName, in.AvatarURL,
		in.ClientSince, in.TariffID, in.TariffName,
	)
	return scanReview(row)
}

func (r *ReviewRepo) List(ctx context.Context, limit, offset int) ([]Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+reviewSelectCols+`
		FROM cabinet_review
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectReviews(rows)
}

func (r *ReviewRepo) Featured(ctx context.Context, limit int) ([]Review, error) {
	if limit <= 0 || limit > 12 {
		limit = 3
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+reviewSelectCols+`
		FROM cabinet_review
		ORDER BY random()
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectReviews(rows)
}

func (r *ReviewRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM cabinet_review`).Scan(&n)
	return n, err
}

// AdminUpdateSilent правит отзыв без метки «отредактировано» для публичной карточки.
func (r *ReviewRepo) AdminUpdateSilent(ctx context.Context, id int64, in ReviewUpsertInput) (*Review, error) {
	if err := validateReviewInput(in); err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE cabinet_review SET
			rating = $2,
			body = $3,
			visibility = $4,
			display_name = $5,
			avatar_url = $6,
			tariff_name = $7,
			original_body = '',
			edited_at = NULL,
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+reviewSelectCols,
		id, in.Rating, in.Body, in.Visibility, in.DisplayName, in.AvatarURL, in.TariffName,
	)
	rev, err := scanReview(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rev, nil
}

func (r *ReviewRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cabinet_review WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func validateReviewInput(in ReviewUpsertInput) error {
	body := strings.TrimSpace(in.Body)
	if len([]rune(body)) < 10 || len([]rune(body)) > 2000 {
		return ErrReviewValidation
	}
	if !validHalfStarRating(in.Rating) {
		return ErrReviewValidation
	}
	switch in.Visibility {
	case ReviewVisibilityAnonymous, ReviewVisibilityName, ReviewVisibilityNameAvatar:
	default:
		return ErrReviewValidation
	}
	return nil
}

func validHalfStarRating(rating float64) bool {
	if rating < 0.5 || rating > 5 {
		return false
	}
	halves := rating * 2
	return math.Abs(halves-math.Round(halves)) < 1e-9
}

type scannable interface {
	Scan(dest ...any) error
}

type rowIterator interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanReviewOrNotFound(row scannable) (*Review, error) {
	rev, err := scanReview(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rev, nil
}

func scanReview(row scannable) (*Review, error) {
	var rev Review
	err := row.Scan(
		&rev.ID, &rev.AccountID, &rev.CustomerID, &rev.Rating, &rev.Body, &rev.Visibility,
		&rev.DisplayName, &rev.AvatarURL, &rev.ClientSince, &rev.TariffID, &rev.TariffName,
		&rev.OriginalBody, &rev.EditedAt, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func collectReviews(rows rowIterator) ([]Review, error) {
	out := make([]Review, 0)
	for rows.Next() {
		rev, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rev)
	}
	return out, rows.Err()
}
