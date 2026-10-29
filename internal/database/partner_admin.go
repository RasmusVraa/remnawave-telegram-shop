package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Отказы админских операций. Каждый — состояние, которое админ должен увидеть
// объяснением, а не пятисоткой.
var (
	ErrPartnerNotFound        = errors.New("partner not found")
	ErrPartnerWrongStatus     = errors.New("partner is in a different status")
	ErrPartnerPayoutNotFound  = errors.New("partner payout not found")
	ErrPartnerPayoutClosed    = errors.New("partner payout is already processed")
	ErrPartnerEarningNotFound = errors.New("partner earning not found")
	// Списание не должно уводить баланс в минус: у колонки CHECK >= 0, и без
	// проверки админ получил бы ошибку базы вместо внятного отказа.
	ErrPartnerBalanceTooLow = errors.New("partner balance is too low for this adjustment")
)

// PartnerAdminRow — строка списка партнёров и заявок в админке.
type PartnerAdminRow struct {
	Partner
	CustomerTelegramID int64
	CustomerUsername   *string
	CustomerEmail      *string
	CustomerIsWebOnly  bool
	// История человека как клиента: по ней видно, живой это аккаунт или
	// пустая регистрация, заведённая ради заявки.
	CustomerCreatedAt time.Time
	CustomerPaidCount int
	CustomerPaidSum   float64
	Customers         int
	PayingCustomers   int
	OpenPayouts       int
}

// PartnerPayoutAdminRow — заявка на вывод вместе с партнёром и его историей.
type PartnerPayoutAdminRow struct {
	PartnerPayout
	CustomerTelegramID int64
	CustomerUsername   *string
	CustomerEmail      *string
	CustomerIsWebOnly  bool
	PartnerTotalEarned float64
	PartnerTotalPaid   float64
	PayoutIndex        int
}

// PartnerOperationRow — строка ленты операций по балансу партнёра. Начисления,
// выплаты и ручные правки в одном списке: иначе сходимость баланса не проверить
// глазами.
type PartnerOperationRow struct {
	At     time.Time
	Kind   string // earning | payout
	Detail string // вид начисления либо статус выплаты
	Amount float64
	Status string
	Ref    *string // номер покупки либо номер перевода
	Note   *string
}

// PartnerPendingWork — счётчик дел для бейджа в меню админки.
type PartnerPendingWork struct {
	Applications int
	Payouts      int
}

const partnerAdminCustomerJoin = `
  JOIN customer c ON c.id = p.customer_id
  LEFT JOIN cabinet_account_customer_link cl ON cl.customer_id = c.id
  LEFT JOIN cabinet_account acc ON acc.id = cl.account_id`

func scanPartnerAdminRow(rows pgx.Rows) (PartnerAdminRow, error) {
	var r PartnerAdminRow
	err := rows.Scan(
		&r.ID, &r.CustomerID, &r.Status, &r.FirstPercent, &r.RenewalPercent, &r.LinksLimit,
		&r.Balance, &r.HoldBalance, &r.ReservedBalance, &r.TotalEarned, &r.TotalPaid,
		&r.PayoutMethod, &r.PayoutDetails, &r.AppAbout, &r.AppChannels, &r.AppExpected,
		&r.AppSubmittedAt, &r.AdminNote, &r.CreatedAt, &r.UpdatedAt, &r.ApprovedAt, &r.ApprovedBy,
		&r.CustomerTelegramID, &r.CustomerUsername, &r.CustomerEmail, &r.CustomerIsWebOnly,
		&r.CustomerCreatedAt, &r.CustomerPaidCount, &r.CustomerPaidSum,
		&r.Customers, &r.PayingCustomers, &r.OpenPayouts,
	)
	return r, err
}

