package remnawavewebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

const eventTorrentBlockerReport = "torrent_blocker.report"

type Texts interface {
	GetText(lang, key string) string
	WithButton(lang, key string, btn models.InlineKeyboardButton) models.InlineKeyboardButton
}

type MessageSender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}

type Handler struct {
	secret          string
	bot             MessageSender
	tm              Texts
	dedup           *deduper
	now             func() time.Time
	resolveCustomer func(ctx context.Context, u remnawave.User) (*database.Customer, error)
}

func NewHandler(secret string, customerRepo *database.CustomerRepository, b MessageSender, tm Texts) *Handler {
	h := &Handler{
		secret: strings.TrimSpace(secret),
		bot:    b,
		tm:     tm,
		dedup:  newDeduper(),
		now:    time.Now,
	}
	h.resolveCustomer = func(ctx context.Context, u remnawave.User) (*database.Customer, error) {
		return remnawave.CustomerFromAdminSearchUser(ctx, customerRepo, u)
	}
	return h
}

type envelope struct {
	Scope     string          `json:"scope"`
	Event     string          `json:"event"`
	Timestamp string          `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

type torrentBlockerData struct {
	Node   nodePayload          `json:"node"`
	User   remnawave.User       `json:"user"`
	Report torrentBlockerReport `json:"report"`
}

type nodePayload struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type torrentBlockerReport struct {
	ActionReport actionReport `json:"actionReport"`
}

type actionReport struct {
	Blocked       bool      `json:"blocked"`
	IP            string    `json:"ip"`
	BlockDuration int       `json:"blockDuration"`
	WillUnblockAt time.Time `json:"willUnblockAt"`
	UserID        string    `json:"userId"`
	ProcessedAt   time.Time `json:"processedAt"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.secret == "" {
		http.Error(w, "webhook not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		slog.Error("remnawave webhook: read body", "error", err)
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	sig := r.Header.Get("X-Remnawave-Signature")
	if !ValidateSignature(h.secret, body, sig) {
		slog.Warn("remnawave webhook: invalid signature")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tsHeader := r.Header.Get("X-Remnawave-Timestamp")
	if !ValidateTimestamp(tsHeader, h.now()) {
		slog.Warn("remnawave webhook: invalid or stale timestamp", "timestamp", tsHeader)
		http.Error(w, "invalid timestamp", http.StatusUnauthorized)
		return
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		slog.Error("remnawave webhook: unmarshal envelope", "error", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Always ACK unknown events so panel retries do not pile up.
	if env.Event != eventTorrentBlockerReport {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"ignored":true}`))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.handleTorrentReport(ctx, env.Data); err != nil {
		slog.Error("remnawave webhook: handle torrent report", "error", err)
		// Transient failures → 5xx so the panel can retry; permanent skips already return nil.
		http.Error(w, "temporary error", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) handleTorrentReport(ctx context.Context, raw json.RawMessage) error {
	var data torrentBlockerData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}
	ar := data.Report.ActionReport
	if !ar.Blocked {
		slog.Info("remnawave webhook: torrent report not blocked, skip notify")
		return nil
	}

	dedupKey := fmt.Sprintf("%s|%s|%s|%s",
		strconv.FormatInt(data.User.ID, 10),
		strings.TrimSpace(data.Node.UUID),
		strings.TrimSpace(ar.IP),
		ar.WillUnblockAt.UTC().Format(time.RFC3339Nano),
	)
	if data.User.ID == 0 {
		dedupKey = fmt.Sprintf("%s|%s|%s|%s",
			strings.TrimSpace(data.User.Username),
			strings.TrimSpace(data.Node.UUID),
			strings.TrimSpace(ar.IP),
			ar.WillUnblockAt.UTC().Format(time.RFC3339Nano),
		)
	}
	if h.dedup.Has(dedupKey, h.now()) {
		slog.Info("remnawave webhook: duplicate torrent report skipped",
			"user_id", data.User.ID,
			"node_uuid", strings.TrimSpace(data.Node.UUID),
		)
		return nil
	}

	customer, err := h.resolveCustomer(ctx, data.User)
	if err != nil {
		return fmt.Errorf("resolve customer: %w", err)
	}
	if customer == nil {
		slog.Warn("remnawave webhook: customer not found",
			"username", data.User.Username,
			"user_id", data.User.ID,
		)
		return nil
	}
	h.notifyCabinet(ctx, customer, data.Node.Name, ar, dedupKey)
	if customer.IsWebOnly || utils.IsSyntheticTelegramID(customer.TelegramID) {
		slog.Info("remnawave webhook: skip web-only/synthetic customer",
			"customer_id", utils.MaskHalfInt64(customer.ID),
		)
		return nil
	}

	if err := h.notifyCustomer(ctx, customer, data.Node.Name, ar); err != nil {
		return err
	}
	h.dedup.Add(dedupKey, h.now())
	return nil
}

func (h *Handler) notifyCustomer(ctx context.Context, customer *database.Customer, nodeName string, ar actionReport) error {
	lang := customer.Language
	if lang == "" {
		lang = config.DefaultLanguage()
	}
	tmpl := h.tm.GetText(lang, "torrent_blocked")
	mins := DurationMinutes(ar.BlockDuration, ar.ProcessedAt, ar.WillUnblockAt)
	text := FormatMessage(tmpl, nodeName, mins, ar.WillUnblockAt)

	_, err := h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: h.buildKeyboard(lang),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	slog.Info("remnawave webhook: torrent block notify sent",
		"customer_id", utils.MaskHalfInt64(customer.ID),
		"node", nodeName,
		"duration_min", mins,
	)
	return nil
}

// notifyCabinet — та же блокировка в ленту кабинета, в том числе web-only
// клиенту: в Telegram ему не написать, а узнать, почему сервер пропал, надо.
func (h *Handler) notifyCabinet(ctx context.Context, customer *database.Customer, nodeName string, ar actionReport, dedupKey string) {
	lang := customer.Language
	if lang == "" {
		lang = config.DefaultLanguage()
	}
	mins := DurationMinutes(ar.BlockDuration, ar.ProcessedAt, ar.WillUnblockAt)
	cabinetnotify.Notify(ctx, customer.ID, lang, cabinetnotify.Event{
		Kind:      database.CabinetNotificationTorrent,
		TitleKey:  "cabinet_notify_torrent_title",
		BodyHTML:  FormatMessage(h.tm.GetText(lang, "torrent_blocked"), nodeName, mins, ar.WillUnblockAt),
		DedupeKey: "torrent:" + dedupKey,
	})
}

func (h *Handler) buildKeyboard(lang string) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton

	// Мой VPN
	var connectBtn models.InlineKeyboardButton
	if handler.IsCabinetTelegramMinimalismActive() {
		if cabinetURL := handler.BuildCabinetWebAppURL("/cabinet/connections"); cabinetURL != "" {
			connectBtn = h.tm.WithButton(lang, "connect_button", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: cabinetURL},
			})
		}
	}
	if connectBtn.Text == "" && connectBtn.CallbackData == "" && connectBtn.WebApp == nil {
		connectBtn = h.tm.WithButton(lang, "connect_button", models.InlineKeyboardButton{
			CallbackData: handler.CallbackConnect,
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{connectBtn})

	// Поддержка
	supportURL := strings.TrimSpace(config.SupportURL())
	if supportURL == "" {
		supportURL = strings.TrimSpace(config.FeedbackURL())
	}
	if supportURL != "" {
		supportBtn := h.tm.WithButton(lang, "support_button", models.InlineKeyboardButton{URL: supportURL})
		rows = append(rows, []models.InlineKeyboardButton{supportBtn})
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
