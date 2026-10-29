package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// Потолок загрузки. 50 MiB — предел Bot API на отправку файла ботом; меньше
// ставить незачем, а больше Telegram всё равно не примет. На стороне nginx для
// этого location нужен свой client_max_body_size, см. SETUP-GUIDE-RU.md.
const broadcastMediaMaxBytes = 50 << 20 // 50 MiB

var (
	broadcastSendMu      sync.Mutex
	broadcastSendRunning bool
)

func tryAcquireBroadcastSend() bool {
	broadcastSendMu.Lock()
	defer broadcastSendMu.Unlock()
	if broadcastSendRunning {
		return false
	}
	broadcastSendRunning = true
	return true
}

func releaseBroadcastSend() {
	broadcastSendMu.Lock()
	broadcastSendRunning = false
	broadcastSendMu.Unlock()
}

type AdminBroadcastHandler struct {
	customers *database.CustomerRepository
	tariffs   *database.TariffRepository
	history   *database.BroadcastRepository
	sender    *broadcast.Sender
	bot       *bot.Bot
}

func NewAdminBroadcast(
	customers *database.CustomerRepository,
	tariffs *database.TariffRepository,
	history *database.BroadcastRepository,
	sender *broadcast.Sender,
	tgBot *bot.Bot,
) *AdminBroadcastHandler {
	return &AdminBroadcastHandler{
		customers: customers,
		tariffs:   tariffs,
		history:   history,
		sender:    sender,
		bot:       tgBot,
	}
}