const partnerAdminSelect = `
SELECT p.id, p.customer_id, p.status, p.first_percent, p.renewal_percent, p.links_limit,
       p.balance, p.hold_balance, p.reserved_balance, p.total_earned, p.total_paid,
       p.payout_method, p.payout_details, p.app_about, p.app_channels, p.app_expected,
       p.app_submitted_at, p.admin_note, p.created_at, p.updated_at, p.approved_at, p.approved_by,
       c.telegram_id, c.telegram_username, acc.email, c.is_web_only,
       c.created_at,
       (SELECT COUNT(*) FROM purchase pu WHERE pu.customer_id = c.id AND pu.status = 'paid'),
       (SELECT COALESCE(SUM(pu.amount), 0) FROM purchase pu WHERE pu.customer_id = c.id AND pu.status = 'paid'),
       (SELECT COUNT(*) FROM partner_attribution a WHERE a.partner_id = p.id),
       (SELECT COUNT(DISTINCT a.customer_id) FROM partner_attribution a
          JOIN purchase pu ON pu.customer_id = a.customer_id AND pu.status = 'paid'
         WHERE a.partner_id = p.id),
       (SELECT COUNT(*) FROM partner_payout po
         WHERE po.partner_id = p.id AND po.status IN ('pending', 'approved'))
  FROM partner p` + partnerAdminCustomerJoin

// ListPartnersByStatus отдаёт партнёров, при пустом status — всех.
// Заявки — это тот же список со status='pending'.
// ListPartnersByStatus возвращает партнёров с любым из перечисленных статусов.
//
// Именно список, а не один статус: админу нужно видеть, например, активных
// вместе с отозванными, а фильтровать на клиенте нельзя — выдача постраничная,
// и отбор после выборки терял бы строки со следующих страниц. Пустой список —
// без фильтра, все подряд.
func (r *PartnerRepository) ListPartnersByStatus(ctx context.Context, statuses []string, limit, offset int) ([]PartnerAdminRow, int, error) {
	countQuery := `SELECT COUNT(*) FROM partner`
	args := []any{}
	where := ""
	if len(statuses) > 0 {
		where = " WHERE p.status = ANY($1)"
		countQuery += ` WHERE status = ANY($1)`
		args = append(args, statuses)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count partners: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}

	query := partnerAdminSelect + where +
		fmt.Sprintf(" ORDER BY p.app_submitted_at DESC NULLS LAST, p.created_at DESC LIMIT $%d OFFSET $%d",
			len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list partners: %w", err)
	}
	defer rows.Close()

	var out []PartnerAdminRow
	for rows.Next() {
		row, err := scanPartnerAdminRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan partner row: %w", err)
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, 0, fmt.Errorf("error iterating partners: %w", rows.Err())
	}
	return out, total, nil
}

// GetPartnerAdminRow — карточка одного партнёра.
func (r *PartnerRepository) GetPartnerAdminRow(ctx context.Context, partnerID int64) (*PartnerAdminRow, error) {
	rows, err := r.pool.Query(ctx, partnerAdminSelect+" WHERE p.id = $1", partnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load partner: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	row, err := scanPartnerAdminRow(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan partner: %w", err)
	}
	return &row, nil
}

// PendingWork — сколько дел ждёт админа. Заявки и выплаты считаются вместе:
// бейдж в меню один, и выплаты не должны висеть незамеченными.
func (r *PartnerRepository) PendingWork(ctx context.Context) (PartnerPendingWork, error) {
	var w PartnerPendingWork
	err := r.pool.QueryRow(ctx, `
SELECT (SELECT COUNT(*) FROM partner WHERE status = 'pending'),
       (SELECT COUNT(*) FROM partner_payout WHERE status IN ('pending', 'approved'))`).
		Scan(&w.Applications, &w.Payouts)
	if err != nil {
		return PartnerPendingWork{}, fmt.Errorf("failed to count partner pending work: %w", err)
	}
	return w, nil
}

// ApproveApplication переводит заявку в работу и фиксирует условия.
//
// Проценты задаются здесь же, а не отдельной операцией после одобрения: это
// главный предмет договорённости, и разносить его на два шага значит одобрять
// вслепую.
func (r *PartnerRepository) ApproveApplication(ctx context.Context, partnerID int64, firstPercent, renewalPercent *float64, note string, adminID int64) (*Partner, error) {
	p, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET status = $2, first_percent = $3, renewal_percent = $4,
		        admin_note = $5, approved_at = now(), approved_by = $6, updated_at = now()
		  WHERE id = $1 AND status = $7
		 RETURNING `+partnerColumns,
		partnerID, PartnerStatusActive, firstPercent, renewalPercent,
		nullableText(note), adminID, PartnerStatusPending))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerWrongStatus
	}
	if err != nil {
		return nil, fmt.Errorf("failed to approve partner application: %w", err)
	}
	return p, nil
}

// RejectApplication отклоняет заявку. Комментарий видит сам заявитель, поэтому
// он часть операции, а не внутренняя пометка.
func (r *PartnerRepository) RejectApplication(ctx context.Context, partnerID int64, note string, adminID int64) (*Partner, error) {
	p, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET status = $2, admin_note = $3, approved_by = $4, updated_at = now()
		  WHERE id = $1 AND status = $5
		 RETURNING `+partnerColumns,
		partnerID, PartnerStatusRejected, nullableText(note), adminID, PartnerStatusPending))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerWrongStatus
	}
	if err != nil {
		return nil, fmt.Errorf("failed to reject partner application: %w", err)
	}
	return p, nil
}

