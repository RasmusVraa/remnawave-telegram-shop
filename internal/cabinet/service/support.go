package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinet/supportbot"
	"remnawave-tg-shop-bot/internal/database"
)

var (
	ErrSupportDisabled   = errors.New("support chat disabled")
	ErrSupportInvalidMsg = errors.New("support invalid message")
	ErrSupportBridge     = errors.New("support bridge failed")
)

// supportTypingTTL — сколько держим «печатает…» после события typing от саппорт-бота.
// Бот шлёт его каждые ~5 с, пока ИИ ждёт паузы клиента и думает; запас покрывает
// задержки сети. Если бот замолк (ответ отменили, тикет ушёл людям), статус гаснет сам.
const supportTypingTTL = 12 * time.Second

type Support struct {
	repo       *repository.SupportRepo
	accounts   *repository.AccountRepo
	identities *repository.IdentityRepo
	links      *repository.AccountCustomerLinkRepo
	customers  *database.CustomerRepository
	bootstrap  *bootstrap.CustomerBootstrap
	sub        *Subscription
	bot        *supportbot.Client

	// «Печатает…» по тикетам кабинета: id тикета → до какого момента показывать.
	// Живёт в памяти процесса — статус короткий, после рестарта бот пришлёт его снова.
	typingMu    sync.Mutex
	typingUntil map[int64]time.Time
}

func NewSupport(
	repo *repository.SupportRepo,
	accounts *repository.AccountRepo,
	identities *repository.IdentityRepo,
	links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository,
	boot *bootstrap.CustomerBootstrap,
	sub *Subscription,
	bot *supportbot.Client,
) *Support {
	return &Support{
		repo:       repo,
		accounts:   accounts,
		identities: identities,
		links:      links,
		customers:  customers,
		bootstrap:  boot,
		sub:        sub,
		bot:        bot,

		typingUntil: make(map[int64]time.Time),
	}
}

