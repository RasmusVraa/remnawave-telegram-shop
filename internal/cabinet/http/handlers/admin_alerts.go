package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/adminalert"
)

// AdminAlerts — кнопка «Тест» в настройках «Оповещения админу».
type AdminAlerts struct {
	test func(ctx context.Context, kind string, draft *adminalert.Draft) (adminalert.TestResult, error)

	mu   sync.Mutex
	last time.Time
}

func NewAdminAlerts() *AdminAlerts {
	return &AdminAlerts{test: adminalert.Test}
}

// adminAlertTestGap — не чаще одного теста за столько: кнопку легко нажать
// десять раз подряд, а это десять сообщений в рабочую группу.
const adminAlertTestGap = 3 * time.Second

// adminAlertTestReq: без draft — маршрут из сохранённых настроек; с draft —
// значения из формы (строки, как в полях ввода; пусто — не задано).
type adminAlertTestReq struct {
	Kind  string `json:"kind"`
	Draft *struct {
		SharedChatID string `json:"shared_chat_id"`
		ChatID       string `json:"chat_id"`
		ThreadID     string `json:"thread_id"`
	} `json:"draft,omitempty"`
}

func parseDraftInt(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil
}

// Test шлёт пробное сообщение по маршруту вида и отвечает, дошло ли.
// Ошибка Telegram — это 200 с ok=false: админ должен увидеть причину, а не «500».
func (h *AdminAlerts) Test(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adminAlertTestReq
	if !decodeJSON(w, r, &req) {
		return
	}
	var draft *adminalert.Draft
	if req.Draft != nil {
		shared, ok1 := parseDraftInt(req.Draft.SharedChatID)
		chat, ok2 := parseDraftInt(req.Draft.ChatID)
		thread, ok3 := parseDraftInt(req.Draft.ThreadID)
		if !ok1 || !ok2 || !ok3 || thread < 0 || thread > 1<<31-1 {
			http.Error(w, "invalid chat or thread id", http.StatusBadRequest)
			return
		}
		draft = &adminalert.Draft{SharedChat: shared, Chat: chat, Thread: int(thread)}
	}
	h.mu.Lock()
	if wait := adminAlertTestGap - time.Since(h.last); wait > 0 {
		h.mu.Unlock()
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	h.last = time.Now()
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	res, err := h.test(ctx, req.Kind, draft)
	switch {
	case errors.Is(err, adminalert.ErrUnknownKind):
		http.Error(w, "unknown kind", http.StatusBadRequest)
		return
	case err != nil:
		slog.Error("admin alert test", "kind", req.Kind, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
