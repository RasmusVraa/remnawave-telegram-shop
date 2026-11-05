package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/cabinet/webauth"
	botcfg "remnawave-tg-shop-bot/internal/config"
)

// WebAuthHandler — вход в приложение по одноразовому токену
// (Telegram /start webauth_* или страница /cabinet/app-auth).
type WebAuthHandler struct {
	svc         *service.Service
	botUsername string
	subs        *cabsvc.Subscription
}

// NewWebAuth — botUsername без @; пустой — берём из BotURL / site links.
func NewWebAuth(svc *service.Service, botUsername string, subs *cabsvc.Subscription) *WebAuthHandler {
	return &WebAuthHandler{
		svc:         svc,
		botUsername: strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		subs:        subs,
	}
}

type webAuthRequestResp struct {
	Token        string `json:"token"`
	BotUsername  string `json:"bot_username"`
	ExpiresIn    int    `json:"expires_in"`
	TelegramURL  string `json:"telegram_url"`
	WebURL       string `json:"web_url"`
	StartPayload string `json:"start_payload"`
}

type webAuthPollReq struct {
	Token string `json:"token"`
}

type webAuthCompleteReq struct {
	Token string `json:"token"`
}

type webAuthPollResp struct {
	AccessToken      string  `json:"access_token"`
	AccessExp        int64   `json:"access_exp"`
	CSRFToken        string  `json:"csrf_token"`
	RefreshToken     string  `json:"refresh_token,omitempty"`
	SubscriptionLink *string `json:"subscription_link,omitempty"`
}

// Request — POST /cabinet/api/auth/deeplink/request
func (h *WebAuthHandler) Request(w http.ResponseWriter, r *http.Request) {
	token, expiresIn, err := h.svc.RequestWebAuthToken()
	if err != nil {
		if errors.Is(err, service.ErrWebAuthDisabled) {
			http.Error(w, "webauth disabled", http.StatusNotImplemented)
			return
		}
		slog.Error("webauth request failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	bot := h.resolveBotUsername()
	payload := webauth.StartPayload(token)
	public := strings.TrimRight(strings.TrimSpace(cabcfg.PublicURL()), "/")
	resp := webAuthRequestResp{
		Token:        token,
		BotUsername:  bot,
		ExpiresIn:    expiresIn,
		StartPayload: payload,
		WebURL:       public + "/cabinet/app-auth?t=" + url.QueryEscape(token),
	}
	if bot != "" {
		resp.TelegramURL = "https://t.me/" + bot + "?start=" + url.QueryEscape(payload)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// Poll — POST /cabinet/api/auth/deeplink/poll
func (h *WebAuthHandler) Poll(w http.ResponseWriter, r *http.Request) {
	var req webAuthPollReq
	if !decodeJSON(w, r, &req) {
		return
	}
	pair, err := h.svc.PollWebAuth(req.Token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWebAuthPending):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		case errors.Is(err, service.ErrWebAuthExpired):
			http.Error(w, "token expired", http.StatusGone)
			return
		case errors.Is(err, service.ErrWebAuthInvalid), errors.Is(err, service.ErrWebAuthDisabled):
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		default:
			slog.Error("webauth poll failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Refresh отдаём в JSON: десктоп-клиент не разделяет cookie jar с браузером.
	out := webAuthPollResp{
		AccessToken:  pair.AccessToken,
		AccessExp:    pair.AccessExp.Unix(),
		CSRFToken:    pair.CSRFToken,
		RefreshToken: pair.RefreshToken,
	}
	if h.subs != nil && pair.AccountID > 0 {
		if sub, err := h.subs.Get(r.Context(), pair.AccountID); err == nil && sub != nil {
			out.SubscriptionLink = sub.SubscriptionLink
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// Complete — POST /cabinet/api/auth/deeplink/complete (залогиненный кабинет).
func (h *WebAuthHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req webAuthCompleteReq
	if !decodeJSON(w, r, &req) {
		return
	}
	err := h.svc.ApproveWebAuthFromAccount(
		r.Context(),
		req.Token,
		claims.AccountID,
		r.UserAgent(),
		middleware.ClientIP(r),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWebAuthInvalid), errors.Is(err, service.ErrWebAuthExpired):
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		case errors.Is(err, service.ErrWebAuthDisabled):
			http.Error(w, "webauth disabled", http.StatusNotImplemented)
			return
		default:
			slog.Error("webauth complete failed", "error", err, "account_id", claims.AccountID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *WebAuthHandler) resolveBotUsername() string {
	if h.botUsername != "" {
		return h.botUsername
	}
	if u := cabcfg.TelegramLoginBotUsername(); u != "" {
		return u
	}
	return botUsernameFromURL(botcfg.BotURL())
}

func botUsernameFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "@") {
		return strings.TrimPrefix(raw, "@")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return strings.TrimPrefix(parts[0], "@")
}
