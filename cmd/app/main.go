package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"remnawave-tg-shop-bot/internal/broadcast"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	cabinethttp "remnawave-tg-shop-bot/internal/cabinet/http"
	cabrepo "remnawave-tg-shop-bot/internal/cabinet/repository"
	cabstartup "remnawave-tg-shop-bot/internal/cabinet/startup"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/heleket"
	"remnawave-tg-shop-bot/internal/moynalog"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/platega"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/receipt"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/remnawavewebhook"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/tgclient"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/tribute"
	"remnawave-tg-shop-bot/internal/yookasa"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// Version, Commit, BuildDate - переменные версии, устанавливаются при сборке через ldflags
var (
	Version   = "4.4.1"
	Commit    = "none"
	BuildDate = "unknown"
)

// main - главная функция приложения, инициализирует все компоненты бота и запускает его
func main() {
	// Контекст отменяется по Ctrl+C и по SIGTERM (docker stop, Kubernetes) —
	// без SIGTERM процесс обрывался посреди обработки, не дождавшись ничего.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Инициализация конфигурации из переменных окружения
	config.InitConfig()
	// Версия нужна и админке кабинета — кладём в конфиг, чтобы не тащить
	// параметром через конструкторы.
	config.SetBuildInfo(Version, Commit, BuildDate)
	printStartupBanner(Version, Commit, BuildDate)
	slog.Info("Application starting", "version", Version, "commit", Commit, "buildDate", BuildDate)

	// Клиент «Мой налог» собираем при наличии учётных данных, даже если сейчас
	// интеграция выключена: MOYNALOG_ENABLED переключается из админки без
	// рестарта, и к моменту включения клиент должен быть готов.
	var moynalogClient *moynalog.Client
	if config.MoynalogHasCredentials() {
		moynalogClient = moynalog.NewClient(
			config.MoynalogUrl(),
			config.MoynalogUsername(),
			config.MoynalogPassword(),
			config.MoynalogProxyURL(),
		)
		slog.Info("Moynalog client initialized", "enabled", config.IsMoynalogEnabled())
	} else {
		slog.Info("Moynalog credentials not set — receipts unavailable")
	}

	// Инициализация системы переводов (поддержка русского и английского языков)
	tm := translation.GetInstance()
	err := tm.InitTranslations("./translations", config.DefaultLanguage())
	if err != nil {
		panic(err)
	}

	// Подключение к базе данных PostgreSQL
	pool, err := initDatabase(ctx, config.DadaBaseUrl())
	if err != nil {
		panic(err)
	}

	// Выполнение миграций базы данных (создание/обновление таблиц)
	err = database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: "./db/migrations", Steps: 0}, pool)
	if err != nil {
		panic(err)
	}

	runtimeSettingsRepo := database.NewRuntimeSettingsRepository(pool)
	if overrides, loadErr := runtimeSettingsRepo.GetAll(ctx); loadErr != nil {
		panic(fmt.Errorf("load runtime settings: %w", loadErr))
	} else if err := config.LoadRuntimeOverrides(overrides); err != nil {
		panic(fmt.Errorf("apply runtime settings: %w", err))
	}

	// Инициализация конфигурации web-кабинета. Делаем сразу после миграций,
	// чтобы startup-check мог обратиться к уже созданной колонке customer.is_web_only
	// и чтобы падать рано при невалидных CABINET_* переменных.
	cabcfg.InitConfig()
	if cabcfg.IsEnabled() {
		if err := cabstartup.VerifySyntheticIDRange(ctx, pool, cabcfg.WebTelegramIDBase()); err != nil {
			panic(fmt.Errorf("cabinet startup check failed: %w", err))
		}
		slog.Info("cabinet startup check passed",
			"web_tg_id_base", cabcfg.WebTelegramIDBase())
	}

	// Инициализация кэша (TTL 30 минут) для временного хранения данных
	cache := cache.NewCache(30 * time.Minute)

	// Создание репозиториев для работы с данными в БД
	customerRepository := database.NewCustomerRepository(pool)                  // Работа с пользователями
	purchaseRepository := database.NewPurchaseRepository(pool)                  // Работа с покупками
	tariffRepository := database.NewTariffRepository(pool)                      // Тарифы (SALES_MODE=tariffs)
	referralRepository := database.NewReferralRepository(pool)                  // Работа с реферальной системой
	referralLedgerRepository := database.NewReferralBonusLedgerRepository(pool) // Журнал реферальных начислений
	promoRepository := database.NewPromoRepository(pool)
	statsRepository := database.NewStatsRepository(pool)
	infraBillingRepository := database.NewInfraBillingRepository(pool)
	loyaltyTierRepository := database.NewLoyaltyTierRepository(pool)
	partnerRepository := database.NewPartnerRepository(pool) // Партнёрская программа

	// Разовое восстановление журнала реферальных начислений по истории покупок.
	// Без него счётчики «заработано дней» обнулились бы в момент обновления:
	// экраны читают журнал, а не пересчитывают историю из purchase. Признак
	// «уже выполнялся» — наличие строк с is_backfilled, а не пустота таблицы,
	// поэтому повторные старты бота ничего не задваивают.
	if err := database.BackfillReferralLedger(ctx, referralLedgerRepository); err != nil {
		// Не фатально: бот работает и с неполной статистикой, а падение на
		// старте из-за неё оставило бы магазин без продаж.
		slog.Error("referral ledger backfill failed", "error", err)
	}

	// Инициализация клиентов для работы с внешними сервисами
	cryptoPayClient := cryptopay.NewCryptoPayClient(config.CryptoPayUrl(), config.CryptoPayToken())                // Криптоплатежи
	remnawaveClient := remnawave.NewClient(config.RemnawaveUrl(), config.RemnawaveToken(), config.RemnawaveMode()) // Remnawave API
	remnawaveClient.SetHwidLimitPin(customerRepository.HwidLimitOverride)                                          // Закреплённые админом лимиты устройств
	yookasaClient := yookasa.NewClient(config.YookasaUrl(), config.YookasaShopId(), config.YookasaSecretKey())     // YooKassa платежи
	plategaClient := platega.NewClient(config.PlategaMerchantID(), config.PlategaSecret())

	// Heleket сам не помнит адрес колбэка — он уходит в каждом счёте, поэтому
	// путь для mux и url_callback разбираем из одной переменной.
	heleketWebhookPath, heleketCallbackURL := heleket.ResolveWebhook(config.GetHeleketWebHookURL(), cabcfg.PublicURL())
	heleketClient := heleket.NewClient(
		config.HeleketMerchantID(),
		config.HeleketAPIKey(),
		heleketCallbackURL,
		config.HeleketCurrency(),
		config.HeleketOrderPrefix(),
		config.HeleketLifetime(),
	)
	if heleketClient.IsConfigured() {
		if heleketCallbackURL == "" {
			slog.Warn("heleket: url_callback пуст — подтверждение оплаты держится только на поллинге",
				"HELEKET_WEBHOOK_URL", config.GetHeleketWebHookURL())
		}
		if heleketClient.Currency() != "RUB" {
			slog.Warn("heleket: валюта счёта не RUB — сумма НЕ конвертируется, покупателю выставят рублёвое число в другой валюте",
				"HELEKET_CURRENCY", heleketClient.Currency())
		}
	}

	// Создание экземпляра Telegram бота с 3 воркерами для параллельной обработки запросов
	botInflight := &handler.Inflight{}
	botOptions := []bot.Option{bot.WithWorkers(3), bot.WithMiddlewares(handler.RecoverMiddleware(botInflight))}
	// Значения по умолчанию go-telegram/bot: long poll и таймаут клиента — минута.
	botPollTimeout := time.Minute
	var botHTTP tgclient.Doer = &http.Client{Timeout: botPollTimeout}
	if proxyURL := config.TelegramProxyURL(); proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			slog.Error("Invalid TELEGRAM_PROXY_URL, proxy disabled", "error", err)
		} else {
			botPollTimeout = 30 * time.Second
			botHTTP = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(parsedURL)}}
		}
	}
	// Общий лимит отправки и повтор после 429 — для всех сообщений процесса.
	botOptions = append(botOptions, bot.WithHTTPClient(botPollTimeout, tgclient.New(botHTTP, tgclient.SendsPerSecond)))
	b, err := bot.New(config.TelegramToken(), botOptions...)
	if err != nil {
		panic(err)
	}

	promoService := promo.NewService(promoRepository, customerRepository, purchaseRepository, remnawaveClient)

	// Очередь чеков «Мой налог»: чек пишется в БД до отправки и переотправляется
	// воркером, если ФНС недоступна. Без включённой интеграции остаётся nil —
	// методы сервиса это переживают (см. receipt.Service.Enabled).
	var receiptQueue *receipt.Service
	if moynalogClient != nil {
		receiptQueue = receipt.NewService(
			database.NewMoynalogReceiptRepository(pool),
			moynalogClient,
			b,
			config.GetAdminTelegramId(),
			config.IsMoynalogEnabled,
			config.MoynalogRetryMaxAge,
		)
	}

	// Инициализация сервиса платежей, который объединяет все платежные системы
	paymentService := payment.NewPaymentService(tm, purchaseRepository, tariffRepository, remnawaveClient, customerRepository, b, cryptoPayClient, yookasaClient, plategaClient, heleketClient, referralRepository, referralLedgerRepository, partnerRepository, cache, receiptQueue, promoService, loyaltyTierRepository)

	// Один Reconciler на вебхук и на поллинг: решение «зачесть / отменить /
	// позвать админа» принимается в одном месте.
	heleketReconciler := heleket.NewReconciler(heleketClient, purchaseRepository, paymentService, paymentService, paymentService)

	// Настройка cron-задачи для проверки статуса счетов (каждые 5 секунд)
	// CryptoPay; YooKassa и Platega — поллинг только если не задан соответствующий WEBHOOK_URL.
	cronScheduler := setupInvoiceChecker(purchaseRepository, cryptoPayClient, paymentService, yookasaClient, plategaClient, heleketReconciler)
	var schedulers []*cron.Cron
	if cronScheduler != nil {
		cronScheduler.Start()
		schedulers = append(schedulers, cronScheduler)
	}

	// Инициализация сервиса уведомлений о подписках
	subService := notification.NewSubscriptionService(customerRepository, purchaseRepository, paymentService, b, tm)
	infraBillingNotifyService := notification.NewInfraBillingNotifyService(remnawaveClient, infraBillingRepository, b, tm)

	// Настройка cron-задачи для проверки истечения подписок (каждый день в 16:00)
	// Отправляет уведомления пользователям об истечении подписки
	subscriptionNotificationCronScheduler := subscriptionChecker(subService, infraBillingNotifyService)
	subscriptionNotificationCronScheduler.Start()
	schedulers = append(schedulers, subscriptionNotificationCronScheduler)

	// Lifecycle notifications cron (отдельно от daily 16:00)
	var lifecycleCronScheduler *cron.Cron
	if config.LifecycleNotifyEnabled() {
		lifecycleRepo := notification.NewLifecycleRepository(pool)
		lifecycleService := notification.NewLifecycleService(
			customerRepository,
			purchaseRepository,
			promoRepository,
			promoService,
			remnawaveClient,
			b,
			tm,
			lifecycleRepo,
		)
		lifecycleCronScheduler = lifecycleChecker(lifecycleService)
		lifecycleCronScheduler.Start()
		schedulers = append(schedulers, lifecycleCronScheduler)
		slog.Info("Lifecycle notifications cron started", "schedule", config.LifecycleCron())
	}

	// Воркер переотправки чеков «Мой налог».
	if receiptQueue.Enabled() {
		receiptCronScheduler := receiptRetryChecker(receiptQueue)
		receiptCronScheduler.Start()
		schedulers = append(schedulers, receiptCronScheduler)
		slog.Info("Moynalog receipt retry cron started",
			"schedule", config.MoynalogRetryCron(), "maxAge", config.MoynalogRetryMaxAge())
	}

	// Раскрытие партнёрского холда: начисления, отлежавшие PARTNER_HOLD_DAYS,
	// становятся доступными к выводу. Раз в час, а не раз в сутки, чтобы дата
	// «деньги придут N-го» не разъезжалась с реальностью на полсуток.
	//
	// Крон стартует всегда, без проверки PARTNER_PROGRAM_ENABLED: это
	// рантайм-настройка, её включают из админки без перезапуска, и гейт на
	// старте запер бы начисления в холде навсегда у всех, кто включил
	// программу уже после запуска бота. Пустая таблица делает тик бесплатным,
	// а выключение программы не должно замораживать уже заработанное.
	partnerHoldCronScheduler := partnerHoldReleaser(partnerRepository)
	partnerHoldCronScheduler.Start()
	schedulers = append(schedulers, partnerHoldCronScheduler)
	slog.Info("Partner hold release cron started", "holdDays", config.PartnerHoldDays())

	// Истёкшие сессии и коды кабинета раньше копились вечно. Таблицы есть и
	// без CABINET_ENABLED (миграции общие), пустая таблица делает тик бесплатным.
	cabinetCleanupScheduler := cabinetExpiredCleaner(pool)
	cabinetCleanupScheduler.Start()
	schedulers = append(schedulers, cabinetCleanupScheduler)

	// Инициализация сервиса синхронизации с Remnawave
	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	// Создание главного обработчика всех команд и callback'ов бота
	// Один отправитель на бота и кабинет: история рассылок общая.
	broadcastRepository := database.NewBroadcastRepository(pool)
	if n, err := broadcastRepository.MarkInterrupted(ctx); err != nil {
		slog.Warn("broadcast history: mark interrupted", "error", err)
	} else if n > 0 {
		slog.Warn("broadcast history: broadcasts interrupted by restart", "count", n)
	}
	broadcastSender := broadcast.NewSender(customerRepository, broadcastRepository, tm)
	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, tariffRepository, cryptoPayClient, yookasaClient, referralRepository, partnerRepository, cache, promoRepository, promoService, remnawaveClient, statsRepository, infraBillingRepository, loyaltyTierRepository, broadcastSender)

	// Получение информации о боте (username и т.д.)
	// Используем контекст с таймаутом для GetMe, чтобы избежать зависания при проблемах с сетью
	// Таймаут 30 секунд применяется к каждой попытке отдельно
	var me *models.User
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		// Создаем новый контекст с таймаутом для каждой попытки
		getMeCtx, getMeCancel := context.WithTimeout(ctx, 30*time.Second)
		me, err = b.GetMe(getMeCtx)
		getMeCancel() // Освобождаем ресурсы контекста

		if err == nil {
			break
		}
		if i < maxRetries-1 {
			slog.Warn("Failed to get bot info, retrying...", "attempt", i+1, "maxRetries", maxRetries, "error", err)
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		slog.Error("Failed to get bot info after retries", "error", err)
		panic(fmt.Errorf("failed to get bot info: %w", err))
	}

	// Настройка кнопки меню бота (показывать список команд)
	_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonCommands{
			Type: models.MenuButtonTypeCommands,
		},
	})

	if err != nil {
		panic(err)
	}

	// Установка списка команд бота для русского языка
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Начать работу с ботом"},
			{Command: "connect", Description: "Подключиться"},
		},
		LanguageCode: "ru",
	})
	if err != nil {
		slog.Warn("Failed to set bot commands for Russian", "error", err)
	}

	// Установка списка команд бота для английского языка
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Start using the bot"},
			{Command: "connect", Description: "Connect"},
		},
		LanguageCode: "en",
	})
	if err != nil {
		slog.Warn("Failed to set bot commands for English", "error", err)
	}

	// Сохранение URL бота в конфигурации (используется для генерации ссылок)
	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))

	// ============================================================================
	// РЕГИСТРАЦИЯ ОБРАБОТЧИКОВ КОМАНД И CALLBACK'ОВ
	// ============================================================================
	// ВАЖНО: Порядок регистрации имеет значение! Обработчики проверяются сверху вниз.
	// Более специфичные обработчики должны регистрироваться раньше общих.

	// --- Обработчики текстовых команд ---

	// /start - главная команда бота, запускает взаимодействие с пользователем
	// Middleware: ForwardUserMessageToAdminMiddleware (пересылает сообщение админу),
	//             SuspiciousUserFilterMiddleware (блокирует подозрительных пользователей)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.ForwardUserMessageToAdminMiddleware, h.SuspiciousUserFilterMiddleware)

	// Согласие с политикой/офертой (gate до основного меню)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackLegalAccept, bot.MatchTypeExact, h.LegalAcceptCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackLegalDecline, bot.MatchTypeExact, h.LegalDeclineCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// /connect - команда для подключения устройств
	// Middleware: SuspiciousUserFilterMiddleware, CreateCustomerIfNotExistMiddleware, RequireLegalAcceptanceMiddleware
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware)

	// /sync - команда для синхронизации пользователей с Remnawave (только для админа)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)

	// /broadcast - команда для массовой рассылки сообщений всем пользователям (только для админа)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/broadcast", bot.MatchTypeExact, h.BroadcastCommandHandler, isAdminMiddleware)

	// --- Обработчики callback-кнопок (inline кнопки) ---

	// Callback для выбора типа рассылки (только для админа)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastAll, bot.MatchTypeExact, h.BroadcastTypeSelectHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastActive, bot.MatchTypeExact, h.BroadcastActiveMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastInactive, bot.MatchTypeExact, h.BroadcastInactiveMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastActivePaid, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastActiveTrial, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastActiveAllSeg, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastInactivePaid, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastInactiveTrial, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastInactiveAllSeg, bot.MatchTypeExact, h.BroadcastSegmentPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastBackAudience, bot.MatchTypeExact, h.BroadcastBackToAudienceHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastBackAdmin, bot.MatchTypeExact, h.BroadcastBackToAdminHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "bc_pt_", bot.MatchTypePrefix, h.BroadcastPaidTariffCallbacksHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для подтверждения рассылки (только для админа)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastConfirm, bot.MatchTypeExact, h.BroadcastConfirmHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для отмены рассылки (только для админа)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastCancel, bot.MatchTypeExact, h.BroadcastCancelHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)

	// Выбор inline-кнопок под рассылку (только для админа)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastToggleMain, bot.MatchTypeExact, h.BroadcastButtonToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastTogglePromo, bot.MatchTypeExact, h.BroadcastButtonToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastToggleVPN, bot.MatchTypeExact, h.BroadcastButtonToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastToggleBuy, bot.MatchTypeExact, h.BroadcastButtonToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastToggleShare, bot.MatchTypeExact, h.BroadcastButtonToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBroadcastButtonsNext, bot.MatchTypeExact, h.BroadcastButtonsNextHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)

	// Админ-панель и промокоды
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackEnterPromo, bot.MatchTypePrefix, h.EnterPromoCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminPanel, bot.MatchTypeExact, h.AdminPanelHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminBroadcast, bot.MatchTypeExact, h.AdminBroadcastShortcutHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSync, bot.MatchTypeExact, h.AdminSyncShortcutHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminPromo, bot.MatchTypeExact, h.AdminPromoOpenHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminTariffs, bot.MatchTypeExact, h.AdminTariffsHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersSubmenu, bot.MatchTypeExact, h.AdminUsersSubmenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersRoot, bot.MatchTypeExact, h.AdminUsersRootHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersSearch, bot.MatchTypeExact, h.AdminUsersSearchHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersStatsSection, bot.MatchTypeExact, h.AdminUsersStatsSectionHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersInactiveMenu, bot.MatchTypeExact, h.AdminUsersInactiveJumpHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersListAllPrefix, bot.MatchTypePrefix, h.AdminUsersListAllRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersListInactivePrefix, bot.MatchTypePrefix, h.AdminUsersListInactiveRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersListPagePickOpenPrefix, bot.MatchTypePrefix, h.AdminUsersListPagePickerOpenHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUsersListPagePickJumpPrefix, bot.MatchTypePrefix, h.AdminUsersListPagePickerJumpHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPaymentsNotifyUserOpenPrefix, bot.MatchTypePrefix, h.AdminPaymentsNotifyOpenUserHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserManagePrefix, bot.MatchTypePrefix, h.AdminUserManageHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserSubscriptionPrefix, bot.MatchTypePrefix, h.AdminUserSubscriptionHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserExtraHwidDecPrefix, bot.MatchTypePrefix, h.AdminUserExtraHwidDecHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserExtraHwidIncPrefix, bot.MatchTypePrefix, h.AdminUserExtraHwidIncHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserTariffMenuPrefix, bot.MatchTypePrefix, h.AdminUserTariffMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserTariffPickPrefix, bot.MatchTypePrefix, h.AdminUserTariffPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDescAskPrefix, bot.MatchTypePrefix, h.AdminUserDescriptionAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDescClearPrefix, bot.MatchTypePrefix, h.AdminUserDescriptionClearHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserReferralsPrefix, bot.MatchTypePrefix, h.AdminUserReferralsHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserSpendPrefix, bot.MatchTypePrefix, h.AdminUserSpendHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserPaymentsPrefix, bot.MatchTypePrefix, h.AdminUserPaymentsHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserMsgHintPrefix, bot.MatchTypePrefix, h.AdminUserMsgHintHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserExtendPrefix, bot.MatchTypePrefix, h.AdminUserExtendHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserResetTrafficAskPrefix, bot.MatchTypePrefix, h.AdminUserResetTrafficAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserResetTrafficConfirmPrefix, bot.MatchTypePrefix, h.AdminUserResetTrafficConfirmHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserHwPresetMenuPrefix, bot.MatchTypePrefix, h.AdminUserHwPresetMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserHwPresetSetPrefix, bot.MatchTypePrefix, h.AdminUserHwPresetSetHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserCalOpenPrefix, bot.MatchTypePrefix, h.AdminUserCalOpenHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserCalNavPrefix, bot.MatchTypePrefix, h.AdminUserCalNavHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserCalPickPrefix, bot.MatchTypePrefix, h.AdminUserCalPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserCalBlankPrefix, bot.MatchTypePrefix, h.AdminUserCalBlankHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserCalManualPrefix, bot.MatchTypePrefix, h.AdminUserCalManualHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserPanelMenuPrefix, bot.MatchTypePrefix, h.AdminUserPanelMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserSquadMenuPrefix, bot.MatchTypePrefix, h.AdminUserSquadMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserSquadPickPrefix, bot.MatchTypePrefix, h.AdminUserSquadPickHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserStrategyMenuPrefix, bot.MatchTypePrefix, h.AdminUserStrategyMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserStrategySetPrefix, bot.MatchTypePrefix, h.AdminUserStrategySetHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserTrafficMenuPrefix, bot.MatchTypePrefix, h.AdminUserTrafficMenuHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserTrafficSetPrefix, bot.MatchTypePrefix, h.AdminUserTrafficSetHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserTrafficCustomPrefix, bot.MatchTypePrefix, h.AdminUserTrafficCustomAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDisableAskPrefix, bot.MatchTypePrefix, h.AdminUserDisableAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDisableConfirmPrefix, bot.MatchTypePrefix, h.AdminUserDisableConfirmHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserEnableAskPrefix, bot.MatchTypePrefix, h.AdminUserEnableAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserEnableConfirmPrefix, bot.MatchTypePrefix, h.AdminUserEnableConfirmHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDeleteAskPrefix, bot.MatchTypePrefix, h.AdminUserDeleteAskHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDeleteConfirmPrefix, bot.MatchTypePrefix, h.AdminUserDeleteConfirmHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDevicesPrefix, bot.MatchTypePrefix, h.AdminUserDevicesHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminUserDevDelPrefix, bot.MatchTypePrefix, h.AdminUserDevDelHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSubsRoot, bot.MatchTypeExact, h.AdminSubsRootHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSubsListPrefix, bot.MatchTypePrefix, h.AdminSubsListRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSubsExpiringListPrefix, bot.MatchTypePrefix, h.AdminSubsExpiringListRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSubsExpiring, bot.MatchTypeExact, h.AdminSubsExpiringHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminSubsStatsJump, bot.MatchTypeExact, h.AdminSubsStatsJumpHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminRefRoot, bot.MatchTypeExact, h.AdminRefRootHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyRoot, bot.MatchTypeExact, h.AdminLoyaltyRootHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyLevels, bot.MatchTypeExact, h.AdminLoyaltyLevelsHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyNew, bot.MatchTypeExact, h.AdminLoyaltyNewHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyRecalcAsk, bot.MatchTypeExact, h.AdminLoyaltyRecalcAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyRecalcRun, bot.MatchTypeExact, h.AdminLoyaltyRecalcRunHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyRules, bot.MatchTypeExact, h.AdminLoyaltyRulesHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyStats, bot.MatchTypeExact, h.AdminLoyaltyStatsHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyCard, bot.MatchTypePrefix, h.AdminLoyaltyTierCardHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyDelAsk, bot.MatchTypePrefix, h.AdminLoyaltyDelAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyDelYes, bot.MatchTypePrefix, h.AdminLoyaltyDelYesHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyEditXP, bot.MatchTypePrefix, h.AdminLoyaltyEditXPAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyEditPct, bot.MatchTypePrefix, h.AdminLoyaltyEditPctAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminLoyaltyEditDn, bot.MatchTypePrefix, h.AdminLoyaltyEditDnAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsRoot, bot.MatchTypeExact, h.AdminStatsRootHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsUsers, bot.MatchTypeExact, h.AdminStatsUsersHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsSubs, bot.MatchTypeExact, h.AdminStatsSubsHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsRevenue, bot.MatchTypeExact, h.AdminStatsRevenueHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsRef, bot.MatchTypeExact, h.AdminStatsRefHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsSummary, bot.MatchTypeExact, h.AdminStatsSummaryHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminStatsFortune, bot.MatchTypeExact, h.AdminStatsFortuneHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraRoot, bot.MatchTypeExact, h.AdminInfraRootHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraNodes, bot.MatchTypeExact, h.AdminInfraNodesHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraNotify, bot.MatchTypeExact, h.AdminInfraNotifyHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraHist, bot.MatchTypePrefix, h.AdminInfraHistoryHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraProv, bot.MatchTypeExact, h.AdminInfraProvidersHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdminInfraToggle, bot.MatchTypePrefix, h.AdminInfraToggleHandler, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "ifn", bot.MatchTypePrefix, h.AdminInfraNodeCRUDRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "ifp", bot.MatchTypePrefix, h.AdminInfraProviderCRUDRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "ifh", bot.MatchTypePrefix, h.AdminInfraHistoryCRUDRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "ifb", bot.MatchTypePrefix, h.AdminInfraWizBackRouter, isAdminMiddleware, h.AnswerCallbackQueryMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTariffNew, bot.MatchTypeExact, h.AdminTariffNewHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "tf_", bot.MatchTypePrefix, h.AdminTariffCallbackRouter, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoRoot, bot.MatchTypeExact, h.PromoRootHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoList, bot.MatchTypePrefix, h.PromoListHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoNew, bot.MatchTypeExact, h.PromoNewMenuHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoStatsAll, bot.MatchTypeExact, h.PromoStatsAllHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoCard, bot.MatchTypePrefix, h.PromoCardHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEdit, bot.MatchTypePrefix, h.PromoEditMenuHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditValid, bot.MatchTypePrefix, h.PromoEditAskValidHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditMax, bot.MatchTypePrefix, h.PromoEditAskMaxHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditSubDays, bot.MatchTypePrefix, h.PromoEditAskSubDaysHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditTrialDays, bot.MatchTypePrefix, h.PromoEditAskTrialDaysHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditSubsTariff, bot.MatchTypePrefix, h.PromoEditSubsTariffMenuHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditSubsTariffSet, bot.MatchTypePrefix, h.PromoEditSubsTariffApplyHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoEditDiscPay, bot.MatchTypePrefix, h.PromoEditAskDiscPayHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoNewType, bot.MatchTypePrefix, h.PromoNewTypeHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoDiscKind, bot.MatchTypePrefix, h.PromoDiscountKindHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoSubDaysScope, bot.MatchTypePrefix, h.PromoSubDaysScopeHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoToggle, bot.MatchTypePrefix, h.PromoToggleHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoFirstPur, bot.MatchTypePrefix, h.PromoFirstPurchaseToggle, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoStat, bot.MatchTypePrefix, h.PromoStatHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoDel, bot.MatchTypePrefix, h.PromoDeleteAskHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoDelYes, bot.MatchTypePrefix, h.PromoDeleteYesHandler, isAdminMiddleware)

	// Callback для реферальной системы
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferral, bot.MatchTypeExact, h.ReferralCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для списка рефералов
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferralList, bot.MatchTypeExact, h.ReferralListCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Экран лояльности (Мой VPN)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackLoyaltyRoot, bot.MatchTypeExact, h.LoyaltyRootCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для покупки подписки
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBuy, bot.MatchTypePrefix, h.BuyCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для пробного периода
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTrial, bot.MatchTypeExact, h.TrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для активации пробного периода
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackActivateTrial, bot.MatchTypeExact, h.ActivateTrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для возврата в главное меню
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackStart, bot.MatchTypePrefix, h.StartCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для продажи подписки (с префиксом, т.к. содержит параметры)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSell, bot.MatchTypePrefix, h.SellCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для подключения устройств
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackConnect, bot.MatchTypePrefix, h.ConnectCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для управления устройствами
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackManageDevices, bot.MatchTypeExact, h.ManageDevicesCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для подтверждения изменения устройств
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAddDeviceConfirm, bot.MatchTypePrefix, h.AddDeviceConfirmCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для применения изменения без оплаты
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAddDeviceApply, bot.MatchTypePrefix, h.AddDeviceApplyCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для докупки устройств
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAddDevice, bot.MatchTypeExact, h.AddDeviceCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для оплаты докупки устройств (с префиксом, т.к. содержит параметры)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAddDevicePayment, bot.MatchTypePrefix, h.AddDevicePaymentCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для продления доп. устройств

	// Callback для истории операций (с префиксом, т.к. содержит параметры страницы)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPurchaseHistory, bot.MatchTypePrefix, h.PurchaseHistoryCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для обработки платежей (с префиксом, т.к. содержит параметры)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPayment, bot.MatchTypePrefix, h.PaymentCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для справки/помощи
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "help", bot.MatchTypeExact, h.HelpCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для просмотра списка устройств
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackDevices, bot.MatchTypeExact, h.DevicesCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Callback для удаления устройства (с префиксом, т.к. содержит HWID устройства)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackDeleteDevice, bot.MatchTypePrefix, h.DeleteDeviceCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware, h.AnswerCallbackQueryMiddleware)

	// Обработчик предварительной проверки платежа (Telegram Stars)
	// Срабатывает перед финальным подтверждением платежа
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.PreCheckoutQuery != nil
	}, h.PreCheckoutCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware)

	// Обработчик успешного платежа (Telegram Stars)
	// Срабатывает после успешной оплаты через Telegram Stars
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.SuccessfulPayment != nil
	}, h.SuccessPaymentHandler, h.SuspiciousUserFilterMiddleware)

	// ============================================================================
	// ОБРАБОТЧИКИ ТЕКСТОВЫХ СООБЩЕНИЙ И REPLY
	// ============================================================================
	// Эти обработчики регистрируются после основных команд, чтобы не перехватывать их

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil || update.Message.Text == "" || strings.HasPrefix(update.Message.Text, "/") {
			return false
		}
		if !handler.UserPromoWaiting(update.Message.From.ID) {
			return false
		}
		// Админ: ввод промокода с главной не пересекается с мастером/редактированием в админке
		if update.Message.From.ID == config.GetAdminTelegramId() {
			if handler.AdminPromoWaiting(update.Message.From.ID) || handler.AdminPromoEditWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminTariffWizardWaiting(update.Message.From.ID) || handler.AdminTariffEditWaiting(update.Message.From.ID) {
				return false
			}
			if handler.InfraBillingWizardWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminLoyaltyWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminUsersSearchWaiting(update.Message.From.ID) || handler.AdminUsersDMWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminUserExpireDateWaiting(update.Message.From.ID) {
				return false
			}
			if handler.AdminUserDescriptionWaiting(update.Message.From.ID) {
				return false
			}
		}
		return true
	}, h.UserPromoMessageHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware, h.RequireLegalAcceptanceMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			(handler.AdminPromoWaiting(update.Message.From.ID) || handler.AdminPromoEditWaiting(update.Message.From.ID)) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminPromoTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			(handler.AdminTariffWizardWaiting(update.Message.From.ID) || handler.AdminTariffEditWaiting(update.Message.From.ID)) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminTariffTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminLoyaltyTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminInfraBillingTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminPromoWaiting(update.Message.From.ID) &&
			!handler.AdminPromoEditWaiting(update.Message.From.ID) &&
			!handler.AdminTariffWizardWaiting(update.Message.From.ID) &&
			!handler.AdminTariffEditWaiting(update.Message.From.ID) &&
			!handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminUserDMMessageHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminPromoWaiting(update.Message.From.ID) &&
			!handler.AdminPromoEditWaiting(update.Message.From.ID) &&
			!handler.AdminTariffWizardWaiting(update.Message.From.ID) &&
			!handler.AdminTariffEditWaiting(update.Message.From.ID) &&
			!handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID)
	}, h.AdminUserTrafficLimitTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminPromoWaiting(update.Message.From.ID) &&
			!handler.AdminPromoEditWaiting(update.Message.From.ID) &&
			!handler.AdminTariffWizardWaiting(update.Message.From.ID) &&
			!handler.AdminTariffEditWaiting(update.Message.From.ID) &&
			!handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminUserExpireDateTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminUserDescriptionWaiting(update.Message.From.ID) &&
			!handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminPromoWaiting(update.Message.From.ID) &&
			!handler.AdminPromoEditWaiting(update.Message.From.ID) &&
			!handler.AdminTariffWizardWaiting(update.Message.From.ID) &&
			!handler.AdminTariffEditWaiting(update.Message.From.ID) &&
			!handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID)
	}, h.AdminUserDescriptionTextHandler)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID == config.GetAdminTelegramId() &&
			update.Message.ReplyToMessage == nil &&
			handler.AdminUsersSearchWaiting(update.Message.From.ID) &&
			!handler.AdminUsersDMWaiting(update.Message.From.ID) &&
			!handler.AdminPromoWaiting(update.Message.From.ID) &&
			!handler.AdminPromoEditWaiting(update.Message.From.ID) &&
			!handler.AdminTariffWizardWaiting(update.Message.From.ID) &&
			!handler.AdminTariffEditWaiting(update.Message.From.ID) &&
			!handler.InfraBillingWizardWaiting(update.Message.From.ID) &&
			!handler.AdminLoyaltyWaiting(update.Message.From.ID) &&
			!handler.AdminUserTrafficLimitWaiting(update.Message.From.ID) &&
			!handler.AdminUserExpireDateWaiting(update.Message.From.ID) &&
			!handler.AdminUserDescriptionWaiting(update.Message.From.ID)
	}, h.AdminUsersSearchMessageHandler)

	// Обработчик черновика рассылки: текст, подпись к фото или фото/файл JPEG|PNG|WebP (после выбора аудитории)
	// НЕ обрабатывает reply-сообщения (они обрабатываются AdminReplyToUser ниже)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil || update.Message.ReplyToMessage != nil {
			return false
		}
		if update.Message.From.ID != config.GetAdminTelegramId() {
			return false
		}
		if !handler.BroadcastAwaitingMessageInput(update.Message.From.ID) {
			return false
		}
		return handler.BroadcastIncomingDraftMessage(update.Message)
	}, h.BroadcastMessageHandler)

	// Обработчик для неизвестных команд от пользователей
	// Пересылает все команды пользователей админу для просмотра
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID != config.GetAdminTelegramId()
	}, h.ForwardUserMessageToAdmin)

	// Обработчик для обычных текстовых сообщений от пользователей (не команд)
	// Пересылает все текстовые сообщения пользователей админу для просмотра
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			update.Message.From.ID != config.GetAdminTelegramId()
	}, h.ForwardUserMessageToAdmin)

	// Обработчик для reply-сообщений от админа
	// Позволяет админу отвечать на сообщения пользователей через reply
	// Извлекает ID пользователя из текста пересланного сообщения и отправляет ответ
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.ReplyToMessage != nil &&
			update.Message.From.ID == config.GetAdminTelegramId()
	}, h.AdminReplyToUser)

	// ============================================================================
	// HTTP СЕРВЕР ДЛЯ HEALTHCHECK И WEBHOOK'ОВ
	// ============================================================================

	// Создание HTTP роутера для обработки внешних запросов
	mux := http.NewServeMux()

	// Endpoint для проверки здоровья сервиса (используется для мониторинга)
	// Проверяет доступность БД и Remnawave API
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient))

	// Webhook для платежной системы Tribute (если включена)
	if config.GetTributeWebHookUrl() != "" {
		tributeHandler := tribute.NewClient(paymentService, customerRepository)
		mux.Handle(config.GetTributeWebHookUrl(), tributeHandler.WebHookHandler())
	}
	if config.IsYookasaEnabled() && strings.TrimSpace(config.GetYookasaWebHookURL()) != "" {
		mux.Handle(config.GetYookasaWebHookURL(), yookasa.NewWebhookHandler(yookasaClient, paymentService, purchaseRepository))
	}
	if config.IsPlategaEnabled() && strings.TrimSpace(config.GetPlategaWebHookURL()) != "" {
		mux.Handle(config.GetPlategaWebHookURL(), platega.NewWebhookHandler(purchaseRepository, paymentService, plategaClient, config.PlategaMerchantID(), config.PlategaSecret()))
	}
	// Монтируем по наличию реквизитов, а не по тумблеру: способ оплаты можно
	// включить из админки без рестарта, и тогда счета создавались бы, а колбэк
	// принимать было бы некому.
	if heleketWebhookPath != "" && heleketClient.IsConfigured() {
		mux.Handle(heleketWebhookPath, heleket.NewWebhookHandler(heleketReconciler, config.HeleketAPIKey()))
		slog.Info("heleket webhook mounted", "path", heleketWebhookPath, "url_callback", heleketCallbackURL)
	}
	if path := config.GetRemnawaveWebhookPath(); path != "" && config.GetRemnawaveWebhookSecret() != "" {
		mux.Handle(path, remnawavewebhook.NewHandler(config.GetRemnawaveWebhookSecret(), customerRepository, b, tm))
		slog.Info("remnawave webhook mounted", "path", path)
	}

	// Web-кабинет: при CABINET_ENABLED=true регистрируем /cabinet/api/*
	// и /cabinet/* на том же mux и том же порту, что и healthcheck.
	// cabcfg.InitConfig() вызван ранее (сразу после миграций), здесь только
	// монтируем роуты.
	if cabcfg.IsEnabled() {
		if err := cabinethttp.Mount(ctx, mux, pool, paymentService, remnawaveClient, promoService, syncService, b, broadcastSender); err != nil {
			panic(fmt.Errorf("failed to mount cabinet routes: %w", err))
		}
		slog.Info("cabinet routes mounted", "prefix", "/cabinet")
	}

	// Запуск HTTP сервера в отдельной горутине
	// Без таймаутов медленный клиент держит соединение сколько угодно, и
	// сотня таких соединений съедает сервер (slowloris). Ответы у нас короткие:
	// долгие операции админки (синхронизация, рассылка) идут в фоне.
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.GetHealthCheckPort()),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("Server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// ============================================================================
	// ЗАПУСК БОТА
	// ============================================================================

	// Логирование информации о версии при старте
	slog.Info("Bot is starting...",
		"version", Version,
		"commit", Commit,
		"buildDate", BuildDate)
	// Запуск бота (блокирующий вызов, работает до получения сигнала прерывания)
	b.Start(ctx)

	shutdown(srv, botInflight, schedulers, pool)
}

