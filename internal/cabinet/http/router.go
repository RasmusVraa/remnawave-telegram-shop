// Package cabinethttp собирает HTTP-роуты web-кабинета:
//   - API под /cabinet/api/*  (JSON, CORS, auth/me/payments endpoints);
//   - SPA под /cabinet/*      (статикa из internal/cabinet/web/dist через go:embed).
//
// Регистрация происходит через Mount(ctx, mux, pool), чтобы кабинет жил на том
// же http.ServeMux, что и существующий healthcheck бота, — один процесс, один
// порт, никаких отдельных серверов.
package cabinethttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/jackc/pgx/v4/pgxpool"

	"remnawave-tg-shop-bot/internal/broadcast"
	adminauth "remnawave-tg-shop-bot/internal/cabinet/admin/auth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	googleoauth "remnawave-tg-shop-bot/internal/cabinet/auth/oauth"
	"remnawave-tg-shop-bot/internal/cabinet/auth/password"
	"remnawave-tg-shop-bot/internal/cabinet/auth/ratelimit"
	"remnawave-tg-shop-bot/internal/cabinet/auth/service"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/http/handlers"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/linking"
	"remnawave-tg-shop-bot/internal/cabinet/mail"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
	"remnawave-tg-shop-bot/internal/cabinet/payments"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/cabinet/supportbot"
	"remnawave-tg-shop-bot/internal/cabinet/tgprofile"
	"remnawave-tg-shop-bot/internal/cabinet/web"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/notification"
	botpayment "remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/statusprobe"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/tariffsquads"
	"remnawave-tg-shop-bot/internal/translation"
)

