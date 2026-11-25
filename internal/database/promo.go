package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PendingDiscountUnlimitedPayments — значение subscription_payments_remaining: безлимит оплат до истечения TTL.
const PendingDiscountUnlimitedPayments = -1

// TechnicalPromoPrefix — коды, начинающиеся с «__», служебные: якоря для pending-скидок
// (__CABINET_FORTUNE__ — колесо фортуны, __LIFECYCLE_WINBACK__ — win-back) и т. п.
// Админ создать такой код не может (валидация), а видеть их в разделе промокодов не должен:
// ни в списке, ни в счётчиках, ни в топе статистики. Любой новый служебный код называем
// с этим префиксом, и он скрывается сам. См. documentation/technical-promo-codes.md.
const TechnicalPromoPrefix = "__"

// TechnicalPromoFortune — служебный промокод-якорь для скидок колеса фортуны (migration 000033).
const TechnicalPromoFortune = "__CABINET_FORTUNE__"

// IsTechnicalPromoCode — служебный ли код (скрывается от админа).
func IsTechnicalPromoCode(code string) bool {
	return strings.HasPrefix(code, TechnicalPromoPrefix)
}

// notTechnicalPromo — SQL-условие «код не служебный» для столбца code.
const notTechnicalPromo = `code NOT LIKE '\_\_%'`

const (
	PromoTypeSubscriptionDays = "subscription_days"
	PromoTypeTrial            = "trial"
	PromoTypeExtraHwid        = "extra_hwid"
	PromoTypeDiscount         = "discount"
)

type PromoCode struct {
	ID                                         int64      `db:"id"`
	Code                                       string     `db:"code"`
	Type                                       string     `db:"type"`
	SubscriptionDays                           *int       `db:"subscription_days"`
	TrialDays                                  *int       `db:"trial_days"`
	ExtraHwidDelta                             *int       `db:"extra_hwid_delta"`
	DiscountPercent                            *int       `db:"discount_percent"`
	DiscountTTLHours                           *int       `db:"discount_ttl_hours"`
	MaxUses                                    *int       `db:"max_uses"`
	UsesCount                                  int        `db:"uses_count"`
	ValidUntil                                 *time.Time `db:"valid_until"`
	Active                                     bool       `db:"active"`
	FirstPurchaseOnly                          bool       `db:"first_purchase_only"`
	RequireCustomerInDB                        bool       `db:"require_customer_in_db"`
	AllowTrialWithoutPayment                   bool       `db:"allow_trial_without_payment"`
	CreatedAt                                  time.Time  `db:"created_at"`
	DiscountMaxSubscriptionPaymentsPerCustomer int        `db:"discount_max_subscription_payments_per_customer"`
	TariffID                                   *int64     `db:"tariff_id"`
	// Note — подпись админа «для кого и зачем» (до 200 символов, миграция 000069).
	// Видна только в web-админке; бот её не читает и не пишет.
	Note *string `db:"note"`
}

// promoColumns — столбцы promo_code в порядке scanPromoRow. Один список на все
// SELECT репозитория, чтобы новая колонка не терялась в одном из них.
var promoColumns = []string{
	"id", "code", "type", "subscription_days", "trial_days", "extra_hwid_delta",
	"discount_percent", "discount_ttl_hours", "max_uses", "uses_count", "valid_until",
	"active", "first_purchase_only", "require_customer_in_db", "allow_trial_without_payment", "created_at",
	"discount_max_subscription_payments_per_customer", "tariff_id", "note",
}

var promoColumnsSQL = strings.Join(promoColumns, ", ")

// scanPromoRow читает строку, выбранную по promoColumns.
func scanPromoRow(row pgx.Row, p *PromoCode) error {
	var tid sql.NullInt64
	if err := row.Scan(
		&p.ID, &p.Code, &p.Type, &p.SubscriptionDays, &p.TrialDays, &p.ExtraHwidDelta,
		&p.DiscountPercent, &p.DiscountTTLHours, &p.MaxUses, &p.UsesCount, &p.ValidUntil,
		&p.Active, &p.FirstPurchaseOnly, &p.RequireCustomerInDB, &p.AllowTrialWithoutPayment, &p.CreatedAt,
		&p.DiscountMaxSubscriptionPaymentsPerCustomer, &tid, &p.Note,
	); err != nil {
		return err
	}
	if tid.Valid {
		v := tid.Int64
		p.TariffID = &v
	}
	return nil
}