type SupportMessageDTO struct {
	ID             int64  `json:"id"`
	Direction      string `json:"direction"`
	Text           string `json:"text"`
	AuthorLabel    string `json:"author_label,omitempty"`
	DeliveryStatus string `json:"delivery_status,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type SupportConversation struct {
	HasOpenTicket bool                `json:"has_open_ticket"`
	TicketStatus  string              `json:"ticket_status,omitempty"`
	UnreadCount   int                 `json:"unread_count"`
	SupportTyping bool                `json:"support_typing"`
	Messages      []SupportMessageDTO `json:"messages"`
}

type SupportSummary struct {
	HasOpenTicket bool   `json:"has_open_ticket"`
	TicketStatus  string `json:"ticket_status,omitempty"`
	UnreadCount   int    `json:"unread_count"`
}

type userContext struct {
	DisplayName         string
	Email               string
	TelegramID          *int64
	TelegramLabel       string
	SubscriptionSummary string
}

func (s *Support) Summary(ctx context.Context, accountID int64) (*SupportSummary, error) {
	if s.repo == nil {
		return nil, ErrSupportDisabled
	}
	unread, err := s.repo.CountUnreadOut(ctx, accountID)
	if err != nil {
		return nil, err
	}
	ticket, err := s.repo.GetOpenTicket(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := &SupportSummary{UnreadCount: unread}
	if ticket != nil {
		out.HasOpenTicket = true
		out.TicketStatus = ticket.Status
	}
	return out, nil
}

func (s *Support) Conversation(ctx context.Context, accountID int64) (*SupportConversation, error) {
	if s.repo == nil {
		return nil, ErrSupportDisabled
	}
	summary, err := s.Summary(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := &SupportConversation{
		HasOpenTicket: summary.HasOpenTicket,
		TicketStatus:  summary.TicketStatus,
		UnreadCount:   summary.UnreadCount,
		Messages:      []SupportMessageDTO{},
	}
	if !summary.HasOpenTicket {
		return out, nil
	}
	ticket, err := s.repo.GetOpenTicket(ctx, accountID)
	if err != nil || ticket == nil {
		return out, err
	}
	msgs, err := s.repo.ListMessagesByTicket(ctx, ticket.ID)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageDTO(m))
	}
	// «Печатает…» имеет смысл, только пока последнее слово за клиентом: запоздавшее
	// событие typing не должно висеть под уже пришедшим ответом.
	if n := len(msgs); n > 0 && msgs[n-1].Direction == repository.SupportMsgIn {
		out.SupportTyping = s.isTyping(ticket.ID, time.Now())
	}
	return out, nil
}

func (s *Support) setTyping(ticketID int64, now time.Time) {
	s.typingMu.Lock()
	defer s.typingMu.Unlock()
	for id, until := range s.typingUntil {
		if !until.After(now) {
			delete(s.typingUntil, id)
		}
	}
	s.typingUntil[ticketID] = now.Add(supportTypingTTL)
}

func (s *Support) clearTyping(ticketID int64) {
	s.typingMu.Lock()
	defer s.typingMu.Unlock()
	delete(s.typingUntil, ticketID)
}

func (s *Support) isTyping(ticketID int64, now time.Time) bool {
	s.typingMu.Lock()
	defer s.typingMu.Unlock()
	until, ok := s.typingUntil[ticketID]
	return ok && until.After(now)
}

func messageDTO(m repository.SupportMessage) SupportMessageDTO {
	dto := SupportMessageDTO{
		ID:          m.ID,
		Direction:   m.Direction,
		Text:        m.Text,
		AuthorLabel: m.AuthorLabel,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.Direction == repository.SupportMsgIn {
		dto.DeliveryStatus = m.DeliveryStatus
	}
	if m.Direction == repository.SupportMsgOut {
		dto.AuthorLabel = "Поддержка"
	}
	return dto
}

func (s *Support) MarkRead(ctx context.Context, accountID int64) error {
	if s.repo == nil {
		return ErrSupportDisabled
	}
	return s.repo.MarkOutMessagesRead(ctx, accountID, time.Now().UTC())
}

func normalizeSupportText(text string) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", ErrSupportInvalidMsg
	}
	if len([]rune(t)) > 4000 {
		return "", ErrSupportInvalidMsg
	}
	return t, nil
}

func (s *Support) SendMessage(ctx context.Context, accountID int64, text string) (*SupportMessageDTO, error) {
	if s.repo == nil || s.bot == nil {
		return nil, ErrSupportDisabled
	}
	msgText, err := normalizeSupportText(text)
	if err != nil {
		return nil, err
	}
	uc, err := s.buildUserContext(ctx, accountID)
	if err != nil {
		return nil, err
	}

	// Idempotency key — генерируем до любых операций.
	// При retry (HTTP OK, но DB fail) тот же UUID не создаст дубль.
	clientMsgID := uuid.New().String()

	var createdTicketID int64
	var bridgeIsNew bool
	var saved *repository.SupportMessage
	var alreadyExists bool

	// Транзакция 1: создаём тикет + сохраняем сообщение (idempotent по client_message_id).
	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockAccount(ctx, tx, accountID); err != nil {
			return err
		}
		ticket, err := s.repo.GetOpenTicketTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if ticket == nil {
			ticket, err = s.repo.CreateTicketTx(ctx, tx, accountID)
			if err != nil {
				return err
			}
		}
		createdTicketID = ticket.ID
		bridgeIsNew = ticket.SupportBotTicketID == nil

		// Сохраняем сообщение сразу — если уже есть с таким client_message_id, это retry.
		var inserted bool
		saved, inserted, err = s.repo.InsertMessageIdempotentTx(ctx, tx, &repository.SupportMessage{
			TicketID:        createdTicketID,
			Direction:       repository.SupportMsgIn,
			Text:            msgText,
			AuthorLabel:     uc.DisplayName,
			ClientMessageID: &clientMsgID,
			DeliveryStatus:  repository.SupportDeliveryPending,
		})
		if err != nil {
			return err
		}
		alreadyExists = !inserted
		return nil
	})
	if err != nil {
		return nil, err
	}

	if alreadyExists && saved != nil && saved.DeliveryStatus == repository.SupportDeliverySent {
		dto := messageDTO(*saved)
		return &dto, nil
	}

	// HTTP к support-bot (с client_message_id для их dedup на случай network retry).
	var tgID *int64
	if uc.TelegramID != nil {
		tgID = uc.TelegramID
	}
	resp, err := s.bot.PostCabinetMessage(ctx, supportbot.PostMessageRequest{
		AccountID:           accountID,
		ShopTicketID:        createdTicketID,
		ClientMessageID:     clientMsgID,
		IsNewTicket:         bridgeIsNew,
		DisplayName:         uc.DisplayName,
		TelegramID:          tgID,
		TelegramLabel:       uc.TelegramLabel,
		Email:               uc.Email,
		SubscriptionSummary: uc.SubscriptionSummary,
		Text:                msgText,
	})
	if err != nil {
		if saved != nil {
			_ = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
				return s.repo.UpdateDeliveryStatusTx(ctx, tx, saved.ID, repository.SupportDeliveryFailed)
			})
			saved.DeliveryStatus = repository.SupportDeliveryFailed
		}
		return nil, fmt.Errorf("%w: %v", ErrSupportBridge, err)
	}

	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockAccount(ctx, tx, accountID); err != nil {
			return err
		}
		if saved != nil {
			if err := s.repo.UpdateDeliveryStatusTx(ctx, tx, saved.ID, repository.SupportDeliverySent); err != nil {
				return err
			}
			saved.DeliveryStatus = repository.SupportDeliverySent
		}
		if resp.SupportBotTicketID > 0 {
			return s.repo.UpdateSupportBotTicketIDTx(ctx, tx, createdTicketID, resp.SupportBotTicketID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	dto := messageDTO(*saved)
	return &dto, nil
}

type SupportWebhookPayload struct {
	Event               string `json:"event"`
	AccountID           int64  `json:"account_id"`
	ShopTicketID        int64  `json:"shop_ticket_id"`
	SupportBotTicketID  int64  `json:"support_bot_ticket_id"`
	SupportBotMessageID int64  `json:"support_bot_message_id"`
	Text                string `json:"text"`
	AuthorLabel         string `json:"author_label"`
}

func (s *Support) HandleWebhook(ctx context.Context, p SupportWebhookPayload) error {
	if s.repo == nil {
		return ErrSupportDisabled
	}
	switch strings.TrimSpace(p.Event) {
	case "message":
		return s.webhookMessage(ctx, p)
	case "closed":
		return s.webhookClosed(ctx, p)
	case "typing":
		return s.webhookTyping(ctx, p)
	default:
		return ErrSupportInvalidMsg
	}
}

func (s *Support) webhookMessage(ctx context.Context, p SupportWebhookPayload) error {
	text, err := normalizeSupportText(p.Text)
	if err != nil {
		return err
	}
	author := "Поддержка"
	var sbMsgID *int64
	if p.SupportBotMessageID > 0 {
		sbMsgID = &p.SupportBotMessageID
	}

	return s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		// Advisory lock по account_id — защита от race condition при параллельных webhook'ах
		if p.AccountID > 0 {
			if err := s.repo.LockAccount(ctx, tx, p.AccountID); err != nil {
				return err
			}
		}
		ticket, err := s.resolveWebhookTicket(ctx, tx, p)
		if err != nil {
			return err
		}
		if ticket.Status != repository.SupportTicketOpen {
			return nil
		}
		_, inserted, err := s.repo.InsertMessageIfNotExistsTx(ctx, tx, &repository.SupportMessage{
			TicketID:            ticket.ID,
			Direction:           repository.SupportMsgOut,
			Text:                text,
			AuthorLabel:         author,
			SupportBotMessageID: sbMsgID,
			DeliveryStatus:      repository.SupportDeliverySent,
		})
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		s.clearTyping(ticket.ID)
		if ticket.SupportBotTicketID == nil && p.SupportBotTicketID > 0 {
			return s.repo.UpdateSupportBotTicketIDTx(ctx, tx, ticket.ID, p.SupportBotTicketID)
		}
		return nil
	})
}

func (s *Support) webhookClosed(ctx context.Context, p SupportWebhookPayload) error {
	return s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		// Advisory lock по account_id — защита от race condition
		if p.AccountID > 0 {
			if err := s.repo.LockAccount(ctx, tx, p.AccountID); err != nil {
				return err
			}
		}
		ticket, err := s.resolveWebhookTicket(ctx, tx, p)
		if err != nil {
			return err
		}
		if ticket.Status != repository.SupportTicketOpen {
			return nil
		}
		s.clearTyping(ticket.ID)
		return s.repo.CloseTicketTx(ctx, tx, ticket.ID, time.Now().UTC())
	})
}

// webhookTyping — ИИ саппорт-бота готовит ответ по тикету: кабинет покажет «печатает…».
func (s *Support) webhookTyping(ctx context.Context, p SupportWebhookPayload) error {
	return s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		ticket, err := s.resolveWebhookTicket(ctx, tx, p)
		if err != nil {
			return err
		}
		if ticket.Status != repository.SupportTicketOpen {
			return nil
		}
		s.setTyping(ticket.ID, time.Now())
		return nil
	})
}

func (s *Support) resolveWebhookTicket(ctx context.Context, tx pgx.Tx, p SupportWebhookPayload) (*repository.SupportTicket, error) {
	if p.SupportBotTicketID > 0 {
		const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket WHERE support_bot_ticket_id = $1 AND account_id = $2 LIMIT 1`
		t, err := scanTicketRow(tx.QueryRow(ctx, q, p.SupportBotTicketID, p.AccountID))
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, repository.ErrSupportNotFound) {
			return nil, err
		}
	}
	if p.ShopTicketID > 0 && p.AccountID > 0 {
		const q = `SELECT id, account_id, support_bot_ticket_id, status, created_at, closed_at
FROM cabinet_support_ticket WHERE id = $1 AND account_id = $2`
		return scanTicketRow(tx.QueryRow(ctx, q, p.ShopTicketID, p.AccountID))
	}
	return nil, repository.ErrSupportNotFound
}