// shutdownTimeout — сколько ждём незавершённую работу при остановке. Docker
// после SIGTERM ждёт 10 секунд и убивает процесс, поэтому укладываемся в 9.
const shutdownTimeout = 9 * time.Second

// shutdown останавливает процесс по порядку: перестаёт принимать HTTP (вебхуки
// оплат, кабинет) и дожидается начатых запросов, затем дожидается начатых
// обработчиков бота и задач cron, и только потом закрывает пул БД — иначе
// начатая выдача подписки обрывалась бы на полпути.
func shutdown(srv *http.Server, botInflight *handler.Inflight, schedulers []*cron.Cron, pool *pgxpool.Pool) {
	slog.Info("Shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Новые задачи cron не стартуют с этого момента; начатые ждём ниже.
	cronDone := make([]context.Context, 0, len(schedulers))
	for _, c := range schedulers {
		cronDone = append(cronDone, c.Stop())
	}
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown", "error", err)
	}
	if left := botInflight.Wait(ctx); left > 0 {
		slog.Warn("Shutdown: bot handlers still running", "count", left)
	}
	for _, done := range cronDone {
		select {
		case <-done.Done():
		case <-ctx.Done():
			slog.Warn("Shutdown: cron job still running")
		}
	}
	pool.Close()
	slog.Info("Shutdown complete")
}