// SetPartnerStatus переключает статус действующего партнёра: заморозка
// (suspended), возврат в работу (active) и отзыв партнёрства (rejected).
//
// Заморозка не отменяет начислений — они продолжают идти, блокируется только
// вывод. Это пауза на разбор, а не наказание задним числом.
func (r *PartnerRepository) SetPartnerStatus(ctx context.Context, partnerID int64, status string, note string, adminID int64) (*Partner, error) {
	switch status {
	case PartnerStatusActive, PartnerStatusSuspended, PartnerStatusRejected:
	default:
		return nil, fmt.Errorf("partner status %q is not settable by admin", status)
	}

	p, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET status = $2,
		        admin_note = COALESCE($3, admin_note),
		        approved_by = $4,
		        updated_at = now()
		  WHERE id = $1
		 RETURNING `+partnerColumns,
		partnerID, status, nullableText(note), adminID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to set partner status: %w", err)
	}
	return p, nil
}

// UpdatePartnerTerms меняет индивидуальные условия. NULL в проценте означает
// «вернуть к глобальному», поэтому передаётся именно указатель.
func (r *PartnerRepository) UpdatePartnerTerms(ctx context.Context, partnerID int64, firstPercent, renewalPercent *float64, linksLimit *int, note string) (*Partner, error) {
	p, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET first_percent = $2, renewal_percent = $3, links_limit = $4,
		        admin_note = COALESCE($5, admin_note), updated_at = now()
		  WHERE id = $1
		 RETURNING `+partnerColumns,
		partnerID, firstPercent, renewalPercent, linksLimit, nullableText(note)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update partner terms: %w", err)
	}
	return p, nil
}

// GrantPartner делает клиента партнёром без заявки.
//
// Договорённости чаще случаются в личке, чем через форму, и заставлять
// человека проходить формальную заявку ради галочки — лишний шаг, на котором
// теряются реальные партнёры. Уже существующего партнёра операция возвращает в
// работу, сохранив его историю.
func (r *PartnerRepository) GrantPartner(ctx context.Context, customerID int64, firstPercent, renewalPercent *float64, note string, adminID int64) (*Partner, error) {
	existing, err := r.FindByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		now := time.Now().UTC()
		return r.Create(ctx, &Partner{
			CustomerID:     customerID,
			Status:         PartnerStatusActive,
			FirstPercent:   firstPercent,
			RenewalPercent: renewalPercent,
			AdminNote:      nullableText(note),
			ApprovedAt:     &now,
			ApprovedBy:     &adminID,
		}, "")
	}
	if existing.Status == PartnerStatusActive {
		return existing, nil
	}

	p, err := scanPartner(r.pool.QueryRow(ctx,
		`UPDATE partner
		    SET status = $2, first_percent = $3, renewal_percent = $4,
		        admin_note = COALESCE($5, admin_note),
		        approved_at = now(), approved_by = $6, updated_at = now()
		  WHERE id = $1
		 RETURNING `+partnerColumns,
		existing.ID, PartnerStatusActive, firstPercent, renewalPercent, nullableText(note), adminID))
	if err != nil {
		return nil, fmt.Errorf("failed to grant partner: %w", err)
	}
	return p, nil
}

