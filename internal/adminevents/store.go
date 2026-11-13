package adminevents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store — чтение и запись admin_event.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore создаёт хранилище журнала.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Insert пишет пачку одним запросом.
func (s *Store) Insert(ctx context.Context, batch []Event) error {
	if len(batch) == 0 {
		return nil
	}
	at := make([]time.Time, len(batch))
	types := make([]string, len(batch))
	levels := make([]int16, len(batch))
	customers := make([]*int64, len(batch))
	accounts := make([]*int64, len(batch))
	telegrams := make([]*int64, len(batch))
	actors := make([]*int64, len(batch))
	sources := make([]string, len(batch))
	payloads := make([]string, len(batch))
	for i, e := range batch {
		at[i] = e.At
		types[i] = e.Type
		levels[i] = int16(e.Level)
		if e.CustomerID > 0 {
			v := e.CustomerID
			customers[i] = &v
		}
		if e.AccountID > 0 {
			v := e.AccountID
			accounts[i] = &v
		}
		if e.TelegramID != 0 {
			v := e.TelegramID
			telegrams[i] = &v
		}
		if e.ActorTgID != 0 {
			v := e.ActorTgID
			actors[i] = &v
		}
		sources[i] = e.Source
		payloads[i] = string(e.payloadJSON())
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO admin_event (created_at, type, level, customer_id, actor_tg_id, source, payload)
		SELECT t.at, t.type, t.level,
		       COALESCE(t.customer_id, (
		           SELECT l.customer_id FROM cabinet_account_customer_link l
		           WHERE l.account_id = t.account_id ORDER BY l.customer_id LIMIT 1), (
		           SELECT c.id FROM customer c WHERE c.telegram_id = t.telegram_id)),
		       t.actor_tg_id, t.source, t.payload::jsonb
		FROM unnest($1::timestamptz[], $2::text[], $3::smallint[], $4::bigint[], $5::bigint[],
		            $6::text[], $7::text[], $8::bigint[], $9::bigint[])
		     AS t(at, type, level, customer_id, actor_tg_id, source, payload, account_id, telegram_id)`,
		at, types, levels, customers, actors, sources, payloads, accounts, telegrams)
	if err != nil {
		return fmt.Errorf("insert admin events: %w", err)
	}
	return nil
}

// ListFilter — выборка ленты.
type ListFilter struct {
	// AfterID — только события новее (для опроса); BeforeID — старше (подгрузка вниз).
	AfterID  int64
	BeforeID int64
	// Categories — пусто: все категории.
	Categories []string
	// BotActions — показывать нажатия в боте (уровень 2).
	BotActions bool
	// CustomerID — история одного клиента. В ней видны все уровни.
	CustomerID int64
	// HideExpired — убрать одиночные «подписка истекла»: в общей ленте их
	// заменяет одна сводная строка за сутки.
	HideExpired bool
	// Since — только события не старше этого момента (период ленты).
	Since time.Time
	Limit int
}

// Row — строка ленты с подписями клиента и админа.
type Row struct {
	ID         int64           `json:"id"`
	At         time.Time       `json:"at"`
	Type       string          `json:"type"`
	Level      int16           `json:"level"`
	Category   string          `json:"category"`
	Source     string          `json:"source"`
	Payload    json.RawMessage `json:"payload"`
	CustomerID *int64          `json:"customer_id,omitempty"`
	// Customer — @username, почта или telegram id. Без маскировки: это админка.
	Customer  *string `json:"customer,omitempty"`
	ActorTgID *int64  `json:"actor_tg_id,omitempty"`
	Actor     *string `json:"actor,omitempty"`
}

const maxListLimit = 200

// List возвращает события новыми сверху.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Row, error) {
	limit := f.Limit
	if limit <= 0 || limit > maxListLimit {
		limit = 50
	}
	where, args := listWhere(f)
	args = append(args, limit)
	q := `
		SELECT e.id, e.created_at, e.type, e.level, e.source, e.payload,
		       e.customer_id, c.telegram_id, c.telegram_username, c.is_web_only, acc.email,
		       e.actor_tg_id, ac.telegram_username
		FROM admin_event e
		LEFT JOIN customer c ON c.id = e.customer_id
		LEFT JOIN LATERAL (
			SELECT a.email FROM cabinet_account_customer_link l
			JOIN cabinet_account a ON a.id = l.account_id
			WHERE l.customer_id = c.id
			ORDER BY l.account_id LIMIT 1
		) acc ON TRUE
		LEFT JOIN customer ac ON ac.telegram_id = e.actor_tg_id
		WHERE ` + where + `
		ORDER BY e.id DESC
		LIMIT $` + fmt.Sprint(len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin events: %w", err)
	}
	defer rows.Close()
	out := make([]Row, 0, limit)
	for rows.Next() {
		var (
			r         Row
			payload   []byte
			tgID      *int64
			username  *string
			webOnly   *bool
			email     *string
			actorName *string
		)
		if err := rows.Scan(&r.ID, &r.At, &r.Type, &r.Level, &r.Source, &payload,
			&r.CustomerID, &tgID, &username, &webOnly, &email,
			&r.ActorTgID, &actorName); err != nil {
			return nil, fmt.Errorf("scan admin event: %w", err)
		}
		r.Payload = json.RawMessage(payload)
		r.Category = CategoryOf(r.Type)
		if r.CustomerID != nil && tgID != nil {
			l := customerLabel(username, email, *tgID, webOnly != nil && *webOnly, *r.CustomerID)
			r.Customer = &l
		}
		if r.ActorTgID != nil {
			l := customerLabel(actorName, nil, *r.ActorTgID, false, 0)
			r.Actor = &l
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// listWhere собирает WHERE. Вынесено ради теста: ошибка в скобках здесь
// показала бы в ленте чужие нажатия или спрятала бы оплаты.
func listWhere(f ListFilter) (string, []any) {
	var (
		conds []string
		args  []any
	)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.AfterID > 0 {
		conds = append(conds, "e.id > "+arg(f.AfterID))
	}
	if f.BeforeID > 0 {
		conds = append(conds, "e.id < "+arg(f.BeforeID))
	}
	if !f.Since.IsZero() {
		conds = append(conds, "e.created_at >= "+arg(f.Since))
	}
	if f.CustomerID > 0 {
		conds = append(conds, "e.customer_id = "+arg(f.CustomerID))
	}
	if f.HideExpired && f.CustomerID == 0 {
		conds = append(conds, "e.type <> "+arg(SubscriptionExpired))
	}

	var types []string
	for _, c := range f.Categories {
		if c == CategoryBot {
			continue
		}
		types = append(types, TypesOf(c)...)
	}
	important := "e.level = 1"
	if len(types) > 0 {
		important = "(e.level = 1 AND e.type = ANY(" + arg(types) + "))"
	}
	switch {
	case f.CustomerID > 0:
		// В истории клиента нажатия видны всегда, если они записаны.
		if len(types) > 0 {
			conds = append(conds, "("+important+" OR e.level = 2)")
		}
	case f.BotActions:
		conds = append(conds, "("+important+" OR e.level = 2)")
	default:
		conds = append(conds, important)
	}
	if len(conds) == 0 {
		return "TRUE", args
	}
	return strings.Join(conds, " AND "), args
}

// customerLabel — как в админских ответах партнёрки: ник, почта или telegram id.
// У web-клиента без почты telegram id служебный и огромный — тогда номер клиента.
func customerLabel(username, email *string, telegramID int64, webOnly bool, customerID int64) string {
	if username != nil {
		if u := strings.TrimPrefix(strings.TrimSpace(*username), "@"); u != "" {
			return "@" + u
		}
	}
	if email != nil {
		if e := strings.TrimSpace(*email); e != "" {
			return e
		}
	}
	if webOnly || telegramID == 0 {
		return fmt.Sprintf("#%d", customerID)
	}
	return fmt.Sprint(telegramID)
}

// Summary — счётчики за период.
type Summary struct {
	Payments int64 `json:"payments"`
	// Revenue — сумма успешных оплат по валютам.
	Revenue        map[string]float64 `json:"revenue"`
	Failed         int64              `json:"failed"`
	FailedProvider string             `json:"failed_provider,omitempty"`
	RegBot         int64              `json:"registrations_bot"`
	RegCabinet     int64              `json:"registrations_cabinet"`
	NodeDowns      int64              `json:"node_downs"`
}

// Summary считает события с since.
func (s *Store) Summary(ctx context.Context, since time.Time) (*Summary, error) {
	out := &Summary{Revenue: map[string]float64{}}
	err := s.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE type = ANY($2)),
			count(*) FILTER (WHERE type = $3),
			count(*) FILTER (WHERE type = $4 AND source = 'bot'),
			count(*) FILTER (WHERE type = $4 AND source <> 'bot'),
			count(*) FILTER (WHERE type = $5 AND source = 'panel')
		FROM admin_event
		WHERE created_at >= $1 AND level = 1`,
		since, PaidTypes, PaymentFailed, UserRegistered, NodeDown,
	).Scan(&out.Payments, &out.Failed, &out.RegBot, &out.RegCabinet, &out.NodeDowns)
	if err != nil {
		return nil, fmt.Errorf("admin events summary: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(NULLIF(payload->>'currency', ''), 'RUB'),
		       COALESCE(sum(CASE WHEN payload->>'amount' ~ '^-?[0-9]+(\.[0-9]+)?$'
		                         THEN (payload->>'amount')::numeric END), 0)::float8
		FROM admin_event
		WHERE created_at >= $1 AND level = 1 AND type = ANY($2)
		GROUP BY 1`, since, PaidTypes)
	if err != nil {
		return nil, fmt.Errorf("admin events revenue: %w", err)
	}
	for rows.Next() {
		var cur string
		var sum float64
		if err := rows.Scan(&cur, &sum); err != nil {
			rows.Close()
			return nil, err
		}
		out.Revenue[cur] = sum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out.Failed > 0 {
		_ = s.pool.QueryRow(ctx, `
			SELECT COALESCE(payload->>'provider', '')
			FROM admin_event
			WHERE created_at >= $1 AND type = $2
			GROUP BY 1 ORDER BY count(*) DESC, 1 LIMIT 1`, since, PaymentFailed).Scan(&out.FailedProvider)
	}
	return out, nil
}

// FailureBurst — провайдер, у которого много ошибок оплаты за короткое время.
type FailureBurst struct {
	Provider string
	Count    int
	Since    time.Time
}

// FailureBursts — провайдеры с min и больше ошибками оплаты начиная с since.
func (s *Store) FailureBursts(ctx context.Context, since time.Time, min int) ([]FailureBurst, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(payload->>'provider', ''), count(*), min(created_at)
		FROM admin_event
		WHERE created_at >= $1 AND type = $2
		GROUP BY 1 HAVING count(*) >= $3
		ORDER BY 2 DESC`, since, PaymentFailed, min)
	if err != nil {
		return nil, fmt.Errorf("admin events failure bursts: %w", err)
	}
	defer rows.Close()
	var out []FailureBurst
	for rows.Next() {
		var b FailureBurst
		if err := rows.Scan(&b.Provider, &b.Count, &b.Since); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ExpiredDigest — сколько подписок истекло начиная с since и сколько из них
// так и не продлили (expire_at клиента всё ещё в прошлом).
func (s *Store) ExpiredDigest(ctx context.Context, since time.Time) (expired, notRenewed int64, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT e.customer_id),
		       count(DISTINCT e.customer_id) FILTER (WHERE c.expire_at IS NULL OR c.expire_at <= now())
		FROM admin_event e
		LEFT JOIN customer c ON c.id = e.customer_id
		WHERE e.created_at >= $1 AND e.type = $2`, since, SubscriptionExpired).Scan(&expired, &notRenewed)
	if err != nil {
		return 0, 0, fmt.Errorf("admin events expired digest: %w", err)
	}
	return expired, notRenewed, nil
}

// ExpiredSince — клиенты, у которых подписка закончилась в (from, to] и о ком
// ещё нет события «истекла» за последние двое суток.
func (s *Store) ExpiredSince(ctx context.Context, from, to time.Time) ([]ExpiredCustomer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.expire_at, c.current_tariff_id
		FROM customer c
		WHERE c.expire_at > $1 AND c.expire_at <= $2
		  AND NOT EXISTS (
			SELECT 1 FROM admin_event e
			WHERE e.customer_id = c.id AND e.type = $3
			  AND e.created_at > $2 - interval '2 days')
		LIMIT 5000`, from, to, SubscriptionExpired)
	if err != nil {
		return nil, fmt.Errorf("admin events expired since: %w", err)
	}
	defer rows.Close()
	var out []ExpiredCustomer
	for rows.Next() {
		var c ExpiredCustomer
		if err := rows.Scan(&c.CustomerID, &c.ExpireAt, &c.TariffID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExpiredCustomer — клиент, у которого только что закончилась подписка.
type ExpiredCustomer struct {
	CustomerID int64
	ExpireAt   time.Time
	TariffID   *int64
}

const purgeChunk = 5000

// Purge удаляет события старше сроков хранения. Пачками, чтобы не держать
// длинную блокировку на большой таблице. Ноль дней — не чистить этот класс.
func (s *Store) Purge(ctx context.Context, now time.Time, importantDays, noisyDays, botDays int) (int64, error) {
	var total int64
	classes := []struct {
		days int
		cond string
		args []any
	}{
		{botDays, "level = 2", nil},
		{noisyDays, "level = 1 AND type = ANY($2)", []any{noisyTypes}},
		{importantDays, "level = 1 AND NOT (type = ANY($2))", []any{noisyTypes}},
	}
	for _, c := range classes {
		if c.days <= 0 {
			continue
		}
		cutoff := now.AddDate(0, 0, -c.days)
		q := `DELETE FROM admin_event WHERE id IN (
			SELECT id FROM admin_event WHERE created_at < $1 AND ` + c.cond + ` LIMIT ` + fmt.Sprint(purgeChunk) + `)`
		args := append([]any{cutoff}, c.args...)
		for {
			tag, err := s.pool.Exec(ctx, q, args...)
			if err != nil {
				return total, fmt.Errorf("purge admin events: %w", err)
			}
			total += tag.RowsAffected()
			if tag.RowsAffected() < purgeChunk {
				break
			}
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	return total, nil
}

// CountByCustomer — сколько событий у клиента (для кнопки «История · N»).
func (s *Store) CountByCustomer(ctx context.Context, customerID int64) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_event WHERE customer_id = $1`, customerID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count admin events: %w", err)
	}
	return n, nil
}

// LastID — id самого свежего события (ноль, если журнал пуст).
func (s *Store) LastID(ctx context.Context) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM admin_event`).Scan(&id)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	return id, nil
}
