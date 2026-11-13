package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	botcfg "remnawave-tg-shop-bot/internal/config"
)

type SupportHandler struct {
	svc *cabsvc.Support
}

func NewSupport(svc *cabsvc.Support) *SupportHandler {
	return &SupportHandler{svc: svc}
}

type supportSendReq struct {
	Text string `json:"text"`
}

func (h *SupportHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	out, err := h.svc.Summary(r.Context(), claims.AccountID)
	if err != nil {
		writeSupportErr(w, err, "support.summary")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SupportHandler) Conversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	out, err := h.svc.Conversation(r.Context(), claims.AccountID)
	if err != nil {
		writeSupportErr(w, err, "support.conversation")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SupportHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	var in supportSendReq
	var files []cabsvc.SupportUpload
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "multipart/form-data" {
		var err error
		in.Text, files, err = readSupportMultipart(w, r)
		if err != nil {
			writeSupportErr(w, err, "support.send")
			return
		}
	} else if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.SendMessage(r.Context(), claims.AccountID, in.Text, files)
	if err != nil {
		writeSupportErr(w, err, "support.send")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// supportTextMaxBytes — поле text в multipart: 4000 символов по 4 байта с запасом.
const supportTextMaxBytes = 1 << 15

// readSupportMultipart читает multipart-сообщение потоком: поле text и части files.
// Лимиты по числу и размеру файлов проверяются на ходу, не дожидаясь конца тела;
// формат файлов проверяет сервис.
func readSupportMultipart(w http.ResponseWriter, r *http.Request) (string, []cabsvc.SupportUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, cabsvc.SupportMaxTotalBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, cabsvc.ErrSupportInvalidMsg
	}
	var text string
	var files []cabsvc.SupportUpload
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, multipartReadErr(err)
		}
		switch part.FormName() {
		case "text":
			b, err := io.ReadAll(io.LimitReader(part, supportTextMaxBytes+1))
			if err != nil {
				return "", nil, multipartReadErr(err)
			}
			if len(b) > supportTextMaxBytes {
				return "", nil, cabsvc.ErrSupportInvalidMsg
			}
			text = string(b)
		case "files":
			if len(files) >= cabsvc.SupportMaxFiles {
				return "", nil, cabsvc.ErrSupportTooManyFiles
			}
			b, err := io.ReadAll(io.LimitReader(part, cabsvc.SupportMaxFileBytes+1))
			if err != nil {
				return "", nil, multipartReadErr(err)
			}
			if len(b) > cabsvc.SupportMaxFileBytes {
				return "", nil, cabsvc.ErrSupportFileTooLarge
			}
			files = append(files, cabsvc.SupportUpload{Name: part.FileName(), Data: b})
		}
		_ = part.Close()
	}
	return text, files, nil
}

func multipartReadErr(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return cabsvc.ErrSupportFileTooLarge
	}
	return cabsvc.ErrSupportInvalidMsg
}

// Attachment — GET /support/attachments/{id}: изображение из чата этого аккаунта.
func (h *SupportHandler) Attachment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	data, mimeType, err := h.svc.Attachment(r.Context(), claims.AccountID, id)
	if errors.Is(err, repository.ErrSupportNotFound) {
		// Чужое и несуществующее одинаково: не подсказываем, что такой id есть.
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeSupportErr(w, err, "support.attachment")
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// Вложение не меняется: браузер берёт его из кэша, пока открыт чат и после.
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *SupportHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	if err := h.svc.MarkRead(r.Context(), claims.AccountID); err != nil {
		writeSupportErr(w, err, "support.read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *SupportHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "support chat disabled", http.StatusNotFound)
		return
	}
	if !supportWebhookAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var p cabsvc.SupportWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.svc.HandleWebhook(r.Context(), p); err != nil {
		writeSupportErr(w, err, "support.webhook")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func supportWebhookAuthorized(r *http.Request) bool {
	secret := botcfg.SupportBridgeSecret()
	if secret == "" {
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

func writeSupportErr(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, cabsvc.ErrSupportDisabled):
		http.Error(w, "support chat disabled", http.StatusNotFound)
	case errors.Is(err, cabsvc.ErrSupportInvalidMsg):
		http.Error(w, "invalid message", http.StatusBadRequest)
	case errors.Is(err, cabsvc.ErrSupportTooManyFiles):
		http.Error(w, "too many files", http.StatusBadRequest)
	case errors.Is(err, cabsvc.ErrSupportFileTooLarge):
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, cabsvc.ErrSupportUnsupportedMedia):
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
	case errors.Is(err, cabsvc.ErrSupportBridge):
		slog.Warn("support bridge failed", "op", op, "error", err)
		http.Error(w, "failed to send message", http.StatusBadGateway)
	default:
		writeServiceErr(w, err, op)
	}
}
