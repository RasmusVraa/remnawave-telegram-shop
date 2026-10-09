// Package handlers — HTTP-хендлеры кабинета: auth, me, и т.д.
//
// Все хендлеры возвращают JSON; ошибки маппятся в HTTP-коды по принципу:
//
//	400 — невалидный вход (ErrInvalidInput),
//	401 — неверные учётки / токен (ErrInvalidCredentials, ErrInvalidToken),
//	403 — email не подтверждён, CSRF, rate-limit уже в отдельном middleware,
//	409 — резерв для конфликтов (не используется на MVP),
//	500 — всё остальное (логируется).
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
	botcfg "remnawave-tg-shop-bot/internal/config"
)

// AuthHandler — группа эндпоинтов /cabinet/api/auth/*.
type AuthHandler struct {
	svc                  *service.Service
	cookieDomain         string
	googleOAuthEnabled   bool
	yandexOAuthEnabled   bool
	vkOAuthEnabled       bool
	telegramLoginBotUser string // username без @ для Login Widget; "" — вход через Telegram не настроен
	telegramOIDCEnabled  bool
	telegramWebAuthMode  string
}

// NewAuth — конструктор. googleOAuthEnabled и telegramLoginBotUser — публичные
// подсказки для SPA (см. GET /auth/bootstrap); не секреты.
func NewAuth(
	svc *service.Service,
	cookieDomain string,
	googleOAuthEnabled bool,
	yandexOAuthEnabled bool,
	vkOAuthEnabled bool,
	telegramLoginBotUser string,
	telegramOIDCEnabled bool,
	telegramWebAuthMode string,
) *AuthHandler {
	return &AuthHandler{
		svc: svc, cookieDomain: cookieDomain,
		googleOAuthEnabled:   googleOAuthEnabled,
		yandexOAuthEnabled:   yandexOAuthEnabled,
		vkOAuthEnabled:       vkOAuthEnabled,
		telegramLoginBotUser: telegramLoginBotUser,
		telegramOIDCEnabled:  telegramOIDCEnabled,
		telegramWebAuthMode:  telegramWebAuthMode,
	}
}

// ============================================================================
// DTOs
// ============================================================================