// fullHealthHandler - HTTP обработчик для проверки здоровья сервиса
// Проверяет доступность базы данных и Remnawave API
// Используется для мониторинга и healthcheck в оркестраторах (Docker, Kubernetes и т.д.)
//
// Эндпоинт открыт наружу, поэтому по компонентам отдаёт только «ok»/«fail»:
// текст ошибки раскрывал адреса и внутренности БД и панели. Подробности
// пишутся в лог. Версия остаётся — по ней проверяют, что выкатилось.
func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Status    string `json:"status"`
			DB        string `json:"db"`
			Remnawave string `json:"remnawave"`
			Time      string `json:"time"`
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildDate string `json:"buildDate"`
		}{Status: "ok", DB: "ok", Remnawave: "ok", Time: time.Now().Format(time.RFC3339),
			Version: Version, Commit: Commit, BuildDate: BuildDate}

		// Проверка доступности базы данных PostgreSQL
		dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer dbCancel()
		if err := pool.Ping(dbCtx); err != nil {
			slog.Error("healthcheck: database", "error", err)
			resp.Status, resp.DB = "fail", "fail"
		}

		// Проверка доступности Remnawave API
		rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer rwCancel()
		if err := rw.Ping(rwCtx); err != nil {
			slog.Error("healthcheck: remnawave", "error", err)
			resp.Status, resp.Remnawave = "fail", "fail"
		}

		code := http.StatusOK
		if resp.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// isAdminMiddleware - middleware для проверки, что запрос пришел от администратора
