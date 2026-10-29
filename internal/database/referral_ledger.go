package database

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Виды начислений в журнале. Совпадают с CHECK-ограничением миграции 000043.
const (
	ReferralBonusKindFirstReferrer  = "first_referrer"
	ReferralBonusKindFirstReferee   = "first_referee"
	ReferralBonusKindRepeatReferrer = "repeat_referrer"
	ReferralBonusKindDefault        = "default_referrer"
	ReferralBonusKindManual         = "manual"
)

// ReferralBonusEntry — одно начисление реферальных дней.
type ReferralBonusEntry struct {
	ID                  int64
	ReferralID          *int64
	ReferrerTelegramID  int64
	RefereeTelegramID   int64
	RecipientTelegramID int64
	RecipientCustomerID *int64
	PurchaseID          *int64
	Months              int
	Days                int
	FirstMonthDays      int
	PerMonthDays        int
	Kind                string
	IsBackfilled        bool
	CreatedAt           time.Time
}

type ReferralBonusLedgerRepository struct {
	pool *pgxpool.Pool
}

func NewReferralBonusLedgerRepository(pool *pgxpool.Pool) *ReferralBonusLedgerRepository {
	return &ReferralBonusLedgerRepository{pool: pool}
}

// Строка принадлежит пригласившему, если дни достались именно ему. Так «сколько
// заработал на рефералах» отделяется от приветственного бонуса, полученного
// когда-то за то, что пригласили его самого, — и правило работает одинаково для
// автоматических начислений и ручных компенсаций.
const referrerDirected = "recipient_telegram_id = referrer_telegram_id"

// Insert записывает начисление. Повтор по той же покупке и тому же виду
// начисления молча игнорируется — за это отвечает частичный уникальный индекс.
// Повторная обработка одного платежа (ретрай вебхука, перезапуск воркера) не
// должна ни падать, ни удваивать историю.
func (r *ReferralBonusLedgerRepository) Insert(ctx context.Context, e ReferralBonusEntry) error {
	if e.Days < 0 {
		return fmt.Errorf("referral ledger: days must not be negative, got %d", e.Days)
	}
	createdAt := e.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	query := sq.Insert("referral_bonus_ledger").
		Columns(
			"referral_id", "referrer_telegram_id", "referee_telegram_id",
			"recipient_telegram_id", "recipient_customer_id",
			"purchase_id", "months", "days", "first_month_days", "per_month_days",
			"kind", "is_backfilled", "created_at",
		).
		Values(
			e.ReferralID, e.ReferrerTelegramID, e.RefereeTelegramID,
			e.RecipientTelegramID, e.RecipientCustomerID,
			e.PurchaseID, e.Months, e.Days, e.FirstMonthDays, e.PerMonthDays,
			e.Kind, e.IsBackfilled, createdAt,
		).
		Suffix("ON CONFLICT DO NOTHING").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert referral bonus query: %w", err)
	}
	if _, err := r.pool.Exec(ctx, sqlStr, args...); err != nil {
		return fmt.Errorf("failed to insert referral bonus: %w", err)
	}
	return nil
}

// EarnedDaysByReferrer — сколько дней пригласивший получил за своих рефералов
// за всё время и за последние 30 дней.
func (r *ReferralBonusLedgerRepository) EarnedDaysByReferrer(ctx context.Context, referrerID int64) (total int, lastMonth int, err error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	q := `
SELECT COALESCE(SUM(days), 0)::int,
       COALESCE(SUM(CASE WHEN created_at >= $2 THEN days ELSE 0 END), 0)::int
FROM referral_bonus_ledger
WHERE referrer_telegram_id = $1 AND ` + referrerDirected
	if err := r.pool.QueryRow(ctx, q, referrerID, cutoff).Scan(&total, &lastMonth); err != nil {
		return 0, 0, fmt.Errorf("failed to sum referral bonus days: %w", err)
	}
	return total, lastMonth, nil
}

// SumReferrerDaysRange — сумма дней, выданных пригласившим, за интервал.
// Нулевой from означает «с самого начала».
func (r *ReferralBonusLedgerRepository) SumReferrerDaysRange(ctx context.Context, from, to time.Time) (int64, error) {
	var sum int64
	var err error
	if from.IsZero() {
		q := `SELECT COALESCE(SUM(days), 0)::bigint FROM referral_bonus_ledger
WHERE ` + referrerDirected + ` AND created_at < $1`
		err = r.pool.QueryRow(ctx, q, to).Scan(&sum)
	} else {
		q := `SELECT COALESCE(SUM(days), 0)::bigint FROM referral_bonus_ledger
WHERE ` + referrerDirected + ` AND created_at >= $1 AND created_at < $2`
		err = r.pool.QueryRow(ctx, q, from, to).Scan(&sum)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to sum referral bonus days for range: %w", err)
	}
	return sum, nil
}