type broadcastAudienceResp struct {
	Audience string `json:"audience"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
}

var broadcastAudienceValues = []string{
	database.BroadcastAudienceAll,
	database.BroadcastAudienceTestBroadcast,
	database.BroadcastAudienceActiveAll,
	database.BroadcastAudienceActivePaid,
	database.BroadcastAudienceActiveTrial,
	database.BroadcastAudienceInactiveAll,
	database.BroadcastAudienceInactivePaid,
	database.BroadcastAudienceInactiveTrial,
}

// Audiences returns available broadcast segments with recipient counts.
func (h *AdminBroadcastHandler) Audiences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	var result []broadcastAudienceResp
	for _, aud := range broadcastAudienceValues {
		recipients, err := h.customers.GetBroadcastRecipients(ctx, aud, nil)
		if err != nil {
			slog.Warn("admin broadcast: audience count", "audience", aud, "error", err)
			continue
		}
		result = append(result, broadcastAudienceResp{
			Audience: aud,
			Label:    aud,
			Count:    len(recipients),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"audiences": result})
}

type broadcastButtonsReq struct {
	Buy      bool `json:"buy"`
	Connect  bool `json:"connect"`
	Promo    bool `json:"promo"`
	MainMenu bool `json:"main_menu"`
	// ShareReferral — «Пригласить друга» с личной реферальной ссылкой получателя.
	ShareReferral bool     `json:"share_referral"`
	Links         []string `json:"links,omitempty"`
}

type broadcastMediaReq struct {
	FileID string `json:"file_id"`
	// photo | video | document — что вернул upload-media.
	Kind string `json:"kind"`
	// Name — имя файла у админа, только для истории рассылок.
	Name string `json:"name,omitempty"`
}

type broadcastSendReq struct {
	// Audiences — выбранные сегменты; человек из нескольких получит сообщение
	// один раз. Audience — прежнее поле для одного сегмента: его шлёт кабинет
	// старой сборки, пока у админа не обновилась вкладка.
	Audiences []string             `json:"audiences,omitempty"`
	Audience  string               `json:"audience,omitempty"`
	TariffID  *int64               `json:"tariff_id,omitempty"`
	Text      string               `json:"text"`
	Buttons   *broadcastButtonsReq `json:"buttons,omitempty"`
	Media     *broadcastMediaReq   `json:"media,omitempty"`
}

func (req *broadcastSendReq) normalize() {
	req.Text = strings.TrimSpace(req.Text)
	req.Audience = strings.TrimSpace(req.Audience)
	if req.Media != nil {
		req.Media.FileID = strings.TrimSpace(req.Media.FileID)
		if req.Media.FileID == "" {
			req.Media = nil
		}
	}
	if req.Buttons != nil {
		for i := range req.Buttons.Links {
			req.Buttons.Links[i] = strings.TrimSpace(req.Buttons.Links[i])
		}
	}
}

// invalidLinkKeys возвращает ключи разделов кабинета, которых нет в реестре broadcast.
// Порядок и дубли чинит NormalizeCabinetLinkKeys уже при сборке клавиатуры; здесь важно
// не проглотить опечатку молча, иначе админ отправит рассылку без ожидаемой кнопки.
func (req *broadcastSendReq) invalidLinkKeys() []string {
	if req.Buttons == nil {
		return nil
	}
	var bad []string
	for _, key := range req.Buttons.Links {
		if !broadcast.IsCabinetLinkKey(key) {
			bad = append(bad, key)
		}
	}
	return bad
}

// resolveAudiences — сегменты запроса в нормальном виде. При ошибке сам
// отвечает клиенту 400 и возвращает ok = false.
func (req *broadcastSendReq) resolveAudiences(w http.ResponseWriter) (audiences []string, ok bool) {
	list := req.Audiences
	if len(list) == 0 && req.Audience != "" {
		list = []string{req.Audience}
	}
	audiences, err := database.NormalizeBroadcastAudiences(list)
	switch {
	case errors.Is(err, database.ErrBroadcastTestCombined):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	case err != nil:
		http.Error(w, "invalid audience", http.StatusBadRequest)
		return nil, false
	case len(audiences) == 0:
		http.Error(w, "audience required", http.StatusBadRequest)
		return nil, false
	}
	return audiences, true
}

func (req *broadcastSendReq) hasContent() bool {
	return req.Text != "" || req.Media != nil
}

func (req *broadcastSendReq) recipientButtons() broadcast.RecipientButtons {
	if req.Buttons == nil {
		return broadcast.RecipientButtons{}
	}
	return broadcast.RecipientButtons{
		Buy:           req.Buttons.Buy,
		Connect:       req.Buttons.Connect,
		Promo:         req.Buttons.Promo,
		MainMenu:      req.Buttons.MainMenu,
		ShareReferral: req.Buttons.ShareReferral,
		Links:         broadcast.NormalizeCabinetLinkKeys(req.Buttons.Links),
	}
}

func (req *broadcastSendReq) recipientMedia() *broadcast.Media {
	if req.Media == nil {
		return nil
	}
	kind := broadcast.MediaKind(strings.TrimSpace(req.Media.Kind))
	switch kind {
	case broadcast.MediaPhoto, broadcast.MediaVideo, broadcast.MediaDocument:
	default:
		// Незнакомый вид — отправляем файлом: он доедет любым, тогда как
		// SendPhoto на видео просто откажет.
		kind = broadcast.MediaDocument
	}
	return &broadcast.Media{
		FileID: req.Media.FileID,
		Kind:   kind,
	}
}

// Preview returns the count of recipients for a given audience/tariff combo.
func (h *AdminBroadcastHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req broadcastSendReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.normalize()
	audiences, ok := req.resolveAudiences(w)
	if !ok {
		return
	}

	recipients, err := h.customers.GetBroadcastRecipientsForAudiences(r.Context(), audiences, req.TariffID)
	if err != nil {
		slog.Error("admin broadcast: preview", "error", err)
		http.Error(w, "failed to count recipients", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"audiences":       audiences,
		"recipient_count": len(recipients),
	})
}

// Tariffs returns tariff list for audience filtering (tariffs mode).
func (h *AdminBroadcastHandler) Tariffs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tariffs, err := h.tariffs.ListActive(r.Context())
	if err != nil {
		slog.Error("admin broadcast: tariffs list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type tariffBrief struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	var items []tariffBrief
	for _, t := range tariffs {
		name := t.Slug
		if t.Name != nil {
			name = *t.Name
		}
		items = append(items, tariffBrief{ID: t.ID, Name: name, Slug: t.Slug})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tariffs": items})
}

// UploadMedia uploads an image to Telegram and returns file_id for broadcast reuse.
func (h *AdminBroadcastHandler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.bot == nil || h.sender == nil {
		http.Error(w, "broadcast unavailable", http.StatusServiceUnavailable)
		return
	}

	adminID := config.GetAdminTelegramId()
	if adminID == 0 {
		http.Error(w, "admin telegram id not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, broadcastMediaMaxBytes+1024)
	if err := r.ParseMultipartForm(broadcastMediaMaxBytes); err != nil { //nolint:gosec // G120: тело ограничено MaxBytesReader строкой выше
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("media")
	if err != nil {
		http.Error(w, "media file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, broadcastMediaMaxBytes))
	if err != nil {
		http.Error(w, "failed to read media", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty media file", http.StatusBadRequest)
		return
	}

	filename := "broadcast.jpg"
	if header != nil && strings.TrimSpace(header.Filename) != "" {
		filename = strings.TrimSpace(header.Filename)
	}

	contentType := ""
	if header != nil {
		contentType = strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = strings.ToLower(strings.TrimSpace(mime.TypeByExtension(filename)))
	}
	kind, ok := broadcastMediaContentType(contentType)
	if !ok {
		http.Error(w, "unsupported media type (jpeg, png, webp, mp4, webm, mov only)", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	upload := &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)}
	var sent *models.Message
	switch kind {
	case broadcast.MediaVideo:
		sent, err = h.bot.SendVideo(ctx, &bot.SendVideoParams{ChatID: adminID, Video: upload})
	case broadcast.MediaPhoto:
		sent, err = h.bot.SendPhoto(ctx, &bot.SendPhotoParams{ChatID: adminID, Photo: upload})
	default:
		sent, err = h.bot.SendDocument(ctx, &bot.SendDocumentParams{ChatID: adminID, Document: upload})
	}
	if err != nil {
		slog.Error("admin broadcast: upload media", "error", err)
		http.Error(w, "failed to upload media to telegram", http.StatusBadGateway)
		return
	}

	fileID, sentKind := extractTelegramFileID(sent)
	if fileID == "" {
		http.Error(w, "failed to resolve telegram file_id", http.StatusBadGateway)
		return
	}

	if sent != nil {
		_, _ = h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    adminID,
			MessageID: sent.ID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"file_id": fileID,
		"kind":    string(sentKind),
	})
}

// broadcastMediaContentType — чем отправлять файл, который принесла админка.
//
// Видео не «картинка покрупнее»: SendPhoto на mp4 просто откажет, а SendDocument
// доставит его файлом, который в чате не проигрывается.
func broadcastMediaContentType(contentType string) (kind broadcast.MediaKind, ok bool) {
	switch contentType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return broadcast.MediaPhoto, true
	case "video/mp4", "video/webm", "video/quicktime":
		return broadcast.MediaVideo, true
	default:
		return "", false
	}
}

// extractTelegramFileID — какой file_id и какого вида получился после загрузки.
//
// Смотрим на то, чем сообщение стало у Telegram, а не на то, чем мы его
// отправляли: mov без нужных кодеков он кладёт в Document, и слать такой файл
// потом методом SendVideo бессмысленно.
func extractTelegramFileID(msg *models.Message) (fileID string, kind broadcast.MediaKind) {
	if msg == nil {
		return "", ""
	}
	if len(msg.Photo) > 0 {
		last := msg.Photo[len(msg.Photo)-1]
		return last.FileID, broadcast.MediaPhoto
	}
	if msg.Video != nil {
		return msg.Video.FileID, broadcast.MediaVideo
	}
	if msg.Document != nil {
		return msg.Document.FileID, broadcast.MediaDocument
	}
	return "", ""
}

// Send starts a Telegram broadcast using the same delivery logic as TG admin panel.
func (h *AdminBroadcastHandler) Send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.bot == nil || h.sender == nil {
		http.Error(w, "broadcast unavailable", http.StatusServiceUnavailable)
		return
	}

	var req broadcastSendReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.normalize()
	audiences, ok := req.resolveAudiences(w)
	if !ok {
		return
	}
	if !req.hasContent() {
		http.Error(w, "text or media required", http.StatusBadRequest)
		return
	}
	if bad := req.invalidLinkKeys(); len(bad) > 0 {
		http.Error(w, "unknown cabinet link buttons: "+strings.Join(bad, ", "), http.StatusBadRequest)
		return
	}

	recipients, err := h.customers.GetBroadcastRecipientsForAudiences(r.Context(), audiences, req.TariffID)
	if err != nil {
		slog.Error("admin broadcast: send", "error", err)
		http.Error(w, "failed to get recipients", http.StatusInternalServerError)
		return
	}
	if len(recipients) == 0 {
		if audiences[0] == database.BroadcastAudienceTestBroadcast {
			// Тестовая рассылка идёт админу — пустой список значит только одно.
			http.Error(w, "test broadcast requires ADMIN_TELEGRAM_ID", http.StatusBadRequest)
			return
		}
		http.Error(w, "no recipients", http.StatusBadRequest)
		return
	}
	if !tryAcquireBroadcastSend() {
		http.Error(w, "broadcast already in progress", http.StatusConflict)
		return
	}

	adminID := config.GetAdminTelegramId()
	recipientCount := len(recipients)
	msg := broadcast.Message{
		Text: req.Text,
		/*
		 * Редактор web-админки вставляет теги Telegram HTML (<b>, <i>,
		 * <a href>), и без ParseMode они уходили получателю как обычный
		 * текст: человек видел «<b>Заголовок</b>» вместо жирного.
		 *
		 * Entities здесь не передаём: Bot API принимает либо разметку,
		 * либо готовые entities, и второе — способ рассылки из
		 * Telegram-админки, где они копируются из сообщения админа.
		 */
		ParseMode: models.ParseModeHTML,
		Media:     req.recipientMedia(),
		Buttons:   req.recipientButtons(),
	}
	opts := broadcast.SendOptions{Source: database.BroadcastSourceCabinet}
	if req.Media != nil {
		opts.MediaName = strings.TrimSpace(req.Media.Name)
	}

	/*
	 * Запись истории заводим здесь, а не в фоне: её id уходит в ответ, и по
	 * нему страница рассылки показывает ход и итог. Не завелась — рассылка
	 * всё равно уходит, просто без карточки итога в кабинете.
	 */
	if h.history != nil {
		rec := broadcast.HistoryRecord(opts.Source, audiences, req.TariffID, msg, opts.MediaName)
		rec.Total = recipientCount
		id, err := h.history.Create(r.Context(), rec)
		if err != nil {
			slog.Warn("admin broadcast: history create", "error", err)
		}
		opts.BroadcastID = id
	}

	utils.Go("cabinet broadcast", func() {
		defer releaseBroadcastSend()
		ctx := context.Background()
		if adminID != 0 {
			_, _ = h.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: adminID,
				Text:   "⏳ Рассылка из web-админки запущена. Ожидайте итоговое сообщение…",
			})
		}
		h.sender.Send(ctx, h.bot, adminID, audiences, req.TariffID, msg, opts)
	})

	slog.Info("admin broadcast: send started",
		"audiences", audiences,
		"recipients", recipientCount,
		"text_len", len(req.Text),
		"has_media", req.Media != nil,
	)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":          "started",
		"recipient_count": recipientCount,
		// 0 — история не записалась; кабинет тогда просто не покажет ход.
		"broadcast_id": opts.BroadcastID,
	})
}

// broadcastHistoryResp — рассылка в истории, как её видит кабинет.
type broadcastHistoryResp struct {
	ID        int64    `json:"id"`
	Source    string   `json:"source"`
	Audiences []string `json:"audiences"`
	TariffID  *int64   `json:"tariff_id"`
	Text      string   `json:"text"`
	// parse_mode пустой — текст без разметки (рассылка из Telegram-админки).
	ParseMode   string          `json:"parse_mode"`
	MediaKind   string          `json:"media_kind"`
	MediaFileID string          `json:"media_file_id"`
	MediaName   string          `json:"media_name"`
	Buttons     json.RawMessage `json:"buttons"`
	Status      string          `json:"status"`
	Total       int             `json:"total"`
	Sent        int             `json:"sent"`
	Blocked     int             `json:"blocked"`
	Failed      int             `json:"failed"`
	Error       string          `json:"error"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
}

