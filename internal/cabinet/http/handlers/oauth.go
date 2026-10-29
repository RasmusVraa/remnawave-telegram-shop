package handlers

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	googleoauth "remnawave-tg-shop-bot/internal/cabinet/auth/oauth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
)

// OAuthHandler — эндпоинты /cabinet/api/auth/google/* и /cabinet/api/auth/telegram.
//
// Google-флоу:
//
//	GET  /auth/google/start         → редирект на Google
//	GET  /auth/google/callback      → обмен code, login или link-required
//	GET  /auth/google/confirm       → подтверждение привязки по email-токену
//
// Telegram-флоу:
//
//	POST /auth/telegram             → Widget или MiniApp (поле source)
type OAuthHandler struct {
	svc          *service.Service
	cookieDomain string
}

// NewOAuth — конструктор.
func NewOAuth(svc *service.Service, cookieDomain string) *OAuthHandler {
	return &OAuthHandler{svc: svc, cookieDomain: cookieDomain}
}

// ============================================================================
// Google
// ============================================================================

// GoogleStart — GET /cabinet/api/auth/google/start.
// Генерирует state+PKCE и редиректит браузер на accounts.google.com.
// Опционально: ?ref=ref_<tg> — сохраняется в server-side state до callback (новая регистрация).
func (h *OAuthHandler) GoogleStart(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	redirectURL, err := h.svc.GoogleStart(ref)
	if err != nil {
		if errors.Is(err, service.ErrGoogleDisabled) {
			http.Error(w, "google oauth disabled", http.StatusNotImplemented)
			return
		}
		slog.Error("google start failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setOAuthStateCookie(w, redirectURL, h.cookieDomain)
	http.Redirect(w, r, redirectURL, http.StatusFound) //nolint:gosec // G710: адрес начала OAuth строит сервер, не пользователь
}

// GoogleCallback — GET /cabinet/api/auth/google/callback?code=...&state=...
// Обменивает code, создаёт/находит аккаунт и выдаёт сессию.
//
// Возможные ответы (браузерный redirect_uri — всегда редирект в SPA, не JSON):
//
//	302 → /cabinet/dashboard — вход/регистрация ok (Set-Cookie: refresh)
//	302 → /cabinet/login?google_link=pending&masked_email=… — нужно подтверждение по письму
//	302 → /cabinet/accounts?google_link_error=… — привязка Google к сессии (ошибка)
//	400/403/500 — ошибка без редиректа
func (h *OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		cabmetrics.RecordAuth("google_callback", "client_error")
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}
	if !consumeOAuthState(w, r, state, h.cookieDomain) {
		http.Redirect(w, r, oauthStateMismatchRedirect, http.StatusFound)
		return
	}

	result, err := h.svc.GoogleCallback(r.Context(), state, code,
		r.UserAgent(), middleware.ClientIP(r), service.RefreshCookieFromRequest(r))
	if err != nil {
		if result.WasLinkAttempt {
			to := "/cabinet/accounts?status=error&reason_code=google_link_unknown"
			switch {
			case errors.Is(err, service.ErrInvalidToken):
				to = "/cabinet/accounts?status=error&reason_code=google_link_session_invalid"
			case errors.Is(err, service.ErrGoogleLinkSessionMismatch):
				to = "/cabinet/accounts?status=error&reason_code=google_link_session_mismatch"
			case errors.Is(err, service.ErrGoogleMergeRequired):
				to = "/cabinet/link/merge?status=merge_required&reason_code=google_merge_candidate_detected&auto=1&provider=google"
			case errors.Is(err, service.ErrGoogleLinkedElsewhere):
				to = "/cabinet/accounts?status=error&reason_code=social_account_occupied&provider=google"
			case errors.Is(err, service.ErrGoogleLinkEmailConflict):
				to = "/cabinet/accounts?status=error&reason_code=email_conflict_with_another_account&provider=google"
			case errors.Is(err, service.ErrInvalidCredentials):
				to = "/cabinet/accounts?status=error&reason_code=account_blocked"
			}
			cabmetrics.RecordAuth("google_callback", "link_flow_error")
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if errors.Is(err, service.ErrGoogleLinkRequired) && result.LinkCtx != nil {
			cabmetrics.RecordAuth("google_callback", "link_required")
			// Браузер пришёл с redirect_uri на API — отдаём JSON бесполезно; ведём в SPA.
			q := url.Values{}
			q.Set("status", "merge_verification_required")
			q.Set("reason_code", "google_link_email_confirmation_required")
			if me := strings.TrimSpace(result.LinkCtx.MaskedEmail); me != "" {
				q.Set("masked_email", me)
			}
			http.Redirect(w, r, "/cabinet/login?"+q.Encode(), http.StatusFound)
			return
		}
		if errors.Is(err, service.ErrGoogleLinkEmailConflict) {
			cabmetrics.RecordAuth("google_callback", "client_error")
			http.Redirect(w, r, "/cabinet/login?status=error&reason_code=email_conflict_with_another_account&provider=google", http.StatusFound)
			return
		}
		if errors.Is(err, service.ErrInvalidToken) {
			cabmetrics.RecordAuth("google_callback", "client_error")
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			cabmetrics.RecordAuth("google_callback", "failure")
			http.Error(w, "account blocked", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrGoogleDisabled) {
			cabmetrics.RecordAuth("google_callback", "client_error")
			http.Error(w, "google oauth disabled", http.StatusNotImplemented)
			return
		}
		slog.Error("google callback failed", "error", err)
		cabmetrics.RecordAuth("google_callback", "server_error")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cabmetrics.RecordAuth("google_callback", "success")
	setRefreshCookie(w, r, result.Pair, h.cookieDomain, "/cabinet/api/auth")
	if result.SuccessRedirect != "" {
		http.Redirect(w, r, result.SuccessRedirect, http.StatusFound)
		return
	}
	// Полный редирект в SPA: иначе пользователь «застревает» на URL колбэка с сырым JSON.
	http.Redirect(w, r, "/cabinet/dashboard", http.StatusFound)
}

// GoogleLinkConfirm — GET /cabinet/api/auth/google/confirm?token=...
// Подтверждает привязку Google к существующему аккаунту по токену из письма.
func (h *OAuthHandler) GoogleLinkConfirm(w http.ResponseWriter, r *http.Request) {
	// GET только показывает кнопку: ссылки из писем открывают почтовые сканеры,
	// и привязка по GET происходила бы без участия человека. Форму сканеры не
	// отправляют — привязка идёт по POST.
	if r.Method == http.MethodGet {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = oauthConfirmPage.Execute(w, map[string]string{"Action": r.URL.Path, "Token": token})
		return
	}
	token := r.PostFormValue("token")
	if token == "" {
		cabmetrics.RecordAuth("google_link_confirm", "client_error")
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	pair, err := h.svc.GoogleLinkConfirm(r.Context(), token, r.UserAgent(), middleware.ClientIP(r))
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			cabmetrics.RecordAuth("google_link_confirm", "client_error")
			http.Error(w, "invalid or expired token", http.StatusBadRequest)
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			cabmetrics.RecordAuth("google_link_confirm", "failure")
			http.Error(w, "account blocked", http.StatusForbidden)
			return
		}
		slog.Error("oauth link confirm failed", "error", err)
		cabmetrics.RecordAuth("google_link_confirm", "server_error")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cabmetrics.RecordAuth("google_link_confirm", "success")
	setRefreshCookie(w, r, pair, h.cookieDomain, "/cabinet/api/auth")
	http.Redirect(w, r, "/cabinet/dashboard", http.StatusSeeOther)
}

// oauthConfirmPage — страница подтверждения привязки по ссылке из письма.
// Без скриптов и стилей: CSP кабинета их не пропускает, а здесь они не нужны.
var oauthConfirmPage = template.Must(template.New("oauth-confirm").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Подтверждение входа</title></head>
<body>
<h1>Подтвердите привязку способа входа</h1>
<p>Нажмите кнопку, чтобы привязать вход через внешний сервис к вашему кабинету.
Если вы этого не запрашивали, просто закройте страницу.</p>
<form method="post" action="{{.Action}}">
<input type="hidden" name="token" value="{{.Token}}">
<button type="submit">Подтвердить привязку</button>
</form>
</body></html>`))

// redirectOAuthLoginError ведёт браузер в SPA при отказе входа через Яндекс/VK.
func redirectOAuthLoginError(w http.ResponseWriter, r *http.Request, err error, result service.GoogleCallbackResult, provider string) bool {
	switch {
	case errors.Is(err, service.ErrGoogleLinkRequired) && result.LinkCtx != nil:
		// Email провайдера совпал с существующим аккаунтом — ждём подтверждения по письму.
		q := url.Values{}
		q.Set("status", "merge_verification_required")
		q.Set("reason_code", "oauth_link_email_confirmation_required")
		q.Set("provider", provider)
		if me := strings.TrimSpace(result.LinkCtx.MaskedEmail); me != "" {
			q.Set("masked_email", me)
		}
		http.Redirect(w, r, "/cabinet/login?"+q.Encode(), http.StatusFound)
		return true
	case errors.Is(err, service.ErrInvalidCredentials):
		http.Redirect(w, r, "/cabinet/login?status=error&reason_code=account_blocked&provider="+provider, http.StatusFound)
		return true
	case errors.Is(err, service.ErrGoogleLinkEmailConflict):
		http.Redirect(w, r, "/cabinet/login?status=error&reason_code=email_conflict_with_another_account&provider="+provider, http.StatusFound)
		return true
	}
	return false
}

func (h *OAuthHandler) YandexStart(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	redirectURL, err := h.svc.YandexStart(ref)
	if err != nil {
		if errors.Is(err, service.ErrYandexDisabled) {
			http.Error(w, "yandex oauth disabled", http.StatusNotImplemented)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setOAuthStateCookie(w, redirectURL, h.cookieDomain)
	http.Redirect(w, r, redirectURL, http.StatusFound) //nolint:gosec // G710: адрес начала OAuth строит сервер, не пользователь
}

func (h *OAuthHandler) YandexCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}
	if !consumeOAuthState(w, r, state, h.cookieDomain) {
		http.Redirect(w, r, oauthStateMismatchRedirect, http.StatusFound)
		return
	}
	result, err := h.svc.YandexCallback(r.Context(), state, code, r.UserAgent(), middleware.ClientIP(r), service.RefreshCookieFromRequest(r))
	if err != nil {
		if result.WasLinkAttempt {
			to := "/cabinet/accounts?status=error&reason_code=yandex_link_unknown"
			switch {
			case errors.Is(err, service.ErrYandexMergeRequired):
				to = "/cabinet/link/merge?status=merge_required&reason_code=yandex_merge_candidate_detected&auto=1&provider=yandex"
			case errors.Is(err, service.ErrInvalidToken):
				to = "/cabinet/accounts?status=error&reason_code=yandex_link_session_invalid"
			case errors.Is(err, service.ErrYandexLinkEmailConflict):
				to = "/cabinet/accounts?status=error&reason_code=email_conflict_with_another_account&provider=yandex"
			}
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if redirectOAuthLoginError(w, r, err, result, "yandex") {
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setRefreshCookie(w, r, result.Pair, h.cookieDomain, "/cabinet/api/auth")
	if result.SuccessRedirect != "" {
		http.Redirect(w, r, result.SuccessRedirect, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/cabinet/dashboard", http.StatusFound)
}

func (h *OAuthHandler) VKStart(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	redirectURL, err := h.svc.VKStart(ref)
	if err != nil {
		if errors.Is(err, service.ErrVKDisabled) {
			http.Error(w, "vk oauth disabled", http.StatusNotImplemented)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setOAuthStateCookie(w, redirectURL, h.cookieDomain)
	http.Redirect(w, r, redirectURL, http.StatusFound) //nolint:gosec // G710: адрес начала OAuth строит сервер, не пользователь
}

func (h *OAuthHandler) VKCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if state == "" || code == "" || deviceID == "" {
		http.Error(w, "missing state, code, or device_id", http.StatusBadRequest)
		return
	}
	if !consumeOAuthState(w, r, state, h.cookieDomain) {
		http.Redirect(w, r, oauthStateMismatchRedirect, http.StatusFound)
		return
	}
	result, err := h.svc.VKCallback(r.Context(), state, code, deviceID, r.UserAgent(), middleware.ClientIP(r), service.RefreshCookieFromRequest(r))
	if err != nil {
		if result.WasLinkAttempt {
			to := "/cabinet/accounts?status=error&reason_code=vk_link_unknown"
			switch {
			case errors.Is(err, service.ErrVKMergeRequired):
				to = "/cabinet/link/merge?status=merge_required&reason_code=vk_merge_candidate_detected&auto=1&provider=vk"
			case errors.Is(err, service.ErrInvalidToken):
				to = "/cabinet/accounts?status=error&reason_code=vk_link_session_invalid"
			case errors.Is(err, service.ErrVKLinkEmailConflict):
				to = "/cabinet/accounts?status=error&reason_code=email_conflict_with_another_account&provider=vk"
			}
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if redirectOAuthLoginError(w, r, err, result, "vk") {
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setRefreshCookie(w, r, result.Pair, h.cookieDomain, "/cabinet/api/auth")
	if result.SuccessRedirect != "" {
		http.Redirect(w, r, result.SuccessRedirect, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/cabinet/dashboard", http.StatusFound)
}

// ============================================================================
// Telegram
// ============================================================================

// TelegramOIDCStart — GET /cabinet/api/auth/telegram/start?ref=...
func (h *OAuthHandler) TelegramOIDCStart(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	redirectURL, err := h.svc.TelegramOIDCStart(service.TelegramOIDCStartInput{
		Mode:        googleoauth.TelegramOIDCModeLogin,
		ReferralRaw: ref,
	})
	if err != nil {
		if errors.Is(err, service.ErrTelegramOIDCDisabled) {
			http.Error(w, "telegram oidc disabled", http.StatusNotImplemented)
			return
		}
		slog.Error("telegram oidc start failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setOAuthStateCookie(w, redirectURL, h.cookieDomain)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// TelegramOIDCCallback — GET /cabinet/api/auth/telegram/callback?code=...&state=...
func (h *OAuthHandler) TelegramOIDCCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Redirect(w, r, "/cabinet/login?status=error&reason_code=telegram_oidc_missing_code", http.StatusFound)
		return
	}
	if !consumeOAuthState(w, r, state, h.cookieDomain) {
		http.Redirect(w, r, oauthStateMismatchRedirect, http.StatusFound)
		return
	}
	res, err := h.svc.TelegramOIDCCallback(r.Context(), state, code, r.UserAgent(), middleware.ClientIP(r), service.RefreshCookieFromRequest(r))
	if err != nil {
		slog.Warn("telegram oidc callback failed", "error", err)
		if res != nil && res.Mode == googleoauth.TelegramOIDCModeLink {
			to := "/cabinet/accounts?status=error&reason_code=telegram_link_unknown"
			switch {
			case errors.Is(err, service.ErrInvalidToken):
				to = "/cabinet/accounts?status=error&reason_code=telegram_link_session_invalid"
			case errors.Is(err, service.ErrGoogleLinkSessionMismatch):
				to = "/cabinet/accounts?status=error&reason_code=telegram_link_session_mismatch"
			case errors.Is(err, bootstrap.ErrTelegramCustomerLinkedElsewhere):
				to = "/cabinet/link/merge?status=merge_required&reason_code=telegram_merge_candidate_detected&auto=1&provider=telegram"
			}
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if errors.Is(err, bootstrap.ErrTelegramCustomerLinkedElsewhere) {
			http.Redirect(w, r, "/cabinet/link/merge?status=merge_required&reason_code=telegram_merge_candidate_detected&auto=1&provider=telegram", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cabinet/login?status=error&reason_code=telegram_oidc_failed", http.StatusFound)
		return
	}
	if res.Mode == googleoauth.TelegramOIDCModeLink {
		to := "/cabinet/accounts?status=linked&reason_code=telegram_linked"
		if res.HasMergeCandidate {
			to = "/cabinet/link/merge?status=merge_required&reason_code=telegram_merge_candidate_detected&auto=1&provider=telegram"
		}
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	if res.Pair == nil {
		http.Redirect(w, r, "/cabinet/login?status=error&reason_code=telegram_oidc_failed", http.StatusFound)
		return
	}
	setRefreshCookie(w, r, res.Pair, h.cookieDomain, "/cabinet/api/auth")
	http.Redirect(w, r, "/cabinet/dashboard", http.StatusFound)
}

// telegramLoginReq — тело POST /cabinet/api/auth/telegram.
// Поля зависят от source.
type telegramLoginReq struct {
	Source string `json:"source"` // "widget" | "miniapp"

	// Widget-поля (source=widget).
	ID        int64  `json:"id,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  int64  `json:"auth_date,omitempty"`
	Hash      string `json:"hash,omitempty"`

	// MiniApp-поля (source=miniapp).
	InitData string `json:"init_data,omitempty"`

	// Реферал (как у POST /auth/register): ref_<telegram_id> реферера.
	ReferralCode string `json:"referral_code,omitempty"`
}

// TelegramLogin — POST /cabinet/api/auth/telegram.
// Принимает source=widget (поля Widget) или source=miniapp (init_data).
// Возвращает токен-пару аналогично /auth/login.
func (h *OAuthHandler) TelegramLogin(w http.ResponseWriter, r *http.Request) {
	var req telegramLoginReq
	if !decodeJSON(w, r, &req) {
		return
	}

	var pair *service.TokenPair
	var err error

	ua := r.UserAgent()
	ip := middleware.ClientIP(r)

	switch req.Source {
	case "widget":
		pair, err = h.svc.TelegramLoginWidget(r.Context(), service.TelegramWidgetInput{
			ID:           req.ID,
			FirstName:    req.FirstName,
			LastName:     req.LastName,
			Username:     req.Username,
			PhotoURL:     req.PhotoURL,
			AuthDate:     req.AuthDate,
			Hash:         req.Hash,
			ReferralCode: req.ReferralCode,
			UserAgent:    ua,
			IP:           ip,
		})
	case "miniapp":
		pair, err = h.svc.TelegramLoginMiniApp(r.Context(), service.TelegramMiniAppInput{
			InitData:     req.InitData,
			ReferralCode: req.ReferralCode,
			UserAgent:    ua,
			IP:           ip,
		})
	default:
		cabmetrics.RecordAuth("telegram_login", "client_error")
		http.Error(w, "unknown source (use 'widget' or 'miniapp')", http.StatusBadRequest)
		return
	}

	if err != nil {
		if errors.Is(err, service.ErrTelegramDisabled) {
			cabmetrics.RecordAuth("telegram_login", "client_error")
			http.Error(w, "telegram login disabled", http.StatusNotImplemented)
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			slog.Warn("telegram login verification failed",
				"source", req.Source,
				"init_data_len", len(req.InitData),
				"user_id", req.ID,
				"ua", ua,
				"ip", ip,
			)
			cabmetrics.RecordAuth("telegram_login", "failure")
			http.Error(w, "telegram verification failed", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, service.ErrInvalidToken) {
			cabmetrics.RecordAuth("telegram_login", "client_error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, service.ErrInvalidInput) {
			cabmetrics.RecordAuth("telegram_login", "client_error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, bootstrap.ErrTelegramCustomerLinkedElsewhere) {
			cabmetrics.RecordAuth("telegram_login", "client_error")
			http.Error(w, "this Telegram account is already linked to another cabinet account", http.StatusConflict)
			return
		}
		slog.Error("telegram login failed", "source", req.Source, "error", err)
		cabmetrics.RecordAuth("telegram_login", "server_error")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	method := "telegram_widget"
	if req.Source == "miniapp" {
		method = "telegram_miniapp"
	}
	cabmetrics.RecordAuth(method, "success")
	setRefreshCookie(w, r, pair, h.cookieDomain, "/cabinet/api/auth")
	writeJSON(w, http.StatusOK, loginResp{
		AccessToken: pair.AccessToken,
		AccessExp:   pair.AccessExp.Unix(),
		CSRFToken:   pair.CSRFToken,
	})
}