// HasBackfilledRows сообщает, что восстановление истории уже выполнялось.
//
// Признак — собственные строки бэкфилла (is_backfilled), а не пустота таблицы.
// Пустота лгала: если бэкфилл упал (недоступность базы на старте) и следом
// прошло хоть одно живое начисление, журнал переставал быть пустым, повтор не
// запускался никогда, и вся историческая статистика «заработано дней»
// оставалась нулевой навсегда. Повторный запуск безопасен: вставка идёт
// ON CONFLICT DO NOTHING по паре «покупка + вид начисления».
func (r *ReferralBonusLedgerRepository) HasBackfilledRows(ctx context.Context) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM referral_bonus_ledger WHERE is_backfilled)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check referral ledger backfill marker: %w", err)
	}
	return exists, nil
}

// HistoricalGrant — одно восстановленное начисление из истории покупок.
type HistoricalGrant struct {
	ReferralID         int64
	ReferrerTelegramID int64
	RefereeTelegramID  int64
	RefereeCustomerID  int64
	PurchaseID         int64
	Months             int
	PaidAt             time.Time
	// Порядковый номер оплаты этого реферала: 1 — первая, дальше повторные.
	PaymentIndex int
}

// ListHistoricalGrants возвращает все оплаты рефералов в хронологическом
// порядке — сырьё для бэкфилла.
//
// Отбор повторяет условия, по которым бонус начислялся до появления журнала:
// оплаченная покупка с ненулевым сроком (month > 0 отсекает доп. устройства) и
// существующая связь referral. Самореферал исключён здесь же — начисления по
// нему не было и в проде, воспроизводить его в истории незачем.
func (r *ReferralBonusLedgerRepository) ListHistoricalGrants(ctx context.Context) ([]HistoricalGrant, error) {
	// Нумерация оплат считается ДО присоединения referral. На referee_id нет
	// UNIQUE — только hash-индекс, поэтому дубликаты связок структурно возможны,
	// и ранжирование после join размножило бы каждую покупку, сдвинув номера:
	// вторая оплата получила бы третий номер и перестала считаться повторной.
	// Отбор совпадает с CountPaidSubscriptionsByCustomer, по которому живой код
	// отличает первую оплату от последующих.
	q := `
WITH ranked AS (
  SELECT p.id, p.customer_id, p.month, p.paid_at,
         ROW_NUMBER() OVER (PARTITION BY p.customer_id ORDER BY p.paid_at, p.id)::int AS payment_index
  FROM purchase p
  WHERE p.status = $1 AND p.month > 0 AND p.paid_at IS NOT NULL
)
SELECT ref.id, ref.referrer_id, ref.referee_id, c.id, r.id, r.month, r.paid_at, r.payment_index
FROM ranked r
JOIN customer c ON c.id = r.customer_id
JOIN referral ref ON ref.referee_id = c.telegram_id
WHERE ref.referrer_id <> ref.referee_id
ORDER BY r.paid_at, r.id`

	rows, err := r.pool.Query(ctx, q, PurchaseStatusPaid)
	if err != nil {
		return nil, fmt.Errorf("failed to query historical referral grants: %w", err)
	}
	defer rows.Close()

	var list []HistoricalGrant
	for rows.Next() {
		var g HistoricalGrant
		if err := rows.Scan(
			&g.ReferralID, &g.ReferrerTelegramID, &g.RefereeTelegramID,
			&g.RefereeCustomerID, &g.PurchaseID, &g.Months, &g.PaidAt, &g.PaymentIndex,
		); err != nil {
			return nil, fmt.Errorf("failed to scan historical referral grant: %w", err)
		}
		list = append(list, g)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating historical referral grants: %w", rows.Err())
	}
	return list, nil
}