// Mount регистрирует роуты кабинета в переданном mux. Возвращает ошибку, если
// встроенная SPA недоступна (ломаный go:embed) или если нельзя собрать
// зависимости сервиса.
//
// ctx нужен, чтобы rate-limiter'ы могли очищать устаревшие бакеты по сигналу
// shutdown'а процесса. Это «сетевая» для rate-limiter'ов, не для API-запросов —
// контекст самого HTTP-запроса живёт отдельно.
//
// paymentService может быть nil — тогда эндпоинты /cabinet/api/payments/*
// не регистрируются. Это нужно для окружений, где оплаты целиком выключены
// (например, локальная разработка без YooKassa/CryptoPay).
// Mount регистрирует роуты кабинета.
// rw — клиент Remnawave API; может быть nil (тогда merge-шаг обновления RW пропускается).
func Mount(ctx context.Context, mux *http.ServeMux, pool *pgxpool.Pool, paymentService *botpayment.PaymentService, rw *remnawave.Client, promoService *promo.Service, syncService *sync.SyncService, tgBot *bot.Bot, broadcastSender *broadcast.Sender) error {
	spaFS, err := web.FS()
	if err != nil {
		return err
	}
	if pool == nil {
		return fmt.Errorf("cabinet http: pool is required")
	}

	cabmetrics.StartGaugeRefresh(ctx, pool, 5*time.Minute)

	// Репозитории поверх pgxpool. Создаются один раз за процесс.
	accountRepo := repository.NewAccountRepo(pool)
	identityRepo := repository.NewIdentityRepo(pool)
	sessionRepo := repository.NewSessionRepo(pool)
	evRepo := repository.NewEmailVerificationRepo(pool)
	prRepo := repository.NewPasswordResetRepo(pool)
	emailMergeCodesRepo := repository.NewEmailMergeVerificationRepo(pool)
	linkRepo := repository.NewAccountCustomerLinkRepo(pool)

	// CustomerRepository живёт в пакете бота; передаём тот же pool, чтобы link
	// и customer писались в одну базу. Bootstrap создаётся здесь, а не внутри
	// auth/service — так тот же сервис переиспользуется будущим merge-сервисом.
	customerRepo := database.NewCustomerRepository(pool)
	referralRepo := database.NewReferralRepository(pool)
	partnerRepo := database.NewPartnerRepository(pool)
	customerBootstrap := bootstrap.NewCustomerBootstrap(customerRepo, linkRepo, accountRepo, referralRepo, partnerRepo)

	// TariffRepository — только для tariffs-режима; в classic-режиме catalog
	// ходит только в env. Создаём всегда и отдаём catalog'у: он сам выбирает
	// ветку по config.SalesMode().
	tariffRepo := database.NewTariffRepository(pool)
	catalogSvc := cabsvc.NewCatalog(tariffRepo)

	// LoyaltyTier — опционально: нужно только /me/subscription, чтобы отдать
	// display_name текущего уровня. Ошибки в этой таблице не должны ломать API.
	loyaltyRepo := database.NewLoyaltyTierRepository(pool)
	purchaseRepo := database.NewPurchaseRepository(pool)
	fortRepo := repository.NewFortuneRepo(pool)
	subscriptionSvc := cabsvc.NewSubscription(customerRepo, tariffRepo, loyaltyRepo, linkRepo, customerBootstrap, rw, purchaseRepo, fortRepo)

	// SMTP + mailer. Если SMTP не настроен — DryRun, чтобы сервис работал в dev.
	mailerSender := mail.NewSender(mail.Config{
		Host:     cabcfg.SMTPHost(),
		Port:     cabcfg.SMTPPort(),
		Username: cabcfg.SMTPUser(),
		Password: cabcfg.SMTPPassword(),
		From:     cabcfg.MailFrom(),
		UseTLS:   cabcfg.SMTPTLS(),
		DryRun:   !cabcfg.SMTPEnabled(),
	})
	mailer := mail.NewMailer(mailerSender)

	// JWT issuer. Issuer = хост кабинета — так access-токен нельзя использовать
	// в другом сервисе, если они разделяют секрет (хотя у нас secret уникален).
	accessTTL := time.Duration(cabcfg.AccessTTLMinutes()) * time.Minute
	refreshTTL := time.Duration(cabcfg.RefreshTTLDays()) * 24 * time.Hour
	jwtIssuer := jwt.NewIssuer(cabcfg.JWTSecret(), accessTTL, cabcfg.PublicURL())

	// Auth service.
	authSvc := service.New(service.Config{
		PublicURL:         cabcfg.PublicURL(),
		CookieDomain:      cabcfg.CookieDomain(),
		RefreshCookiePath: "/cabinet/api/auth",
		AccessTTL:         accessTTL,
		RefreshTTL:        refreshTTL,
		EmailVerifyTTL:    24 * time.Hour,
		PasswordResetTTL:  30 * time.Minute,
		DefaultLanguage:   "ru",
		AntiEnumLatency:   300 * time.Millisecond,
		PasswordParams:    password.DefaultParams(),
		PasswordPolicy:    password.DefaultPolicy(),
	}, accountRepo, identityRepo, sessionRepo, evRepo, prRepo, jwtIssuer, mailer, customerBootstrap, emailMergeCodesRepo)
	authSvc.SetTelegramCustomerLookup(customerRepo, linkRepo)

	// Google OAuth (опционально).
	oauthStateStore := googleoauth.NewStateStore()
	oauthStateStore.RunGC(ctx)
	var oauthHandler *handlers.OAuthHandler
	if cabcfg.GoogleEnabled() {
		googleProvider := googleoauth.NewGoogleProvider(
			cabcfg.GoogleClientID(),
			cabcfg.GoogleClientSecret(),
			cabcfg.GoogleRedirectURL(),
			oauthStateStore,
		)
		authSvc.SetGoogle(ctx, googleProvider)
		oauthHandler = handlers.NewOAuth(authSvc, cabcfg.CookieDomain())
	}
	if cabcfg.YandexEnabled() {
		yandexStateStore := googleoauth.NewStateStore()
		yandexStateStore.RunGC(ctx)
		yandexProvider := googleoauth.NewYandexProvider(
			cabcfg.YandexClientID(),
			cabcfg.YandexClientSecret(),
			cabcfg.YandexRedirectURL(),
			yandexStateStore,
		)
		authSvc.SetYandex(ctx, yandexProvider)
		if oauthHandler == nil {
			oauthHandler = handlers.NewOAuth(authSvc, cabcfg.CookieDomain())
		}
	}
	if cabcfg.VKEnabled() {
		vkStateStore := googleoauth.NewStateStore()
		vkStateStore.RunGC(ctx)
		vkProvider := googleoauth.NewVKProvider(
			cabcfg.VKClientID(),
			cabcfg.VKClientSecret(),
			cabcfg.VKRedirectURL(),
			vkStateStore,
		)
		authSvc.SetVK(ctx, vkProvider)
		if oauthHandler == nil {
			oauthHandler = handlers.NewOAuth(authSvc, cabcfg.CookieDomain())
		}
	}

	// Telegram HMAC (Mini App initData + Login Widget 1.0): токены задаём всегда, если они
	// есть в окружении. CABINET_TELEGRAM_WEB_AUTH_MODE=oidc касается только веб-OAuth 2.0,
	// иначе POST /auth/telegram (miniapp) получает 501 из-за пустых telegramTokens.
	cabTelegramWidgetToken := cabcfg.TelegramLoginBotToken()
	if cabTelegramWidgetToken == "" {
		cabTelegramWidgetToken = config.TelegramToken()
	}
	if strings.TrimSpace(cabTelegramWidgetToken) != "" || strings.TrimSpace(config.TelegramToken()) != "" {
		authSvc.SetTelegramTokens(cabTelegramWidgetToken, config.TelegramToken())
	}
	if cabcfg.TelegramWidgetEnabled() {
		if oauthHandler == nil {
			oauthHandler = handlers.NewOAuth(authSvc, cabcfg.CookieDomain())
		}
	}
	if cabcfg.TelegramOIDCEnabled() {
		tgOIDCStore := googleoauth.NewTelegramOIDCStateStore()
		tgOIDCProvider := googleoauth.NewTelegramOIDCProvider(
			cabcfg.TelegramOIDCClientID(),
			cabcfg.TelegramOIDCClientSecret(),
			cabcfg.TelegramOIDCRedirectURL(),
			tgOIDCStore,
		)
		authSvc.SetTelegramOIDC(ctx, tgOIDCProvider)
		if oauthHandler == nil {
			oauthHandler = handlers.NewOAuth(authSvc, cabcfg.CookieDomain())
		}
	}

	tgWidgetBot := ""
	if cabcfg.TelegramWidgetEnabled() {
		tgWidgetBot = cabcfg.TelegramLoginBotUsername()
	}
	authHandler := handlers.NewAuth(
		authSvc,
		cabcfg.CookieDomain(),
		cabcfg.GoogleEnabled(),
		cabcfg.YandexEnabled(),
		cabcfg.VKEnabled(),
		tgWidgetBot,
		cabcfg.TelegramOIDCEnabled(),
		cabcfg.TelegramWebAuthMode(),
	)
	adminChecker := adminauth.NewChecker(identityRepo, accountRepo)

	contentHandler := handlers.NewCabinetContentHandler()
	// Профили Telegram (имя + аватарка) тянутся ботом и кэшируются в процессе.
	// Без бота сервис nil — кабинет тогда обходится данными OAuth-провайдеров.
	tgProfiles := tgprofile.New(tgBot)
	avatarSecret := []byte(cabcfg.JWTSecret())

	meHandler := handlers.NewMe(authSvc, accountRepo, identityRepo, linkRepo, customerBootstrap,
		paymentService, purchaseRepo, rw, customerRepo, database.NewDeviceNameRepository(pool), adminChecker, tgProfiles, avatarSecret, cabcfg.CookieDomain(), tgWidgetBot, cabcfg.GoogleEnabled(), cabcfg.YandexEnabled(), cabcfg.VKEnabled(), cabcfg.TelegramOIDCEnabled())
	tariffsHandler := handlers.NewTariffs(catalogSvc)
	subscriptionHandler := handlers.NewSubscription(subscriptionSvc)
	// Приглашения «подключить ещё устройство»: токен подписывается тем же
	// секретом кабинета (connectinvite выводит из него отдельный ключ).
	connectInviteHandler := handlers.NewConnectInvite(subscriptionSvc, cabcfg.JWTSecret())

	activityHandler := handlers.NewCabinetActivity(linkRepo, identityRepo, customerRepo, referralRepo, purchaseRepo, cabcfg.PublicURL())
	// Уведомления партнёрской программы. Без бота nil — тогда все вызовы
	// уведомлений становятся no-op, а сама программа работает как прежде.
	partnerNotifier := notification.NewPartnerNotifier(tgBot, customerRepo, translation.GetInstance(), cabcfg.PublicURL())
	partnerHandler := handlers.NewPartner(customerBootstrap, customerRepo, partnerRepo, cabcfg.PublicURL(), config.BotURL(), partnerNotifier)

	promoRepo := database.NewPromoRepository(pool)
	fortuneSvc := cabsvc.NewFortuneService(pool, linkRepo, customerBootstrap, customerRepo, purchaseRepo, promoRepo, fortRepo, rw)
	fortuneHandler := handlers.NewFortune(fortuneSvc)

	var promoCodesHandler *handlers.PromoCodesHandler
	if promoService != nil {
		promoCodesHandler = handlers.NewPromoCodes(customerBootstrap, customerRepo, linkRepo, promoService)
	}

	var supportHandler *handlers.SupportHandler
	if config.SupportBotAPIEnabled() {
		supportRepo := repository.NewSupportRepo(pool)
		supportBotClient := supportbot.NewClient(config.SupportBotAPIURL(), config.SupportBridgeSecret())
		supportSvc := cabsvc.NewSupport(
			supportRepo,
			accountRepo,
			identityRepo,
			linkRepo,
			customerRepo,
			customerBootstrap,
			subscriptionSvc,
			supportBotClient,
		)
		supportHandler = handlers.NewSupport(supportSvc)
	}

	// Платёжный слой кабинета: собираем, только если бот прокинул PaymentService.
	var paymentsHandler *handlers.PaymentsHandler
	if paymentService != nil {
		checkoutRepo := repository.NewCheckoutRepo(pool)
		checkoutCfg := payments.DefaultConfig()
		checkoutCfg.PublicURL = cabcfg.PublicURL()
		checkoutSvc := payments.NewCheckoutService(
			checkoutCfg,
			checkoutRepo, linkRepo, customerRepo, purchaseRepo, tariffRepo, accountRepo,
			customerBootstrap, paymentService,
			loyaltyRepo, promoService,
			rw,
		)
		paymentsHandler = handlers.NewPayments(checkoutSvc)
	}

	// Merge/Link service — Этап 8.
	nonceStore := linking.NewNonceStore()
	claimStore := linking.NewClaimStore()
	nonceStore.RunGC(ctx)
	claimStore.RunGC(ctx)
	mergeAuditRepo := repository.NewMergeAuditRepo(pool)
	mergeService := linking.New(
		pool, nonceStore, claimStore,
		customerRepo, linkRepo, mergeAuditRepo, accountRepo, identityRepo,
		mailer, cabTelegramWidgetToken,
		rw,
	)
	authSvc.SetMergeEmailPeerClaimSaver(mergeService.SaveEmailPeerClaim)
	authSvc.SetMergeTelegramClaimSaver(mergeService.SaveTelegramOIDCClaim)
	linkHandler := handlers.NewLink(mergeService)

	// Rate-limiters — по одному на правило (см. mvp-tz.md 8.3).
	loginIPLim := ratelimit.New(ratelimit.Rule{Count: 5, Interval: time.Minute})
	loginEmailLim := ratelimit.New(ratelimit.Rule{Count: 10, Interval: time.Hour})
	registerIPLim := ratelimit.New(ratelimit.Rule{Count: 3, Interval: time.Hour})
	forgotEmailLim := ratelimit.New(ratelimit.Rule{Count: 3, Interval: time.Hour})
	resendVerifyAcctLim := ratelimit.New(ratelimit.Rule{Count: 3, Interval: time.Hour})
	// Подтверждение email по коду из письма (публичный POST без сессии).
	verifyEmailConfirmIPLim := ratelimit.New(ratelimit.Rule{Count: 15, Interval: time.Minute})
	verifyResendPublicIPLim := ratelimit.New(ratelimit.Rule{Count: 10, Interval: time.Minute})
	// Платёжный лимитер: 20 запросов/минуту на account. Ключ — account_id,
	// потому что пользователь не должен страдать от NAT'а общего IP (а в
	// кабинет он уже авторизован, так что account_id точнее IP).
	paymentsAcctLim := ratelimit.New(ratelimit.Rule{Count: 20, Interval: time.Minute})
	// Подписка: 60 rpm/аккаунт — с запасом под polling статуса после оплаты
	// (UI дёргает раз в 3–5 сек на /checkout → /payments/:id/status → /me/subscription).
	subscriptionAcctLim := ratelimit.New(ratelimit.Rule{Count: 60, Interval: time.Minute})

	// Публичный резолв приглашения: 30/мин/IP. Эндпоинт открыт без авторизации
	// и в encrypted-режиме дёргает внешний crypto.happ.su, поэтому лимит стоит
	// и по IP, и по токену (см. connectTokenLim) — первый режет перебор с
	// одного адреса, второй ограничивает один разосланный токен.
	connectPublicIPLim := ratelimit.New(ratelimit.Rule{Count: 30, Interval: time.Minute})
	connectTokenLim := ratelimit.New(ratelimit.Rule{Count: 30, Interval: time.Minute})

	deleteAcctLim := ratelimit.New(ratelimit.Rule{Count: 5, Interval: time.Hour})
	trialActivateAcctLim := ratelimit.New(ratelimit.Rule{Count: 5, Interval: time.Hour})
	supportAcctLim := ratelimit.New(ratelimit.Rule{Count: 10, Interval: time.Minute})
	// Webhook от support-bot: 100/min/IP — внутренний endpoint, но защита от flood при утечке секрета
	supportWebhookIPLim := ratelimit.New(ratelimit.Rule{Count: 100, Interval: time.Minute})

	for _, lim := range []*ratelimit.Limiter{loginIPLim, loginEmailLim, registerIPLim, forgotEmailLim, resendVerifyAcctLim, verifyEmailConfirmIPLim, verifyResendPublicIPLim, paymentsAcctLim, subscriptionAcctLim, connectPublicIPLim, connectTokenLim, deleteAcctLim, trialActivateAcctLim, supportAcctLim, supportWebhookIPLim} {
		lim.RunGC(ctx)
	}

	// Rate-limiters для OAuth/Telegram.
	// Google: 20/ч/IP — дорогой flow с внешним запросом к Google; IP достаточно
	// (нет авторизации до callback'а).
	oauthIPLim := ratelimit.New(ratelimit.Rule{Count: 20, Interval: time.Hour})
	// Telegram: 10/min/IP — простой POST, но HMAC-проверка дешёвая.
	telegramIPLim := ratelimit.New(ratelimit.Rule{Count: 10, Interval: time.Minute})
	// Link/Merge: 10/min/account — защита от brute-force на confirm + flood на merge.
	linkAcctLim := ratelimit.New(ratelimit.Rule{Count: 10, Interval: time.Minute})

	for _, lim := range []*ratelimit.Limiter{oauthIPLim, telegramIPLim, linkAcctLim} {
		lim.RunGC(ctx)
	}

	// API-mux.
	api := http.NewServeMux()
	api.Handle("/cabinet/api/metrics", wrapMetricsBasicAuth(cabmetrics.Handler()))

	adminBootstrapHandler := handlers.NewAdminBootstrap()
	adminAcctLim := ratelimit.New(ratelimit.Rule{Count: 120, Interval: time.Minute})
	adminAcctLim.RunGC(ctx)

	statsRepo := database.NewStatsRepository(pool)
	infraBillingRepo := database.NewInfraBillingRepository(pool)
	runtimeSettingsRepo := database.NewRuntimeSettingsRepository(pool)

	adminStatsHandler := handlers.NewAdminStats(statsRepo, loyaltyRepo, customerRepo, promoRepo, partnerRepo)
	adminOverviewHandler := handlers.NewAdminOverview(statsRepo, partnerRepo, rw)
	adminUsersHandler := handlers.NewAdminUsers(customerRepo, purchaseRepo, referralRepo, tariffRepo, loyaltyRepo, rw, avatarSecret)
	// CheckoutRepo — только на чтение (ключ идемпотентности/провайдер в модалке платежа);
	// не зависит от того, собран ли checkoutSvc (PaymentService может быть nil).
	adminPaymentsCheckoutRepo := repository.NewCheckoutRepo(pool)
	adminPaymentsHandler := handlers.NewAdminPayments(purchaseRepo, tariffRepo, promoRepo, adminPaymentsCheckoutRepo, customerRepo)
	adminPromosHandler := handlers.NewAdminPromos(promoRepo)
	// Применение состава сквадов к действующим подписчикам тарифа.
	// rw может быть nil (панель не сконфигурирована) — сервис это переживает,
	// массовое применение просто отвечает 503.
	tariffSquadsSvc := tariffsquads.New(customerRepo, tariffRepo, rw)
	adminTariffsHandler := handlers.NewAdminTariffs(tariffRepo, tariffSquadsSvc)
	adminLoyaltyHandler := handlers.NewAdminLoyalty(loyaltyRepo, customerRepo, purchaseRepo)
	adminPartnersHandler := handlers.NewAdminPartners(partnerRepo, customerRepo, config.BotURL(), partnerNotifier)
	adminBroadcastHandler := handlers.NewAdminBroadcast(customerRepo, tariffRepo, broadcastSender, tgBot)
	adminInfraHandler := handlers.NewAdminInfra(rw, infraBillingRepo)
	adminSettingsHandler := handlers.NewAdminSettings(runtimeSettingsRepo)
	adminSquadsHandler := handlers.NewAdminSquads(rw)
	var adminSyncHandler *handlers.AdminSyncHandler
	if syncService != nil {
		adminSyncHandler = handlers.NewAdminSync(syncService)
	}

	registerAPIRoutes(api, authHandler, contentHandler, meHandler, tariffsHandler, subscriptionHandler, connectInviteHandler, activityHandler, partnerHandler, promoCodesHandler, oauthHandler, paymentsHandler, linkHandler, fortuneHandler, supportHandler, jwtIssuer,
		adminChecker, adminBootstrapHandler, adminStatsHandler, adminOverviewHandler, adminUsersHandler, adminPaymentsHandler, adminPromosHandler, adminTariffsHandler, adminLoyaltyHandler, adminPartnersHandler, adminBroadcastHandler, adminInfraHandler, adminSettingsHandler, adminSquadsHandler, adminSyncHandler, adminAcctLim,
		loginIPLim, loginEmailLim, registerIPLim, forgotEmailLim, resendVerifyAcctLim, verifyEmailConfirmIPLim, verifyResendPublicIPLim, paymentsAcctLim, subscriptionAcctLim, connectPublicIPLim, connectTokenLim, deleteAcctLim, trialActivateAcctLim, supportAcctLim, supportWebhookIPLim,
		oauthIPLim, telegramIPLim, linkAcctLim)

	statusTargets := statusprobe.NewKVTargets(runtimeSettingsRepo)
	statusRunner := statusprobe.NewRunner(statusprobe.NewKVHistory(runtimeSettingsRepo))
	publicStatus := handlers.NewPublicStatus(rw, statusRunner, statusTargets)
	api.Handle("/cabinet/api/public/status",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(publicStatus.Get),
		}),
	)
	adminStatusTargets := handlers.NewAdminStatusTargets(statusTargets, statusRunner)
	api.Handle("/cabinet/api/admin/status/targets",
		middleware.Chain(
			http.HandlerFunc(adminStatusTargets.Handle),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_status_targets")),
		),
	)
	api.Handle("/cabinet/api/admin/status/probe",
		middleware.Chain(
			http.HandlerFunc(adminStatusTargets.Probe),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_status_probe")),
		),
	)

	// 404 JSON на любой неизвестный /cabinet/api/*.
	api.HandleFunc("/cabinet/api/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
	})

	// Внешняя цепочка для API: request_id/recover/logger/CORS/security/http metrics.
	apiChain := middleware.Chain(
		api,
		middleware.RequestID(),
		middleware.Recover(),
		middleware.Logger(),
		middleware.CORS(cabcfg.AllowedOrigins()),
		middleware.SecurityHeadersAPI(),
		middleware.HTTPMetrics(),
	)

	// SPA chain — без CORS (same-origin).
	spa := buildSPAHandler(spaFS)
	spaChain := middleware.Chain(
		spa,
		middleware.RequestID(),
		middleware.Recover(),
		middleware.Logger(),
		middleware.SecurityHeadersSPA(),
		middleware.HTTPMetrics(),
	)

	mux.Handle("/cabinet/api/", apiChain)
	mux.Handle("/cabinet/", spaChain)

	// Публичный лендинг — корень домена (https://host/). Старый адрес /landing
	// оставлен, чтобы уже разосланные ссылки не сломались. {$} — только «/»,
	// вебхуки и /healthcheck на том же mux не перехватываются.
	// Ассеты SPA по-прежнему под /cabinet/ (vite base).
	mux.Handle("/{$}", spaChain)
	mux.Handle("/landing", spaChain)
	mux.Handle("/landing/", spaChain)
	mux.Handle("/status", spaChain)
	mux.Handle("/status/", spaChain)

	// Часто открывают кабинет как https://host/login без префикса /cabinet — иначе 404.
	registerCabinetRootRedirects(mux)
	return nil
}

