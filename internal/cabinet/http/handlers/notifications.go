package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/database"
)

// NotificationsHandler — колокольчик кабинета: лента, счётчик, отметки
// «прочитано» и подписка браузера на push.
type NotificationsHandler struct {
	links *repository.AccountCustomerLinkRepo
	svc   *cabinetnotify.Service
}

func NewNotifications(links *repository.AccountCustomerLinkRepo, svc *cabinetnotify.Service) *NotificationsHandler {
	return &NotificationsHandler{links: links, svc: svc}
}

type notificationButtonDTO struct {
	Key string `json:"key"`
}

type notificationDTO struct {
	ID        int64                   `json:"id"`
	Kind      string                  `json:"kind"`
	Important bool                    `json:"important"`
	Title     string                  `json:"title"`
	Body      string                  `json:"body"`
	Buttons   []notificationButtonDTO `json:"buttons"`
	CreatedAt time.Time               `json:"created_at"`
	Read      bool                    `json:"read"`
}

type notificationsUnreadDTO struct {
	Total     int `json:"total"`
	Important int `json:"important"`
	// LatestImportant — плашка на главной: свежее непрочитанное важное.
	LatestImportant *notificationDTO `json:"latest_important"`
}

func toNotificationDTO(it database.CabinetNotificationItem) notificationDTO {
	buttons := make([]notificationButtonDTO, 0, len(it.Buttons))
	for _, b := range it.Buttons {
		buttons = append(buttons, notificationButtonDTO{Key: b.Key})
	}
	return notificationDTO{
		ID:        it.ID,
		Kind:      it.Kind,
		Important: it.Important,
		Title:     it.Title,
		Body:      it.Body,
		Buttons:   buttons,
		CreatedAt: it.CreatedAt,
		Read:      it.ReadAt != nil,
	}
}

// customerID — клиент бота за аккаунтом кабинета. Нет связки — уведомлений
// у человека просто нет (ok = true, id = 0), это не ошибка.
func (h *NotificationsHandler) customerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	if h.svc == nil {
		http.Error(w, "notifications unavailable", http.StatusServiceUnavailable)
		return 0, false
	}
	link, err := h.links.FindByAccountID(r.Context(), claims.AccountID)
	if err != nil || link == nil {
		return 0, true
	}
	return link.CustomerID, true
}

func (h *NotificationsHandler) unread(r *http.Request, customerID int64) (notificationsUnreadDTO, error) {
	out := notificationsUnreadDTO{}
	if customerID == 0 {
		return out, nil
	}
	repo := h.svc.Repo()
	u, err := repo.Unread(r.Context(), customerID)
	if err != nil {
		return out, err
	}
	out.Total, out.Important = u.Total, u.Important
	if u.Important > 0 {
		it, err := repo.LatestUnreadImportant(r.Context(), customerID)
		if err != nil {
			return out, err
		}
		if it != nil {
			dto := toNotificationDTO(*it)
			out.LatestImportant = &dto
		}
	}
	return out, nil
}

const (
	notificationsDefaultLimit = 20
	notificationsMaxLimit     = 50
)