// Пропускает обработчик только если пользователь является админом
// Работает как с обычными сообщениями, так и с callback queries
func isAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var userID int64
		// Определяем ID пользователя в зависимости от типа обновления
		if update.Message != nil {
			userID = update.Message.From.ID
		} else if update.CallbackQuery != nil {
			userID = update.CallbackQuery.From.ID
		} else {
			return // Неизвестный тип обновления, пропускаем
		}

		// Проверяем, является ли пользователь админом
		if userID == config.GetAdminTelegramId() {
			next(ctx, b, update) // Пропускаем в обработчик
		} else {
			return // Не админ, блокируем запрос
		}
	}
}

// subscriptionChecker - настраивает cron-задачу для проверки истечения подписок
// Запускается каждый день в 16:00 (формат cron: "0 16 * * *")
// Отправляет уведомления пользователям об истечении подписки
func subscriptionChecker(subService *notification.SubscriptionService, infraNotify *notification.InfraBillingNotifyService) *cron.Cron {
	c := newCron()

	// Добавляем задачу: каждый день в 16:00 проверять истечение подписок
	_, err := c.AddFunc("0 16 * * *", func() {
		err := subService.ProcessSubscriptionExpiration()
		if err != nil {
			slog.Error("Error sending subscription notifications", "error", err)
		}
		if infraNotify != nil {
			ctx := context.Background()
			if err := infraNotify.ProcessInfraBillingReminders(ctx); err != nil {
				slog.Error("Error sending infra billing reminders", "error", err)
			}
		}
	})

	if err != nil {
		panic(err)
	}
	return c
}