// AdjustBalance — ручная правка баланса админом: компенсация, возврат, снятие
// ошибочного начисления.
//
// Пишется в тот же журнал, что и автоматические начисления, а не правит баланс
// молча: иначе в ленте операций появляется дыра, и расхождение баланса нечем
// объяснить. Отрицательная сумма — списание; уйти в минус нельзя.
func (r *PartnerRepository) AdjustBalance(ctx context.Context, partnerID int64, amount float64, note string, adminID int64) error {
	if amount == 0 {
		return fmt.Errorf("partner adjustment: amount must not be zero")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin partner adjustment tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE partner
		    SET balance = balance + $2,
		        total_earned = total_earned + $2,
		        updated_at = now()
		  WHERE id = $1 AND balance + $2 >= 0`,
		partnerID, amount)
	if err != nil {
		return fmt.Errorf("failed to adjust partner balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Партнёра нет либо списание больше остатка — для админа это один и тот
		// же «операция не прошла», но различить полезно.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM partner WHERE id = $1)`, partnerID).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check partner existence: %w", err)
		}
		if !exists {
			return ErrPartnerNotFound
		}
		return ErrPartnerBalanceTooLow
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO partner_earning (partner_id, amount, kind, status, note, percent, base_amount, base_amount_rub, base_currency)
		 VALUES ($1, $2, $3, $4, $5, 0, 0, 0, 'RUB')`,
		partnerID, amount, PartnerEarningKindAdjustment, PartnerEarningAvailable,
		nullableText(fmt.Sprintf("%s (админ #%d)", note, adminID))); err != nil {
		return fmt.Errorf("failed to insert partner adjustment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit partner adjustment tx: %w", err)
	}
	return nil
}

// CancelEarning отменяет начисление и снимает деньги оттуда, куда они попали:
// из холда, если начисление ещё лежало, иначе с доступного баланса.
//
// Нужно при возврате платежа: клиент деньги забрал, партнёру платить не за что.
func (r *PartnerRepository) CancelEarning(ctx context.Context, earningID int64, note string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin cancel earning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var partnerID int64
	var amount float64
	var status string
	err = tx.QueryRow(ctx,
		`SELECT partner_id, amount, status FROM partner_earning WHERE id = $1 FOR UPDATE`, earningID).
		Scan(&partnerID, &amount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPartnerEarningNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to load partner earning: %w", err)
	}
	if status == PartnerEarningCancelled {
		return nil // уже отменено — повтор безопасен
	}

	column := "hold_balance"
	if status == PartnerEarningAvailable {
		column = "balance"
	}
	// GREATEST не даёт уйти в минус: деньги могли быть уже выведены, и тогда
	// снимать их с остатка нечем — расхождение честнее оставить в журнале, чем
	// уронить операцию.
	if _, err := tx.Exec(ctx,
		`UPDATE partner
		    SET `+column+` = GREATEST(`+column+` - $2, 0),
		        total_earned = total_earned - $2,
		        updated_at = now()
		  WHERE id = $1`, partnerID, amount); err != nil {
		return fmt.Errorf("failed to revert partner balance: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE partner_earning SET status = $2, note = COALESCE($3, note) WHERE id = $1`,
		earningID, PartnerEarningCancelled, nullableText(note)); err != nil {
		return fmt.Errorf("failed to cancel partner earning: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit cancel earning tx: %w", err)
	}
	return nil
}

// ListPayoutsAdmin — очередь заявок на вывод. При пустом status отдаёт все.
func (r *PartnerRepository) ListPayoutsAdmin(ctx context.Context, status string, limit, offset int) ([]PartnerPayoutAdminRow, int, error) {
	countQuery := `SELECT COUNT(*) FROM partner_payout`
	where := ""
	args := []any{}
	if status == "open" {
		where = ` WHERE po.status IN ('pending', 'approved')`
		countQuery += ` WHERE status IN ('pending', 'approved')`
	} else if status != "" {
		where = ` WHERE po.status = $1`
		countQuery += ` WHERE status = $1`
		args = append(args, status)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count partner payouts: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}

	query := `
SELECT po.id, po.partner_id, po.amount, po.status, po.method, po.details_snapshot,
       po.admin_comment, po.external_ref, po.requested_at, po.processed_at, po.processed_by,
       c.telegram_id, c.telegram_username, acc.email, c.is_web_only,
       p.total_earned, p.total_paid,
       (SELECT COUNT(*) FROM partner_payout prev
         WHERE prev.partner_id = po.partner_id AND prev.requested_at <= po.requested_at)
  FROM partner_payout po
  JOIN partner p ON p.id = po.partner_id
  JOIN customer c ON c.id = p.customer_id
  LEFT JOIN cabinet_account_customer_link cl ON cl.customer_id = c.id
  LEFT JOIN cabinet_account acc ON acc.id = cl.account_id` + where +
		fmt.Sprintf(" ORDER BY po.requested_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list partner payouts: %w", err)
	}
	defer rows.Close()

	var out []PartnerPayoutAdminRow
	for rows.Next() {
		var row PartnerPayoutAdminRow
		if err := rows.Scan(&row.ID, &row.PartnerID, &row.Amount, &row.Status, &row.Method,
			&row.DetailsSnapshot, &row.AdminComment, &row.ExternalRef, &row.RequestedAt,
			&row.ProcessedAt, &row.ProcessedBy,
			&row.CustomerTelegramID, &row.CustomerUsername, &row.CustomerEmail, &row.CustomerIsWebOnly,
			&row.PartnerTotalEarned, &row.PartnerTotalPaid, &row.PayoutIndex); err != nil {
			return nil, 0, fmt.Errorf("failed to scan partner payout row: %w", err)
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, 0, fmt.Errorf("error iterating partner payouts: %w", rows.Err())
	}
	return out, total, nil
}