type PromoRedemption struct {
	ID          int64     `db:"id"`
	PromoCodeID int64     `db:"promo_code_id"`
	CustomerID  int64     `db:"customer_id"`
	UsedAt      time.Time `db:"used_at"`
}

type PendingDiscount struct {
	ID                            int64      `db:"id"`
	CustomerID                    int64      `db:"customer_id"`
	PromoCodeID                   int64      `db:"promo_code_id"`
	Percent                       int        `db:"percent"`
	ExpiresAt                     *time.Time `db:"expires_at"`
	UntilFirstPurchase            bool       `db:"until_first_purchase"`
	CreatedAt                     time.Time  `db:"created_at"`
	SubscriptionPaymentsRemaining int        `db:"subscription_payments_remaining"`
}

// openInvoiceWindow — сколько неоплаченный счёт со скидкой держит её за собой.
// Кассы закрывают счёт за минуты–часы, но статус в базе не всегда доходит до
// cancel: счёт Platega месячной давности так и остаётся new и иначе занимал
// бы скидку навсегда. Счета Stars не истекают — их отменяет
// CancelOpenTelegramPurchasesWithPromo.
const openInvoiceWindow = "24 hours"

// CountOpenPurchasesWithPromo — неоплаченные (new/pending) счета клиента со
// скидкой по этому промокоду за последние сутки: каждый из них ещё может быть
// оплачен со скидкой.
func (r *PromoRepository) CountOpenPurchasesWithPromo(ctx context.Context, customerID, promoCodeID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM purchase
		 WHERE customer_id = $1 AND promo_code_id = $2 AND status IN ('new', 'pending')
		   AND created_at > now() - $3::interval`,
		customerID, promoCodeID, openInvoiceWindow).Scan(&n)
	return n, err
}

// CancelOpenTelegramPurchasesWithPromo отменяет неоплаченные счета Stars клиента
// с этим промокодом. Ссылка на счёт Stars не истекает, поэтому такой счёт иначе
// держал бы скидку вечно; оплату отменённого счёта отклоняет pre_checkout.
func (r *PromoRepository) CancelOpenTelegramPurchasesWithPromo(ctx context.Context, customerID, promoCodeID int64) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
		UPDATE purchase SET status = 'cancel'
		 WHERE customer_id = $1 AND promo_code_id = $2 AND invoice_type = 'telegram'
		   AND status IN ('new', 'pending')`,
		customerID, promoCodeID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

type PromoRepository struct {
	pool *pgxpool.Pool
}

func NewPromoRepository(pool *pgxpool.Pool) *PromoRepository {
	return &PromoRepository{pool: pool}
}

func (r *PromoRepository) Create(ctx context.Context, p *PromoCode) (int64, error) {
	builder := sq.Insert("promo_code").
		Columns(
			"code", "type", "subscription_days", "trial_days", "extra_hwid_delta",
			"discount_percent", "discount_ttl_hours", "max_uses", "uses_count", "valid_until",
			"active", "first_purchase_only", "require_customer_in_db", "allow_trial_without_payment",
			"discount_max_subscription_payments_per_customer", "tariff_id", "note",
		).
		Values(
			p.Code, p.Type, p.SubscriptionDays, p.TrialDays, p.ExtraHwidDelta,
			p.DiscountPercent, p.DiscountTTLHours, p.MaxUses, p.UsesCount, p.ValidUntil,
			p.Active, p.FirstPurchaseOnly, p.RequireCustomerInDB, p.AllowTrialWithoutPayment,
			p.DiscountMaxSubscriptionPaymentsPerCustomer, p.TariffID, p.Note,
		).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&id)
	return id, err
}

func (r *PromoRepository) FindByID(ctx context.Context, id int64) (*PromoCode, error) {
	return r.scanOne(ctx, sq.Select(promoColumns...).From("promo_code").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar))
}

func (r *PromoRepository) FindByCode(ctx context.Context, codeUpper string) (*PromoCode, error) {
	return r.scanOne(ctx, sq.Select(promoColumns...).From("promo_code").Where(sq.Eq{"code": codeUpper}).PlaceholderFormat(sq.Dollar))
}