// List — GET /me/notifications?limit=&offset=.
func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := notificationsDefaultLimit
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, notificationsMaxLimit)
	}
	offset := 0
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}
	items := []notificationDTO{}
	total := 0
	if customerID != 0 {
		list, n, err := h.svc.Repo().List(r.Context(), customerID, limit, offset)
		if err != nil {
			slog.Error("notifications: list", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		total = n
		for _, it := range list {
			items = append(items, toNotificationDTO(it))
		}
	}
	unread, err := h.unread(r, customerID)
	if err != nil {
		slog.Error("notifications: unread", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "unread": unread})
}

// Unread — GET /me/notifications/unread: счётчик колокольчика и плашка важного.
func (h *NotificationsHandler) Unread(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	unread, err := h.unread(r, customerID)
	if err != nil {
		slog.Error("notifications: unread", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, unread)
}

type notificationsReadReq struct {
	IDs []int64 `json:"ids"`
	All bool    `json:"all"`
}

// maxReadBatch — сколько id принимает одна отметка. Лента отдаёт по 50, и
// фронт копит отметки пачкой — больше в одном запросе не бывает.
const maxReadBatch = 200

// Read — POST /me/notifications/read: {"ids":[…]} или {"all":true}.
func (h *NotificationsHandler) Read(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	var req notificationsReadReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) > maxReadBatch {
		req.IDs = req.IDs[:maxReadBatch]
	}
	if customerID != 0 {
		repo := h.svc.Repo()
		var err error
		if req.All {
			err = repo.MarkAllRead(r.Context(), customerID)
		} else {
			err = repo.MarkRead(r.Context(), customerID, req.IDs)
		}
		if err != nil {
			slog.Error("notifications: mark read", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	unread, err := h.unread(r, customerID)
	if err != nil {
		slog.Error("notifications: unread", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, unread)
}

// Delete — POST /me/notifications/delete: {"ids":[…]} — крестик на уведомлении.
// Убирает только у этого человека: у остальных получателей рассылка остаётся.
func (h *NotificationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	var req notificationsReadReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) > maxReadBatch {
		req.IDs = req.IDs[:maxReadBatch]
	}
	if customerID != 0 {
		if err := h.svc.Repo().Hide(r.Context(), customerID, req.IDs); err != nil {
			slog.Error("notifications: hide", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	unread, err := h.unread(r, customerID)
	if err != nil {
		slog.Error("notifications: unread", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, unread)
}

// PushKey — GET /me/push/key: публичный VAPID-ключ для pushManager.subscribe.
func (h *NotificationsHandler) PushKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.customerID(w, r); !ok {
		return
	}
	key, err := h.svc.VapidPublicKey(r.Context())
	if err != nil {
		slog.Error("notifications: vapid key", "error", err)
		http.Error(w, "push unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": key})
}

// pushSubscribeReq — PushSubscription.toJSON() браузера как есть. decodeJSON
// отвергает незнакомые поля, а браузер всегда кладёт expirationTime (обычно
// null): без этого поля в структуре каждая подписка получала 400.
type pushSubscribeReq struct {
	Endpoint       string   `json:"endpoint"`
	ExpirationTime *float64 `json:"expirationTime"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// pushEndpointOK — endpoint push-сервиса браузера. Только https и разумной
// длины: адрес потом дёргает сервер, и подсунуть туда внутренний хост нельзя.
func pushEndpointOK(endpoint string) bool {
	return strings.HasPrefix(endpoint, "https://") && len(endpoint) <= 1024
}

// PushSubscribe — POST /me/push/subscribe: подписка этого браузера.
func (h *NotificationsHandler) PushSubscribe(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	var req pushSubscribeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	if !pushEndpointOK(req.Endpoint) || req.Keys.P256dh == "" || req.Keys.Auth == "" ||
		len(req.Keys.P256dh) > 256 || len(req.Keys.Auth) > 128 {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if customerID == 0 {
		http.Error(w, "no customer", http.StatusConflict)
		return
	}
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	err := h.svc.Repo().UpsertPushSubscription(r.Context(), database.CabinetPushSubscription{
		CustomerID: customerID,
		Endpoint:   req.Endpoint,
		P256dh:     req.Keys.P256dh,
		Auth:       req.Keys.Auth,
	}, ua)
	if err != nil {
		slog.Error("notifications: push subscribe", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": true})
}

// PushUnsubscribe — POST /me/push/unsubscribe: {"endpoint": "…"}.
func (h *NotificationsHandler) PushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	customerID, ok := h.customerID(w, r)
	if !ok {
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if customerID != 0 && strings.TrimSpace(req.Endpoint) != "" {
		if err := h.svc.Repo().DeletePushSubscription(r.Context(), customerID, strings.TrimSpace(req.Endpoint)); err != nil {
			slog.Error("notifications: push unsubscribe", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": false})
}
