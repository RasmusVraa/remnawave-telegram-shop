package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Статусы отзыва (cabinet_review.status).
const (
	ReviewStatusPending   = "pending"
	ReviewStatusPublished = "published"
	ReviewStatusHidden    = "hidden"
)

// Как показывать автора (cabinet_review.visibility).
const (
	ReviewVisibilityAnonymous  = "anonymous"
	ReviewVisibilityName       = "name"
	ReviewVisibilityNameAvatar = "name_avatar"
)

// Источник аватара (cabinet_review.avatar_kind).
const (
	ReviewAvatarNone     = ""
	ReviewAvatarTelegram = "telegram"
	ReviewAvatarURL      = "url"
)

const (
	ReviewBodyMin  = 5
	ReviewBodyMax  = 1000
	ReviewReplyMax = 1000
)

// ErrReviewValidation — входные данные не прошли проверку.
var ErrReviewValidation = errors.New("repository: invalid review")

// Review — строка cabinet_review.
type Review struct {
	ID           int64
	AccountID    *int64
	CustomerID   *int64
	Rating       int
	Body         string
	OriginalBody string
	Status       string
	Visibility   string
	DisplayName  string
	AvatarKind   string
	AvatarURL    string
	TelegramID   *int64
	ClientSince  *time.Time
	TariffLabel  string
	ByAdmin      bool
	ReplyBody    string
	ReplyAt      *time.Time
	PublishedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ReviewInput — данные отзыва от клиента или от имени клиента.
type ReviewInput struct {
	AccountID   int64
	CustomerID  *int64
	Rating      int
	Body        string
	Visibility  string
	DisplayName string
	AvatarKind  string
	AvatarURL   string
	TelegramID  *int64
	ClientSince *time.Time
	TariffLabel string
	// PublishedAt — дата отзыва, которую задал админ (старые отзывы из канала); nil — сейчас.
	PublishedAt *time.Time
}

// ReviewAdminUpdate — правка админом; nil — поле не трогаем.
type ReviewAdminUpdate struct {
	Rating      *int
	Body        *string
	Visibility  *string
	DisplayName *string
	Status      *string
	ReplyBody   *string
	// PublishedAt — поправить дату отзыва (и публикации, если он опубликован).
	PublishedAt *time.Time
}

// ReviewAdminFilter — фильтр списка в админке.
type ReviewAdminFilter struct {
	Status string // "" — любой
	Search string
}

// ReviewCounts — сколько отзывов в каждом статусе.
type ReviewCounts struct {
	All       int64
	Pending   int64
	Published int64
	Hidden    int64
}

// ReviewRepo — cabinet_review.
type ReviewRepo struct {
	pool *pgxpool.Pool
}

func NewReviewRepo(pool *pgxpool.Pool) *ReviewRepo { return &ReviewRepo{pool: pool} }

const reviewCols = `id, account_id, customer_id, rating, body, original_body, status, visibility,
	display_name, avatar_kind, avatar_url, telegram_id, client_since, tariff_label, by_admin,
	reply_body, reply_at, published_at, created_at, updated_at`

func scanReview(row pgx.Row) (*Review, error) {
	var r Review
	err := row.Scan(&r.ID, &r.AccountID, &r.CustomerID, &r.Rating, &r.Body, &r.OriginalBody, &r.Status,
		&r.Visibility, &r.DisplayName, &r.AvatarKind, &r.AvatarURL, &r.TelegramID, &r.ClientSince,
		&r.TariffLabel, &r.ByAdmin, &r.ReplyBody, &r.ReplyAt, &r.PublishedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

func collectReviews(rows pgx.Rows) ([]Review, error) {
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func validReviewVisibility(v string) bool {
	switch v {
	case ReviewVisibilityAnonymous, ReviewVisibilityName, ReviewVisibilityNameAvatar:
		return true
	}
	return false
}

func validReviewStatus(v string) bool {
	switch v {
	case ReviewStatusPending, ReviewStatusPublished, ReviewStatusHidden:
		return true
	}
	return false
}

func validReviewBody(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= ReviewBodyMin && n <= ReviewBodyMax
}

func (in ReviewInput) validate() error {
	// Без аккаунта отзыв допустим только от имени клиента, поэтому нужен customer_id.
	if (in.AccountID <= 0 && in.CustomerID == nil) || in.Rating < 1 || in.Rating > 5 || !validReviewBody(in.Body) || !validReviewVisibility(in.Visibility) {
		return ErrReviewValidation
	}
	return nil
}

func (r *ReviewRepo) FindByID(ctx context.Context, id int64) (*Review, error) {
	return scanReview(r.pool.QueryRow(ctx, `SELECT `+reviewCols+` FROM cabinet_review WHERE id = $1`, id))
}

// CustomerEmail — почта аккаунта кабинета клиента для подсказки в поиске; пусто, если аккаунта нет.
func (r *ReviewRepo) CustomerEmail(ctx context.Context, customerID int64) (string, bool) {
	var email *string
	err := r.pool.QueryRow(ctx, `SELECT a.email FROM cabinet_account_customer_link l
		JOIN cabinet_account a ON a.id = l.account_id
		WHERE l.customer_id = $1 AND l.link_status = 'linked' ORDER BY l.id LIMIT 1`, customerID).Scan(&email)
	if err != nil {
		return "", false
	}
	if email == nil {
		return "", true
	}
	return *email, true
}

// FindByCustomerNoAccount — отзыв от имени клиента без аккаунта кабинета.
func (r *ReviewRepo) FindByCustomerNoAccount(ctx context.Context, customerID int64) (*Review, error) {
	return scanReview(r.pool.QueryRow(ctx, `SELECT `+reviewCols+` FROM cabinet_review WHERE customer_id = $1 AND account_id IS NULL`, customerID))
}

func (r *ReviewRepo) FindByAccountID(ctx context.Context, accountID int64) (*Review, error) {
	return scanReview(r.pool.QueryRow(ctx, `SELECT `+reviewCols+` FROM cabinet_review WHERE account_id = $1`, accountID))
}

// UpsertClient сохраняет отзыв самого клиента. moderated — новые отзывы идут «на
// проверку». Скрытый админом отзыв после правки клиента скрытым и остаётся, иначе
// спам вернулся бы одним нажатием. Правка клиентом снимает следы тихой правки
// админа (original_body): текст снова целиком клиентский.
// created=true — запись создана, а не обновлена.
func (r *ReviewRepo) UpsertClient(ctx context.Context, in ReviewInput, moderated bool) (rev *Review, created bool, err error) {
	if err := in.validate(); err != nil || in.AccountID <= 0 {
		return nil, false, ErrReviewValidation
	}
	fresh := ReviewStatusPublished
	if moderated {
		fresh = ReviewStatusPending
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO cabinet_review (
			account_id, customer_id, rating, body, status, visibility, display_name,
			avatar_kind, avatar_url, telegram_id, client_since, tariff_label, published_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, $13)
		ON CONFLICT (account_id) DO UPDATE SET
			-- Сбой чтения клиента не должен стирать то, что уже знали об авторе.
			customer_id   = COALESCE(EXCLUDED.customer_id, cabinet_review.customer_id),
			rating        = EXCLUDED.rating,
			body          = EXCLUDED.body,
			-- Клиент не менял текст (поправил оценку или галочки): правка админа остаётся правкой, оригинал цел.
			original_body = CASE WHEN EXCLUDED.body = cabinet_review.body THEN cabinet_review.original_body ELSE '' END,
			status        = CASE WHEN cabinet_review.status = 'hidden' THEN 'hidden' ELSE EXCLUDED.status END,
			visibility    = EXCLUDED.visibility,
			display_name  = EXCLUDED.display_name,
			avatar_kind   = EXCLUDED.avatar_kind,
			avatar_url    = EXCLUDED.avatar_url,
			telegram_id   = EXCLUDED.telegram_id,
			client_since  = COALESCE(EXCLUDED.client_since, cabinet_review.client_since),
			tariff_label  = CASE WHEN EXCLUDED.tariff_label = '' THEN cabinet_review.tariff_label ELSE EXCLUDED.tariff_label END,
			published_at  = CASE
				WHEN cabinet_review.status = 'hidden' THEN cabinet_review.published_at
				WHEN EXCLUDED.status = 'published' THEN COALESCE(cabinet_review.published_at, NOW())
				ELSE NULL END,
			updated_at    = NOW()
		RETURNING `+reviewCols+`, (xmax = 0)`,
		in.AccountID, in.CustomerID, in.Rating, in.Body, fresh, in.Visibility, in.DisplayName,
		in.AvatarKind, in.AvatarURL, in.TelegramID, in.ClientSince, in.TariffLabel, publishedAt(fresh))
	rev, created, err = scanReviewCreated(row)
	return rev, created, err
}

// UpsertByAdmin — отзыв «от имени клиента»: статус задаёт админ, пометка by_admin.
func (r *ReviewRepo) UpsertByAdmin(ctx context.Context, in ReviewInput, status string) (*Review, error) {
	if err := in.validate(); err != nil || !validReviewStatus(status) || (in.PublishedAt != nil && !validReviewDate(*in.PublishedAt)) {
		return nil, ErrReviewValidation
	}
	var account any // nil — клиент без аккаунта кабинета
	target := `ON CONFLICT (customer_id) WHERE account_id IS NULL`
	if in.AccountID > 0 {
		account, target = in.AccountID, `ON CONFLICT (account_id)`
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO cabinet_review (
			account_id, customer_id, rating, body, status, visibility, display_name,
			avatar_kind, avatar_url, telegram_id, client_since, tariff_label, by_admin, published_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, TRUE, $13, COALESCE($14::timestamptz, NOW()))
		`+target+` DO UPDATE SET
			rating        = EXCLUDED.rating,
			body          = EXCLUDED.body,
			status        = EXCLUDED.status,
			visibility    = EXCLUDED.visibility,
			display_name  = EXCLUDED.display_name,
			avatar_kind   = EXCLUDED.avatar_kind,
			avatar_url    = EXCLUDED.avatar_url,
			telegram_id   = EXCLUDED.telegram_id,
			published_at  = CASE
				WHEN EXCLUDED.status = 'published' AND $14::timestamptz IS NOT NULL THEN $14::timestamptz
				WHEN EXCLUDED.status = 'published' THEN COALESCE(cabinet_review.published_at, NOW())
				ELSE cabinet_review.published_at END,
			created_at    = COALESCE($14::timestamptz, cabinet_review.created_at),
			updated_at    = NOW()
		RETURNING `+reviewCols,
		account, in.CustomerID, in.Rating, in.Body, status, in.Visibility, in.DisplayName,
		in.AvatarKind, in.AvatarURL, in.TelegramID, in.ClientSince, in.TariffLabel, adminPublishedAt(status, in.PublishedAt), in.PublishedAt)
	return scanReview(row)
}

// adminPublishedAt — метка публикации у отзыва от имени клиента: заданная админом дата или сейчас.
func adminPublishedAt(status string, when *time.Time) *time.Time {
	if status != ReviewStatusPublished {
		return nil
	}
	if when != nil {
		return when
	}
	return publishedAt(status)
}

// validReviewDate — дата отзыва не в будущем и не древнее 2015 года.
func validReviewDate(t time.Time) bool {
	return t.Before(time.Now().Add(24*time.Hour)) && t.After(time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
}

// publishedAt — метка публикации для новой записи: только у опубликованной.
func publishedAt(status string) *time.Time {
	if status != ReviewStatusPublished {
		return nil
	}
	now := time.Now()
	return &now
}

func scanReviewCreated(row pgx.Row) (*Review, bool, error) {
	var r Review
	var created bool
	err := row.Scan(&r.ID, &r.AccountID, &r.CustomerID, &r.Rating, &r.Body, &r.OriginalBody, &r.Status,
		&r.Visibility, &r.DisplayName, &r.AvatarKind, &r.AvatarURL, &r.TelegramID, &r.ClientSince,
		&r.TariffLabel, &r.ByAdmin, &r.ReplyBody, &r.ReplyAt, &r.PublishedAt, &r.CreatedAt, &r.UpdatedAt, &created)
	if err != nil {
		return nil, false, err
	}
	return &r, created, nil
}

// AdminUpdate применяет правку админом. Правка текста запоминает первый оригинал;
// ответ команды получает метку времени. replyChanged — ответ стал непустым и новым
// (по нему шлём уведомление клиенту).
func (r *ReviewRepo) AdminUpdate(ctx context.Context, id int64, u ReviewAdminUpdate) (rev *Review, replyChanged bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cur, err := scanReview(tx.QueryRow(ctx, `SELECT `+reviewCols+` FROM cabinet_review WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, false, err
	}
	if u.Rating != nil {
		if *u.Rating < 1 || *u.Rating > 5 {
			return nil, false, ErrReviewValidation
		}
		cur.Rating = *u.Rating
	}
	if u.Body != nil {
		body := strings.TrimSpace(*u.Body)
		if !validReviewBody(body) {
			return nil, false, ErrReviewValidation
		}
		if body != cur.Body && cur.OriginalBody == "" {
			cur.OriginalBody = cur.Body
		}
		cur.Body = body
	}
	if u.Visibility != nil {
		if !validReviewVisibility(*u.Visibility) {
			return nil, false, ErrReviewValidation
		}
		cur.Visibility = *u.Visibility
	}
	if u.DisplayName != nil {
		name := strings.TrimSpace(*u.DisplayName)
		if utf8.RuneCountInString(name) > 120 {
			return nil, false, ErrReviewValidation
		}
		cur.DisplayName = name
	}
	if u.Status != nil {
		if !validReviewStatus(*u.Status) {
			return nil, false, ErrReviewValidation
		}
		if *u.Status == ReviewStatusPublished && cur.PublishedAt == nil {
			now := time.Now()
			cur.PublishedAt = &now
		}
		cur.Status = *u.Status
	}
	if u.PublishedAt != nil {
		if !validReviewDate(*u.PublishedAt) {
			return nil, false, ErrReviewValidation
		}
		cur.CreatedAt = *u.PublishedAt
		if cur.Status == ReviewStatusPublished {
			d := *u.PublishedAt
			cur.PublishedAt = &d
		}
	}
	if u.ReplyBody != nil {
		reply := strings.TrimSpace(*u.ReplyBody)
		if utf8.RuneCountInString(reply) > ReviewReplyMax {
			return nil, false, ErrReviewValidation
		}
		if reply != cur.ReplyBody {
			replyChanged = reply != ""
			cur.ReplyBody = reply
			if reply == "" {
				cur.ReplyAt = nil
			} else {
				now := time.Now()
				cur.ReplyAt = &now
			}
		}
	}

	row := tx.QueryRow(ctx, `
		UPDATE cabinet_review SET rating=$2, body=$3, original_body=$4, status=$5, visibility=$6,
			display_name=$7, reply_body=$8, reply_at=$9, published_at=$10, created_at=$11, updated_at=NOW()
		WHERE id=$1 RETURNING `+reviewCols,
		id, cur.Rating, cur.Body, cur.OriginalBody, cur.Status, cur.Visibility, cur.DisplayName,
		cur.ReplyBody, cur.ReplyAt, cur.PublishedAt, cur.CreatedAt)
	rev, err = scanReview(row)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return rev, replyChanged, nil
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

// ListPublished — опубликованные, новые сверху.
func (r *ReviewRepo) ListPublished(ctx context.Context, limit, offset int) ([]Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `SELECT `+reviewCols+` FROM cabinet_review
		WHERE status = 'published' ORDER BY published_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	return collectReviews(rows)
}

// Featured — случайные опубликованные для лендинга.
func (r *ReviewRepo) Featured(ctx context.Context, limit int) ([]Review, error) {
	if limit <= 0 || limit > 12 {
		limit = 6
	}
	rows, err := r.pool.Query(ctx, `SELECT `+reviewCols+` FROM cabinet_review
		WHERE status = 'published' ORDER BY random() LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return collectReviews(rows)
}

func (r *ReviewRepo) CountPublished(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM cabinet_review WHERE status = 'published'`).Scan(&n)
	return n, err
}

// Counts — число отзывов по статусам для вкладок админки.
func (r *ReviewRepo) Counts(ctx context.Context) (ReviewCounts, error) {
	var c ReviewCounts
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*),
		COUNT(*) FILTER (WHERE status = 'pending'),
		COUNT(*) FILTER (WHERE status = 'published'),
		COUNT(*) FILTER (WHERE status = 'hidden') FROM cabinet_review`).Scan(&c.All, &c.Pending, &c.Published, &c.Hidden)
	return c, err
}

// AdminList — список для админки: новые сверху, «на проверке» не прячутся за другими.
func (r *ReviewRepo) AdminList(ctx context.Context, f ReviewAdminFilter, limit, offset int) ([]Review, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	where := []string{"TRUE"}
	args := []any{}
	if f.Status != "" {
		if !validReviewStatus(f.Status) {
			return nil, 0, ErrReviewValidation
		}
		args = append(args, f.Status)
		where = append(where, "status = $"+strconv.Itoa(len(args)))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+escapeLike(s)+"%")
		p := "$" + strconv.Itoa(len(args))
		cond := fmt.Sprintf("(body ILIKE %s OR display_name ILIKE %s", p, p)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			args = append(args, n)
			q := "$" + strconv.Itoa(len(args))
			cond += fmt.Sprintf(" OR customer_id = %s OR telegram_id = %s", q, q)
		}
		where = append(where, cond+")")
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM cabinet_review WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, `SELECT `+reviewCols+` FROM cabinet_review WHERE `+cond+
		` ORDER BY (status = 'pending') DESC, created_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)-1)+
		` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	items, err := collectReviews(rows)
	return items, total, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