func (r *PromoRepository) scanOne(ctx context.Context, builder sq.SelectBuilder) (*PromoCode, error) {
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}
	var p PromoCode
	if err := scanPromoRow(r.pool.QueryRow(ctx, sqlStr, args...), &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// List — нетехнические промокоды, новые сверху, без фильтров.
func (r *PromoRepository) List(ctx context.Context, offset, limit int) ([]PromoCode, int, error) {
	list, counts, err := r.ListFiltered(ctx, PromoListFilter{}, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return list, counts.Status.All, nil
}

// Статусы промокода для фильтра списка админки. Порядок проверки — как на
// клиенте (promoStatus.ts): истёк → выключен → исчерпан → активен; каждая
// строка попадает ровно в один из четырёх. «Есть лимит» — max_uses > 0: ноль
// promo/service.go считает безлимитом. Время — now() базы.
const (
	PromoStatusAll    = "all"
	PromoStatusActive = "active"
	PromoStatusEnded  = "ended" // истёк или исчерпан
	PromoStatusOff    = "off"   // выключен и не истёк

	promoExpiredCond   = `(valid_until IS NOT NULL AND valid_until < now())`
	promoExhaustedCond = `(max_uses IS NOT NULL AND max_uses > 0 AND uses_count >= max_uses)`

	promoStatusActiveCond = `(active AND NOT ` + promoExpiredCond + ` AND NOT ` + promoExhaustedCond + `)`
	promoStatusOffCond    = `(NOT ` + promoExpiredCond + ` AND NOT active)`
	promoStatusEndedCond  = `(` + promoExpiredCond + ` OR (active AND ` + promoExhaustedCond + `))`
)

// Сортировки списка промокодов в админке.
const (
	PromoSortNew      = "new"
	PromoSortUses     = "uses"
	PromoSortExpiring = "expiring"
	PromoSortCode     = "code"
)

// Направление сортировки; пусто — естественное для ключа (код — по алфавиту,
// новые и активации — по убыванию, срок — ближайшие первыми).
const (
	PromoSortAsc  = "asc"
	PromoSortDesc = "desc"
)

// PromoListFilter — фильтры списка промокодов. Пустые поля — без фильтра;
// значения проверяет handler, сюда неизвестное не доходит.
type PromoListFilter struct {
	Status string // all | active | ended | off
	Type   string // subscription_days | trial | extra_hwid | discount
	Query  string // подстрока кода или заметки, без учёта регистра
	Sort   string // new | uses | expiring | code
	Dir    string // asc | desc | "" — см. promoSortOrder
}

// PromoStatusCounts — сколько промокодов в каждой вкладке статуса.
type PromoStatusCounts struct {
	All, Active, Ended, Off int
}

// PromoTypeCounts — сколько промокодов каждого типа.
type PromoTypeCounts struct {
	All, SubscriptionDays, Discount, Trial, ExtraHwid int
}

// PromoListCounts — счётчики вкладок и фильтра типа. Status считается с учётом
// типа и поиска, но без статуса; Type — с учётом статуса и поиска, но без
// типа: число на вкладке совпадает с тем, что покажет клик по ней.
type PromoListCounts struct {
	Status PromoStatusCounts
	Type   PromoTypeCounts
}

func promoStatusCond(status string) string {
	switch status {
	case PromoStatusActive:
		return promoStatusActiveCond
	case PromoStatusEnded:
		return promoStatusEndedCond
	case PromoStatusOff:
		return promoStatusOffCond
	default:
		return "TRUE"
	}
}

// promoSortOrder — ORDER BY списка. Значения только из констант выше, ввод
// пользователя сюда не попадает (handler отклоняет неизвестное).
func promoSortOrder(sort, dir string) string {
	switch sort {
	case PromoSortUses:
		if dir == PromoSortAsc {
			return "uses_count ASC, id DESC"
		}
		return "uses_count DESC, id DESC"
	case PromoSortExpiring:
		if dir == PromoSortDesc {
			// Самые дальние сроки первыми, бессрочные — в конце.
			return "CASE WHEN valid_until IS NULL THEN 1 ELSE 0 END, valid_until DESC, id DESC"
		}
		return "CASE WHEN valid_until > now() THEN 0 WHEN valid_until IS NULL THEN 1 ELSE 2 END, valid_until ASC, id DESC"
	case PromoSortCode:
		if dir == PromoSortDesc {
			return "code DESC, id DESC"
		}
		return "code ASC, id DESC"
	default:
		if dir == PromoSortAsc {
			return "id ASC"
		}
		return "id DESC"
	}
}

// escapeLikePattern экранирует спецсимволы LIKE (`\`, `%`, `_`) для ESCAPE '\'.
func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// promoListBaseWhere — условия, общие для списка и счётчиков: не служебный код и
// поиск. Возвращает SQL-фрагмент и аргументы, начиная с $1.
func promoListBaseWhere(query string) (string, []any) {
	where := notTechnicalPromo
	var args []any
	if query != "" {
		args = append(args, "%"+escapeLikePattern(query)+"%")
		where += ` AND (code ILIKE $1 ESCAPE '\' OR note ILIKE $1 ESCAPE '\')`
	}
	return where, args
}

// ListFiltered — страница промокодов админки с фильтрами и счётчики вкладок.
// Служебные коды (`__…`) не попадают ни в список, ни в счётчики. Общее число
// строк под фильтрами — counts.Status по выбранному статусу.
func (r *PromoRepository) ListFiltered(ctx context.Context, f PromoListFilter, offset, limit int) ([]PromoCode, PromoListCounts, error) {
	var counts PromoListCounts
	if offset < 0 {
		offset = 0
	}
	baseWhere, args := promoListBaseWhere(f.Query)

	typeCond := "TRUE"
	if f.Type != "" {
		args = append(args, f.Type)
		typeCond = fmt.Sprintf("type = $%d", len(args))
	}
	statusCond := promoStatusCond(f.Status)

	countsSQL := `
SELECT
  COUNT(*) FILTER (WHERE ` + typeCond + `)::int,
  COUNT(*) FILTER (WHERE ` + typeCond + ` AND ` + promoStatusActiveCond + `)::int,
  COUNT(*) FILTER (WHERE ` + typeCond + ` AND ` + promoStatusEndedCond + `)::int,
  COUNT(*) FILTER (WHERE ` + typeCond + ` AND ` + promoStatusOffCond + `)::int,
  COUNT(*) FILTER (WHERE ` + statusCond + `)::int,
  COUNT(*) FILTER (WHERE ` + statusCond + ` AND type = 'subscription_days')::int,
  COUNT(*) FILTER (WHERE ` + statusCond + ` AND type = 'discount')::int,
  COUNT(*) FILTER (WHERE ` + statusCond + ` AND type = 'trial')::int,
  COUNT(*) FILTER (WHERE ` + statusCond + ` AND type = 'extra_hwid')::int
FROM promo_code
WHERE ` + baseWhere
	if err := r.pool.QueryRow(ctx, countsSQL, args...).Scan(
		&counts.Status.All, &counts.Status.Active, &counts.Status.Ended, &counts.Status.Off,
		&counts.Type.All, &counts.Type.SubscriptionDays, &counts.Type.Discount, &counts.Type.Trial, &counts.Type.ExtraHwid,
	); err != nil {
		return nil, counts, fmt.Errorf("promo list counts: %w", err)
	}

	args = append(args, limit, offset)
	listSQL := `SELECT ` + promoColumnsSQL + `
FROM promo_code
WHERE ` + baseWhere + ` AND ` + typeCond + ` AND ` + statusCond + `
ORDER BY ` + promoSortOrder(f.Sort, f.Dir) + fmt.Sprintf(`
LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, counts, fmt.Errorf("promo list: %w", err)
	}
	defer rows.Close()
	var list []PromoCode
	for rows.Next() {
		var p PromoCode
		if err := scanPromoRow(rows, &p); err != nil {
			return nil, counts, err
		}
		list = append(list, p)
	}
	return list, counts, rows.Err()
}

// PromoRedemptionsSummary — активации нетехнических промокодов: за скользящие
// 24 часа и за всё время.
type PromoRedemptionsSummary struct {
	Last24h int
	Total   int
}

// RedemptionsSummary — сводка активаций для шапки списка. «За 24 ч» —
// скользящее окно от now() базы, не «сегодня»: так число не зависит от пояса.
func (r *PromoRepository) RedemptionsSummary(ctx context.Context) (PromoRedemptionsSummary, error) {
	var s PromoRedemptionsSummary
	err := r.pool.QueryRow(ctx, `
SELECT COUNT(*) FILTER (WHERE pr.used_at >= now() - interval '24 hours')::int, COUNT(*)::int
FROM promo_redemption pr
JOIN promo_code pc ON pc.id = pr.promo_code_id
WHERE pc.code NOT LIKE '\_\_%'`).Scan(&s.Last24h, &s.Total)
	return s, err
}

// PromoPayback — оплаты со скидкой промокода: покупки с purchase.promo_code_id
// этого кода (столько, сколько оплат даёт скидка). Есть только у скидки: у дней,
// триала и доп. устройства кода в покупке нет, а «все оплаты после ввода» —
// не заслуга кода (решение владельца 16.11). Рубли и Stars — по правилу
// SumPaidSpendBreakdown, прочие валюты в выручку не входят.
type PromoPayback struct {
	PaidCustomers int
	Payments      int
	RevenueRub    float64
	RevenueStars  float64
}

// PromoDetailStats — цифры карточки промокода в админке.
type PromoDetailStats struct {
	Redemptions24h   int
	PendingDiscounts int
	Payback          PromoPayback
}

// promoPaymentMatchCond — оплаченная покупка p засчитывается промокоду pc: только
// у скидки и только покупка с этой скидкой. У остальных типов оплат нет.
const promoPaymentMatchCond = `p.status = 'paid' AND pc.type = 'discount' AND p.promo_code_id = pc.id`

// DetailStats — активации за 24 ч, невыкупленные скидки и (у скидки) отдача.
func (r *PromoRepository) DetailStats(ctx context.Context, promo *PromoCode) (PromoDetailStats, error) {
	var s PromoDetailStats
	if err := r.pool.QueryRow(ctx, `
SELECT
  (SELECT COUNT(*)::int FROM promo_redemption WHERE promo_code_id = $1 AND used_at >= now() - interval '24 hours'),
  (SELECT COUNT(*)::int FROM customer_pending_discount WHERE promo_code_id = $1)`,
		promo.ID).Scan(&s.Redemptions24h, &s.PendingDiscounts); err != nil {
		return s, fmt.Errorf("promo detail counters: %w", err)
	}

	if promo.Type != PromoTypeDiscount {
		return s, nil
	}
	sums := `
  COUNT(DISTINCT p.customer_id)::int,
  COUNT(*)::int,
  COALESCE(SUM(p.amount) FILTER (WHERE ` + purchaseRubCond + `), 0)::float8,
  COALESCE(SUM(p.amount) FILTER (WHERE ` + purchaseStarsCond + `), 0)::float8`
	// Точная связь: скидка записана в покупку. Активация для этого не нужна.
	q := `SELECT` + sums + `
FROM purchase p
WHERE p.status = 'paid' AND p.promo_code_id = $1`
	pb := &s.Payback
	if err := r.pool.QueryRow(ctx, q, promo.ID).Scan(&pb.PaidCustomers, &pb.Payments, &pb.RevenueRub, &pb.RevenueStars); err != nil {
		return s, fmt.Errorf("promo payback: %w", err)
	}
	return s, nil
}

func (r *PromoRepository) CountTotals(ctx context.Context) (total, active, inactive int, err error) {
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*)::int, COUNT(*) FILTER (WHERE active)::int, COUNT(*) FILTER (WHERE NOT active)::int FROM promo_code WHERE `+notTechnicalPromo).Scan(&total, &active, &inactive)
	return
}

// PromoAdminStatsTopRow — промокод в топе по активациям.
type PromoAdminStatsTopRow struct {
	ID          int64
	Code        string
	Active      bool
	UsesCount   int
	Redemptions int
	// Что код даёт. Без этого в статистике стоит голый код, и понять, за что
	// его гасят — за дни, скидку или устройства, — нельзя.
	Type             string
	SubscriptionDays *int
	TrialDays        *int
	ExtraHwidDelta   *int
	DiscountPercent  *int
}

// PromoAdminStatsSnapshot — агрегат для админ-статистики промокодов.
type PromoAdminStatsSnapshot struct {
	Total            int
	Active           int
	Inactive         int
	TotalRedemptions int
	RedemptionsToday int
	TopByRedemptions []PromoAdminStatsTopRow
}

// AdminStatsSnapshot — сводка по промокодам для кабинета.
func (r *PromoRepository) AdminStatsSnapshot(ctx context.Context) (PromoAdminStatsSnapshot, error) {
	var snap PromoAdminStatsSnapshot
	total, active, inactive, err := r.CountTotals(ctx)
	if err != nil {
		return snap, err
	}
	snap.Total = total
	snap.Active = active
	snap.Inactive = inactive

	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM promo_redemption`).Scan(&snap.TotalRedemptions); err != nil {
		return snap, err
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM promo_redemption WHERE used_at >= $1`,
		startOfDay,
	).Scan(&snap.RedemptionsToday); err != nil {
		return snap, err
	}

	rows, err := r.pool.Query(ctx, `
SELECT pc.id, pc.code, pc.active, pc.uses_count, COUNT(pr.id)::int AS redemptions,
       pc.type, pc.subscription_days, pc.trial_days, pc.extra_hwid_delta, pc.discount_percent
FROM promo_code pc
LEFT JOIN promo_redemption pr ON pr.promo_code_id = pc.id
WHERE pc.code NOT LIKE '\_\_%'
GROUP BY pc.id
ORDER BY redemptions DESC, pc.code ASC
LIMIT 12`)
	if err != nil {
		return snap, err
	}
	defer rows.Close()

	for rows.Next() {
		var row PromoAdminStatsTopRow
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Active, &row.UsesCount, &row.Redemptions,
			&row.Type, &row.SubscriptionDays, &row.TrialDays, &row.ExtraHwidDelta, &row.DiscountPercent,
		); err != nil {
			return snap, err
		}
		snap.TopByRedemptions = append(snap.TopByRedemptions, row)
	}
	return snap, rows.Err()
}