type registerReq struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Language     string `json:"language"`
	ReferralCode string `json:"referral_code"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResp struct {
	AccessToken string `json:"access_token"`
	AccessExp   int64  `json:"access_exp"` // unix seconds
	CSRFToken   string `json:"csrf_token"`
}

type forgotReq struct {
	Email string `json:"email"`
}

type resetReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type verifyConfirmReq struct {
	Token string `json:"token"`
}

type messageResp struct {
	Message string `json:"message"`
}

// cabinetSiteLinks собирает публичные URL из env бота (те же, что используются в Telegram-версии).
func cabinetSiteLinks() map[string]string {
	m := make(map[string]string)
	add := func(key, val string) {
		if s := strings.TrimSpace(val); s != "" {
			m[key] = s
		}
	}
	add("bot", botcfg.BotURL())
	add("server_status", botcfg.ServerStatusURL())
	add("support", botcfg.SupportURL())
	add("feedback", botcfg.FeedbackURL())
	add("channel", botcfg.ChannelURL())
	add("tos", botcfg.TosURL())
	add("video_guide", botcfg.VideoGuideURL())
	add("server_selection", botcfg.ServerSelectionURL())
	add("public_offer", botcfg.PublicOfferURL())
	add("privacy_policy", botcfg.PrivacyPolicyURL())
	add("terms_of_service", botcfg.TermsOfServiceURL())
	if len(m) == 0 {
		return nil
	}
	return m
}

// AuthBootstrap — GET /cabinet/api/auth/bootstrap.
// Публичные флаги для страницы логина: какие альтернативные провайдеры доступны.
func (h *AuthHandler) AuthBootstrap(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"google_oauth_enabled":   h.googleOAuthEnabled,
		"yandex_oauth_enabled":   h.yandexOAuthEnabled,
		"vk_oauth_enabled":       h.vkOAuthEnabled,
		"telegram_oidc_enabled":  h.telegramOIDCEnabled,
		"telegram_web_auth_mode": h.telegramWebAuthMode,
		// Совпадает с FORTUNE_ENABLED: скрыть пункт меню в SPA; /fortune по прямой ссылке остаётся.
		"fortune_nav_visible": cabcfg.GetFortuneWheel().Enabled,
		// Совпадает с PARTNER_PROGRAM_ENABLED: выключенная программа не должна
		// светиться пунктом меню, который ведёт на «раздел недоступен».
		"partner_nav_visible":  botcfg.PartnerProgramEnabled(),
		"partner_max_percent":  maxPartnerPercent(),
		"support_chat_enabled": botcfg.SupportBotAPIEnabled(),
		"turnstile_enabled":    cabcfg.TurnstileEnabled(),
		"pwa_enabled":          cabcfg.PWAEnabled(),
		"pwa_app_name":         cabcfg.PWAAppName(),
		"pwa_short_name":       cabcfg.PWAShortName(),
		"light_theme_enabled":  cabcfg.LightThemeEnabled(),
		// Уже с учётом авто-расписания (CABINET_DECOR_AUTO_ENABLED): в праздничном
		// окне отдаём тему окна, вне окон — выбранную админом вручную.
		"decor_theme": cabcfg.EffectiveDecorTheme(),
		"landing":     cabcfg.LandingPublic(),
		// CABINET_SUBSCRIPTION_SHOW_LOYALTY: плашка уровня на /subscription.
		// Раздел /loyalty и плашка в профиле от флага не зависят.
		"subscription_loyalty_visible": cabcfg.SubscriptionLoyaltyVisible(),
		// Шифрование deep link'ов подключения (см. /me/deeplink): фронт узнаёт,
		// что для Happ/INCY надо запросить зашифрованную ссылку вместо .../add/.
		"deeplink_happ_encrypt": cabcfg.DeeplinkHappEncryptEnabled(),
		"deeplink_incy_encrypt": cabcfg.DeeplinkIncyEncryptEnabled(),
		"payment_providers": map[string]bool{
			"yookassa":          botcfg.IsYookasaEnabled(),
			"cryptopay":         botcfg.IsCryptoPayEnabled(),
			"telegram":          botcfg.IsTelegramStarsEnabled(),
			"platega_sbp":       botcfg.IsPlategaEnabled() && botcfg.IsPlategaSBPEnabled(),
			"platega_cards":     botcfg.IsPlategaEnabled() && botcfg.IsPlategaCardsEnabled(),
			"platega_acquiring": botcfg.IsPlategaEnabled() && botcfg.IsPlategaAcquiringEnabled(),
			"platega_worldwide": botcfg.IsPlategaEnabled() && botcfg.IsPlategaWorldwideEnabled(),
			"platega_crypto":    botcfg.IsPlategaEnabled() && botcfg.IsPlategaCryptoEnabled(),
			"heleket":           botcfg.IsHeleketEnabled(),
		},
	}
	if cabcfg.TurnstileEnabled() && strings.TrimSpace(cabcfg.TurnstileSiteKey()) != "" {
		body["turnstile_site_key"] = cabcfg.TurnstileSiteKey()
	}
	if h.telegramLoginBotUser != "" {
		body["telegram_widget_bot"] = h.telegramLoginBotUser
	}
	if links := cabinetSiteLinks(); len(links) > 0 {
		body["site_links"] = links
	}
	body["brand_name"] = cabcfg.BrandName()
	if u := cabcfg.BrandLogoURLForClient(); u != "" {
		body["brand_logo_url"] = u
	}
	if u := cabcfg.SupportLogoURLForClient(); u != "" {
		body["support_logo_url"] = u
	}
	// Не кэшировать у nginx/браузере: после смены env фронт должен сразу увидеть флаги.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, body)
}

// ============================================================================
// Регистрация
// ============================================================================

// Register — POST /cabinet/api/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.svc.Register(r.Context(), service.RegisterInput{
		Email:        req.Email,
		Password:     req.Password,
		Language:     req.Language,
		ReferralCode: req.ReferralCode,
		UserAgent:    r.UserAgent(),
		IP:           middleware.ClientIP(r),
	})
	if err != nil {
		cabmetrics.RecordAuth("email_register", "failure")
		writeServiceErr(w, err, "register")
		return
	}
	cabmetrics.RecordAuth("email_register", "success")
	writeJSON(w, http.StatusOK, messageResp{Message: res.Message})
}

// ============================================================================
// Логин / логаут / refresh
// ============================================================================

// Login — POST /cabinet/api/auth/login. В ответе access_token + csrf_token.
// refresh уходит в HttpOnly cookie.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	tp, err := h.svc.Login(r.Context(), service.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IP:        middleware.ClientIP(r),
	})
	if err != nil {
		cabmetrics.RecordAuth("email_login", "failure")
		writeServiceErr(w, err, "login")
		return
	}
	cabmetrics.RecordAuth("email_login", "success")
	h.setAuthCookies(w, r, tp)
	writeJSON(w, http.StatusOK, loginResp{
		AccessToken: tp.AccessToken,
		AccessExp:   tp.AccessExp.Unix(),
		CSRFToken:   tp.CSRFToken,
	})
}

// Refresh — POST /cabinet/api/auth/refresh. Читает refresh из cookie, ротирует.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	refresh := service.RefreshCookieFromRequest(r)
	tp, err := h.svc.Refresh(r.Context(), refresh, r.UserAgent(), middleware.ClientIP(r))
	if err != nil {
		// На reuse-detection стираем cookies — иначе SPA будет долбить refresh.
		if errors.Is(err, service.ErrReused) || errors.Is(err, service.ErrInvalidToken) {
			h.clearAuthCookies(w, r)
		}
		writeServiceErr(w, err, "refresh")
		return
	}
	h.setAuthCookies(w, r, tp)
	writeJSON(w, http.StatusOK, loginResp{
		AccessToken: tp.AccessToken,
		AccessExp:   tp.AccessExp.Unix(),
		CSRFToken:   tp.CSRFToken,
	})
}

// Logout — POST /cabinet/api/auth/logout. Идемпотентно.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	refresh := service.RefreshCookieFromRequest(r)
	if err := h.svc.Logout(r.Context(), refresh); err != nil {
		slog.Warn("logout failed", "error", err)
	}
	h.clearAuthCookies(w, r)
	writeJSON(w, http.StatusOK, messageResp{Message: "logged out"})
}

// ============================================================================
// Пароль (forgot / reset)
// ============================================================================

// ForgotPassword — POST /cabinet/api/auth/password/forgot. Всегда 200.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		slog.Warn("forgot failed", "error", err)
	}
	writeJSON(w, http.StatusOK, messageResp{Message: "if the email is registered, a reset link was sent"})
}

// ResetPassword — POST /cabinet/api/auth/password/reset.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		writeServiceErr(w, err, "reset")
		return
	}
	writeJSON(w, http.StatusOK, messageResp{Message: "password updated"})
}

// ============================================================================
// Email verification (код из письма; старые ссылки с ?token= ещё поддерживаются)
// ============================================================================

// ResendVerifyEmailPublic — POST /cabinet/api/auth/email/verify/resend-public.
// Тело как у forgot: { "email" }. Всегда 200; Turnstile + rate-limit как у forgot.
func (h *AuthHandler) ResendVerifyEmailPublic(w http.ResponseWriter, r *http.Request) {
	var req forgotReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResendVerifyEmailPublic(r.Context(), req.Email); err != nil {
		slog.Warn("resend verify public failed", "error", err)
	}
	writeJSON(w, http.StatusOK, messageResp{Message: "if the account exists and email is not verified, a code was sent"})
}

// ConfirmEmail — POST /cabinet/api/auth/email/verify/confirm.
func (h *AuthHandler) ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyConfirmReq
	if !decodeJSON(w, r, &req) {
		return
	}
	tp, err := h.svc.ConfirmEmail(r.Context(), req.Token, r.UserAgent(), middleware.ClientIP(r))
	if err != nil {
		writeServiceErr(w, err, "confirm_email")
		return
	}
	h.setAuthCookies(w, r, tp)
	writeJSON(w, http.StatusOK, loginResp{
		AccessToken: tp.AccessToken,
		AccessExp:   tp.AccessExp.Unix(),
		CSRFToken:   tp.CSRFToken,
	})
}

// ============================================================================
// Cookie helpers
// ============================================================================

const refreshCookiePath = "/cabinet/api/auth"

// setAuthCookies — обёртка вокруг пакетного setRefreshCookie (util.go).
func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, r *http.Request, tp *service.TokenPair) {
	setRefreshCookie(w, r, tp, h.cookieDomain, refreshCookiePath)
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	domain, secure := cookieScope(r, h.cookieDomain)
	http.SetCookie(w, &http.Cookie{
		Name:     service.RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		Domain:   domain,
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	clearCabCsrfCookie(w, r, h.cookieDomain)
}

// maxPartnerPercent — большая из двух ставок партнёрской программы. Идёт в
// бейдж «до N%» рядом с пунктом меню: обещание в меню обязано совпадать с тем,
// что человек увидит на самой странице.
func maxPartnerPercent() float64 {
	first, renewal := botcfg.PartnerFirstPercent(), botcfg.PartnerRenewalPercent()
	if renewal > first {
		return renewal
	}
	return first
}
