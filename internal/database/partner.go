package database

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Статусы и виды партнёрской программы. Совпадают с CHECK-ограничениями
// миграции 000044.
const (
	PartnerStatusPending   = "pending"
	PartnerStatusActive    = "active"
	PartnerStatusSuspended = "suspended"
	PartnerStatusRejected  = "rejected"

	PartnerEarningKindFirst      = "first"
	PartnerEarningKindRenewal    = "renewal"
	PartnerEarningKindAdjustment = "adjustment"

	PartnerEarningHold      = "hold"
	PartnerEarningAvailable = "available"
	PartnerEarningCancelled = "cancelled"

	PartnerAttributionSourceTelegram = "tg_start"
	PartnerAttributionSourceWeb      = "web"
	PartnerAttributionSourceAdmin    = "admin"
)

// partnerCodeAlphabet — без 0/O/1/I/l: код диктуют голосом и переписывают из
// сообщений, похожие символы превращаются в потерянные переходы.
const partnerCodeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

const partnerCodeLength = 6

type Partner struct {
	ID              int64
	CustomerID      int64
	Status          string
	FirstPercent    *float64
	RenewalPercent  *float64
	LinksLimit      *int
	Balance         float64
	HoldBalance     float64
	ReservedBalance float64
	TotalEarned     float64
	TotalPaid       float64
	PayoutMethod    *string
	PayoutDetails   *string
	AppAbout        *string
	AppChannels     *string
	AppExpected     *string
	AppSubmittedAt  *time.Time
	AdminNote       *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ApprovedAt      *time.Time
	ApprovedBy      *int64
}

// IsActive — партнёр получает начисления. Замороженный (suspended) продолжает
// зарабатывать: блокируется только вывод, потому что заморозка — это пауза на
// разбор, а не наказание задним числом.
func (p *Partner) IsActive() bool {
	return p != nil && (p.Status == PartnerStatusActive || p.Status == PartnerStatusSuspended)
}

// CanWithdraw — вывод разрешён только полностью активному партнёру.
func (p *Partner) CanWithdraw() bool {
	return p != nil && p.Status == PartnerStatusActive
}

