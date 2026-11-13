package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinet/supportbot"
	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
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
	ID             int64                  `json:"id"`
	Direction      string                 `json:"direction"`
	Text           string                 `json:"text"`
	AuthorLabel    string                 `json:"author_label,omitempty"`
	DeliveryStatus string                 `json:"delivery_status,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	Attachments    []SupportAttachmentDTO `json:"attachments,omitempty"`
}

// SupportAttachmentDTO — изображение в сообщении; ID — для GET /support/attachments/{id}.
type SupportAttachmentDTO struct {
	ID     int64   `json:"id"`
	Mime   string  `json:"mime"`
	Width  *int    `json:"width,omitempty"`
	Height *int    `json:"height,omitempty"`
	Size   *int    `json:"size,omitempty"`
	Name   *string `json:"name,omitempty"`
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
	CustomerID          *int64
	RemnawaveUserID     *int64
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
	ids := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	atts, err := s.repo.ListAttachmentsByMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageDTO(m, atts[m.ID]))
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

func messageDTO(m repository.SupportMessage, atts []repository.SupportAttachment) SupportMessageDTO {
	dto := SupportMessageDTO{
		ID:          m.ID,
		Direction:   m.Direction,
		Text:        m.Text,
		AuthorLabel: m.AuthorLabel,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
	}
	for _, a := range atts {
		dto.Attachments = append(dto.Attachments, SupportAttachmentDTO{
			ID:     a.ID,
			Mime:   a.Mime,
			Width:  a.Width,
			Height: a.Height,
			Size:   a.SizeBytes,
			Name:   a.Name,
		})
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

// normalizeSupportText — текст сообщения; пустой можно только с вложениями (allowEmpty).
func normalizeSupportText(text string, allowEmpty bool) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" && !allowEmpty {
		return "", ErrSupportInvalidMsg
	}
	if len([]rune(t)) > 4000 {
		return "", ErrSupportInvalidMsg
	}
	return t, nil
}

// SendMessage — сообщение клиента в поддержку: текст и до SupportMaxFiles изображений.
func (s *Support) SendMessage(ctx context.Context, accountID int64, text string, files []SupportUpload) (*SupportMessageDTO, error) {
	if s.repo == nil || s.bot == nil {
		return nil, ErrSupportDisabled
	}
	if err := validateSupportUploads(files); err != nil {
		return nil, err
	}
	msgText, err := normalizeSupportText(text, len(files) > 0)
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
		atts, err := s.repo.ListAttachmentsByMessages(ctx, []int64{saved.ID})
		if err != nil {
			return nil, err
		}
		dto := messageDTO(*saved, atts[saved.ID])
		return &dto, nil
	}

	// HTTP к support-bot (с client_message_id для их dedup на случай network retry).
	var tgID *int64
	if uc.TelegramID != nil {
		tgID = uc.TelegramID
	}
	req := supportbot.PostMessageRequest{
		AccountID:           accountID,
		ShopTicketID:        createdTicketID,
		ClientMessageID:     clientMsgID,
		IsNewTicket:         bridgeIsNew,
		DisplayName:         uc.DisplayName,
		TelegramID:          tgID,
		CustomerID:          uc.CustomerID,
		RemnawaveUserID:     uc.RemnawaveUserID,
		TelegramLabel:       uc.TelegramLabel,
		Email:               uc.Email,
		SubscriptionSummary: uc.SubscriptionSummary,
		Text:                msgText,
	}
	var resp *supportbot.PostMessageResponse
	if len(files) > 0 {
		uploads := make([]supportbot.UploadFile, 0, len(files))
		for _, f := range files {
			uploads = append(uploads, supportbot.UploadFile{Name: f.Name, Data: f.Data})
		}
		resp, err = s.bot.PostCabinetMessageWithFiles(ctx, req, uploads)
	} else {
		resp, err = s.bot.PostCabinetMessage(ctx, req)
	}
	if err != nil {
		if saved != nil {
			_ = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
				return s.repo.UpdateDeliveryStatusTx(ctx, tx, saved.ID, repository.SupportDeliveryFailed)
			})
			saved.DeliveryStatus = repository.SupportDeliveryFailed
		}
		return nil, bridgeSendErr(err)
	}

	var savedAtts []repository.SupportAttachment

	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockAccount(ctx, tx, accountID); err != nil {
			return err
		}
		if saved != nil {
			if err := s.repo.UpdateDeliveryStatusTx(ctx, tx, saved.ID, repository.SupportDeliverySent); err != nil {
				return err
			}
			saved.DeliveryStatus = repository.SupportDeliverySent
			if items := attachmentsFromBot(resp.Attachments); len(items) > 0 {
				var err error
				if savedAtts, err = s.repo.InsertAttachmentsTx(ctx, tx, saved.ID, items); err != nil {
					return err
				}
			}
		}
		if resp.SupportBotTicketID > 0 {
			return s.repo.UpdateSupportBotTicketIDTx(ctx, tx, createdTicketID, resp.SupportBotTicketID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var customerID int64
	if uc.CustomerID != nil {
		customerID = *uc.CustomerID
	}
	adminevents.Emit(adminevents.Event{
		Type: adminevents.SupportMessage, CustomerID: customerID, AccountID: accountID,
		Source:  adminevents.SourceCabinet,
		Payload: map[string]any{"preview": adminevents.Preview(msgText, 100), "images": len(files)},
	})

	dto := messageDTO(*saved, savedAtts)
	return &dto, nil
}

// bridgeSendErr — ошибка support-bot для клиента: отказ по файлам — понятной
// причиной (бот проверяет их так же, как мы, но мог обновиться раньше), остальное — ErrSupportBridge.
func bridgeSendErr(err error) error {
	var se *supportbot.StatusError
	if errors.As(err, &se) {
		switch {
		case se.Status == http.StatusRequestEntityTooLarge:
			return ErrSupportFileTooLarge
		case se.Status == http.StatusUnsupportedMediaType:
			return ErrSupportUnsupportedMedia
		case se.Status == http.StatusBadRequest && strings.Contains(se.Body, "too_many_files"):
			return ErrSupportTooManyFiles
		}
	}
	return fmt.Errorf("%w: %v", ErrSupportBridge, err)
}

// supportImageMimes — что кабинет показывает как вложение; остальное от бота не сохраняем
// и с его Content-Type не отдаём (защита от text/html под видом картинки).
var supportImageMimes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/bmp": true,
	"image/gif": true, "image/heic": true, "image/heif": true, "image/avif": true, "image/tiff": true,
}

// attachmentsFromBot — описание вложений из ответа или webhook support-bot.
func attachmentsFromBot(in []supportbot.AttachmentInfo) []repository.SupportAttachment {
	out := make([]repository.SupportAttachment, 0, len(in))
	for _, a := range in {
		mime := strings.ToLower(strings.TrimSpace(a.Mime))
		if a.ID <= 0 || !supportImageMimes[mime] {
			continue
		}
		var name *string
		if a.Name != nil {
			if n := []rune(strings.TrimSpace(*a.Name)); len(n) > 0 {
				v := string(n[:min(len(n), 255)])
				name = &v
			}
		}
		out = append(out, repository.SupportAttachment{
			SupportBotAttachmentID: a.ID,
			Mime:                   mime,
			SizeBytes:              a.Size,
			Width:                  a.Width,
			Height:                 a.Height,
			Name:                   name,
		})
	}
	return out
}

// Attachment — байты и MIME изображения для кабинета. Чужое и несуществующее —
// repository.ErrSupportNotFound.
func (s *Support) Attachment(ctx context.Context, accountID, attachmentID int64) ([]byte, string, error) {
	if s.repo == nil || s.bot == nil {
		return nil, "", ErrSupportDisabled
	}
	a, err := s.repo.GetAttachmentForAccount(ctx, attachmentID, accountID)
	if err != nil {
		return nil, "", err
	}
	data, _, err := s.bot.FetchAttachment(ctx, a.SupportBotAttachmentID, accountID)
	if errors.Is(err, supportbot.ErrNotFound) {
		return nil, "", repository.ErrSupportNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrSupportBridge, err)
	}
	return data, a.Mime, nil
}

type SupportWebhookPayload struct {
	Event               string                      `json:"event"`
	AccountID           int64                       `json:"account_id"`
	ShopTicketID        int64                       `json:"shop_ticket_id"`
	SupportBotTicketID  int64                       `json:"support_bot_ticket_id"`
	SupportBotMessageID int64                       `json:"support_bot_message_id"`
	Text                string                      `json:"text"`
	AuthorLabel         string                      `json:"author_label"`
	Attachments         []supportbot.AttachmentInfo `json:"attachments,omitempty"`
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
	items := attachmentsFromBot(p.Attachments)
	text, err := normalizeSupportText(p.Text, len(items) > 0)
	if err != nil {
		return err
	}
	author := "Поддержка"
	var sbMsgID *int64
	if p.SupportBotMessageID > 0 {
		sbMsgID = &p.SupportBotMessageID
	}

	var notifyAccountID, notifyMsgID int64
	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
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
		savedMsg, inserted, err := s.repo.InsertMessageIfNotExistsTx(ctx, tx, &repository.SupportMessage{
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
		if savedMsg != nil && len(items) > 0 {
			if _, err := s.repo.InsertAttachmentsTx(ctx, tx, savedMsg.ID, items); err != nil {
				return err
			}
		}
		notifyAccountID = ticket.AccountID
		if savedMsg != nil {
			notifyMsgID = savedMsg.ID
		}
		s.clearTyping(ticket.ID)
		if ticket.SupportBotTicketID == nil && p.SupportBotTicketID > 0 {
			return s.repo.UpdateSupportBotTicketIDTx(ctx, tx, ticket.ID, p.SupportBotTicketID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if notifyAccountID > 0 {
		s.notifyCabinetReply(ctx, notifyAccountID, notifyMsgID, text, len(items) > 0)
		adminevents.Emit(adminevents.Event{
			Type: adminevents.SupportReply, AccountID: notifyAccountID, Source: adminevents.SourceCabinet,
			Payload: map[string]any{
				"preview": adminevents.Preview(text, 100), "author": adminevents.Preview(p.AuthorLabel, 60),
				"images": len(items),
			},
		})
	}
	return nil
}

// notifyCabinetReply — ответ оператора в ленту кабинета и push: чат поддержки
// открыт не всегда, а ответ ждут.
func (s *Support) notifyCabinetReply(ctx context.Context, accountID, msgID int64, text string, hasImages bool) {
	if s.links == nil || s.customers == nil {
		return
	}
	link, err := s.links.FindByAccountID(ctx, accountID)
	if err != nil || link == nil {
		return
	}
	lang := ""
	if c, err := s.customers.FindById(ctx, link.CustomerID); err == nil && c != nil {
		lang = c.Language
	}
	preview := []rune(strings.TrimSpace(text))
	if len(preview) == 0 && hasImages {
		label := translation.GetInstance().GetText(lang, "cabinet_notify_support_photo")
		// Переводы бота лежат на диске у владельца: без нового ключа GetText
		// вернёт само имя ключа, и оно попадёт в ленту.
		if label == "cabinet_notify_support_photo" {
			label = "📷"
		}
		preview = []rune(label)
	}
	if len(preview) > 300 {
		preview = append(preview[:299], '…')
	}
	cabinetnotify.Notify(ctx, link.CustomerID, lang, cabinetnotify.Event{
		Kind:      database.CabinetNotificationSupport,
		TitleKey:  "cabinet_notify_support_title",
		BodyHTML:  html.EscapeString(string(preview)),
		Buttons:   []string{"support"},
		DedupeKey: fmt.Sprintf("support:%d", msgID),
	})
}

func (s *Support) webhookClosed(ctx context.Context, p SupportWebhookPayload) error {
	var closedAccountID int64
	err := s.repo.WithTx(ctx, func(tx pgx.Tx) error {
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
		if err := s.repo.CloseTicketTx(ctx, tx, ticket.ID, time.Now().UTC()); err != nil {
			return err
		}
		closedAccountID = ticket.AccountID
		return nil
	})
	if err == nil && closedAccountID > 0 {
		adminevents.Emit(adminevents.Event{
			Type: adminevents.SupportClosed, AccountID: closedAccountID, Source: adminevents.SourceCabinet,
		})
	}
	return err
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
	var customerID, rwUserID *int64
	if s.bootstrap != nil && s.customers != nil {
		link, linkErr := s.bootstrap.EnsureForAccount(ctx, accountID, acc.Language)
		if linkErr == nil && link != nil {
			cust, custErr := s.customers.FindById(ctx, link.CustomerID)
			if custErr == nil && cust != nil {
				id := cust.ID
				customerID = &id
				rwUserID = s.remnawaveUserIDFor(ctx, cust)
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
		CustomerID:          customerID,
		RemnawaveUserID:     rwUserID,
	}, nil
}

// supportPanelLookupTimeout — сколько ждём панель при поиске профиля для саппорт-бота.
// Профиль нужен только оператору в /profile, и задерживать из-за него сообщение
// клиента нельзя: не нашли за это время — отправляем без id.
const supportPanelLookupTimeout = 3 * time.Second

// remnawaveUserIDFor — id профиля клиента в панели для /profile в саппорт-боте.
//
// Без него бот не найдёт профиль web-only клиента: telegramId у такого профиля
// пустой, а логин "<customer_id>_<email>" боту неизвестен. Обычно id уже сохранён
// у клиента; если нет — ищем тем же резолвом, что и кабинет, и он его запомнит.
func (s *Support) remnawaveUserIDFor(ctx context.Context, c *database.Customer) *int64 {
	if c.RemnawaveUserID != nil && *c.RemnawaveUserID > 0 {
		return c.RemnawaveUserID
	}
	// Без подписки профиля в панели нет — не гоняем поиск на каждом сообщении.
	hasSub := c.ExpireAt != nil || (c.SubscriptionLink != nil && strings.TrimSpace(*c.SubscriptionLink) != "")
	if !hasSub || s.sub == nil || s.sub.rw == nil {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, supportPanelLookupTimeout)
	defer cancel()
	user, err := ResolveRemnawaveCustomerUser(lookupCtx, s.sub.rw, s.customers, c)
	if err != nil {
		if !errors.Is(err, remnawave.ErrUserNotFound) && !errors.Is(err, remnawave.ErrNotFound) {
			slog.Warn("support: remnawave profile lookup failed (non-fatal)",
				"customer_id", c.ID, "error", err)
		}
		return nil
	}
	if user == nil || user.ID <= 0 {
		return nil
	}
	id := user.ID
	return &id
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
