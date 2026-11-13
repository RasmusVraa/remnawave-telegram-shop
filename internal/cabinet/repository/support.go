package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSupportNotFound = errors.New("support: not found")

const (
	SupportTicketOpen   = "open"
	SupportTicketClosed = "closed"

	SupportMsgIn  = "in"
	SupportMsgOut = "out"

	SupportDeliveryPending = "pending"
	SupportDeliverySent    = "sent"
	SupportDeliveryFailed  = "failed"
)

type SupportTicket struct {
	ID                 int64
	AccountID          int64
	SupportBotTicketID *int64
	Status             string
	CreatedAt          time.Time
	ClosedAt           *time.Time
}

type SupportMessage struct {
	ID                  int64
	TicketID            int64
	Direction           string
	Text                string
	AuthorLabel         string
	SupportBotMessageID *int64
	ClientMessageID     *string // UUID idempotency key для исходящих сообщений
	DeliveryStatus      string
	CreatedAt           time.Time
	ReadAt              *time.Time
}

type SupportRepo struct {
	pool *pgxpool.Pool
}

func NewSupportRepo(pool *pgxpool.Pool) *SupportRepo {
	return &SupportRepo{pool: pool}
}

func (r *SupportRepo) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("support: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *SupportRepo) LockAccount(ctx context.Context, tx pgx.Tx, accountID int64) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, accountID)
	if err != nil {
		return fmt.Errorf("support: advisory lock: %w", err)
	}
	return nil
}

func scanTicket(row pgx.Row) (*SupportTicket, error) {
	var t SupportTicket
	var sbID *int64
	var closedAt *time.Time
	if err := row.Scan(&t.ID, &t.AccountID, &sbID, &t.Status, &t.CreatedAt, &closedAt); err != nil {
		return nil, err
	}
	t.SupportBotTicketID = sbID
	t.ClosedAt = closedAt
	return &t, nil
}

func (r *SupportRepo) GetOpenTicket(ctx context.Context, accountID int64) (*SupportTicket, error) {
	const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket
WHERE account_id = $1 AND status = $2
LIMIT 1`
	t, err := scanTicket(r.pool.QueryRow(ctx, q, accountID, SupportTicketOpen))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("support: get open ticket: %w", err)
	}
	return t, nil
}

func (r *SupportRepo) GetOpenTicketTx(ctx context.Context, tx pgx.Tx, accountID int64) (*SupportTicket, error) {
	const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket
WHERE account_id = $1 AND status = $2
LIMIT 1`
	t, err := scanTicket(tx.QueryRow(ctx, q, accountID, SupportTicketOpen))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("support: get open ticket tx: %w", err)
	}
	return t, nil
}

func (r *SupportRepo) GetTicketByIDForAccount(ctx context.Context, ticketID, accountID int64) (*SupportTicket, error) {
	const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket
WHERE id = $1 AND account_id = $2`
	t, err := scanTicket(r.pool.QueryRow(ctx, q, ticketID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSupportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("support: get ticket: %w", err)
	}
	return t, nil
}

func (r *SupportRepo) GetOpenTicketBySupportBotID(ctx context.Context, supportBotTicketID int64) (*SupportTicket, error) {
	const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket
WHERE support_bot_ticket_id = $1 AND status = $2
LIMIT 1`
	t, err := scanTicket(r.pool.QueryRow(ctx, q, supportBotTicketID, SupportTicketOpen))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSupportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("support: get by sb ticket: %w", err)
	}
	return t, nil
}

func (r *SupportRepo) CreateTicketTx(ctx context.Context, tx pgx.Tx, accountID int64) (*SupportTicket, error) {
	const q = `INSERT INTO cabinet_support_ticket (account_id, status)
VALUES ($1, $2)
RETURNING id, account_id, support_bot_ticket_id, status, created_at, closed_at`
	t, err := scanTicket(tx.QueryRow(ctx, q, accountID, SupportTicketOpen))
	if err != nil {
		return nil, fmt.Errorf("support: create ticket: %w", err)
	}
	return t, nil
}

