package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	adminauth "remnawave-tg-shop-bot/internal/cabinet/admin/auth"
	googleoauth "remnawave-tg-shop-bot/internal/cabinet/auth/oauth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/cabinet/tgprofile"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// MeHandler — эндпоинты /cabinet/api/me/*.
type MeHandler struct {
	svc          *service.Service
	accounts     *repository.AccountRepo
	ids          *repository.IdentityRepo
	links        *repository.AccountCustomerLinkRepo
	customers    *database.CustomerRepository
	deviceNames  *database.DeviceNameRepository
	bootstrap    *bootstrap.CustomerBootstrap
	payments     *payment.PaymentService
	purchases    *database.PurchaseRepository
	rw           *remnawave.Client
	adminChecker *adminauth.Checker
	tgProfiles   *tgprofile.Service
	// avatarSecret — секрет кабинета: им подписываются ссылки на аватарку.
	avatarSecret        []byte
	cookieDomain        string
	telegramWidgetBot   string // username без @ для Login Widget; "" — виджет недоступен
	googleOAuthEnabled  bool
	yandexOAuthEnabled  bool
	vkOAuthEnabled      bool
	telegramOIDCEnabled bool
}

// NewMe — конструктор. links может быть nil в тестах; тогда /me не отдаст
// customer_id, но работать будет.
func NewMe(
	svc *service.Service,
	accounts *repository.AccountRepo,
	ids *repository.IdentityRepo,
	links *repository.AccountCustomerLinkRepo,
	boot *bootstrap.CustomerBootstrap,
	payments *payment.PaymentService,
	purchases *database.PurchaseRepository,
	rw *remnawave.Client,
	customers *database.CustomerRepository,
	deviceNames *database.DeviceNameRepository,
	adminChecker *adminauth.Checker,
	tgProfiles *tgprofile.Service,
	avatarSecret []byte,
	cookieDomain string,
	telegramWidgetBot string,
	googleOAuthEnabled bool,
	yandexOAuthEnabled bool,
	vkOAuthEnabled bool,
	telegramOIDCEnabled bool,
) *MeHandler {
	return &MeHandler{
		svc: svc, accounts: accounts, ids: ids, links: links, bootstrap: boot, payments: payments, purchases: purchases, rw: rw,
		customers: customers, deviceNames: deviceNames, adminChecker: adminChecker, tgProfiles: tgProfiles, avatarSecret: avatarSecret,
		cookieDomain: cookieDomain, telegramWidgetBot: telegramWidgetBot,
		googleOAuthEnabled: googleOAuthEnabled, yandexOAuthEnabled: yandexOAuthEnabled, vkOAuthEnabled: vkOAuthEnabled, telegramOIDCEnabled: telegramOIDCEnabled,
	}
}

type meDeviceItem struct {
	HWID        string `json:"hwid"`
	Platform    string `json:"platform,omitempty"`
	OSVersion   string `json:"os_version,omitempty"`
	DeviceModel string `json:"device_model,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	// CustomName — название, которое пользователь дал устройству в кабинете.
	CustomName string `json:"custom_name,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type meDevicesResp struct {
	Enabled     bool           `json:"enabled"`
	DeviceLimit int            `json:"device_limit"`
	Connected   int            `json:"connected"`
	Devices     []meDeviceItem `json:"devices"`
}

type meDeleteDeviceReq struct {
	HWID string `json:"hwid"`
}

type meRenameDeviceReq struct {
	HWID string `json:"hwid"`
	Name string `json:"name"`
}

