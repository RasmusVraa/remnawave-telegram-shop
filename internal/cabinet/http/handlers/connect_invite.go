package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/connectinvite"
	"remnawave-tg-shop-bot/internal/cabinet/deeplink"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
)

// ConnectInviteHandler — приглашение «подключить ещё одно устройство».
//
// Задача пары эндпоинтов: перенести подписку на устройство, с которого в
// кабинет не зайти (второй телефон, чужой компьютер, телевизор). Владелец
// получает короткую ссылку с токеном, пересылает её или показывает QR, а
// получатель открывает страницу /connect и подключается без авторизации.
//
//   - Create (за RequireAuth) — выпускает токен приглашения;
//   - Resolve (публичный)     — меняет токен на deep link подключения.
//
// Сырая ссылка подписки уходит наружу только в plain-режиме, то есть когда
// шифрование deep link выключено администратором и ссылка и так открыто
// подставляется в happ://add/ на обычной странице подключения. Если хотя бы
// одно шифрование включено, Resolve отдаёт только зашифрованные ссылки: новый
// канал не должен быть слабее самого строгого настроенного режима.
type ConnectInviteHandler struct {
	svc    *cabsvc.Subscription
	secret []byte
	// deeplinkCache гасит повторные обращения к внешнему crypto.happ.su:
	// публичный эндпоинт открыт без авторизации, и один разосланный токен
	// легко даёт десятки открытий страницы.
	deeplinkCache *deeplinkCache
}

// NewConnectInvite — конструктор. secret — тот же секрет кабинета, что
// подписывает JWT; connectinvite выводит из него собственный ключ.
func NewConnectInvite(svc *cabsvc.Subscription, secret []byte) *ConnectInviteHandler {
	return &ConnectInviteHandler{
		svc:           svc,
		secret:        secret,
		deeplinkCache: newDeeplinkCache(5 * time.Minute),
	}
}

type connectInviteResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Create — GET /cabinet/api/me/connect-invite.
//
// Стоит за теми же барьерами, что /me/subscription и /me/deeplink: токен
// обменивается на подписку, значит по чувствительности равен ей.
func (h *ConnectInviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Приглашение без подписки бессмысленно: получатель упрётся в пустую
	// страницу. Проверяем здесь, а не только в Resolve, чтобы владелец узнал
	// об этом сразу.
	resp, err := h.svc.Get(r.Context(), claims.AccountID)
	if err != nil {
		if handleAccountGone(w, err, "connect_invite.issue", claims.AccountID) {
			return
		}
		slog.Error("connect-invite: get subscription failed", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if resp.SubscriptionLink == nil || strings.TrimSpace(*resp.SubscriptionLink) == "" {
		http.Error(w, "no subscription", http.StatusConflict)
		return
	}

	token, expiresAt, err := connectinvite.Issue(h.secret, claims.AccountID, connectinvite.DefaultTTL, time.Now())
	if err != nil {
		slog.Error("connect-invite: issue failed", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, connectInviteResponse{
		URL:       connectInviteURL(token),
		ExpiresAt: expiresAt,
	})
}

// connectInviteURL собирает адрес страницы-приглашения.
//
// Токен уезжает во фрагмент (#t=...), а не в query: фрагмент не попадает ни в
// логи сервера, ни в Referer, ни в запрос превью-краулера мессенджера, который
// разворачивает ссылку в чате. SPA читает его на клиенте и меняет на deep link
// через Resolve.
func connectInviteURL(token string) string {
	base := strings.TrimRight(strings.TrimSpace(cabcfg.PublicURL()), "/")
	return base + "/cabinet/connect#t=" + token
}

type connectResolveResponse struct {
	// Mode — encrypted (отдаём только зашифрованные deep link) либо plain
	// (отдаём ссылку подписки, страница собирает deep link сама).
	Mode string `json:"mode"`
	// SubscriptionLink заполняется только в plain-режиме.
	SubscriptionLink string `json:"subscription_link,omitempty"`
	// Links — готовые deep link по ключам приложений (happ, incy).
	Links map[string]string `json:"links,omitempty"`
}

// Resolve — GET /cabinet/api/public/connect?t=<token>.
//
// Единственный публичный эндпоинт кабинета, отдающий подписку. Открыт
// намеренно: получатель приглашения не имеет и не может иметь аккаунта.
// Ограничители — неугадываемый токен, его срок жизни и rate-limit в роутере.
func (h *ConnectInviteHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("t"))
	accountID, expiresAt, err := connectinvite.ParseWithExpiry(h.secret, token, time.Now())
	if err != nil {
		if errors.Is(err, connectinvite.ErrExpired) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "invite_expired"})
			return
		}
		// Битый токен и неверная подпись отвечают одинаково: гостю разница не
		// нужна, а перебирающему — тем более.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite_invalid"})
		return
	}

	resp, err := h.svc.Get(r.Context(), accountID)
	if err != nil {
		// Аккаунт-владелец удалён — приглашение мертво. Гостю здесь не нужен
		// 401 (сессии у него нет): отвечаем так же, как на битый токен.
		if errors.Is(err, bootstrap.ErrAccountGone) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite_invalid"})
			return
		}
		slog.Error("connect-invite: resolve failed", "account_id", accountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Владелец сбросил ссылку подписки — значит, отзывал доступ. Приглашение,
	// выпущенное раньше, иначе выдало бы гостю уже новую ссылку: Resolve
	// берёт её из подписки в момент открытия.
	if resp.LinkResetAt != nil && connectinvite.IssuedBefore(expiresAt, *resp.LinkResetAt) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "invite_expired"})
		return
	}
	if resp.SubscriptionLink == nil || strings.TrimSpace(*resp.SubscriptionLink) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no_subscription"})
		return
	}
	if resp.ExpireAt != nil && !resp.ExpireAt.After(time.Now()) {
		// Подключать устройство к истёкшей подписке нечего — гость увидит
		// внятное «подписка закончилась» вместо неработающего VPN.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "subscription_expired"})
		return
	}
	subLink := strings.TrimSpace(*resp.SubscriptionLink)

	happEnabled := cabcfg.DeeplinkHappEncryptEnabled()
	incyEnabled := cabcfg.DeeplinkIncyEncryptEnabled()

	w.Header().Set("Cache-Control", "no-store")

	if !happEnabled && !incyEnabled {
		writeJSON(w, http.StatusOK, connectResolveResponse{
			Mode:             "plain",
			SubscriptionLink: subLink,
		})
		return
	}

	links := make(map[string]string, 2)
	var lastErr error
	if happEnabled {
		// Формат ссылки входит в ключ кэша: сменив его в админке, админ должен
		// увидеть эффект сразу, а не через TTL.
		happVersion := cabcfg.DeeplinkHappCryptVersion()
		href, encErr := h.deeplinkCache.get("happ:"+happVersion, subLink, func() (string, error) {
			return deeplink.EncryptHapp(r.Context(), subLink, happVersion)
		})
		if encErr != nil {
			lastErr = encErr
		} else {
			links["happ"] = href
		}
	}
	if incyEnabled {
		href, encErr := h.deeplinkCache.get("incy", subLink, func() (string, error) {
			return deeplink.EncryptINCY(subLink, cabcfg.BrandName())
		})
		if encErr != nil {
			lastErr = encErr
		} else {
			links["incy"] = href
		}
	}

	if len(links) == 0 {
		// Все включённые шифрования отвалились (обычно — недоступен
		// crypto.happ.su). Сырую ссылку в этом случае не отдаём: админ включил
		// шифрование осознанно, и обход настройки из-за сетевого сбоя — не то
		// поведение, которого от нас ждут.
		slog.Error("connect-invite: encrypt failed", "account_id", accountID, "error", lastErr.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "deeplink_unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, connectResolveResponse{Mode: "encrypted", Links: links})
}

// deeplinkCache — TTL-кэш зашифрованных deep link, общий на процесс.
type deeplinkCache struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]deeplinkCacheEntry
}

type deeplinkCacheEntry struct {
	link      string
	expiresAt time.Time
}

// deeplinkCacheMaxEntries — потолок против роста на длинном хвосте подписок;
// при переполнении кэш сбрасывается целиком (дешевле LRU, промах стоит одного
// повторного шифрования).
const deeplinkCacheMaxEntries = 1024

func newDeeplinkCache(ttl time.Duration) *deeplinkCache {
	return &deeplinkCache{ttl: ttl, entries: make(map[string]deeplinkCacheEntry)}
}

// get возвращает готовую ссылку из кэша либо вычисляет её через build.
func (c *deeplinkCache) get(app, subLink string, build func() (string, error)) (string, error) {
	key := app + "|" + subLink
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Before(e.expiresAt) {
		c.mu.Unlock()
		return e.link, nil
	}
	c.mu.Unlock()

	// Шифрование Happ — сетевой вызов, поэтому вне мьютекса: параллельные
	// запросы по одной подписке в худшем случае сделают лишний вызов.
	link, err := build()
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	if len(c.entries) >= deeplinkCacheMaxEntries {
		c.entries = make(map[string]deeplinkCacheEntry, deeplinkCacheMaxEntries)
	}
	c.entries[key] = deeplinkCacheEntry{link: link, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()

	return link, nil
}