func (r *SupportRepo) UpdateSupportBotTicketIDTx(ctx context.Context, tx pgx.Tx, ticketID int64, supportBotTicketID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE cabinet_support_ticket SET support_bot_ticket_id = $2 WHERE id = $1`,
		ticketID, supportBotTicketID,
	)
	if err != nil {
		return fmt.Errorf("support: update sb ticket id: %w", err)
	}
	return nil
}

func (r *SupportRepo) DeleteTicketTx(ctx context.Context, tx pgx.Tx, ticketID int64) error {
	_, err := tx.Exec(ctx, `DELETE FROM cabinet_support_ticket WHERE id = $1`, ticketID)
	if err != nil {
		return fmt.Errorf("support: delete ticket: %w", err)
	}
	return nil
}

func (r *SupportRepo) CloseTicketTx(ctx context.Context, tx pgx.Tx, ticketID int64, at time.Time) error {
	_, err := tx.Exec(ctx,
		`UPDATE cabinet_support_ticket SET status = $2, closed_at = $3 WHERE id = $1 AND status = $4`,
		ticketID, SupportTicketClosed, at, SupportTicketOpen,
	)
	if err != nil {
		return fmt.Errorf("support: close ticket: %w", err)
	}
	return nil
}

func (r *SupportRepo) InsertMessageTx(ctx context.Context, tx pgx.Tx, m *SupportMessage) (*SupportMessage, error) {
	status := m.DeliveryStatus
	if status == "" {
		status = SupportDeliverySent
	}
	const q = `INSERT INTO cabinet_support_message (ticket_id, direction, text, author_label, support_bot_message_id, client_message_id, delivery_status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, ticket_id, direction, text, author_label, support_bot_message_id, client_message_id, delivery_status, created_at, read_at`
	var out SupportMessage
	var sbID *int64
	var clientMsgID *string
	var readAt *time.Time
	err := tx.QueryRow(ctx, q, m.TicketID, m.Direction, m.Text, m.AuthorLabel, m.SupportBotMessageID, m.ClientMessageID, status).
		Scan(&out.ID, &out.TicketID, &out.Direction, &out.Text, &out.AuthorLabel, &sbID, &clientMsgID, &out.DeliveryStatus, &out.CreatedAt, &readAt)
	if err != nil {
		return nil, fmt.Errorf("support: insert message: %w", err)
	}
	out.SupportBotMessageID = sbID
	out.ClientMessageID = clientMsgID
	out.ReadAt = readAt
	return &out, nil
}

func (r *SupportRepo) UpdateDeliveryStatusTx(ctx context.Context, tx pgx.Tx, messageID int64, status string) error {
	_, err := tx.Exec(ctx,
		`UPDATE cabinet_support_message SET delivery_status = $2 WHERE id = $1`,
		messageID, status,
	)
	if err != nil {
		return fmt.Errorf("support: update delivery status: %w", err)
	}
	return nil
}

// InsertMessageIdempotentTx вставляет сообщение с idempotency key.
// При конфликте по client_message_id возвращает существующее сообщение без ошибки.
// Возвращает (message, true) если вставлено новое, (message, false) если уже существовало.
func (r *SupportRepo) InsertMessageIdempotentTx(ctx context.Context, tx pgx.Tx, m *SupportMessage) (*SupportMessage, bool, error) {
	if m.ClientMessageID == nil {
		out, err := r.InsertMessageTx(ctx, tx, m)
		return out, err == nil, err
	}

	// Сначала проверяем, есть ли уже сообщение с таким client_message_id
	const existsQ = `SELECT id, ticket_id, direction, text, author_label, support_bot_message_id, client_message_id, delivery_status, created_at, read_at
FROM cabinet_support_message WHERE client_message_id = $1`
	var existing SupportMessage
	var sbID *int64
	var clientMsgID *string
	var readAt *time.Time
	err := tx.QueryRow(ctx, existsQ, *m.ClientMessageID).
		Scan(&existing.ID, &existing.TicketID, &existing.Direction, &existing.Text, &existing.AuthorLabel, &sbID, &clientMsgID, &existing.DeliveryStatus, &existing.CreatedAt, &readAt)
	if err == nil {
		existing.SupportBotMessageID = sbID
		existing.ClientMessageID = clientMsgID
		existing.ReadAt = readAt
		return &existing, false, nil // уже существует
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("support: check client message: %w", err)
	}

	// Вставляем новое
	out, err := r.InsertMessageTx(ctx, tx, m)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (r *SupportRepo) InsertMessageIfNotExistsTx(ctx context.Context, tx pgx.Tx, m *SupportMessage) (*SupportMessage, bool, error) {
	if m.SupportBotMessageID != nil {
		const existsQ = `SELECT id FROM cabinet_support_message WHERE support_bot_message_id = $1`
		var existingID int64
		err := tx.QueryRow(ctx, existsQ, *m.SupportBotMessageID).Scan(&existingID)
		if err == nil {
			return nil, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("support: check sb message: %w", err)
		}
	}
	out, err := r.InsertMessageTx(ctx, tx, m)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (r *SupportRepo) ListMessagesByTicket(ctx context.Context, ticketID int64) ([]SupportMessage, error) {
	const q = `SELECT id, ticket_id, direction, text, author_label, support_bot_message_id, client_message_id, delivery_status, created_at, read_at
FROM cabinet_support_message
WHERE ticket_id = $1
ORDER BY created_at ASC, id ASC`
	rows, err := r.pool.Query(ctx, q, ticketID)
	if err != nil {
		return nil, fmt.Errorf("support: list messages: %w", err)
	}
	defer rows.Close()
	var out []SupportMessage
	for rows.Next() {
		var m SupportMessage
		var sbID *int64
		var clientMsgID *string
		var readAt *time.Time
		if err := rows.Scan(&m.ID, &m.TicketID, &m.Direction, &m.Text, &m.AuthorLabel, &sbID, &clientMsgID, &m.DeliveryStatus, &m.CreatedAt, &readAt); err != nil {
			return nil, fmt.Errorf("support: scan message: %w", err)
		}
		m.SupportBotMessageID = sbID
		m.ClientMessageID = clientMsgID
		m.ReadAt = readAt
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SupportRepo) CountUnreadOut(ctx context.Context, accountID int64) (int, error) {
	const q = `SELECT COUNT(*)
FROM cabinet_support_message m
JOIN cabinet_support_ticket t ON t.id = m.ticket_id
WHERE t.account_id = $1 AND t.status = $2
  AND m.direction = $3 AND m.read_at IS NULL`
	var n int
	if err := r.pool.QueryRow(ctx, q, accountID, SupportTicketOpen, SupportMsgOut).Scan(&n); err != nil {
		return 0, fmt.Errorf("support: count unread: %w", err)
	}
	return n, nil
}

func (r *SupportRepo) MarkOutMessagesRead(ctx context.Context, accountID int64, at time.Time) error {
	const q = `UPDATE cabinet_support_message m
SET read_at = $3
FROM cabinet_support_ticket t
WHERE m.ticket_id = t.id
  AND t.account_id = $1 AND t.status = $2
  AND m.direction = $4 AND m.read_at IS NULL`
	_, err := r.pool.Exec(ctx, q, accountID, SupportTicketOpen, at, SupportMsgOut)
	if err != nil {
		return fmt.Errorf("support: mark read: %w", err)
	}
	return nil
}

func (r *SupportRepo) CountMessagesByTicket(ctx context.Context, ticketID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM cabinet_support_message WHERE ticket_id = $1`
	var n int
	if err := r.pool.QueryRow(ctx, q, ticketID).Scan(&n); err != nil {
		return 0, fmt.Errorf("support: count messages: %w", err)
	}
	return n, nil
}

// SupportAttachment — изображение в сообщении чата поддержки. Файл хранит
// support-bot (в Telegram), здесь — описание и его id там.
type SupportAttachment struct {
	ID                     int64
	MessageID              int64
	Position               int
	SupportBotAttachmentID int64
	Mime                   string
	SizeBytes              *int
	Width                  *int
	Height                 *int
	Name                   *string
	CreatedAt              time.Time
}

const supportAttachmentCols = `id, message_id, position, support_bot_attachment_id, mime, size_bytes, width, height, name, created_at`

func scanSupportAttachment(row pgx.Row) (*SupportAttachment, error) {
	var a SupportAttachment
	if err := row.Scan(&a.ID, &a.MessageID, &a.Position, &a.SupportBotAttachmentID, &a.Mime,
		&a.SizeBytes, &a.Width, &a.Height, &a.Name, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// InsertAttachmentsTx сохраняет вложения сообщения; Position берётся из порядка в items.
func (r *SupportRepo) InsertAttachmentsTx(ctx context.Context, tx pgx.Tx, messageID int64, items []SupportAttachment) ([]SupportAttachment, error) {
	const q = `INSERT INTO cabinet_support_attachment (message_id, position, support_bot_attachment_id, mime, size_bytes, width, height, name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING ` + supportAttachmentCols
	out := make([]SupportAttachment, 0, len(items))
	for i, it := range items {
		a, err := scanSupportAttachment(tx.QueryRow(ctx, q, messageID, i, it.SupportBotAttachmentID, it.Mime,
			it.SizeBytes, it.Width, it.Height, it.Name))
		if err != nil {
			return nil, fmt.Errorf("support: insert attachment: %w", err)
		}
		out = append(out, *a)
	}
	return out, nil
}

// ListAttachmentsByMessages — вложения нескольких сообщений одним запросом (лента чата).
func (r *SupportRepo) ListAttachmentsByMessages(ctx context.Context, messageIDs []int64) (map[int64][]SupportAttachment, error) {
	out := make(map[int64][]SupportAttachment)
	if len(messageIDs) == 0 {
		return out, nil
	}
	const q = `SELECT ` + supportAttachmentCols + `
FROM cabinet_support_attachment
WHERE message_id = ANY($1)
ORDER BY message_id, position, id`
	rows, err := r.pool.Query(ctx, q, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("support: list attachments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanSupportAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("support: scan attachment: %w", err)
		}
		out[a.MessageID] = append(out[a.MessageID], *a)
	}
	return out, rows.Err()
}

// GetAttachmentForAccount — вложение, только если оно из тикета этого аккаунта.
// Чужое и несуществующее неразличимы: ErrSupportNotFound.
func (r *SupportRepo) GetAttachmentForAccount(ctx context.Context, attachmentID, accountID int64) (*SupportAttachment, error) {
	const q = `SELECT a.id, a.message_id, a.position, a.support_bot_attachment_id, a.mime, a.size_bytes, a.width, a.height, a.name, a.created_at
FROM cabinet_support_attachment a
JOIN cabinet_support_message m ON m.id = a.message_id
JOIN cabinet_support_ticket t ON t.id = m.ticket_id
WHERE a.id = $1 AND t.account_id = $2`
	a, err := scanSupportAttachment(r.pool.QueryRow(ctx, q, attachmentID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSupportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("support: get attachment: %w", err)
	}
	return a, nil
}