// InsertBatch пишет пачку строк одной транзакцией. Используется бэкфиллом:
// восстановление истории должно быть «всё или ничего», иначе журнал остаётся
// наполненным наполовину. Повторный запуск безопасен — ON CONFLICT DO NOTHING
// по паре «покупка + вид начисления».
func (r *ReferralBonusLedgerRepository) InsertBatch(ctx context.Context, entries []ReferralBonusEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin referral ledger batch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := `
INSERT INTO referral_bonus_ledger
    (referral_id, referrer_telegram_id, referee_telegram_id,
     recipient_telegram_id, recipient_customer_id,
     purchase_id, months, days, first_month_days, per_month_days, kind, is_backfilled, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT DO NOTHING`

	for _, e := range entries {
		createdAt := e.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		if _, err := tx.Exec(ctx, q,
			e.ReferralID, e.ReferrerTelegramID, e.RefereeTelegramID,
			e.RecipientTelegramID, e.RecipientCustomerID,
			e.PurchaseID, e.Months, e.Days, e.FirstMonthDays, e.PerMonthDays,
			e.Kind, e.IsBackfilled, createdAt,
		); err != nil {
			return fmt.Errorf("failed to insert referral ledger row: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit referral ledger batch: %w", err)
	}
	return nil
}

// ReferralBonusFeedEntry — строка ленты «откуда взялись дни».
//
// Кто именно принёс начисление, здесь ещё в открытом виде: маскировкой
// занимается слой, который отдаёт данные наружу, — ему же видно, кто смотрит.
type ReferralBonusFeedEntry struct {
	ID                int64
	Days              int
	Months            int
	Kind              string
	CreatedAt         time.Time
	RefereeTelegramID int64
	RefereeUsername   *string
	RefereeEmail      *string
}

// ListRecentByReferrer — последние начисления, доставшиеся пригласившему.
//
// Фильтр тот же referrerDirected, что и у суммы «заработано дней»: лента
// обязана складываться ровно в то число, которое человек видит над ней.
// Приветственный бонус, полученный когда-то за то, что пригласили его самого,
// в эту сумму не входит и в ленте не показывается.
func (r *ReferralBonusLedgerRepository) ListRecentByReferrer(ctx context.Context, referrerID int64, limit int) ([]ReferralBonusFeedEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	// Почта берётся подзапросом, а не join: связок аккаунта с клиентом
	// структурно может быть несколько, и join размножил бы строку журнала —
	// одно начисление показалось бы двумя.
	q := `
SELECT l.id, l.days, l.months, l.kind, l.created_at, l.referee_telegram_id,
       c.telegram_username,
       (SELECT a.email
          FROM cabinet_account_customer_link lk
          JOIN cabinet_account a ON a.id = lk.account_id
         WHERE lk.customer_id = c.id
         LIMIT 1)
FROM referral_bonus_ledger l
LEFT JOIN customer c ON c.telegram_id = l.referee_telegram_id
WHERE l.referrer_telegram_id = $1 AND ` + referrerDirected + `
ORDER BY l.created_at DESC, l.id DESC
LIMIT $2`

	rows, err := r.pool.Query(ctx, q, referrerID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query referral bonus feed: %w", err)
	}
	defer rows.Close()

	list := make([]ReferralBonusFeedEntry, 0, limit)
	for rows.Next() {
		var e ReferralBonusFeedEntry
		if err := rows.Scan(
			&e.ID, &e.Days, &e.Months, &e.Kind, &e.CreatedAt,
			&e.RefereeTelegramID, &e.RefereeUsername, &e.RefereeEmail,
		); err != nil {
			return nil, fmt.Errorf("failed to scan referral bonus feed row: %w", err)
		}
		list = append(list, e)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral bonus feed: %w", rows.Err())
	}
	return list, nil
}

// SumDaysByReferee — сколько дней принёс каждый приглашённый.
//
// Отдельным запросом, а не расчётом по ленте: лента обрезана последними
// строками, и суммировать её значило бы показать «этот привёл 3 дня» там, где
// он привёл тридцать, — просто потому, что старые начисления не поместились.
//
// Фильтр тот же referrerDirected, поэтому сумма по всем приглашённым сходится
// с earned_days_total. Ключ — telegram id приглашённого.
func (r *ReferralBonusLedgerRepository) SumDaysByReferee(ctx context.Context, referrerID int64) (map[int64]int, error) {
	q := `
SELECT referee_telegram_id, COALESCE(SUM(days), 0)::int
FROM referral_bonus_ledger
WHERE referrer_telegram_id = $1 AND ` + referrerDirected + `
GROUP BY referee_telegram_id`

	rows, err := r.pool.Query(ctx, q, referrerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query referral days by referee: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]int)
	for rows.Next() {
		var refereeID int64
		var days int
		if err := rows.Scan(&refereeID, &days); err != nil {
			return nil, fmt.Errorf("failed to scan referral days by referee: %w", err)
		}
		out[refereeID] = days
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral days by referee: %w", rows.Err())
	}
	return out, nil
}