// lifecycleChecker - настраивает cron для lifecycle-уведомлений (no-connect, win-back, trial expiring)
// Запускается по расписанию из LIFECYCLE_CRON (по умолчанию каждые 30 минут)
func lifecycleChecker(lifecycleService *notification.LifecycleService) *cron.Cron {
	c := newCron()

	cronSchedule := config.LifecycleCron()
	_, err := c.AddFunc(cronSchedule, func() {
		if err := lifecycleService.ProcessLifecycleNotifications(); err != nil {
			slog.Error("Error processing lifecycle notifications", "error", err)
		}
	})

	if err != nil {
		panic(fmt.Sprintf("Failed to add lifecycle cron job: %v", err))
	}
	return c
}

// partnerHoldReleaser - cron раскрытия партнёрского холда: переводит отлежавшие
// начисления в доступные к выводу и двигает остатки партнёров.
//
// Ошибка тика не фатальна: следующий час подберёт всё, что не успело раскрыться,
// потому что выборка идёт по состоянию строк, а не по времени последнего запуска.
func partnerHoldReleaser(partners *database.PartnerRepository) *cron.Cron {
	c := newCron()

	_, err := c.AddFunc("0 * * * *", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		released, err := partners.ReleaseDueHolds(ctx, time.Now().UTC())
		if err != nil {
			slog.Error("Error releasing partner holds", "error", err)
			return
		}
		for _, r := range released {
			slog.Info("Partner hold released",
				"partner_id", r.PartnerID, "amount", r.Amount, "entries", r.Count)
		}
	})

	if err != nil {
		panic(fmt.Sprintf("Failed to add partner hold cron job: %v", err))
	}
	return c
}