type meResp struct {
	ID              int64    `json:"id"`
	Email           *string  `json:"email,omitempty"`
	EmailVerified   bool     `json:"email_verified"`
	Language        string   `json:"language"`
	Providers       []string `json:"providers"`
	HasTelegramLink bool     `json:"has_telegram_link"` // true, если identity провайдера telegram привязана
	HasPassword     bool     `json:"has_password"`      // false — только OAuth/Telegram, смена пароля недоступна
	// CanUseEmailPasswordLogin — true, если у аккаунта задан пароль (вход/привязка сценариев «как через почту»).
	CanUseEmailPasswordLogin bool `json:"can_use_email_password_login"`
	// CustomerID — id customer'а, привязанного к этому аккаунту. nil в редком
	// случае, если bootstrap ещё не успел отработать (сразу после регистрации,
	// до первого обращения к /me). UI должен перезапросить /me.
	CustomerID *int64 `json:"customer_id,omitempty"`
	// TelegramWidgetBot — username бота (без @) для Telegram Login Widget при привязке.
	TelegramWidgetBot   string `json:"telegram_widget_bot,omitempty"`
	GoogleOAuthEnabled  bool   `json:"google_oauth_enabled"`
	YandexOAuthEnabled  bool   `json:"yandex_oauth_enabled"`
	VKOAuthEnabled      bool   `json:"vk_oauth_enabled"`
	TelegramOIDCEnabled bool   `json:"telegram_oidc_enabled"`
	// RegisteredAt — дата создания аккаунта кабинета (ISO 8601).
	RegisteredAt string `json:"registered_at"`
	// CanDeleteAccountUI — флаг показа блока self-delete в профиле.
	CanDeleteAccountUI bool `json:"can_delete_account_ui"`
	// TelegramID — числовой id пользователя Telegram, если известен (identity или customer без synthetic).
	TelegramID *int64 `json:"telegram_id,omitempty"`
	// GoogleMaskedEmail / YandexMaskedEmail / VKMaskedEmail — маска почты из identity (без сырого sub).
	GoogleMaskedEmail *string `json:"google_masked_email,omitempty"`
	YandexMaskedEmail *string `json:"yandex_masked_email,omitempty"`
	VKMaskedEmail     *string `json:"vk_masked_email,omitempty"`
	// IsAdmin — true, если linked Telegram identity == ADMIN_TELEGRAM_ID.
	IsAdmin bool `json:"is_admin"`
	// identityProfile — имя, ник и аватарка для шапки профиля (поля инлайнятся).
	identityProfile
}