func scanTicketRow(row pgx.Row) (*repository.SupportTicket, error) {
	var t repository.SupportTicket
	var sbID *int64
	var closedAt *time.Time
	err := row.Scan(&t.ID, &t.AccountID, &sbID, &t.Status, &t.CreatedAt, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrSupportNotFound
	}
	if err != nil {
		return nil, err
	}
	t.SupportBotTicketID = sbID
	t.ClosedAt = closedAt
	return &t, nil
}

func (s *Support) buildUserContext(ctx context.Context, accountID int64) (*userContext, error) {
	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil || acc == nil {
		return nil, fmt.Errorf("support: account: %w", err)
	}
	email := ""
	if acc.Email != nil {
		email = strings.TrimSpace(*acc.Email)
	}
	maskedEmail := email
	if email != "" {
		maskedEmail = maskSupportEmail(email)
	}

	var telegramID *int64
	tgLabel := "не привязан"
	if s.identities != nil {
		ids, _ := s.identities.ListByAccount(ctx, accountID)
		for _, id := range ids {
			if id.Provider == repository.ProviderTelegram && id.ProviderUserID != "" {
				if v, parseErr := parseInt64(id.ProviderUserID); parseErr == nil {
					telegramID = &v
					tgLabel = fmt.Sprintf("%d", v)
					break
				}
			}
		}
	}

	displayName := maskedEmail
	if displayName == "" {
		displayName = fmt.Sprintf("account #%d", accountID)
	}

	subSummary := "нет данных"
	if s.bootstrap != nil && s.customers != nil {
		link, linkErr := s.bootstrap.EnsureForAccount(ctx, accountID, acc.Language)
		if linkErr == nil && link != nil {
			cust, custErr := s.customers.FindById(ctx, link.CustomerID)
			if custErr == nil && cust != nil {
				if cust.TelegramUsername != nil {
					un := strings.TrimSpace(*cust.TelegramUsername)
					if un != "" {
						if !strings.HasPrefix(un, "@") {
							un = "@" + un
						}
						if telegramID != nil {
							tgLabel = fmt.Sprintf("%s (%d)", un, *telegramID)
						} else {
							tgLabel = un
						}
						displayName = un
					}
				}
				subSummary = formatSubscriptionSummary(cust)
			}
		}
	}

	return &userContext{
		DisplayName:         displayName,
		Email:               maskedEmail,
		TelegramID:          telegramID,
		TelegramLabel:       tgLabel,
		SubscriptionSummary: subSummary,
	}, nil
}

func formatSubscriptionSummary(c *database.Customer) string {
	if c == nil {
		return "нет данных"
	}
	now := time.Now().UTC()
	if c.ExpireAt != nil && c.ExpireAt.After(now) {
		return fmt.Sprintf("active до %s", c.ExpireAt.UTC().Format("02.01.2006"))
	}
	if c.ExpireAt != nil {
		return fmt.Sprintf("истекла %s", c.ExpireAt.UTC().Format("02.01.2006"))
	}
	return "нет активной подписки"
}

func parseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	_, err := fmt.Sscan(s, &n)
	return n, err
}

func maskSupportEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return email
	}
	local := email[:at]
	domain := email[at+1:]
	if len(local) <= 1 {
		return local + "***@" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + domain
}