// ApprovePayout — «принял в работу, перевожу». Деньги не двигаются: они уже
// зарезервированы с момента подачи заявки.
func (r *PartnerRepository) ApprovePayout(ctx context.Context, payoutID int64, comment string, adminID int64) (*PartnerPayout, error) {
	payout, err := scanPartnerPayout(r.pool.QueryRow(ctx,
		`UPDATE partner_payout
		    SET status = $2, admin_comment = COALESCE($3, admin_comment),
		        processed_by = $4
		  WHERE id = $1 AND status = $5
	  RETURNING `+partnerPayoutColumns,
		payoutID, PartnerPayoutApproved, nullableText(comment), adminID, PartnerPayoutPending))
	// Пустой результат здесь означает не «нет такой заявки», а «статус уже не
	// pending»: строка существует, под условие WHERE она просто не попала.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerPayoutClosed
	}
	if err != nil {
		return nil, fmt.Errorf("failed to approve partner payout: %w", err)
	}
	return payout, nil
}

// MarkPayoutPaid закрывает заявку: резерв уходит в выплаченное.
//
// externalRef — номер перевода. Это единственное доказательство в споре «денег
// не приходило», поэтому он часть операции, а не необязательная пометка.
func (r *PartnerRepository) MarkPayoutPaid(ctx context.Context, payoutID int64, externalRef, comment string, adminID int64) (*PartnerPayout, error) {
	return r.finishPayout(ctx, payoutID, PartnerPayoutPaid, externalRef, comment, adminID)
}

// RejectPayout возвращает зарезервированную сумму на доступный баланс.
func (r *PartnerRepository) RejectPayout(ctx context.Context, payoutID int64, comment string, adminID int64) (*PartnerPayout, error) {
	return r.finishPayout(ctx, payoutID, PartnerPayoutRejected, "", comment, adminID)
}

// finishPayout — общий хвост для «выплачено» и «отклонено»: обе операции
// снимают резерв, но по-разному распоряжаются деньгами.
func (r *PartnerRepository) finishPayout(ctx context.Context, payoutID int64, status, externalRef, comment string, adminID int64) (*PartnerPayout, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin finish payout tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var partnerID int64
	var amount float64
	var current string
	err = tx.QueryRow(ctx,
		`SELECT partner_id, amount, status FROM partner_payout WHERE id = $1 FOR UPDATE`, payoutID).
		Scan(&partnerID, &amount, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartnerPayoutNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load partner payout: %w", err)
	}
	if current != PartnerPayoutPending && current != PartnerPayoutApproved {
		return nil, ErrPartnerPayoutClosed
	}

	// Выплачено — резерв превращается в выплаченное; отклонено — возвращается
	// на доступный баланс. GREATEST страхует от рассинхрона резерва.
	moneyMove := `reserved_balance = GREATEST(reserved_balance - $2, 0), total_paid = total_paid + $2`
	if status == PartnerPayoutRejected {
		moneyMove = `reserved_balance = GREATEST(reserved_balance - $2, 0), balance = balance + $2`
	}
	if _, err := tx.Exec(ctx,
		`UPDATE partner SET `+moneyMove+`, updated_at = now() WHERE id = $1`,
		partnerID, amount); err != nil {
		return nil, fmt.Errorf("failed to move partner payout money: %w", err)
	}

	payout, err := scanPartnerPayout(tx.QueryRow(ctx,
		`UPDATE partner_payout
		    SET status = $2, external_ref = COALESCE($3, external_ref),
		        admin_comment = COALESCE($4, admin_comment),
		        processed_at = now(), processed_by = $5
		  WHERE id = $1
	  RETURNING `+partnerPayoutColumns,
		payoutID, status, nullableText(externalRef), nullableText(comment), adminID))
	if err != nil {
		return nil, fmt.Errorf("failed to finish partner payout: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit finish payout tx: %w", err)
	}
	return payout, nil
}