// Me — GET /cabinet/api/me.
func (h *MeHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	acc, err := h.accounts.FindByID(r.Context(), claims.AccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Токен валиден, а аккаунта нет — он удалён. 404 фронт не считает
			// причиной разлогина и вкладка продолжает жить до конца TTL, поэтому
			// отвечаем 401: refresh упадёт (cabinet_session удалён каскадом) и
			// клиент выйдет из сессии сразу.
			handleAccountGone(w, bootstrap.ErrAccountGone, "me.get", claims.AccountID)
			return
		}
		slog.Error("me: find account failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := h.ids.ListLinkedByAccount(r.Context(), acc.ID)
	if err != nil {
		slog.Error("me: list identities failed", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	providers := make([]string, 0, len(ids))
	providerSeen := make(map[string]struct{}, len(ids))
	hasTelegram := false
	// После слияния двух аккаунтов с настоящими Telegram на выжившем может
	// оказаться несколько telegram-привязок (вход работает через любую).
	// Клиенту показываем ту, по которой его адресует бот, — см. ниже.
	var telegramIdentityIDs []int64
	for _, id := range ids {
		if id.Provider == repository.ProviderEmail && acc.PasswordHash == nil {
			// Email считается способом входа только при наличии парольного логина.
			continue
		}
		if _, ok := providerSeen[id.Provider]; !ok {
			providerSeen[id.Provider] = struct{}{}
			providers = append(providers, id.Provider)
		}
		if id.Provider == repository.ProviderTelegram {
			hasTelegram = true
			s := strings.TrimSpace(id.ProviderUserID)
			if s != "" {
				if v, perr := strconv.ParseInt(s, 10, 64); perr == nil {
					telegramIdentityIDs = append(telegramIdentityIDs, v)
				}
			}
		}
	}

	// customer_id: читаем из link'а. Если link'а ещё нет — пробуем дошить через
	// bootstrap (idempotent). Ошибки bootstrap не роняют /me — поле просто
	// останется nil, клиент перезапросит.
	var customerID *int64
	if h.links != nil {
		link, err := h.links.FindByAccountID(r.Context(), acc.ID)
		if err == nil {
			id := link.CustomerID
			customerID = &id
		} else if errors.Is(err, repository.ErrNotFound) && h.bootstrap != nil {
			if link2, err2 := h.bootstrap.EnsureForAccount(r.Context(), acc.ID, acc.Language); err2 == nil {
				id := link2.CustomerID
				customerID = &id
			} else if handleAccountGone(w, err2, "me.get", acc.ID) {
				// Аккаунт удалён между FindByID и bootstrap'ом — ответ уже записан.
				return
			} else {
				slog.Warn("me: bootstrap failed", "account_id", acc.ID, "error", err2)
			}
		} else if err != nil {
			slog.Warn("me: find link failed", "account_id", acc.ID, "error", err)
		}
	}

	var linkedCustomer *database.Customer
	if h.customers != nil && customerID != nil {
		cust, errCust := h.customers.FindById(r.Context(), *customerID)
		if errCust != nil {
			slog.Warn("me: find customer failed", "customer_id", *customerID, "error", errCust.Error())
		} else if cust != nil {
			linkedCustomer = cust
		}
	}

	telegramUserID := pickTelegramID(telegramIdentityIDs, linkedCustomer)

	if hasTelegram {
		if _, found := providerSeen[repository.ProviderTelegram]; !found {
			providers = append(providers, repository.ProviderTelegram)
			providerSeen[repository.ProviderTelegram] = struct{}{}
		}
	}

	var googleMasked, yandexMasked, vkMasked *string
	for _, id := range ids {
		if id.ProviderEmail == nil {
			continue
		}
		raw := strings.TrimSpace(*id.ProviderEmail)
		if raw == "" {
			continue
		}
		m := maskIdentityHintEmail(raw)
		if m == "" {
			continue
		}
		switch id.Provider {
		case repository.ProviderGoogle:
			if googleMasked == nil {
				s := m
				googleMasked = &s
			}
		case repository.ProviderYandex:
			if yandexMasked == nil {
				s := m
				yandexMasked = &s
			}
		case repository.ProviderVK:
			if vkMasked == nil {
				s := m
				vkMasked = &s
			}
		}
	}

	isAdmin := middleware.ResolveIsAdmin(r.Context(), h.adminChecker, claims)

	resp := meResp{
		ID:                       acc.ID,
		Email:                    acc.Email,
		EmailVerified:            acc.EmailVerified(),
		Language:                 acc.Language,
		Providers:                providers,
		HasTelegramLink:          hasTelegram,
		HasPassword:              acc.PasswordHash != nil,
		CanUseEmailPasswordLogin: acc.PasswordHash != nil,
		CustomerID:               customerID,
		GoogleOAuthEnabled:       h.googleOAuthEnabled,
		YandexOAuthEnabled:       h.yandexOAuthEnabled,
		VKOAuthEnabled:           h.vkOAuthEnabled,
		TelegramOIDCEnabled:      h.telegramOIDCEnabled,
		RegisteredAt:             acc.CreatedAt.UTC().Format(time.RFC3339),
		CanDeleteAccountUI:       cabcfg.ProfileDeleteEnabled(),
		TelegramID:               telegramUserID,
		GoogleMaskedEmail:        googleMasked,
		YandexMaskedEmail:        yandexMasked,
		VKMaskedEmail:            vkMasked,
		IsAdmin:                  isAdmin,
		identityProfile:          h.resolveIdentityProfile(r.Context(), acc.ID, ids, linkedCustomer, telegramUserID),
	}
	if h.telegramWidgetBot != "" {
		resp.TelegramWidgetBot = h.telegramWidgetBot
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *MeHandler) TelegramLinkOIDCStart(w http.ResponseWriter, r *http.Request) {
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
	redirectURL, err := h.svc.TelegramOIDCStart(service.TelegramOIDCStartInput{
		Mode:      googleoauth.TelegramOIDCModeLink,
		AccountID: claims.AccountID,
	})
	if err != nil {
		if errors.Is(err, service.ErrTelegramOIDCDisabled) {
			http.Error(w, "telegram oidc disabled", http.StatusNotImplemented)
			return
		}
		slog.Error("telegram link oidc start failed", "account_id", claims.AccountID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// fetch() с redirect:manual на внешний oauth.telegram.org даёт opaqueredirect (status 0)
	// без читаемого Location — SPA запрашивает JSON и сама делает assign.
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirectURL})
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// GoogleLinkStart — GET /cabinet/api/me/google/link/start.
// OAuth state привязывает Google к текущему account_id; в callback проверяется refresh-cookie.
func (h *MeHandler) GoogleLinkStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.googleOAuthEnabled {
		http.Error(w, "google oauth disabled", http.StatusNotImplemented)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	redirectURL, err := h.svc.GoogleLinkStart(claims.AccountID)
	if err != nil {
		if errors.Is(err, service.ErrGoogleDisabled) {
			http.Error(w, "google oauth disabled", http.StatusNotImplemented)
			return
		}
		if errors.Is(err, service.ErrInvalidInput) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		slog.Error("google link start failed", "account_id", claims.AccountID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirectURL})
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *MeHandler) YandexLinkStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.yandexOAuthEnabled {
		http.Error(w, "yandex oauth disabled", http.StatusNotImplemented)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	redirectURL, err := h.svc.YandexLinkStart(claims.AccountID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirectURL})
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *MeHandler) VKLinkStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.vkOAuthEnabled {
		http.Error(w, "vk oauth disabled", http.StatusNotImplemented)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	redirectURL, err := h.svc.VKLinkStart(claims.AccountID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirectURL})
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

type putLanguageReq struct {
	Language string `json:"language"`
}

// PutLanguage — PUT /cabinet/api/me/language. Принимает "ru" | "en".
// При наличии связки account↔customer обновляет и customer.language (бот, письма).
func (h *MeHandler) PutLanguage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req putLanguageReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.accounts.UpdateLanguage(r.Context(), claims.AccountID, req.Language); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.links != nil && h.customers != nil {
		if link, err := h.links.FindByAccountID(r.Context(), claims.AccountID); err == nil && link != nil {
			if err := h.customers.UpdateFields(r.Context(), link.CustomerID, map[string]interface{}{"language": req.Language}); err != nil {
				slog.Error("me: sync customer language failed", "error", err.Error(), "customer_id", link.CustomerID)
			}
		} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
			slog.Error("me: find link for language sync failed", "error", err.Error())
		}
	}
	writeJSON(w, http.StatusOK, messageResp{Message: "language updated"})
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PutPassword — PUT /cabinet/api/me/password. Меняет пароль и выдаёт новую сессию
// (как login), чтобы SPA не теряла авторизацию.
func (h *MeHandler) PutPassword(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req changePasswordReq
	if !decodeJSON(w, r, &req) {
		return
	}
	pair, err := h.svc.ChangePassword(r.Context(), claims.AccountID,
		req.CurrentPassword, req.NewPassword, r.UserAgent(), middleware.ClientIP(r))
	if err != nil {
		writeServiceErr(w, err, "change_password")
		return
	}
	setRefreshCookie(w, r, pair, h.cookieDomain, refreshCookiePath)
	writeJSON(w, http.StatusOK, loginResp{
		AccessToken: pair.AccessToken,
		AccessExp:   pair.AccessExp.Unix(),
		CSRFToken:   pair.CSRFToken,
	})
}

// ResendVerifyEmail — POST /cabinet/api/me/email/verify/resend.
func (h *MeHandler) ResendVerifyEmail(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.svc.ResendVerify(r.Context(), claims.AccountID); err != nil {
		slog.Warn("resend verify failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, messageResp{Message: "verification email sent"})
}

type linkEmailReq struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
}

type linkEmailCodeReq struct {
	Code string `json:"code"`
}

// PostLinkEmail — POST /cabinet/api/me/email/link.
// Привязка email+пароля к текущему аккаунту (OAuth/Telegram без пароля); письмо подтверждения.
func (h *MeHandler) PostLinkEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req linkEmailReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Password) != strings.TrimSpace(req.PasswordConfirm) {
		http.Error(w, "password mismatch", http.StatusBadRequest)
		return
	}
	res, err := h.svc.LinkEmailToAccount(r.Context(), claims.AccountID, req.Email, req.Password)
	if err != nil {
		writeServiceErr(w, err, "link_email")
		return
	}
	if res.Status == service.LinkEmailOutcomeMergeRequired {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":      res.Status,
			"reason_code": res.ReasonCode,
		})
		return
	}
	if res.Status == service.LinkEmailOutcomeMergeVerificationRequired {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":       res.Status,
			"reason_code":  res.ReasonCode,
			"masked_email": res.MaskedEmail,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      res.Status,
		"reason_code": res.ReasonCode,
		"message":     "verification email sent",
	})
}