func (r *PromoRepository) UpdateFields(ctx context.Context, id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	builder := sq.Update("promo_code").SetMap(fields).Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PromoRepository) Delete(ctx context.Context, id int64) error {
	sqlStr, args, err := sq.Delete("promo_code").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PromoRepository) HasRedemption(ctx context.Context, promoID, customerID int64) (bool, error) {
	sqlStr, args, err := sq.Select("1").From("promo_redemption").
		Where(sq.Eq{"promo_code_id": promoID, "customer_id": customerID}).Limit(1).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return false, err
	}
	var one int
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *PromoRepository) CountRedemptions(ctx context.Context, promoID int64) (int, error) {
	sqlStr, args, err := sq.Select("COUNT(*)").From("promo_redemption").Where(sq.Eq{"promo_code_id": promoID}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, err
	}
	var n int
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&n)
	return n, err
}

func (r *PromoRepository) CountRedemptionsToday(ctx context.Context, promoID int64) (int, error) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	q := `SELECT COUNT(*) FROM promo_redemption WHERE promo_code_id = $1 AND used_at >= $2`
	var n int
	err := r.pool.QueryRow(ctx, q, promoID, start).Scan(&n)
	return n, err
}

type PromoRedemptionRow struct {
	UsedAt     time.Time `db:"used_at"`
	TelegramID int64     `db:"telegram_id"`
}