// ListOperations — единая лента движения денег партнёра.
//
// UNION ALL, а не две выборки в Go: только так limit и сортировка по дате
// работают на объединённом списке, а не на каждой половине отдельно.
func (r *PartnerRepository) ListOperations(ctx context.Context, partnerID int64, limit, offset int) ([]PartnerOperationRow, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Всего строк — сумма двух журналов. Отдельным запросом, а не оконной
	// функцией поверх UNION: считать total в той же выборке значило бы
	// протащить через LIMIT весь объединённый набор.
	var total int
	if err := r.pool.QueryRow(ctx, `
SELECT (SELECT count(*) FROM partner_earning WHERE partner_id = $1)
     + (SELECT count(*) FROM partner_payout  WHERE partner_id = $1)`, partnerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count partner operations: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
SELECT at, kind, detail, amount, status, ref, note FROM (
    SELECT e.created_at AS at,
           'earning' AS kind,
           e.kind AS detail,
           e.amount AS amount,
           e.status AS status,
           e.purchase_id::text AS ref,
           e.note AS note
      FROM partner_earning e
     WHERE e.partner_id = $1
    UNION ALL
    SELECT po.requested_at,
           'payout',
           po.method,
           -po.amount,
           po.status,
           po.external_ref,
           po.admin_comment
      FROM partner_payout po
     WHERE po.partner_id = $1
) ops
ORDER BY at DESC
LIMIT $2 OFFSET $3`, partnerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list partner operations: %w", err)
	}
	defer rows.Close()

	var out []PartnerOperationRow
	for rows.Next() {
		var op PartnerOperationRow
		var detail *string
		if err := rows.Scan(&op.At, &op.Kind, &detail, &op.Amount, &op.Status, &op.Ref, &op.Note); err != nil {
			return nil, 0, fmt.Errorf("failed to scan partner operation: %w", err)
		}
		if detail != nil {
			op.Detail = *detail
		}
		out = append(out, op)
	}
	if rows.Err() != nil {
		return nil, 0, fmt.Errorf("error iterating partner operations: %w", rows.Err())
	}
	return out, total, nil
}

// LedgerBalanceCheck — сверка денормализованного баланса с журналом.
//
// Балансы в partner — производные от partner_earning и partner_payout. Пока все
// операции идут транзакциями, они сходятся; расхождение означает баг, и увидеть
// его надо раньше, чем партнёр.
type LedgerBalanceCheck struct {
	PartnerID     int64
	Balance       float64
	HoldBalance   float64
	LedgerHold    float64
	LedgerPayable float64
	Drift         float64
}

func (r *PartnerRepository) CheckBalances(ctx context.Context) ([]LedgerBalanceCheck, error) {
	rows, err := r.pool.Query(ctx, `
SELECT p.id, p.balance, p.hold_balance,
       COALESCE(h.total, 0) AS ledger_hold,
       COALESCE(a.total, 0) - p.total_paid - p.reserved_balance AS ledger_payable
  FROM partner p
  LEFT JOIN (SELECT partner_id, SUM(amount) AS total FROM partner_earning
              WHERE status = 'hold' GROUP BY partner_id) h ON h.partner_id = p.id
  LEFT JOIN (SELECT partner_id, SUM(amount) AS total FROM partner_earning
              WHERE status = 'available' GROUP BY partner_id) a ON a.partner_id = p.id`)
	if err != nil {
		return nil, fmt.Errorf("failed to check partner balances: %w", err)
	}
	defer rows.Close()

	var out []LedgerBalanceCheck
	for rows.Next() {
		var c LedgerBalanceCheck
		if err := rows.Scan(&c.PartnerID, &c.Balance, &c.HoldBalance, &c.LedgerHold, &c.LedgerPayable); err != nil {
			return nil, fmt.Errorf("failed to scan balance check: %w", err)
		}
		c.Drift = (c.Balance - c.LedgerPayable) + (c.HoldBalance - c.LedgerHold)
		out = append(out, c)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating balance checks: %w", rows.Err())
	}
	return out, nil
}