// registerCabinetRootRedirects — GET/HEAD /login, /register → /cabinet/… (сохраняем query).
func registerCabinetRootRedirects(mux *http.ServeMux) {
	type pair struct{ from, to string }
	for _, p := range []pair{
		{"/login", "/cabinet/login"},
		{"/register", "/cabinet/register"},
	} {
		from, to := p.from, p.to
		mux.HandleFunc(from, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			target := to
			if q := r.URL.RawQuery; q != "" {
				target += "?" + q
			}
			http.Redirect(w, r, target, http.StatusFound)
		})
	}
}

// registerAPIRoutes регистрирует конкретные эндпоинты. Вынесено отдельно, чтобы
// Mount не раздувался и чтобы было видно, какой middleware применён к какому роуту.
//
// paymentsHandler может быть nil — если PaymentService в процесс не
// прокинут, роуты /payments/* просто не регистрируются, и общий 404-хендлер
// /cabinet/api/ их закроет JSON-ом.
func registerAPIRoutes(
	api *http.ServeMux,
	auth *handlers.AuthHandler,
	content *handlers.CabinetContentHandler,
	me *handlers.MeHandler,
	tariffs *handlers.TariffsHandler,
	subscription *handlers.SubscriptionHandler,
	connectInvite *handlers.ConnectInviteHandler,
	activity *handlers.CabinetActivityHandler,
	partner *handlers.PartnerHandler,
	promocodes *handlers.PromoCodesHandler,
	oauthH *handlers.OAuthHandler,
	pay *handlers.PaymentsHandler,
	link *handlers.LinkHandler,
	fortune *handlers.FortuneHandler,
	support *handlers.SupportHandler,
	jwtIssuer *jwt.Issuer,
	adminChecker *adminauth.Checker,
	adminBootstrap *handlers.AdminBootstrapHandler,
	adminStats *handlers.AdminStatsHandler,
	adminOverview *handlers.AdminOverviewHandler,
	adminUsers *handlers.AdminUsersHandler,
	adminPayments *handlers.AdminPaymentsHandler,
	adminPromos *handlers.AdminPromosHandler,
	adminTariffs *handlers.AdminTariffsHandler,
	adminLoyalty *handlers.AdminLoyaltyHandler,
	adminPartners *handlers.AdminPartnersHandler,
	adminBroadcast *handlers.AdminBroadcastHandler,
	adminInfra *handlers.AdminInfraHandler,
	adminSettings *handlers.AdminSettingsHandler,
	adminSquads *handlers.AdminSquadsHandler,
	adminSync *handlers.AdminSyncHandler,
	adminAcctLim,
	loginIPLim, loginEmailLim, registerIPLim, forgotEmailLim, resendVerifyAcctLim, verifyEmailConfirmIPLim, verifyResendPublicIPLim, paymentsAcctLim, subscriptionAcctLim, connectPublicIPLim, connectTokenLim, deleteAcctLim, trialActivateAcctLim, supportAcctLim, supportWebhookIPLim,
	oauthIPLim, telegramIPLim, linkAcctLim *ratelimit.Limiter,
) {
	// Healthz — без middleware, открыт всем.
	api.HandleFunc("/cabinet/api/healthz", healthz)

	// GET /auth/bootstrap — флаги OAuth/Telegram для SPA (логин до выдачи JWT).
	api.Handle("/cabinet/api/auth/bootstrap",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(auth.AuthBootstrap),
		}),
	)
	// GET /content/faq — runtime JSON-контент кабинета (из /translations/cabinet/FAQ.json).
	api.Handle("/cabinet/api/content/faq",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(content.FAQ),
		}),
	)
	// GET /content/app-config — runtime JSON-гайды подключения устройств.
	api.Handle("/cabinet/api/content/app-config",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(content.AppConfig),
		}),
	)
	// GET /content/i18n/{lang} — runtime UI-переводы (из volume /translations/cabinet/i18n).
	api.Handle("/cabinet/api/content/i18n/{lang}",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(content.I18n),
		}),
	)
	// GET /public/pwa-manifest.webmanifest — runtime PWA manifest из CABINET_PWA_*.
	api.Handle("/cabinet/api/public/pwa-manifest.webmanifest",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(content.PWAManifest),
		}),
	)

	// GET /public/brand-logo — статичный логотип с диска (CABINET_BRAND_LOGO_FILE), без auth.
	if p := cabcfg.BrandLogoFile(); p != "" {
		api.Handle("/cabinet/api/public/brand-logo",
			methodRouter(map[string]http.Handler{
				http.MethodGet: http.HandlerFunc(handlers.ServeBrandLogo(p)),
			}),
		)
	}
	if p := cabcfg.SupportLogoFile(); p != "" {
		api.Handle("/cabinet/api/public/support-logo",
			methodRouter(map[string]http.Handler{
				http.MethodGet: http.HandlerFunc(handlers.ServeBrandLogo(p)),
			}),
		)
	}

	// GET /public/connect?t=<token> — резолв приглашения «подключить ещё
	// устройство» в deep link. Единственный публичный эндпоинт, отдающий
	// подписку: получатель приглашения по определению не авторизован, ему
	// прислали ссылку на другое устройство. Барьеры — подпись и срок токена
	// плюс два rate-limit'а (по IP и по самому токену).
	api.Handle("/cabinet/api/public/connect",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(connectInvite.Resolve),
				middleware.RateLimit(connectPublicIPLim, ipKey("public-connect")),
				middleware.RateLimit(connectTokenLim, queryKey("public-connect", "t")),
			),
		}),
	)

	// GET /tariffs — публичная витрина. Без RequireAuth: показывается на
	// странице логина/регистрации; чтения из БД минимальные, rate-limit не
	// ставим (есть кэш на клиенте и Cache-Control в ответе).
	api.Handle("/cabinet/api/tariffs",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(tariffs.List),
		}),
	)

	// POST /auth/register (rate-limit 3/h/IP).
	api.Handle("/cabinet/api/auth/register",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.Register),
			middleware.RequireTurnstile(),
			middleware.RateLimit(registerIPLim, ipKey("register")),
		)),
	)

	// POST /auth/login (rate-limit 5/min/IP + 10/h/email).
	api.Handle("/cabinet/api/auth/login",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.Login),
			middleware.RequireTurnstile(),
			middleware.RateLimit(loginIPLim, ipKey("login")),
			middleware.RateLimit(loginEmailLim, emailBodyKey("login")),
		)),
	)

	// POST /auth/refresh. Без rate-limit (rotation сама по себе ограничивает
	// злоупотребление — два запроса подряд триггерят reuse detection).
	// CSRF обязателен: атакующий с чужого origin'а не сможет прочитать
	// csrf_token cookie и подставить его в X-CSRF-Token.
	api.Handle("/cabinet/api/auth/refresh",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.Refresh),
			middleware.CSRF(),
		)),
	)

	// POST /auth/logout. Идемпотентный, но тоже мутирующий — CSRF нужен, чтобы
	// кто-то с чужого origin'а не «разлогинил» пользователя через CSRF-атаку.
	api.Handle("/cabinet/api/auth/logout",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.Logout),
			middleware.CSRF(),
		)),
	)

	// POST /auth/password/forgot (rate-limit 3/h/email).
	api.Handle("/cabinet/api/auth/password/forgot",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.ForgotPassword),
			middleware.RequireTurnstile(),
			middleware.RateLimit(forgotEmailLim, emailBodyKey("forgot")),
		)),
	)

	// POST /auth/email/verify/resend-public — повторная отправка кода без JWT (после регистрации).
	api.Handle("/cabinet/api/auth/email/verify/resend-public",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.ResendVerifyEmailPublic),
			middleware.RequireTurnstile(),
			middleware.RateLimit(verifyResendPublicIPLim, ipKey("email_verify_resend_public")),
			middleware.RateLimit(forgotEmailLim, emailBodyKey("email_verify_resend_public")),
		)),
	)

	// POST /auth/password/reset.
	api.Handle("/cabinet/api/auth/password/reset", onlyPOST(http.HandlerFunc(auth.ResetPassword)))

	// POST /auth/email/verify/confirm (rate-limit по IP — защита перебора кода).
	api.Handle("/cabinet/api/auth/email/verify/confirm",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(auth.ConfirmEmail),
			middleware.RateLimit(verifyEmailConfirmIPLim, ipKey("email_verify_confirm")),
		)),
	)

	// GET /avatar?t=<token> — аватарка владельца токена. Маршрут вне RequireAuth
	// намеренно: <img> не умеет слать Authorization, доступ открывает
	// подписанная ссылка из /me (см. пакет avatartoken).
	api.Handle("/cabinet/api/avatar",
		methodRouter(map[string]http.Handler{
			http.MethodGet: http.HandlerFunc(me.Avatar),
		}),
	)

	// ======== Защищённые эндпоинты (RequireAuth + CSRF для мутирующих) ========

	// GET /me.
	api.Handle("/cabinet/api/me",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.Me),
				middleware.RequireAuth(jwtIssuer),
			),
		}),
	)

	// PUT /me/language.
	api.Handle("/cabinet/api/me/language",
		methodRouter(map[string]http.Handler{
			http.MethodPut: middleware.Chain(
				http.HandlerFunc(me.PutLanguage),
				middleware.RequireAuth(jwtIssuer),
				middleware.CSRF(),
			),
		}),
	)

	// PUT /me/password — смена пароля (новая сессия в ответе).
	api.Handle("/cabinet/api/me/password",
		methodRouter(map[string]http.Handler{
			http.MethodPut: middleware.Chain(
				http.HandlerFunc(me.PutPassword),
				middleware.RequireAuth(jwtIssuer),
				middleware.CSRF(),
			),
		}),
	)

	// POST /me/email/verify/resend.
	api.Handle("/cabinet/api/me/email/verify/resend",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.ResendVerifyEmail),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(resendVerifyAcctLim, accountKey("resend_verify")),
		)),
	)

	// POST /me/email/link — привязка email+пароля к OAuth/Telegram-аккаунту (письмо подтверждения).
	api.Handle("/cabinet/api/me/email/link",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostLinkEmail),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(resendVerifyAcctLim, accountKey("email_link")),
		)),
	)
	// POST /me/email/link/verify-code — подтверждение merge-кода для email-конфликта OAuth-only.
	api.Handle("/cabinet/api/me/email/link/verify-code",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostLinkEmailVerifyCode),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(resendVerifyAcctLim, accountKey("email_link_verify_code")),
		)),
	)

	// POST /me/identities/unlink — снятие привязки google|telegram (тело: {"provider":"google"}).
	api.Handle("/cabinet/api/me/identities/unlink",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostIdentityUnlink),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
		)),
	)
	api.Handle("/cabinet/api/me/telegram/link/start",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.TelegramLinkOIDCStart),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(oauthIPLim, accountKey("telegram_link_oidc_start")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/google/link/start",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.GoogleLinkStart),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(oauthIPLim, accountKey("google_link_start")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/yandex/link/start",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.YandexLinkStart),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(oauthIPLim, accountKey("yandex_link_start")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/vk/link/start",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.VKLinkStart),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(oauthIPLim, accountKey("vk_link_start")),
			),
		}),
	)

	// POST /me/account/delete — удаление аккаунта кабинета (тело: {"confirm":"DELETE"}).
	api.Handle("/cabinet/api/me/account/delete",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostAccountDelete),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(deleteAcctLim, accountKey("delete_account")),
		)),
	)

	// GET /me/trial — trial-параметры (дни/лимиты) и возможность активации.
	api.Handle("/cabinet/api/me/trial",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.GetTrial),
				middleware.RequireAuth(jwtIssuer),
			),
		}),
	)

	// POST /me/trial/activate — активация пробного периода.
	api.Handle("/cabinet/api/me/trial/activate",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostTrialActivate),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(trialActivateAcctLim, accountKey("trial_activate")),
		)),
	)
	api.Handle("/cabinet/api/me/devices",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.GetDevices),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("devices")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/traffic-usage",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(me.GetTrafficUsage),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("traffic_usage")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/devices/delete",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.DeleteDevice),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireVerifiedEmail(),
			middleware.CSRF(),
			middleware.RateLimit(subscriptionAcctLim, accountKey("devices_delete")),
		)),
	)
	api.Handle("/cabinet/api/me/devices/rename",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.RenameDevice),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireVerifiedEmail(),
			middleware.CSRF(),
			middleware.RateLimit(subscriptionAcctLim, accountKey("devices_rename")),
		)),
	)
	api.Handle("/cabinet/api/me/hwid-extra/apply",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(me.PostHwidExtraApply),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireVerifiedEmail(),
			middleware.CSRF(),
			middleware.RateLimit(subscriptionAcctLim, accountKey("hwid_extra_apply")),
		)),
	)

	// OAuth/Telegram маршруты включаются только если хотя бы один провайдер настроен.
	if oauthH != nil {
		// GET /auth/google/start — редирект на Google. Rate-limit 20/ч/IP.
		api.Handle("/cabinet/api/auth/google/start",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.GoogleStart),
					middleware.RateLimit(oauthIPLim, ipKey("google_start")),
				),
			}),
		)
		// GET /auth/google/callback — обмен code → сессия (или pending link).
		api.Handle("/cabinet/api/auth/google/callback",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.GoogleCallback),
					middleware.RateLimit(oauthIPLim, ipKey("google_callback")),
				),
			}),
		)
		api.Handle("/cabinet/api/auth/yandex/start",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.YandexStart),
					middleware.RateLimit(oauthIPLim, ipKey("yandex_start")),
				),
			}),
		)
		api.Handle("/cabinet/api/auth/yandex/callback",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.YandexCallback),
					middleware.RateLimit(oauthIPLim, ipKey("yandex_callback")),
				),
			}),
		)
		api.Handle("/cabinet/api/auth/vk/start",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.VKStart),
					middleware.RateLimit(oauthIPLim, ipKey("vk_start")),
				),
			}),
		)
		api.Handle("/cabinet/api/auth/vk/callback",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.VKCallback),
					middleware.RateLimit(oauthIPLim, ipKey("vk_callback")),
				),
			}),
		)
		// GET /auth/google/confirm?token=... — подтверждение привязки по ссылке из письма.
		api.Handle("/cabinet/api/auth/google/confirm",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.GoogleLinkConfirm),
					middleware.RateLimit(oauthIPLim, ipKey("google_confirm")),
				),
			}),
		)
		// POST /auth/telegram — Widget или MiniApp. Rate-limit 10/min/IP.
		api.Handle("/cabinet/api/auth/telegram",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(oauthH.TelegramLogin),
				middleware.RateLimit(telegramIPLim, ipKey("telegram_login")),
			)),
		)
		api.Handle("/cabinet/api/auth/telegram/start",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.TelegramOIDCStart),
					middleware.RateLimit(oauthIPLim, ipKey("telegram_oidc_start")),
				),
			}),
		)
		api.Handle("/cabinet/api/auth/telegram/callback",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(oauthH.TelegramOIDCCallback),
					middleware.RateLimit(oauthIPLim, ipKey("telegram_oidc_callback")),
				),
			}),
		)
	}

	// GET /me/subscription. Требует подтверждённого email:
	// subscription_link — по сути секрет, это барьер против утечек с
	// неподтверждённых (и потенциально угнанных) почт.
	api.Handle("/cabinet/api/me/subscription",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(subscription.Get),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("subscription")),
			),
		}),
	)
	// GET /me/connect-invite — короткая ссылка-приглашение на устройство без
	// доступа к кабинету. Барьеры те же, что у /me/subscription: токен
	// обменивается на подписку.
	api.Handle("/cabinet/api/me/connect-invite",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(connectInvite.Create),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("connect-invite")),
			),
		}),
	)
	// GET /me/deeplink?app=happ|incy — зашифрованный deep link подключения.
	// Те же барьеры, что и /me/subscription: по сути отдаёт ссылку подписки.
	api.Handle("/cabinet/api/me/deeplink",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(subscription.Deeplink),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("deeplink")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/loyalty",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(subscription.Loyalty),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("loyalty")),
			),
		}),
	)
	api.Handle("/cabinet/api/me/loyalty/history",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(subscription.LoyaltyHistory),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("loyalty-history")),
			),
		}),
	)

	if fortune != nil {
		api.Handle("/cabinet/api/fortune/status",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(fortune.Status),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(subscriptionAcctLim, accountKey("fortune")),
				),
			}),
		)
		api.Handle("/cabinet/api/fortune/recent-wins",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(fortune.RecentWins),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(subscriptionAcctLim, accountKey("fortune_feed")),
				),
			}),
		)
		api.Handle("/cabinet/api/fortune/spin",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(fortune.Spin),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.CSRF(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("fortune_spin")),
			)),
		)
	}

	if activity != nil {
		api.Handle("/cabinet/api/me/referrals",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(activity.GetReferrals),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(subscriptionAcctLim, accountKey("referrals")),
				),
			}),
		)
		api.Handle("/cabinet/api/me/purchases",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(activity.GetPurchases),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(subscriptionAcctLim, accountKey("purchases")),
				),
			}),
		)
	}

	// Кабинет партнёра. Чтение — только авторизация и лимит; всё, что меняет
	// деньги или ссылки, дополнительно проходит CSRF, как остальные мутирующие
	// ручки кабинета.
	if partner != nil {
		partnerRead := func(h http.HandlerFunc, route string) http.Handler {
			return middleware.Chain(h,
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.RateLimit(subscriptionAcctLim, accountKey(route)),
			)
		}
		partnerWrite := func(h http.HandlerFunc, route string) http.Handler {
			return middleware.Chain(h,
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.CSRF(),
				middleware.RateLimit(subscriptionAcctLim, accountKey(route)),
			)
		}

		api.Handle("/cabinet/api/me/partner",
			methodRouter(map[string]http.Handler{
				http.MethodGet: partnerRead(partner.GetState, "partner_state"),
			}),
		)
		api.Handle("/cabinet/api/me/partner/apply",
			methodRouter(map[string]http.Handler{
				http.MethodPost: partnerWrite(partner.Apply, "partner_apply"),
			}),
		)
		api.Handle("/cabinet/api/me/partner/customers",
			methodRouter(map[string]http.Handler{
				http.MethodGet: partnerRead(partner.GetCustomers, "partner_customers"),
			}),
		)
		api.Handle("/cabinet/api/me/partner/earnings",
			methodRouter(map[string]http.Handler{
				http.MethodGet: partnerRead(partner.GetEarnings, "partner_earnings"),
			}),
		)
		api.Handle("/cabinet/api/me/partner/links",
			methodRouter(map[string]http.Handler{
				http.MethodPost: partnerWrite(partner.CreateLink, "partner_link_create"),
			}),
		)
		// Слэш на конце — префиксный маршрут: id потока разбирается из пути.
		api.Handle("/cabinet/api/me/partner/links/",
			partnerWrite(partner.LinkByID, "partner_link_edit"),
		)
		api.Handle("/cabinet/api/me/partner/payout-details",
			methodRouter(map[string]http.Handler{
				http.MethodPut: partnerWrite(partner.PutPayoutDetails, "partner_payout_details"),
			}),
		)
		api.Handle("/cabinet/api/me/partner/payouts",
			methodRouter(map[string]http.Handler{
				http.MethodGet:  partnerRead(partner.Payouts, "partner_payouts"),
				http.MethodPost: partnerWrite(partner.Payouts, "partner_payout_create"),
			}),
		)
	}

	if support != nil {
		api.Handle("/cabinet/api/support/summary",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(support.Summary),
					middleware.RequireAuth(jwtIssuer),
					middleware.RateLimit(subscriptionAcctLim, accountKey("support_summary")),
				),
			}),
		)
		api.Handle("/cabinet/api/support/conversation",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(support.Conversation),
					middleware.RequireAuth(jwtIssuer),
					middleware.RateLimit(subscriptionAcctLim, accountKey("support_conversation")),
				),
			}),
		)
		api.Handle("/cabinet/api/support/messages",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(support.SendMessage),
				middleware.RequireAuth(jwtIssuer),
				middleware.CSRF(),
				middleware.RateLimit(supportAcctLim, accountKey("support_send")),
			)),
		)
		api.Handle("/cabinet/api/support/read",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(support.MarkRead),
				middleware.RequireAuth(jwtIssuer),
				middleware.CSRF(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("support_read")),
			)),
		)
		api.Handle("/cabinet/api/internal/support/webhook",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(support.Webhook),
				middleware.RateLimit(supportWebhookIPLim, ipKey("support_webhook")),
			)),
		)
	}

	if promocodes != nil {
		api.Handle("/cabinet/api/promocodes/state",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(promocodes.GetState),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(subscriptionAcctLim, accountKey("promocodes_state")),
				),
			}),
		)
		api.Handle("/cabinet/api/promocodes/apply",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(promocodes.Apply),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.CSRF(),
				middleware.RateLimit(subscriptionAcctLim, accountKey("promocodes_apply")),
			)),
		)
	}

	// Платёжный слой — только если бот прокинул PaymentService.
	if pay != nil {
		// POST /payments/checkout. RequireAuth + CSRF + 20/min/account.
		api.Handle("/cabinet/api/payments/checkout",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(pay.Checkout),
				middleware.RequireAuth(jwtIssuer),
				middleware.CSRF(),
				middleware.RateLimit(paymentsAcctLim, accountKey("payments")),
			)),
		)

		// GET /payments/preview — расчёт суммы (upgrade/downgrade как у бота), без CSRF.
		api.Handle("/cabinet/api/payments/preview",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(pay.Preview),
					middleware.RequireAuth(jwtIssuer),
					middleware.RateLimit(paymentsAcctLim, accountKey("payments")),
				),
			}),
		)
		api.Handle("/cabinet/api/payments/hwid/preview",
			methodRouter(map[string]http.Handler{
				http.MethodGet: middleware.Chain(
					http.HandlerFunc(pay.HwidPreview),
					middleware.RequireAuth(jwtIssuer),
					middleware.RequireVerifiedEmail(),
					middleware.RateLimit(paymentsAcctLim, accountKey("payments")),
				),
			}),
		)
		api.Handle("/cabinet/api/payments/hwid/checkout",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(pay.HwidCheckout),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireVerifiedEmail(),
				middleware.CSRF(),
				middleware.RateLimit(paymentsAcctLim, accountKey("payments")),
			)),
		)

		// GET /payments/{id}/status. Префиксный маршрут на ServeMux — сам хендлер
		// разбирает :id из пути. Без CSRF (идемпотентный GET), но тот же 20/min/account.
		api.Handle("/cabinet/api/payments/",
			middleware.Chain(
				http.HandlerFunc(pay.Status),
				middleware.RequireAuth(jwtIssuer),
				middleware.RateLimit(paymentsAcctLim, accountKey("payments")),
			),
		)
	}

	// ======== Link / Merge (Этап 8) ========
	// Все маршруты требуют RequireAuth + CSRF (мутирующие POST) + 10/min/account.

	// POST /link/telegram/start — генерация nonce для Telegram Widget.
	api.Handle("/cabinet/api/link/telegram/start",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(link.TelegramStart),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(linkAcctLim, accountKey("link_tg_start")),
		)),
	)

	// POST /link/telegram/confirm — валидация Telegram payload + nonce.
	api.Handle("/cabinet/api/link/telegram/confirm",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(link.TelegramConfirm),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(linkAcctLim, accountKey("link_tg_confirm")),
		)),
	)

	// POST /link/merge/preview — dry-run merge, требует confirmed claim.
	api.Handle("/cabinet/api/link/merge/preview",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(link.MergePreview),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(linkAcctLim, accountKey("link_merge_preview")),
		)),
	)

	// POST /link/merge/confirm — реальный merge (Idempotency-Key header обязателен).
	api.Handle("/cabinet/api/link/merge/confirm",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(link.MergeConfirm),
			middleware.RequireAuth(jwtIssuer),
			middleware.CSRF(),
			middleware.RateLimit(linkAcctLim, accountKey("link_merge_confirm")),
		)),
	)

	// ======== Admin panel (RequireAuth + RequireAdmin) ========

	api.Handle("/cabinet/api/admin/bootstrap",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminBootstrap.Bootstrap),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_bootstrap")),
			),
		}),
	)

	api.Handle("/cabinet/api/admin/squads",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminSquads.List),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_squads")),
			),
		}),
	)

	api.Handle("/cabinet/api/admin/overview",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminOverview.Overview),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_overview")),
			),
		}),
	)

	// Admin Stats
	api.Handle("/cabinet/api/admin/stats",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.Stats),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/stats/timeseries",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.TimeSeries),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats_timeseries")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/stats/insights",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.Insights),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats_insights")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/stats/fortune",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.FortuneStats),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats_fortune")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/stats/loyalty",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.LoyaltyStats),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats_loyalty")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/stats/promos",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminStats.PromoStats),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_stats_promos")),
			),
		}),
	)

	// Admin Users
	api.Handle("/cabinet/api/admin/users",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminUsers.List),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_users")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/users/search",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminUsers.Search),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_users_search")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/users/",
		middleware.Chain(
			http.HandlerFunc(adminUsers.HandleByID),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_users_byid")),
		),
	)

	// Admin Payments (только чтение: список / карточка / CSV-экспорт).
	api.Handle("/cabinet/api/admin/payments",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminPayments.List),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_payments")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/payments/",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminPayments.HandleByID),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_payments_byid")),
			),
		}),
	)

	// Admin Promos
	api.Handle("/cabinet/api/admin/promos",
		middleware.Chain(
			http.HandlerFunc(adminPromos.Handle),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_promos")),
		),
	)
	api.Handle("/cabinet/api/admin/promos/",
		middleware.Chain(
			http.HandlerFunc(adminPromos.HandleByID),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_promos_byid")),
		),
	)

	// Admin Tariffs
	api.Handle("/cabinet/api/admin/tariffs",
		middleware.Chain(
			http.HandlerFunc(adminTariffs.Handle),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_tariffs")),
		),
	)
	api.Handle("/cabinet/api/admin/tariffs/",
		middleware.Chain(
			http.HandlerFunc(adminTariffs.HandleByID),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_tariffs_byid")),
		),
	)

	// Admin Partners — партнёрская программа: заявки, партнёры, выплаты.
	// Один префиксный маршрут: разбор действия внутри обработчика, иначе
	// полтора десятка регистраций с одинаковой цепочкой middleware.
	if adminPartners != nil {
		adminPartnersChain := func(h http.Handler, route string) http.Handler {
			return middleware.Chain(h,
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.CSRF(),
				middleware.RateLimit(adminAcctLim, accountKey(route)),
			)
		}
		api.Handle("/cabinet/api/admin/partners",
			adminPartnersChain(http.HandlerFunc(adminPartners.Handle), "admin_partners"))
		api.Handle("/cabinet/api/admin/partners/",
			adminPartnersChain(http.HandlerFunc(adminPartners.Handle), "admin_partners_action"))
	}

	// Admin Loyalty
	api.Handle("/cabinet/api/admin/loyalty/tiers",
		middleware.Chain(
			http.HandlerFunc(adminLoyalty.Handle),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_loyalty")),
		),
	)
	api.Handle("/cabinet/api/admin/loyalty/tiers/",
		middleware.Chain(
			http.HandlerFunc(adminLoyalty.HandleByID),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_loyalty_byid")),
		),
	)
	api.Handle("/cabinet/api/admin/loyalty/recalc",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(adminLoyalty.TriggerRecalc),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_loyalty_recalc")),
		)),
	)

	// Admin Broadcast
	api.Handle("/cabinet/api/admin/broadcast/audiences",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminBroadcast.Audiences),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_broadcast_audiences")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/broadcast/preview",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(adminBroadcast.Preview),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_broadcast_preview")),
		)),
	)
	api.Handle("/cabinet/api/admin/broadcast/tariffs",
		methodRouter(map[string]http.Handler{
			http.MethodGet: middleware.Chain(
				http.HandlerFunc(adminBroadcast.Tariffs),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.RateLimit(adminAcctLim, accountKey("admin_broadcast_tariffs")),
			),
		}),
	)
	api.Handle("/cabinet/api/admin/broadcast/send",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(adminBroadcast.Send),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_broadcast_send")),
		)),
	)
	api.Handle("/cabinet/api/admin/broadcast/upload-media",
		onlyPOST(middleware.Chain(
			http.HandlerFunc(adminBroadcast.UploadMedia),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_broadcast_upload")),
		)),
	)

	// Admin Infra
	api.Handle("/cabinet/api/admin/infra/nodes",
		middleware.Chain(
			http.HandlerFunc(adminInfra.HandleNodes),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_nodes")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/nodes/",
		middleware.Chain(
			http.HandlerFunc(adminInfra.DeleteNode),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_nodes_del")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/providers",
		middleware.Chain(
			http.HandlerFunc(adminInfra.HandleProviders),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_providers")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/providers/",
		middleware.Chain(
			http.HandlerFunc(adminInfra.DeleteProvider),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_providers_del")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/history",
		middleware.Chain(
			http.HandlerFunc(adminInfra.HandleHistory),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_history")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/history/",
		middleware.Chain(
			http.HandlerFunc(adminInfra.DeleteHistory),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_history_del")),
		),
	)
	api.Handle("/cabinet/api/admin/infra/settings",
		middleware.Chain(
			http.HandlerFunc(adminInfra.HandleSettings),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_infra_settings")),
		),
	)

	api.Handle("/cabinet/api/admin/settings",
		middleware.Chain(
			http.HandlerFunc(adminSettings.Handle),
			middleware.RequireAuth(jwtIssuer),
			middleware.RequireAdmin(adminChecker),
			middleware.CSRF(),
			middleware.RateLimit(adminAcctLim, accountKey("admin_bot_settings")),
		),
	)

	// Admin Sync
	if adminSync != nil {
		api.Handle("/cabinet/api/admin/sync",
			onlyPOST(middleware.Chain(
				http.HandlerFunc(adminSync.TriggerSync),
				middleware.RequireAuth(jwtIssuer),
				middleware.RequireAdmin(adminChecker),
				middleware.CSRF(),
				middleware.RateLimit(adminAcctLim, accountKey("admin_sync")),
			)),
		)
	}
}

// ============================================================================
// Helpers: rate-limit key-функции и method-гварды
// ============================================================================

// ipKey возвращает функцию, формирующую ключ <route>:<ip>.
func ipKey(route string) middleware.KeyFunc {
	return func(r *http.Request) string {
		ip := middleware.ClientIP(r)
		if ip == "" {
			return ""
		}
		return route + ":" + ip
	}
}

// queryKey строит ключ <route>:<значение query-параметра>. Нужен там, где
// лимит логично вешать на сам предъявленный токен, а не только на IP.
func queryKey(route, param string) middleware.KeyFunc {
	return func(r *http.Request) string {
		v := strings.TrimSpace(r.URL.Query().Get(param))
		if v == "" {
			return ""
		}
		return route + ":" + v
	}
}

// emailBodyKey читает JSON-поле `email` из тела запроса для построения ключа
// <route>:<email>. Тело после чтения пересоздаётся, чтобы следующий handler
// смог его прочитать заново.
func emailBodyKey(route string) middleware.KeyFunc {
	return func(r *http.Request) string {
		raw, err := readAndRestoreBody(r, 64*1024)
		if err != nil || len(raw) == 0 {
			return ""
		}
		var body struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return ""
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" {
			return ""
		}
		return route + ":" + email
	}
}

// accountKey берёт account_id из JWT claims (должен быть после RequireAuth!).
func accountKey(route string) middleware.KeyFunc {
	return func(r *http.Request) string {
		claims := middleware.AuthClaims(r)
		if claims == nil {
			return ""
		}
		return fmt.Sprintf("%s:%d", route, claims.AccountID)
	}
}

// readAndRestoreBody читает тело запроса и кладёт его обратно через buffer,
// чтобы следующий handler тоже смог прочитать JSON. 64 KiB лимит — защита
// от DoS на rate-limit middleware.
func readAndRestoreBody(r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, limit)
	defer func() { _ = r.Body.Close() }()
	// Читаем всё целиком; тело маленькое (≤64 KiB), это нормально.
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return buf, nil
}

// onlyPOST отбивает любой метод, кроме POST. Удобный помощник — у http.ServeMux
// из Go 1.22 есть {METHOD} в pattern'ах, но тогда мы завязываемся на точный
// шаблон без префиксов, а здесь mux общий и без этого.
func onlyPOST(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// methodRouter выбирает handler по HTTP-методу. Для 2-3 методов на эндпоинт —
// проще, чем отдельный mux на каждую пару.
func methodRouter(handlers map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := handlers[r.Method]; ok {
			h.ServeHTTP(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}

func healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": "cabinet-api"})
}

// ============================================================================
// SPA (из старой версии Mount)
// ============================================================================

func buildSPAHandler(spaFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(spaFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/cabinet")
		if trimmed == "" || trimmed == "/" {
			serveIndex(w, r, spaFS)
			return
		}
		filePath := strings.TrimPrefix(trimmed, "/")
		if _, err := fs.Stat(spaFS, filePath); err != nil {
			serveIndex(w, r, spaFS)
			return
		}

		// Cache policy:
		// - hashed assets: aggressive immutable cache
		// - fonts: длинный кэш, но без immutable — имена без хэша содержимого
		//   (web/cabinet/public/fonts). Повторный заход не должен снова тянуть
		//   шрифты: пока их нет, заголовки рисуются системным и потом подменяются.
		// - app shell / worker / manifest: always revalidate
		// - other static files: short shared cache
		switch {
		case strings.HasPrefix(filePath, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(filePath, "fonts/"):
			w.Header().Set("Cache-Control", "public, max-age=2592000")
		case filePath == "sw.js" || strings.HasSuffix(filePath, ".webmanifest") || strings.HasSuffix(filePath, ".html"):
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		http.StripPrefix("/cabinet", fileServer).ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, _ *http.Request, spaFS fs.FS) {
	data, err := fs.ReadFile(spaFS, "index.html")
	if err != nil {
		http.Error(w, "cabinet SPA is not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