// cabinetExpiredCleaner — раз в сутки удаляет сессии и одноразовые коды
// кабинета, истёкшие больше недели назад.
func cabinetExpiredCleaner(pool *pgxpool.Pool) *cron.Cron {
	c := newCron()
	_, err := c.AddFunc("30 4 * * *", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		deleted, err := cabrepo.PurgeExpired(ctx, pool, time.Now().Add(-cabrepo.ExpiredRetention))
		if err != nil {
			slog.Error("Error purging expired cabinet sessions and codes", "error", err)
		}
		for table, n := range deleted {
			if n > 0 {
				slog.Info("Purged expired cabinet rows", "table", table, "rows", n)
			}
		}
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to add cabinet cleanup cron job: %v", err))
	}
	return c
}

// receiptRetryChecker - cron переотправки чеков «Мой налог», застрявших из-за
// недоступности ФНС или протухшей авторизации. Расписание — MOYNALOG_RETRY_CRON.
func receiptRetryChecker(receiptQueue *receipt.Service) *cron.Cron {
	c := newCron()

	_, err := c.AddFunc(config.MoynalogRetryCron(), func() {
		// Своя граница по времени: тик не должен висеть вечно, если ФНС
		// отвечает медленно, иначе следующий запуск наложится на текущий.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := receiptQueue.ProcessDue(ctx); err != nil {
			slog.Error("Error processing moynalog receipt queue", "error", err)
		}
	})

	if err != nil {
		panic(fmt.Sprintf("Failed to add moynalog receipt retry cron job: %v", err))
	}
	return c
}