// PostLinkEmailVerifyCode — POST /cabinet/api/me/email/link/verify-code.
// Подтверждает merge-код для сценария "email занят OAuth-only аккаунтом без пароля".
func (h *MeHandler) PostLinkEmailVerifyCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req linkEmailCodeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.svc.ConfirmEmailMergeCode(r.Context(), claims.AccountID, req.Code)
	if err != nil {
		writeServiceErr(w, err, "link_email_verify_code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":       res.Status,
		"reason_code":  res.ReasonCode,
		"masked_email": res.MaskedEmail,
	})
}

// identityUnlinkReq — POST /cabinet/api/me/identities/unlink.
type identityUnlinkReq struct {
	Provider string `json:"provider"` // "telegram" | "google" | "email"
}

// PostIdentityUnlink — снимает привязку OAuth/Telegram/email с аккаунта, если остаётся хотя бы один способ входа.
func (h *MeHandler) PostIdentityUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req identityUnlinkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p := strings.TrimSpace(strings.ToLower(req.Provider))
	if p != repository.ProviderGoogle && p != repository.ProviderTelegram && p != repository.ProviderEmail &&
		p != repository.ProviderYandex && p != repository.ProviderVK {
		http.Error(w, `provider must be "google", "yandex", "vk", "telegram", or "email"`, http.StatusBadRequest)
		return
	}
	if p == repository.ProviderTelegram {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "telegram_unlink_forbidden",
			"message": "telegram cannot be unlinked from the cabinet",
		})
		return
	}
	ctx := r.Context()
	acc, err := h.accounts.FindByID(ctx, claims.AccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		slog.Error("me: identity unlink load account", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := h.ids.ListByAccount(ctx, claims.AccountID)
	if err != nil {
		slog.Error("me: identity unlink list identities", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !canUnlinkProvider(acc, ids, p) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "last_sign_in_method",
			"message": "cannot remove the last way to sign in",
		})
		return
	}
	if p == repository.ProviderEmail {
		hasGoogleIdentity := false
		hasEmailIdent := false
		for _, id := range ids {
			if id.Provider == repository.ProviderGoogle {
				hasGoogleIdentity = true
			}
			if id.Provider == repository.ProviderEmail {
				hasEmailIdent = true
			}
		}
		// Security guard: for Google-only accounts email is part of Google sign-in identity.
		// Unlinking email here can effectively remove the account access path semantics.
		if hasGoogleIdentity && acc.PasswordHash == nil && acc.Email != nil && strings.TrimSpace(*acc.Email) != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "email_managed_by_google",
				"message": "email is managed by google sign-in and cannot be unlinked",
			})
			return
		}
		if !hasEmailIdent {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_email_identity"})
			return
		}
		n, err := h.ids.SoftUnlinkByAccountAndProvider(ctx, claims.AccountID, p)
		if err != nil {
			slog.Error("me: identity soft unlink", "provider", p, "error", err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := h.accounts.ClearEmailLoginFields(ctx, claims.AccountID); err != nil {
			slog.Error("me: identity unlink clear email fields", "error", err.Error())
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "soft_unlinked": true, "rows": n})
		return
	}

	n, err := h.ids.SoftUnlinkByAccountAndProvider(ctx, claims.AccountID, p)
	if err != nil {
		slog.Error("me: identity soft unlink", "provider", p, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "soft_unlinked": true, "rows": n})
}