func toBroadcastHistoryResp(rec database.BroadcastRecord) broadcastHistoryResp {
	buttons := rec.Buttons
	if len(buttons) == 0 {
		buttons = json.RawMessage(`{}`)
	}
	audiences := rec.Audiences
	if audiences == nil {
		audiences = []string{}
	}
	return broadcastHistoryResp{
		ID:          rec.ID,
		Source:      rec.Source,
		Audiences:   audiences,
		TariffID:    rec.TariffID,
		Text:        rec.Text,
		ParseMode:   rec.ParseMode,
		MediaKind:   rec.MediaKind,
		MediaFileID: rec.MediaFileID,
		MediaName:   rec.MediaName,
		Buttons:     buttons,
		Status:      rec.Status,
		Total:       rec.Total,
		Sent:        rec.Sent,
		Blocked:     rec.Blocked,
		Failed:      rec.Failed,
		Error:       rec.Error,
		StartedAt:   rec.StartedAt,
		FinishedAt:  rec.FinishedAt,
	}
}

const (
	broadcastHistoryDefaultLimit = 20
	broadcastHistoryMaxLimit     = 50
)

// History — страница истории рассылок, свежие сверху.
func (h *AdminBroadcastHandler) History(w http.ResponseWriter, r *http.Request) {
	if h.history == nil {
		http.Error(w, "broadcast history unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	limit := broadcastHistoryDefaultLimit
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, broadcastHistoryMaxLimit)
	}
	offset := 0
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}

	records, total, err := h.history.List(r.Context(), limit, offset)
	if err != nil {
		slog.Error("admin broadcast: history list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	items := make([]broadcastHistoryResp, 0, len(records))
	for _, rec := range records {
		items = append(items, toBroadcastHistoryResp(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// HistoryItem — одна рассылка: по ней страница следит за ходом отправки.
func (h *AdminBroadcastHandler) HistoryItem(w http.ResponseWriter, r *http.Request) {
	if h.history == nil {
		http.Error(w, "broadcast history unavailable", http.StatusServiceUnavailable)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/cabinet/api/admin/broadcast/history/")
	id, err := strconv.ParseInt(strings.Trim(raw, "/"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	rec, err := h.history.Get(r.Context(), id)
	if err != nil {
		slog.Error("admin broadcast: history item", "id", id, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toBroadcastHistoryResp(*rec))
}

// CountByAudience is a helper endpoint to count for specific audience via query param.
func (h *AdminBroadcastHandler) CountByAudience(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	audience := r.URL.Query().Get("audience")
	if audience == "" {
		audience = database.BroadcastAudienceAll
	}

	var tariffID *int64
	if t := r.URL.Query().Get("tariff_id"); t != "" {
		v, err := strconv.ParseInt(t, 10, 64)
		if err == nil {
			tariffID = &v
		}
	}

	recipients, err := h.customers.GetBroadcastRecipients(r.Context(), audience, tariffID)
	if err != nil {
		slog.Error("admin broadcast: count", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"audience":        audience,
		"recipient_count": len(recipients),
	})
}