// initDatabase - инициализирует пул соединений с базой данных PostgreSQL
// Настраивает максимальное и минимальное количество соединений для оптимизации производительности
func initDatabase(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	// Настройка пула соединений
	config.MaxConns = 20 // Максимальное количество одновременных соединений
	config.MinConns = 5  // Минимальное количество соединений (поддерживается постоянно)

	// Пул один на всё: бот, кабинет, cron, статистика. Без ограничений один
	// зависший запрос или забытая открытой транзакция держали бы соединение и
	// блокировки сколько угодно, и пул в 20 соединений кончался бы. Значения из
	// строки подключения (options=...) имеют приоритет. Миграции идут отдельным
	// пулом без statement_timeout — см. database.RunMigrations.
	if _, ok := config.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		config.ConnConfig.RuntimeParams["statement_timeout"] = "30s"
	}
	if _, ok := config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"]; !ok {
		config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "5min"
	}

	return pgxpool.NewWithConfig(ctx, config)
}

// setupInvoiceChecker - настраивает cron-задачи для проверки статуса счетов
// heleketPollTimeout — потолок на один обход pending-счетов Heleket, чтобы
// зависший ответ кассы не держал задачу до следующего запуска.
const heleketPollTimeout = 4 * time.Minute

// invoicePollTimeout — потолок на один проход поллера CryptoPay, YooKassa и
// Platega. Запуск каждые 5 секунд, но проход с медленной кассой или панелью
// может длиться дольше — тогда следующие запуски пропускаются, а не копятся.
const invoicePollTimeout = 2 * time.Minute

// invoicePollJob оборачивает проход поллера касс. Проходы не накладываются:
// иначе два прохода шли бы по одному списку pending и упирались в блокировку
// покупки. Паника в проходе не роняет процесс — без recover она убила бы вместе
// с поллером и бота, и кабинет, и приём вебхуков.
func invoicePollJob(run func(ctx context.Context)) cron.Job {
	return cron.NewChain(
		cron.Recover(cronSlogLogger{}),
		cron.SkipIfStillRunning(cron.DiscardLogger),
	).Then(cron.FuncJob(func() {
		ctx, cancel := context.WithTimeout(context.Background(), invoicePollTimeout)
		defer cancel()
		run(ctx)
	}))
}

// newCron создаёт планировщик, у которого любая задача переживает свою панику
// и не запускается повторно, пока не закончился предыдущий проход. У robfig/cron
// по умолчанию нет ни того ни другого: паника в задаче роняла весь процесс.
func newCron(opts ...cron.Option) *cron.Cron {
	chain := cron.WithChain(cron.Recover(cronSlogLogger{}), cron.SkipIfStillRunning(cron.DiscardLogger))
	return cron.New(append([]cron.Option{chain}, opts...)...)
}

// cronSlogLogger направляет сообщения cron (в первую очередь пойманные паники) в slog.
type cronSlogLogger struct{}

func (cronSlogLogger) Info(msg string, keysAndValues ...interface{}) {
	slog.Info("cron: "+msg, keysAndValues...)
}

func (cronSlogLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	slog.Error("cron: "+msg, append([]interface{}{"error", err}, keysAndValues...)...)
}

// logProcessPurchaseError пишет ошибку обработки покупки из поллера. Занятая
// блокировка — не сбой: покупку прямо сейчас доводит вебхук или соседний проход.
func logProcessPurchaseError(msg string, err error, args ...any) {
	if errors.Is(err, database.ErrPurchaseLocked) {
		slog.Info(msg+": purchase is being processed elsewhere", args...)
		return
	}
	slog.Error(msg, append(args, "error", err)...)
}

// Проверяет оплаченные счета в CryptoPay и YooKassa каждые 5 секунд
// Если счет оплачен, обрабатывает покупку и активирует подписку
func setupInvoiceChecker(
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
	yookasaClient *yookasa.Client,
	plategaClient *platega.Client,
	heleketReconciler *heleket.Reconciler,
) *cron.Cron {
	// Задачи регистрируем по наличию реквизитов, а включённость проверяем внутри
	// самой задачи. Способы оплаты переключаются из админки без рестарта: если
	// решать здесь, то метод, включённый после старта, создавал бы счета,
	// которые никто не проверяет, — оплата зависала бы навсегда.
	yookPoll := config.YookasaHasCredentials() && strings.TrimSpace(config.GetYookasaWebHookURL()) == ""
	plategaPoll := config.PlategaHasCredentials() && strings.TrimSpace(config.GetPlategaWebHookURL()) == ""
	// Heleket опрашиваем всегда, даже когда вебхук настроен: крипта необратима,
	// и пропущенный колбэк означал бы, что человек заплатил и ничего не получил.
	heleketPoll := config.HeleketHasCredentials() && heleketReconciler != nil
	if !config.CryptoPayHasCredentials() && !yookPoll && !plategaPoll && !heleketPoll {
		return nil
	}
	c := newCron(cron.WithSeconds()) // Включаем поддержку секунд в расписании

	// Задача для проверки счетов CryptoPay (каждые 5 секунд)
	if config.CryptoPayHasCredentials() {
		_, err := c.AddJob("*/5 * * * * *", invoicePollJob(func(ctx context.Context) {
			if !config.IsCryptoPayEnabled() {
				return
			}
			checkCryptoPayInvoice(ctx, purchaseRepository, cryptoPayClient, paymentService)
		}))

		if err != nil {
			panic(err)
		}
	}

	// Задача для проверки счетов YooKassa (каждые 5 секунд), если нет вебхука
	if yookPoll {
		_, err := c.AddJob("*/5 * * * * *", invoicePollJob(func(ctx context.Context) {
			if !config.IsYookasaEnabled() {
				return
			}
			checkYookasaInvoice(ctx, purchaseRepository, yookasaClient, paymentService)
		}))

		if err != nil {
			panic(err)
		}
	}

	if plategaPoll && plategaClient != nil && plategaClient.IsConfigured() {
		_, err := c.AddJob("*/5 * * * * *", invoicePollJob(func(ctx context.Context) {
			if !config.IsPlategaEnabled() {
				return
			}
			checkPlategaInvoice(ctx, purchaseRepository, plategaClient, paymentService)
		}))
		if err != nil {
			panic(err)
		}
	}

	if heleketPoll {
		// С вебхуком поллинг — лишь страховка, поэтому раз в минуту; без него он
		// единственный способ узнать об оплате, и частота как у остальных касс.
		schedule := "*/5 * * * * *"
		if strings.TrimSpace(config.GetHeleketWebHookURL()) != "" {
			schedule = "0 * * * * *"
		}
		// Намеренно без проверки IsHeleketEnabled: выключение метода в админке
		// должно прекращать выдачу новых счетов, а не бросать уже выставленные.
		// Оплату по ним всё равно не вернуть, поэтому pending дозакрываем всегда.
		// Проходы не должны накладываться: при большом числе pending один обход
		// длится дольше периода, и два поллера пошли бы по одному списку.
		_, err := c.AddJob(schedule, cron.NewChain(cron.Recover(cronSlogLogger{}), cron.SkipIfStillRunning(cron.DiscardLogger)).Then(cron.FuncJob(func() {
			ctx, cancel := context.WithTimeout(context.Background(), heleketPollTimeout)
			defer cancel()
			heleketReconciler.PollPending(ctx)
		})))
		if err != nil {
			panic(err)
		}
	}

	return c
}