func canUnlinkProvider(acc *repository.Account, ids []repository.Identity, provider string) bool {
	remaining := 0
	for _, id := range ids {
		if id.Provider != provider {
			remaining++
		}
	}
	if provider == repository.ProviderEmail {
		// Снимаем логин по паролю только если остаётся OAuth/Telegram.
		return remaining > 0
	}
	if remaining > 0 {
		return true
	}
	passwordOK := acc.PasswordHash != nil && acc.Email != nil && strings.TrimSpace(*acc.Email) != ""
	return passwordOK
}

func (h *MeHandler) GetDevices(w http.ResponseWriter, r *http.Request) {
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
	if h.rw == nil || h.bootstrap == nil || h.customers == nil {
		writeJSON(w, http.StatusOK, meDevicesResp{Enabled: false, Devices: []meDeviceItem{}})
		return
	}
	link, err := h.bootstrap.EnsureForAccount(r.Context(), claims.AccountID, "")
	if err != nil || link == nil {
		if handleAccountGone(w, err, "me.devices", claims.AccountID) {
			return
		}
		writeJSON(w, http.StatusOK, meDevicesResp{Enabled: false, Devices: []meDeviceItem{}})
		return
	}
	c, err := h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil || c == nil {
		writeJSON(w, http.StatusOK, meDevicesResp{Enabled: false, Devices: []meDeviceItem{}})
		return
	}
	rwUser, err := cabsvc.ResolveRemnawaveCustomerUser(r.Context(), h.rw, h.customers, c)
	limit := cabsvc.HwidDeviceLimitFromUser(rwUser)
	if err != nil {
		writeJSON(w, http.StatusOK, meDevicesResp{Enabled: true, DeviceLimit: limit, Devices: []meDeviceItem{}})
		return
	}
	devs, err := h.rw.GetUserDevices(r.Context(), rwUser.ID)
	if err != nil {
		writeJSON(w, http.StatusOK, meDevicesResp{Enabled: true, DeviceLimit: limit, Devices: []meDeviceItem{}})
		return
	}
	// Без названий список всё равно нужен: ошибка БД не должна прятать устройства.
	var names map[string]string
	if h.deviceNames != nil {
		names, err = h.deviceNames.ListByCustomer(r.Context(), c.ID)
		if err != nil {
			slog.Warn("me: list device names failed", "account_id", claims.AccountID, "error", err.Error())
		}
	}
	out := make([]meDeviceItem, 0, len(devs))
	for _, d := range devs {
		item := meDeviceItem{
			HWID:       d.Hwid,
			CustomName: names[d.Hwid],
			CreatedAt:  d.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:  d.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if d.Platform != nil {
			item.Platform = *d.Platform
		}
		if d.OsVersion != nil {
			item.OSVersion = *d.OsVersion
		}
		if d.DeviceModel != nil {
			item.DeviceModel = *d.DeviceModel
		}
		if d.UserAgent != nil {
			item.UserAgent = *d.UserAgent
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, meDevicesResp{
		Enabled:     true,
		DeviceLimit: limit,
		Connected:   len(out),
		Devices:     out,
	})
}

func (h *MeHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req meDeleteDeviceReq
	if !decodeJSON(w, r, &req) {
		return
	}
	hwid := strings.TrimSpace(req.HWID)
	if hwid == "" {
		http.Error(w, "missing hwid", http.StatusBadRequest)
		return
	}
	c, rwUser, ok := h.resolveDeviceOwner(w, r, claims.AccountID, "me.delete_device")
	if !ok {
		return
	}
	if err := h.rw.DeleteUserDevice(r.Context(), rwUser.ID, hwid); err != nil {
		slog.Warn("me: delete device failed", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "delete device failed", http.StatusBadRequest)
		return
	}
	// Название уходит вместе с устройством: при повторном подключении того же
	// HWID оно не вернётся. Сбой здесь не отменяет уже сделанное удаление.
	if h.deviceNames != nil {
		if err := h.deviceNames.Delete(r.Context(), c.ID, hwid); err != nil {
			slog.Warn("me: delete device name failed", "account_id", claims.AccountID, "error", err.Error())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// RenameDevice — POST /cabinet/api/me/devices/rename: задаёт своё название
// устройству. Пустое имя сбрасывает его к исходному из Remnawave.
func (h *MeHandler) RenameDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req meRenameDeviceReq
	if !decodeJSON(w, r, &req) {
		return
	}
	hwid := strings.TrimSpace(req.HWID)
	if hwid == "" {
		http.Error(w, "missing hwid", http.StatusBadRequest)
		return
	}
	name := strings.Join(strings.Fields(req.Name), " ")
	if utf8.RuneCountInString(name) > database.DeviceNameMaxLen {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	if h.deviceNames == nil {
		http.Error(w, "devices are unavailable", http.StatusNotImplemented)
		return
	}
	c, rwUser, ok := h.resolveDeviceOwner(w, r, claims.AccountID, "me.rename_device")
	if !ok {
		return
	}
	// Переименовать можно только своё подключённое устройство: иначе таблица
	// копила бы названия для произвольных строк.
	devs, err := h.rw.GetUserDevices(r.Context(), rwUser.ID)
	if err != nil {
		slog.Warn("me: rename device: list devices failed", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "rename device failed", http.StatusBadGateway)
		return
	}
	found := false
	for _, d := range devs {
		if d.Hwid == hwid {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if name == "" {
		err = h.deviceNames.Delete(r.Context(), c.ID, hwid)
	} else {
		err = h.deviceNames.Upsert(r.Context(), c.ID, hwid, name)
	}
	if err != nil {
		slog.Warn("me: rename device failed", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "rename device failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "custom_name": name})
}

// resolveDeviceOwner находит клиента и пользователя Remnawave для действий над
// устройствами. При неудаче сам пишет ответ и возвращает ok=false.
func (h *MeHandler) resolveDeviceOwner(w http.ResponseWriter, r *http.Request, accountID int64, op string) (*database.Customer, *remnawave.User, bool) {
	if h.rw == nil || h.bootstrap == nil || h.customers == nil {
		http.Error(w, "devices are unavailable", http.StatusNotImplemented)
		return nil, nil, false
	}
	link, err := h.bootstrap.EnsureForAccount(r.Context(), accountID, "")
	if err != nil || link == nil {
		if handleAccountGone(w, err, op, accountID) {
			return nil, nil, false
		}
		http.Error(w, "subscription not found", http.StatusNotFound)
		return nil, nil, false
	}
	c, err := h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil || c == nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return nil, nil, false
	}
	rwUser, err := cabsvc.ResolveRemnawaveCustomerUser(r.Context(), h.rw, h.customers, c)
	if err != nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return nil, nil, false
	}
	return c, rwUser, true
}

type meHwidExtraApplyReq struct {
	TargetLimit int `json:"target_limit"`
}

// PostHwidExtraApply — POST /cabinet/api/me/hwid-extra/apply: бесплатное уменьшение лимита (как CallbackAddDeviceApply в боте).
func (h *MeHandler) PostHwidExtraApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req meHwidExtraApplyReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TargetLimit <= 0 {
		http.Error(w, "bad target_limit", http.StatusBadRequest)
		return
	}
	if !config.HwidExtraDevicesEnabled() || h.rw == nil || h.bootstrap == nil || h.customers == nil || h.purchases == nil {
		http.Error(w, "unavailable", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	link, err := h.bootstrap.EnsureForAccount(ctx, claims.AccountID, "")
	if err != nil || link == nil {
		if handleAccountGone(w, err, "me.hwid_extra_apply", claims.AccountID) {
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c, err := h.customers.FindById(ctx, link.CustomerID)
	if err != nil || c == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if c.IsWebOnly || utils.IsSyntheticTelegramID(c.TelegramID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	paid, err := h.purchases.HasPaidSubscription(ctx, c.ID)
	if err != nil || !paid {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	now := time.Now()
	if c.ExpireAt == nil || !c.ExpireAt.After(now) {
		http.Error(w, "subscription inactive", http.StatusBadRequest)
		return
	}
	if err := cabsvc.CleanupExpiredExtraHwid(ctx, h.rw, h.customers, c); err != nil {
		slog.Warn("me: hwid extra apply cleanup", "error", err.Error())
		http.Error(w, "cleanup failed", http.StatusBadGateway)
		return
	}
	if fresh, ferr := h.customers.FindById(ctx, c.ID); ferr == nil && fresh != nil {
		c = fresh
	}
	u, err := h.rw.GetUserTrafficInfo(ctx, c.TelegramID)
	if err != nil {
		http.Error(w, "remnawave error", http.StatusBadGateway)
		return
	}
	lim := cabsvc.BuildHwidExtraLimits(c, u)
	cur := lim.CurrentLimit
	if req.TargetLimit == cur {
		http.Error(w, "noop", http.StatusBadRequest)
		return
	}
	if req.TargetLimit > cur {
		http.Error(w, "use payment flow to increase", http.StatusBadRequest)
		return
	}
	if req.TargetLimit < lim.BaseLimit {
		http.Error(w, "below tariff base", http.StatusBadRequest)
		return
	}
	newExtra := req.TargetLimit - lim.BaseLimit
	if newExtra < 0 {
		newExtra = 0
	}
	if _, err := h.rw.UpdateUserDeviceLimit(ctx, c.TelegramID, req.TargetLimit); err != nil {
		slog.Warn("me: hwid apply update panel", "error", err.Error())
		http.Error(w, "panel update failed", http.StatusBadGateway)
		return
	}
	updates := map[string]interface{}{
		"extra_hwid":            newExtra,
		"extra_hwid_expires_at": nil,
	}
	if newExtra > 0 {
		updates["extra_hwid_expires_at"] = c.ExpireAt
	}
	if err := h.customers.UpdateFields(ctx, c.ID, updates); err != nil {
		slog.Warn("me: hwid apply db", "error", err.Error())
		http.Error(w, "db update failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "device_limit": req.TargetLimit})
}

type trialResp struct {
	Enabled     bool `json:"enabled"`
	CanActivate bool `json:"can_activate"`
	Days        int  `json:"days"`
	TrafficGB   int  `json:"traffic_gb"`
	DeviceLimit int  `json:"device_limit"`
}

// PostAccountDelete — POST /cabinet/api/me/account/delete.
// Необратимо: удаляет аккаунт кабинета; web-only customer
// удаляется вместе с покупками (CASCADE). Сессии и cookies сбрасываются.
func (h *MeHandler) PostAccountDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.rw != nil && h.links != nil && h.customers != nil {
		link, lerr := h.links.FindByAccountID(r.Context(), claims.AccountID)
		if lerr != nil && !errors.Is(lerr, repository.ErrNotFound) {
			slog.Warn("me: delete account: find link failed", "account_id", claims.AccountID, "error", lerr.Error())
		}
		if lerr == nil && link != nil {
			cust, cerr := h.customers.FindById(r.Context(), link.CustomerID)
			if cerr != nil {
				slog.Warn("me: delete account: find customer failed", "account_id", claims.AccountID, "customer_id", link.CustomerID, "error", cerr.Error())
			} else if cust != nil {
				user, uerr := h.resolveRemnawaveUserForDelete(r.Context(), cust)
				if uerr != nil && !errors.Is(uerr, remnawave.ErrUserNotFound) {
					slog.Error("me: delete account: remnawave lookup failed", "account_id", claims.AccountID, "telegram_id", utils.MaskHalfInt64(cust.TelegramID), "error", uerr.Error())
					http.Error(w, "failed to delete subscription", http.StatusBadGateway)
					return
				}
				if user != nil {
					if derr := h.rw.DeleteUser(r.Context(), user.ID); derr != nil {
						slog.Error("me: delete account: remnawave delete failed", "account_id", claims.AccountID, "user_id", user.ID, "error", derr.Error())
						http.Error(w, "failed to delete subscription", http.StatusBadGateway)
						return
					}
				}
			}
		}
	}

	if err := h.accounts.DeleteAccountForUser(r.Context(), claims.AccountID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("me: delete account", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	clearCabinetSessionCookies(w, r, h.cookieDomain)
	writeJSON(w, http.StatusOK, messageResp{Message: "account deleted"})
}

// resolveRemnawaveUserForDelete пытается найти пользователя панели для удаления.
// Основной путь: by-telegram-id; fallback: по subscription_link и username-префиксу customerID_.
func (h *MeHandler) resolveRemnawaveUserForDelete(ctx context.Context, cust *database.Customer) (*remnawave.User, error) {
	user, err := h.rw.GetUserTrafficInfo(ctx, cust.TelegramID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, remnawave.ErrUserNotFound) {
		return nil, err
	}

	users, listErr := h.rw.GetUsers(ctx)
	if listErr != nil {
		return nil, listErr
	}
	subURL := strings.TrimSpace(derefStr(cust.SubscriptionLink))
	namePrefix := strconv.FormatInt(cust.ID, 10) + "_"

	for i := range users {
		u := &users[i]
		if subURL != "" && strings.TrimSpace(u.SubscriptionUrl) == subURL {
			return u, nil
		}
		if strings.HasPrefix(strings.TrimSpace(u.Username), namePrefix) {
			return u, nil
		}
	}
	return nil, remnawave.ErrUserNotFound
}

// GetTrial — GET /cabinet/api/me/trial.
// Возвращает trial-параметры из env и можно ли активировать trial для текущего аккаунта.
func (h *MeHandler) GetTrial(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resp := trialResp{
		Enabled:     config.TrialDays() > 0,
		Days:        config.TrialDays(),
		TrafficGB:   int(config.TrialTrafficLimit() / (1024 * 1024 * 1024)),
		DeviceLimit: config.TrialHwidLimit(),
		CanActivate: false,
	}
	if !resp.Enabled || h.links == nil || h.customers == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	link, err := h.links.FindByAccountID(r.Context(), claims.AccountID)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			slog.Warn("me trial: link lookup failed", "account_id", claims.AccountID, "error", err.Error())
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	cust, err := h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil {
		slog.Warn("me trial: customer lookup failed", "customer_id", link.CustomerID, "error", err.Error())
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.CanActivate = cust != nil && strings.TrimSpace(derefStr(cust.SubscriptionLink)) == ""
	writeJSON(w, http.StatusOK, resp)
}

// PostTrialActivate — POST /cabinet/api/me/trial/activate.
// Активирует trial через ту же логику PaymentService, что и в Telegram-боте.
func (h *MeHandler) PostTrialActivate(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if config.TrialDays() <= 0 || h.payments == nil || h.links == nil || h.customers == nil {
		http.Error(w, "trial disabled", http.StatusNotFound)
		return
	}
	link, err := h.links.FindByAccountID(r.Context(), claims.AccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, "customer link not found", http.StatusNotFound)
			return
		}
		slog.Error("me trial: find link", "account_id", claims.AccountID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cust, err := h.customers.FindById(r.Context(), link.CustomerID)
	if err != nil || cust == nil {
		slog.Error("me trial: find customer", "customer_id", link.CustomerID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(derefStr(cust.SubscriptionLink)) != "" {
		http.Error(w, "trial already used", http.StatusConflict)
		return
	}
	linkURL, err := h.payments.ActivateTrial(r.Context(), cust.TelegramID)
	if err != nil {
		slog.Error("me trial: activate failed", "customer_id", cust.ID, "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"subscription_link": linkURL,
		"message":           "trial activated",
	})
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// maskIdentityHintEmail — та же логика, что maskEmail в auth/oauth: для UI /me без утечки полного адреса.
func maskIdentityHintEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.IndexByte(email, '@')
	if at < 1 {
		if email == "" {
			return ""
		}
		return email
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 2 {
		return local[:1] + "***" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + domain
}