type PartnerLink struct {
	ID         int64
	PartnerID  int64
	Code       string
	Name       string
	IsDefault  bool
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

type PartnerAttribution struct {
	CustomerID int64
	PartnerID  int64
	LinkID     *int64
	Source     string
	AttachedAt time.Time
}

type PartnerEarning struct {
	ID            int64
	PartnerID     int64
	CustomerID    *int64
	PurchaseID    *int64
	LinkID        *int64
	BaseAmount    float64
	BaseCurrency  string
	BaseAmountRub float64
	Percent       float64
	Amount        float64
	Kind          string
	Status        string
	HoldUntil     *time.Time
	Note          *string
	CreatedAt     time.Time
	ReleasedAt    *time.Time
}

// PartnerLinkResolution — результат разбора кода из ссылки: сам поток плюс
// состояние его партнёра. Одним запросом, потому что вызывающему всегда нужно
// и то и другое.
type PartnerLinkResolution struct {
	Link          PartnerLink
	PartnerID     int64
	PartnerStatus string
}

// HoldRelease — итог раскрытия холда по одному партнёру, для уведомления.
type HoldRelease struct {
	PartnerID int64
	Amount    float64
	Count     int
}

type PartnerRepository struct {
	pool *pgxpool.Pool
}

func NewPartnerRepository(pool *pgxpool.Pool) *PartnerRepository {
	return &PartnerRepository{pool: pool}
}

const partnerColumns = `id, customer_id, status, first_percent, renewal_percent, links_limit,
	balance, hold_balance, reserved_balance, total_earned, total_paid,
	payout_method, payout_details, app_about, app_channels, app_expected,
	app_submitted_at, admin_note, created_at, updated_at, approved_at, approved_by`

func scanPartner(row pgx.Row) (*Partner, error) {
	var p Partner
	err := row.Scan(
		&p.ID, &p.CustomerID, &p.Status, &p.FirstPercent, &p.RenewalPercent, &p.LinksLimit,
		&p.Balance, &p.HoldBalance, &p.ReservedBalance, &p.TotalEarned, &p.TotalPaid,
		&p.PayoutMethod, &p.PayoutDetails, &p.AppAbout, &p.AppChannels, &p.AppExpected,
		&p.AppSubmittedAt, &p.AdminNote, &p.CreatedAt, &p.UpdatedAt, &p.ApprovedAt, &p.ApprovedBy,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByCustomerID возвращает партнёра клиента. nil без ошибки, если клиент не
// партнёр — это обычное состояние, а не сбой.
func (r *PartnerRepository) FindByCustomerID(ctx context.Context, customerID int64) (*Partner, error) {
	p, err := scanPartner(r.pool.QueryRow(ctx,
		`SELECT `+partnerColumns+` FROM partner WHERE customer_id = $1`, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find partner by customer: %w", err)
	}
	return p, nil
}

func (r *PartnerRepository) FindByID(ctx context.Context, id int64) (*Partner, error) {
	p, err := scanPartner(r.pool.QueryRow(ctx,
		`SELECT `+partnerColumns+` FROM partner WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find partner by id: %w", err)
	}
	return p, nil
}

// Create заводит партнёра вместе с основной ссылкой: партнёр без ссылки
// бесполезен, а две отдельные операции оставили бы его без неё при сбое между
// ними. status задаёт вызывающий: заявка создаётся как pending, ручное
// назначение админом — сразу active.
func (r *PartnerRepository) Create(ctx context.Context, p *Partner, defaultLinkName string) (*Partner, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create partner tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanPartner(tx.QueryRow(ctx,
		`INSERT INTO partner (customer_id, status, first_percent, renewal_percent, links_limit,
			payout_method, payout_details, app_about, app_channels, app_expected, app_submitted_at,
			admin_note, approved_at, approved_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING `+partnerColumns,
		p.CustomerID, p.Status, p.FirstPercent, p.RenewalPercent, p.LinksLimit,
		p.PayoutMethod, p.PayoutDetails, p.AppAbout, p.AppChannels, p.AppExpected, p.AppSubmittedAt,
		p.AdminNote, p.ApprovedAt, p.ApprovedBy,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to insert partner: %w", err)
	}

	if _, err := insertUniqueLink(ctx, tx, created.ID, defaultLinkName, true); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit create partner tx: %w", err)
	}
	return created, nil
}

// Отказы при подаче заявки. Оба — нормальные состояния, о которых партнёру
// надо сказать словами, а не показать ошибку сервера.
var (
	ErrPartnerApplicationPending = errors.New("partner application is already pending")
	ErrPartnerAlreadyActive      = errors.New("customer is already a partner")
)

// SubmitApplication подаёт заявку на партнёрство или переподаёт её после отказа.
//
// Отдельной таблицы заявок нет намеренно: истории подач не существует, важен
// только текущий статус и последняя версия анкеты. Повторная подача после
// отказа переписывает поля той же строки.
//
// autoApprove включает партнёра сразу — режим «программа открыта всем».
func (r *PartnerRepository) SubmitApplication(ctx context.Context, customerID int64, about, channels, expected string, autoApprove bool) (*Partner, error) {
	status := PartnerStatusPending
	var approvedAt *time.Time
	if autoApprove {
		status = PartnerStatusActive
		now := time.Now().UTC()
		approvedAt = &now
	}

	existing, err := r.FindByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		now := time.Now().UTC()
		return r.Create(ctx, &Partner{
			CustomerID:     customerID,
			Status:         status,
			AppAbout:       nullableText(about),
			AppChannels:    nullableText(channels),
			AppExpected:    nullableText(expected),
			AppSubmittedAt: &now,
			ApprovedAt:     approvedAt,
		}, "")
	}

	switch existing.Status {
	case PartnerStatusActive, PartnerStatusSuspended:
		return nil, ErrPartnerAlreadyActive
	case PartnerStatusPending:
		return nil, ErrPartnerApplicationPending
	}

	// Отказ — единственное состояние, из которого можно подать заново.
	// Прошлый комментарий админа стирается: он относился к отклонённой анкете.
	updated, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET status = $2, app_about = $3, app_channels = $4, app_expected = $5,
		        app_submitted_at = now(), admin_note = NULL, approved_at = $6,
		        updated_at = now()
		  WHERE id = $1
		 RETURNING `+partnerColumns,
		existing.ID, status, nullableText(about), nullableText(channels), nullableText(expected), approvedAt))
	if err != nil {
		return nil, fmt.Errorf("failed to resubmit partner application: %w", err)
	}
	return updated, nil
}

func nullableText(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// CreateLink добавляет партнёру поток. Проверку лимита делает вызывающий: она
// зависит от настроек, а не от схемы.
func (r *PartnerRepository) CreateLink(ctx context.Context, partnerID int64, name string) (*PartnerLink, error) {
	return insertUniqueLink(ctx, r.pool, partnerID, name, false)
}

// linkExecutor — общий знаменатель pgxpool.Pool и pgx.Tx: вставка ссылки нужна
// и внутри транзакции создания партнёра, и отдельно.
type linkExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// insertUniqueLink подбирает свободный код. Коллизия шестизначного кода
// маловероятна, но при росте базы неизбежна, поэтому несколько попыток вместо
// надежды: ON CONFLICT DO NOTHING не вернёт строку, и это сигнал взять другой код.
func insertUniqueLink(ctx context.Context, ex linkExecutor, partnerID int64, name string, isDefault bool) (*PartnerLink, error) {
	if name == "" {
		name = "Основная ссылка"
	}
	for attempt := 0; attempt < 8; attempt++ {
		code, err := generatePartnerCode()
		if err != nil {
			return nil, err
		}
		var l PartnerLink
		err = ex.QueryRow(ctx,
			`INSERT INTO partner_link (partner_id, code, name, is_default)
			 VALUES ($1,$2,$3,$4)
			 ON CONFLICT (code) DO NOTHING
			 RETURNING id, partner_id, code, name, is_default, archived_at, created_at`,
			partnerID, code, name, isDefault,
		).Scan(&l.ID, &l.PartnerID, &l.Code, &l.Name, &l.IsDefault, &l.ArchivedAt, &l.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // код занят — берём следующий
		}
		if err != nil {
			return nil, fmt.Errorf("failed to insert partner link: %w", err)
		}
		return &l, nil
	}
	return nil, errors.New("failed to allocate unique partner link code")
}

func generatePartnerCode() (string, error) {
	out := make([]byte, partnerCodeLength)
	max := big.NewInt(int64(len(partnerCodeAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("failed to generate partner code: %w", err)
		}
		out[i] = partnerCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

// ResolveLinkCode ищет рабочую ссылку по коду. Архивные потоки и партнёры вне
// работы не резолвятся: ссылка перестала действовать — переход считается
// обычным заходом без источника.
func (r *PartnerRepository) ResolveLinkCode(ctx context.Context, code string) (*PartnerLinkResolution, error) {
	if code == "" {
		return nil, nil
	}
	var res PartnerLinkResolution
	err := r.pool.QueryRow(ctx,
		`SELECT l.id, l.partner_id, l.code, l.name, l.is_default, l.archived_at, l.created_at,
		        p.id, p.status
		   FROM partner_link l
		   JOIN partner p ON p.id = l.partner_id
		  WHERE l.code = $1 AND l.archived_at IS NULL AND p.status IN ('active', 'suspended')`,
		code,
	).Scan(&res.Link.ID, &res.Link.PartnerID, &res.Link.Code, &res.Link.Name,
		&res.Link.IsDefault, &res.Link.ArchivedAt, &res.Link.CreatedAt,
		&res.PartnerID, &res.PartnerStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve partner link code: %w", err)
	}
	return &res, nil
}

// FindDefaultLink возвращает основную ссылку партнёра.
func (r *PartnerRepository) FindDefaultLink(ctx context.Context, partnerID int64) (*PartnerLink, error) {
	var l PartnerLink
	err := r.pool.QueryRow(ctx,
		`SELECT id, partner_id, code, name, is_default, archived_at, created_at
		   FROM partner_link WHERE partner_id = $1 AND is_default`, partnerID,
	).Scan(&l.ID, &l.PartnerID, &l.Code, &l.Name, &l.IsDefault, &l.ArchivedAt, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find default partner link: %w", err)
	}
	return &l, nil
}

// AttributionByCustomer — за каким партнёром закреплён клиент.
func (r *PartnerRepository) AttributionByCustomer(ctx context.Context, customerID int64) (*PartnerAttribution, error) {
	var a PartnerAttribution
	err := r.pool.QueryRow(ctx,
		`SELECT customer_id, partner_id, link_id, source, attached_at
		   FROM partner_attribution WHERE customer_id = $1`, customerID,
	).Scan(&a.CustomerID, &a.PartnerID, &a.LinkID, &a.Source, &a.AttachedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find partner attribution: %w", err)
	}
	return &a, nil
}

// AttachAttribution закрепляет клиента за партнёром.
//
// Три правила программы держит сам запрос, а не вызывающий код: они одинаковы
// для бота, кабинета и админки, и разъехались бы, будь они переписаны в каждом
// из трёх мест.
//
//  1. First touch. ON CONFLICT DO NOTHING: закреплённый клиент остаётся за своим
//     партнёром навсегда, повторный переход по чужой ссылке ничего не меняет.
//     Иначе партнёры перекупали бы друг у друга уже приведённую базу.
//  2. Только клиенты без истории оплат. Иначе достаточно разослать ссылку по
//     действующей базе магазина и получать процент с продлений людей, которых
//     никто не приводил.
//  3. Никакого пересечения с реферальной программой. Реферал уже приносит
//     пригласившему дни подписки; закрепи его ещё и за партнёром — и одна
//     оплата оплачивается дважды.
//
// Возвращает true, если закрепление произошло именно сейчас. false без ошибки —
// нормальный исход: клиент занят, уже платил или пришёл по реферальной ссылке.
func (r *PartnerRepository) AttachAttribution(ctx context.Context, customerID, partnerID int64, linkID *int64, source string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO partner_attribution (customer_id, partner_id, link_id, source)
		 SELECT $1, $2, $3, $4
		  WHERE NOT EXISTS (
		            SELECT 1 FROM purchase
		             WHERE customer_id = $1 AND status = $5)
		    AND NOT EXISTS (
		            SELECT 1 FROM referral rf
		              JOIN customer c ON c.telegram_id = rf.referee_id
		             WHERE c.id = $1)
		 ON CONFLICT (customer_id) DO NOTHING`,
		customerID, partnerID, linkID, source, PurchaseStatusPaid)
	if err != nil {
		return false, fmt.Errorf("failed to attach partner attribution: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// InsertEarning записывает начисление и двигает баланс партнёра одной
// транзакцией: журнал и остаток обязаны меняться вместе, иначе сходимость
// баланса становится вопросом веры.
//
// Повтор по той же покупке молча игнорируется — за это отвечает частичный
// уникальный индекс. Возвращает false, если строка уже была: вызывающий по
// этому признаку решает, слать ли уведомление, а падать тут нечему.
func (r *PartnerRepository) InsertEarning(ctx context.Context, e PartnerEarning) (bool, error) {
	if e.Amount <= 0 {
		return false, fmt.Errorf("partner earning: amount must be positive, got %.2f", e.Amount)
	}
	if e.Status != PartnerEarningHold && e.Status != PartnerEarningAvailable {
		return false, fmt.Errorf("partner earning: unexpected status %q", e.Status)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin partner earning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := sq.Insert("partner_earning").
		Columns("partner_id", "customer_id", "purchase_id", "link_id",
			"base_amount", "base_currency", "base_amount_rub", "percent", "amount",
			"kind", "status", "hold_until", "note").
		Values(e.PartnerID, e.CustomerID, e.PurchaseID, e.LinkID,
			e.BaseAmount, e.BaseCurrency, e.BaseAmountRub, e.Percent, e.Amount,
			e.Kind, e.Status, e.HoldUntil, e.Note).
		Suffix("ON CONFLICT DO NOTHING RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build insert partner earning query: %w", err)
	}

	var id int64
	err = tx.QueryRow(ctx, sqlStr, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // уже начислено за эту покупку
	}
	if err != nil {
		return false, fmt.Errorf("failed to insert partner earning: %w", err)
	}

	// Начисление в холде и начисление, доступное сразу (PARTNER_HOLD_DAYS=0),
	// попадают в разные остатки, но одинаково увеличивают заработанное.
	balanceColumn := "hold_balance"
	if e.Status == PartnerEarningAvailable {
		balanceColumn = "balance"
	}
	if _, err := tx.Exec(ctx,
		`UPDATE partner
		    SET `+balanceColumn+` = `+balanceColumn+` + $2,
		        total_earned = total_earned + $2,
		        updated_at = now()
		  WHERE id = $1`, e.PartnerID, e.Amount); err != nil {
		return false, fmt.Errorf("failed to update partner balance: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit partner earning tx: %w", err)
	}
	return true, nil
}

// ReleaseDueHolds переводит отлежавшие начисления в доступные к выводу и
// синхронно двигает остатки. Одним запросом, потому что раскрытие холда — это
// перекладывание денег между двумя колонками одной строки: разорви его на два
// запроса, и падение между ними оставит партнёру деньги в обоих остатках сразу.
//
// GREATEST не даёт уйти в минус: у hold_balance есть CHECK >= 0, и накопленное
// расхождение в копейку роняло бы весь тик крона, а не одну строку.
func (r *PartnerRepository) ReleaseDueHolds(ctx context.Context, now time.Time) ([]HoldRelease, error) {
	rows, err := r.pool.Query(ctx, `
WITH due AS (
    UPDATE partner_earning
       SET status = 'available', released_at = $1
     WHERE status = 'hold' AND hold_until IS NOT NULL AND hold_until <= $1
 RETURNING partner_id, amount
), agg AS (
    SELECT partner_id, SUM(amount) AS total, COUNT(*) AS cnt
      FROM due GROUP BY partner_id
), moved AS (
    UPDATE partner p
       SET balance = p.balance + agg.total,
           hold_balance = GREATEST(p.hold_balance - agg.total, 0),
           updated_at = $1
      FROM agg
     WHERE p.id = agg.partner_id
 RETURNING agg.partner_id, agg.total, agg.cnt
)
SELECT partner_id, total, cnt FROM moved`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to release partner holds: %w", err)
	}
	defer rows.Close()

	var released []HoldRelease
	for rows.Next() {
		var h HoldRelease
		if err := rows.Scan(&h.PartnerID, &h.Amount, &h.Count); err != nil {
			return nil, fmt.Errorf("failed to scan released partner hold: %w", err)
		}
		released = append(released, h)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating released partner holds: %w", rows.Err())
	}
	return released, nil
}

// HasEarningForCustomer сообщает, начислялось ли партнёру хоть что-то за этого
// клиента. По этому признаку оплата считается первой или повторной.
//
// Считать по журналу, а не по числу оплат клиента, надёжнее: закрепление
// возможно только для клиента без оплат, значит первое начисление и есть первая
// оплата, — но при этом ответ не зависит ни от порядка вызовов внутри
// finalizePurchase, ни от того, какие виды покупок сейчас засчитываются.
// Отменённые начисления тоже учитываются: первая оплата состоялась, даже если
// деньги за неё потом сняли.
func (r *PartnerRepository) HasEarningForCustomer(ctx context.Context, partnerID, customerID int64) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM partner_earning
		     WHERE partner_id = $1 AND customer_id = $2 AND kind <> 'adjustment')`,
		partnerID, customerID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check partner earnings for customer: %w", err)
	}
	return exists, nil
}

// CountLinks — сколько рабочих потоков у партнёра. Архивные не считаются: они
// закрыты для новых переходов и лимит занимать не должны.
func (r *PartnerRepository) CountLinks(ctx context.Context, partnerID int64) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM partner_link WHERE partner_id = $1 AND archived_at IS NULL`,
		partnerID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count partner links: %w", err)
	}
	return n, nil
}

// LinkHasHistory сообщает, приходил ли по ссылке хоть кто-то. Пустую ссылку
// можно удалить физически, ссылку с историей — только заархивировать.
func (r *PartnerRepository) LinkHasHistory(ctx context.Context, linkID int64) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
SELECT EXISTS (SELECT 1 FROM partner_attribution WHERE link_id = $1)
    OR EXISTS (SELECT 1 FROM partner_earning WHERE link_id = $1)`, linkID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check partner link history: %w", err)
	}
	return exists, nil
}