type PromoRedemptionDetailRow struct {
	UsedAt           time.Time `db:"used_at"`
	CustomerID       int64     `db:"customer_id"`
	TelegramUsername *string   `db:"telegram_username"`
	Nickname         *string   `db:"nickname"`
	// Для аватарки: ссылку подписывает handler, web-only и синтетическим id её не дают.
	TelegramID int64 `db:"telegram_id"`
	IsWebOnly  bool  `db:"is_web_only"`
	// Оплаты этого человека со скидкой промокода (правило PromoPayback);
	// у остальных типов — нули.
	PaidCount int     `db:"paid_count"`
	PaidRub   float64 `db:"paid_rub"`
	PaidStars float64 `db:"paid_stars"`
}

func (r *PromoRepository) ListRecentRedemptions(ctx context.Context, promoID int64, limit int) ([]PromoRedemptionRow, error) {
	q := `
SELECT pr.used_at, c.telegram_id
FROM promo_redemption pr
JOIN customer c ON c.id = pr.customer_id
WHERE pr.promo_code_id = $1
ORDER BY pr.used_at DESC
LIMIT $2`
	rows, err := r.pool.Query(ctx, q, promoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PromoRedemptionRow
	for rows.Next() {
		var row PromoRedemptionRow
		if err := rows.Scan(&row.UsedAt, &row.TelegramID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CustomerRedemptionRow — одна строка «Истории промокодов» клиента: что он ввёл и что получил.
// Тип и значение берутся из самого промокода; удалённый админом код из истории пропадает
// (promo_redemption уходит каскадом) — это решение владельца.
type CustomerRedemptionRow struct {
	UsedAt           time.Time
	Code             string
	Type             string
	SubscriptionDays *int
	TrialDays        *int
	ExtraHwidDelta   *int
	DiscountPercent  *int
}

// ListCustomerRedemptions — введённые клиентом промокоды, свежие сверху. Служебные коды
// (`__…`, например колесо фортуны) клиент не вводил, поэтому в историю не попадают.
func (r *PromoRepository) ListCustomerRedemptions(ctx context.Context, customerID int64, offset, limit int) ([]CustomerRedemptionRow, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
SELECT pr.used_at, pc.code, pc.type, pc.subscription_days, pc.trial_days, pc.extra_hwid_delta, pc.discount_percent
FROM promo_redemption pr
JOIN promo_code pc ON pc.id = pr.promo_code_id
WHERE pr.customer_id = $1 AND pc.code NOT LIKE '\_\_%'
ORDER BY pr.used_at DESC, pr.id DESC
LIMIT $2 OFFSET $3`, customerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomerRedemptionRow
	for rows.Next() {
		var row CustomerRedemptionRow
		if err := rows.Scan(&row.UsedAt, &row.Code, &row.Type, &row.SubscriptionDays, &row.TrialDays,
			&row.ExtraHwidDelta, &row.DiscountPercent); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PromoRepository) ListRedemptionsPage(ctx context.Context, promoID int64, offset, limit int) ([]PromoRedemptionDetailRow, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	q := `
SELECT pr.used_at, c.id, c.telegram_username, c.telegram_id, c.is_web_only,
  (SELECT NULLIF(TRIM(ii.raw_profile_json->>'first_name'), '')
   FROM cabinet_account_customer_link l
   JOIN cabinet_identity ii ON ii.account_id = l.account_id AND ii.unlinked_at IS NULL
   WHERE l.customer_id = c.id AND l.link_status = 'linked'
   ORDER BY ii.created_at DESC
   LIMIT 1) AS nickname,
  pay.paid_count, pay.paid_rub, pay.paid_stars
FROM promo_redemption pr
JOIN promo_code pc ON pc.id = pr.promo_code_id
JOIN customer c ON c.id = pr.customer_id
LEFT JOIN LATERAL (
  SELECT COUNT(*)::int AS paid_count,
    COALESCE(SUM(p.amount) FILTER (WHERE ` + purchaseRubCond + `), 0)::float8 AS paid_rub,
    COALESCE(SUM(p.amount) FILTER (WHERE ` + purchaseStarsCond + `), 0)::float8 AS paid_stars
  FROM purchase p
  WHERE p.customer_id = pr.customer_id AND ` + promoPaymentMatchCond + `
) pay ON TRUE
WHERE pr.promo_code_id = $1
ORDER BY pr.used_at DESC, pr.id DESC
LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, promoID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PromoRedemptionDetailRow
	for rows.Next() {
		var row PromoRedemptionDetailRow
		if err := rows.Scan(&row.UsedAt, &row.CustomerID, &row.TelegramUsername, &row.TelegramID, &row.IsWebOnly, &row.Nickname,
			&row.PaidCount, &row.PaidRub, &row.PaidStars); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// FindByCodeForUpdate locks promo row by normalized code.
func (r *PromoRepository) FindByCodeForUpdate(ctx context.Context, tx pgx.Tx, codeUpper string) (*PromoCode, error) {
	q := `SELECT ` + promoColumnsSQL + ` FROM promo_code WHERE code = $1 FOR UPDATE`
	var p PromoCode
	if err := scanPromoRow(tx.QueryRow(ctx, q, codeUpper), &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// FindByIDForUpdate loads promo row inside transaction with FOR UPDATE lock.
func (r *PromoRepository) FindByIDForUpdate(ctx context.Context, tx pgx.Tx, id int64) (*PromoCode, error) {
	q := `SELECT ` + promoColumnsSQL + ` FROM promo_code WHERE id = $1 FOR UPDATE`
	var p PromoCode
	if err := scanPromoRow(tx.QueryRow(ctx, q, id), &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *PromoRepository) InsertRedemption(ctx context.Context, tx pgx.Tx, promoID, customerID int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO promo_redemption (promo_code_id, customer_id) VALUES ($1, $2)`, promoID, customerID)
	return err
}

func (r *PromoRepository) IncrementUses(ctx context.Context, tx pgx.Tx, promoID int64) error {
	_, err := tx.Exec(ctx, `UPDATE promo_code SET uses_count = uses_count + 1 WHERE id = $1`, promoID)
	return err
}

func (r *PromoRepository) DeleteRedemption(ctx context.Context, tx pgx.Tx, promoID, customerID int64) error {
	_, err := tx.Exec(ctx, `DELETE FROM promo_redemption WHERE promo_code_id = $1 AND customer_id = $2`, promoID, customerID)
	return err
}

func (r *PromoRepository) DecrementUses(ctx context.Context, tx pgx.Tx, promoID int64) error {
	_, err := tx.Exec(ctx, `UPDATE promo_code SET uses_count = GREATEST(uses_count - 1, 0) WHERE id = $1`, promoID)
	return err
}

func (r *PromoRepository) GetPendingDiscountByCustomerID(ctx context.Context, customerID int64) (*PendingDiscount, error) {
	q := `SELECT id, customer_id, promo_code_id, percent, expires_at, until_first_purchase, created_at, subscription_payments_remaining
		FROM customer_pending_discount WHERE customer_id = $1`
	row := r.pool.QueryRow(ctx, q, customerID)
	var d PendingDiscount
	err := row.Scan(&d.ID, &d.CustomerID, &d.PromoCodeID, &d.Percent, &d.ExpiresAt, &d.UntilFirstPurchase, &d.CreatedAt, &d.SubscriptionPaymentsRemaining)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *PromoRepository) UpsertPendingDiscount(ctx context.Context, tx pgx.Tx, customerID, promoCodeID int64, percent int, expiresAt *time.Time, untilFirst bool, subscriptionPaymentsRemaining int) error {
	_, err := tx.Exec(ctx, `
INSERT INTO customer_pending_discount (customer_id, promo_code_id, percent, expires_at, until_first_purchase, subscription_payments_remaining)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (customer_id) DO UPDATE SET
  promo_code_id = EXCLUDED.promo_code_id,
  percent = EXCLUDED.percent,
  expires_at = EXCLUDED.expires_at,
  until_first_purchase = EXCLUDED.until_first_purchase,
  subscription_payments_remaining = EXCLUDED.subscription_payments_remaining,
  created_at = CURRENT_TIMESTAMP
`, customerID, promoCodeID, percent, expiresAt, untilFirst, subscriptionPaymentsRemaining)
	return err
}

// GetPendingDiscountByCustomerIDForUpdate блокирует строку pending discount в транзакции.
func (r *PromoRepository) GetPendingDiscountByCustomerIDForUpdate(ctx context.Context, tx pgx.Tx, customerID int64) (*PendingDiscount, error) {
	q := `SELECT id, customer_id, promo_code_id, percent, expires_at, until_first_purchase, created_at, subscription_payments_remaining
		FROM customer_pending_discount WHERE customer_id = $1 FOR UPDATE`
	row := tx.QueryRow(ctx, q, customerID)
	var d PendingDiscount
	err := row.Scan(&d.ID, &d.CustomerID, &d.PromoCodeID, &d.Percent, &d.ExpiresAt, &d.UntilFirstPurchase, &d.CreatedAt, &d.SubscriptionPaymentsRemaining)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// UpdatePendingDiscountRemainingTx обновляет счётчик оставшихся оплат подписки со скидкой.
func (r *PromoRepository) UpdatePendingDiscountRemainingTx(ctx context.Context, tx pgx.Tx, customerID int64, remaining int) error {
	_, err := tx.Exec(ctx, `UPDATE customer_pending_discount SET subscription_payments_remaining = $2 WHERE customer_id = $1`, customerID, remaining)
	return err
}

func (r *PromoRepository) DeletePendingDiscount(ctx context.Context, customerID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM customer_pending_discount WHERE customer_id = $1`, customerID)
	return err
}

func (r *PromoRepository) DeletePendingDiscountTx(ctx context.Context, tx pgx.Tx, customerID int64) error {
	_, err := tx.Exec(ctx, `DELETE FROM customer_pending_discount WHERE customer_id = $1`, customerID)
	return err
}

// Pool exposes underlying pool for transactions from service layer.
func (r *PromoRepository) Pool() *pgxpool.Pool {
	return r.pool
}

// ErrPromoValidation is returned when activation fails (maps to neutral user message).
var ErrPromoValidation = errors.New("promo validation failed")

func ValidationErrorf(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrPromoValidation, fmt.Sprintf(format, args...))
}