// checkYookasaInvoice - проверяет статус счетов YooKassa
// Находит все ожидающие оплаты покупки и проверяет их статус в YooKassa
// Если счет оплачен, обрабатывает покупку и активирует подписку
func checkYookasaInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	yookasaClient *yookasa.Client,
	paymentService *payment.PaymentService,
) {
	// Получаем все покупки со статусом "ожидает оплаты" для YooKassa
	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeYookasa,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return // Нет ожидающих покупок
	}

	// Проверяем каждую покупку
	for _, purchase := range *pendingPurchases {
		// Получаем информацию о счете из YooKassa
		invoice, err := yookasaClient.GetPayment(ctx, *purchase.YookasaID)

		if err != nil {
			if errors.Is(err, yookasa.ErrPaymentNotFound) {
				slog.Warn("YooKassa invoice not found, canceling purchase", "invoiceId", purchase.YookasaID, "purchaseId", purchase.ID)
				if cancelErr := paymentService.CancelYookassaPayment(purchase.ID); cancelErr != nil {
					slog.Error("Error canceling invoice after not found", "invoiceId", purchase.YookasaID, "purchaseId", purchase.ID, "error", cancelErr)
				}
				continue
			}
			slog.Error("Error getting invoice", "invoiceId", purchase.YookasaID, "error", err)
			continue
		}

		if err := invoice.MatchesPurchase(&purchase); err != nil {
			slog.Error("YooKassa payment does not match purchase", "purchaseId", purchase.ID, "error", err)
			continue
		}

		// Если счет отменен, отменяем покупку в нашей системе
		if invoice.IsCancelled() {
			err := paymentService.CancelYookassaPayment(purchase.ID)
			if err != nil {
				slog.Error("Error canceling invoice", "invoiceId", invoice.ID, "purchaseId", purchase.ID, "error", err)
			}
			continue
		}

		// Если счет еще не оплачен, пропускаем
		if !invoice.Paid {
			continue
		}

		// Счет оплачен - обрабатываем текущую pending-покупку.
		// Не зависим от invoice.Metadata["purchaseId"], т.к. некоторые провайдеры/ответы
		// могут вернуть metadata в нестабильном формате и это приводило к "eternal pending".
		username := ""
		if invoice.Metadata != nil {
			username = invoice.Metadata["username"]
		}
		ctxWithValue := context.WithValue(ctx, remnawave.CtxKeyUsername, username)
		purchaseId := int(purchase.ID)
		err = paymentService.ProcessPurchaseById(ctxWithValue, purchase.ID)
		if err != nil {
			logProcessPurchaseError("yookassa poll: process purchase", err, "invoiceId", invoice.ID, "purchaseId", purchaseId)
		} else {
			slog.Info("Invoice processed", "invoiceId", invoice.ID, "purchaseId", purchaseId)
		}
	}
}

// checkPlategaInvoice — поллинг pending-покупок Platega (если не используется вебхук).
func checkPlategaInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	plategaClient *platega.Client,
	paymentService *payment.PaymentService,
) {
	if plategaClient == nil || !plategaClient.IsConfigured() {
		return
	}
	for _, inv := range database.PlategaInvoiceTypes() {
		pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
			ctx,
			inv,
			database.PurchaseStatusPending,
		)
		if err != nil {
			slog.Error("platega poll: find pending", "invoice_type", inv, "error", err)
			continue
		}
		if pendingPurchases == nil || len(*pendingPurchases) == 0 {
			continue
		}
		for _, purchase := range *pendingPurchases {
			if purchase.PlategaID == nil || strings.TrimSpace(*purchase.PlategaID) == "" {
				continue
			}
			tx, err := plategaClient.GetTransaction(ctx, strings.TrimSpace(*purchase.PlategaID))
			if err != nil {
				var apiErr *platega.APIError
				if errors.As(err, &apiErr) && apiErr.IsNotFound() {
					slog.Warn("Platega transaction not found, canceling purchase", "tx", *purchase.PlategaID, "purchaseId", purchase.ID)
					if cancelErr := paymentService.CancelPlategaPayment(purchase.ID); cancelErr != nil {
						slog.Error("Error canceling platega purchase after not found", "purchaseId", purchase.ID, "error", cancelErr)
					}
					continue
				}
				slog.Error("platega poll: get transaction", "tx", *purchase.PlategaID, "error", err)
				continue
			}
			switch tx.Status {
			case platega.StatusCanceled, platega.StatusChargebacked:
				if cancelErr := paymentService.CancelPlategaPayment(purchase.ID); cancelErr != nil {
					slog.Error("platega poll: cancel", "purchaseId", purchase.ID, "error", cancelErr)
				}
			case platega.StatusConfirmed:
				if err := platega.CheckTransaction(tx, &purchase); err != nil {
					slog.Error("platega poll: transaction does not match purchase", "purchaseId", purchase.ID, "error", err)
					continue
				}
				if err := paymentService.ProcessPurchaseById(ctx, purchase.ID); err != nil {
					logProcessPurchaseError("platega poll: process purchase", err, "purchaseId", purchase.ID)
				} else {
					slog.Info("platega poll: processed", "purchaseId", purchase.ID)
				}
			default:
			}
		}
	}
}

// checkCryptoPayInvoice - проверяет статус счетов CryptoPay
// Находит все ожидающие оплаты покупки и проверяет их статус в CryptoPay
// Если счет оплачен, обрабатывает покупку и активирует подписку
func checkCryptoPayInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
) {
	// Получаем все покупки со статусом "ожидает оплаты" для CryptoPay
	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeCrypto,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return // Нет ожидающих покупок
	}

	// Собираем ID всех счетов для массовой проверки
	var invoiceIDs []string
	for _, purchase := range *pendingPurchases {
		if purchase.CryptoInvoiceID != nil {
			invoiceIDs = append(invoiceIDs, fmt.Sprintf("%d", *purchase.CryptoInvoiceID))
		}
	}

	if len(invoiceIDs) == 0 {
		return
	}

	// Запрашиваем статус всех счетов одним запросом (оптимизация)
	stringInvoiceIDs := strings.Join(invoiceIDs, ",")
	invoices, err := cryptoPayClient.GetInvoices("", "", "", stringInvoiceIDs, 0, 0)
	if err != nil {
		log.Printf("Error getting invoices: %v", err)
		return
	}

	// Обрабатываем оплаченные счета
	for _, invoice := range *invoices {
		if invoice.InvoiceID != nil && invoice.IsPaid() {
			// Извлекаем данные из payload счета (формат: "purchaseId=123&username=user").
			// username не экранируется при создании счёта, поэтому разбираем вручную, а
			// не url.ParseQuery. Счёт с битым payload пропускаем: раньше индексация без
			// проверок паниковала, и один такой счёт останавливал обработку всех.
			idPart, userPart, _ := strings.Cut(invoice.Payload, "&")
			idStr, _ := strings.CutPrefix(idPart, "purchaseId=")
			purchaseID, err := strconv.Atoi(idStr)
			if err != nil || purchaseID <= 0 {
				slog.Error("cryptopay poll: no purchase id in invoice payload", "invoiceId", invoice.InvoiceID)
				continue
			}
			username := strings.TrimPrefix(userPart, "username=")
			// Передаем username в контексте для логирования
			ctxWithUsername := context.WithValue(ctx, remnawave.CtxKeyUsername, username)
			feeAmt := ""
			if invoice.FeeAmount != nil {
				feeAmt = *invoice.FeeAmount
			}
			ctxPaid := payment.WithCryptoNotifyMeta(ctxWithUsername, payment.CryptoNotifyMeta{
				Hash:          invoice.Hash,
				Status:        invoice.Status,
				CurrencyType:  invoice.CurrencyType,
				Asset:         invoice.Asset,
				PaidAsset:     invoice.PaidAsset,
				PaidAmount:    invoice.PaidAmount,
				PayUrl:        invoice.PayUrl,
				BotInvoiceUrl: invoice.BotInvoiceUrl,
				FeeAmount:     feeAmt,
			})
			err = paymentService.ProcessPurchaseById(ctxPaid, int64(purchaseID))
			if err != nil {
				logProcessPurchaseError("cryptopay poll: process purchase", err, "invoiceId", invoice.InvoiceID, "purchaseId", purchaseID)
			} else {
				slog.Info("Invoice processed", "invoiceId", invoice.InvoiceID, "purchaseId", purchaseID)
			}
		}
	}
}
