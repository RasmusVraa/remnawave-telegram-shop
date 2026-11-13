package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

type LifecycleService struct {
	customerRepo    *database.CustomerRepository
	purchaseRepo    *database.PurchaseRepository
	promoRepo       *database.PromoRepository
	promoService    *promo.Service
	remnawaveClient *remnawave.Client
	bot             *bot.Bot
	tm              *translation.Manager
	lifecycleRepo   *LifecycleRepository
}

func NewLifecycleService(
	customerRepo *database.CustomerRepository,
	purchaseRepo *database.PurchaseRepository,
	promoRepo *database.PromoRepository,
	promoService *promo.Service,
	remnawaveClient *remnawave.Client,
	bot *bot.Bot,
	tm *translation.Manager,
	lifecycleRepo *LifecycleRepository,
) *LifecycleService {
	return &LifecycleService{
		customerRepo:    customerRepo,
		purchaseRepo:    purchaseRepo,
		promoRepo:       promoRepo,
		promoService:    promoService,
		remnawaveClient: remnawaveClient,
		bot:             bot,
		tm:              tm,
		lifecycleRepo:   lifecycleRepo,
	}
}

// ProcessLifecycleNotifications запускает все активные сценарии lifecycle-уведомлений.
func (s *LifecycleService) ProcessLifecycleNotifications() error {
	ctx := context.Background()

	if config.LifecycleNoConnectPaidEnabled() {
		if err := s.processNoConnectPaid(ctx); err != nil {
			slog.Error("lifecycle: no_connect_paid failed", "error", err)
		}
	}

	if config.LifecycleNoConnectTrialEnabled() {
		if err := s.processNoConnectTrial(ctx); err != nil {
			slog.Error("lifecycle: no_connect_trial failed", "error", err)
		}
	}

	if config.LifecycleWinbackEnabled() {
		if err := s.processWinback(ctx); err != nil {
			slog.Error("lifecycle: winback failed", "error", err)
		}
	}

	return nil
}

func (s *LifecycleService) processNoConnectPaid(ctx context.Context) error {
	delayHours := config.LifecycleNoConnectDelayHours()
	maxAgeHours := config.LifecycleNoConnectMaxAgeHours()

	candidates, err := s.lifecycleRepo.FindNoConnectPaidCandidates(ctx, delayHours, maxAgeHours)
	if err != nil {
		return fmt.Errorf("find no_connect_paid candidates: %w", err)
	}

	slog.Info("lifecycle: no_connect_paid candidates", "count", len(candidates))

	for _, customerID := range candidates {
		if err := s.sendNoConnectPaidNotify(ctx, customerID); err != nil {
			slog.Error("lifecycle: send no_connect_paid failed", "customer_id", utils.MaskHalfInt64(customerID), "error", err)
		}
	}

	return nil
}

func (s *LifecycleService) sendNoConnectPaidNotify(ctx context.Context, customerID int64) error {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil || customer == nil {
		return fmt.Errorf("find customer: %w", err)
	}

	if customer.IsWebOnly || utils.IsSyntheticTelegramID(customer.TelegramID) {
		return nil
	}

	// Проверка подключения через Remnawave
	connected, err := IsUserConnected(ctx, s.remnawaveClient, customer.TelegramID)
	if err != nil {
		return fmt.Errorf("check connection: %w", err)
	}
	if connected {
		slog.Debug("lifecycle: user already connected, skip", "customer_id", utils.MaskHalfInt64(customerID))
		return nil
	}

	support := s.buildSupportContact()
	text := fmt.Sprintf(s.tm.GetText(customer.Language, "lifecycle_no_connect_paid"), support)
	keyboard := s.buildNoConnectKeyboard(customer.Language)

	_, err = s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	if err := s.lifecycleRepo.MarkNotifySent(ctx, customerID, "no_connect_paid", "once"); err != nil {
		slog.Error("lifecycle: mark sent failed", "customer_id", utils.MaskHalfInt64(customerID), "error", err)
	}

	slog.Info("lifecycle: no_connect_paid sent", "customer_id", utils.MaskHalfInt64(customerID))
	return nil
}

func (s *LifecycleService) processNoConnectTrial(ctx context.Context) error {
	delayHours := config.LifecycleNoConnectDelayHours()
	maxAgeHours := config.LifecycleNoConnectMaxAgeHours()

	candidates, err := s.lifecycleRepo.FindNoConnectTrialCandidates(ctx, delayHours, maxAgeHours)
	if err != nil {
		return fmt.Errorf("find no_connect_trial candidates: %w", err)
	}

	slog.Info("lifecycle: no_connect_trial candidates", "count", len(candidates))

	for _, customerID := range candidates {
		if err := s.sendNoConnectTrialNotify(ctx, customerID); err != nil {
			slog.Error("lifecycle: send no_connect_trial failed", "customer_id", utils.MaskHalfInt64(customerID), "error", err)
		}
	}

	return nil
}

func (s *LifecycleService) sendNoConnectTrialNotify(ctx context.Context, customerID int64) error {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil || customer == nil {
		return fmt.Errorf("find customer: %w", err)
	}

	if customer.IsWebOnly || utils.IsSyntheticTelegramID(customer.TelegramID) {
		return nil
	}

	connected, err := IsUserConnected(ctx, s.remnawaveClient, customer.TelegramID)
	if err != nil {
		return fmt.Errorf("check connection: %w", err)
	}
	if connected {
		return nil
	}

	support := s.buildSupportContact()
	text := fmt.Sprintf(s.tm.GetText(customer.Language, "lifecycle_no_connect_trial"), support)
	keyboard := s.buildNoConnectKeyboard(customer.Language)

	_, err = s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	if err := s.lifecycleRepo.MarkNotifySent(ctx, customerID, "no_connect_trial", "once"); err != nil {
		slog.Error("lifecycle: mark sent failed", "customer_id", utils.MaskHalfInt64(customerID), "error", err)
	}

	slog.Info("lifecycle: no_connect_trial sent", "customer_id", utils.MaskHalfInt64(customerID))
	return nil
}

func (s *LifecycleService) processWinback(ctx context.Context) error {
	daysAfterExpiry := config.LifecycleWinbackDaysAfterExpiry()

	candidates, err := s.lifecycleRepo.FindWinbackCandidates(ctx, daysAfterExpiry)
	if err != nil {
		return fmt.Errorf("find winback candidates: %w", err)
	}

	slog.Info("lifecycle: winback candidates", "count", len(candidates))

	for _, c := range candidates {
		if err := s.sendWinbackNotify(ctx, c); err != nil {
			slog.Error("lifecycle: send winback failed", "customer_id", utils.MaskHalfInt64(c.CustomerID), "error", err)
		}
	}

	return nil
}

func (s *LifecycleService) sendWinbackNotify(ctx context.Context, candidate WinbackCandidate) error {
	// Получаем системный promo_code __lifecycle_winback__
	lifecyclePromo, err := s.promoRepo.FindByCode(ctx, "__LIFECYCLE_WINBACK__")
	if err != nil || lifecyclePromo == nil {
		return fmt.Errorf("get lifecycle promo code: %w", err)
	}

	percent := config.LifecycleWinbackDiscountPercent()
	ttlHours := config.LifecycleWinbackDiscountTTLHours()

	// Стакуем скидку
	if err := s.promoService.GrantStackedPendingDiscount(ctx, candidate.CustomerID, lifecyclePromo.ID, percent, ttlHours); err != nil {
		return fmt.Errorf("grant stacked discount: %w", err)
	}

	// Получаем итоговую скидку для отображения
	finalDiscount, err := s.promoRepo.GetPendingDiscountByCustomerID(ctx, candidate.CustomerID)
	if err != nil || finalDiscount == nil {
		return fmt.Errorf("get final discount: %w", err)
	}

	daysExpired := int(time.Since(candidate.ExpireAt).Hours() / 24)
	loyaltyCap := config.LoyaltyMaxTotalDiscountPercent()
	timeLeft := s.formatTimeLeft(finalDiscount.ExpiresAt, candidate.Language)

	text := fmt.Sprintf(
		s.tm.GetText(candidate.Language, "lifecycle_winback"),
		daysExpired,
		finalDiscount.Percent,
		loyaltyCap,
		timeLeft,
	)

	keyboard := s.buildWinbackKeyboard(candidate.Language, finalDiscount.Percent)

	_, err = s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      candidate.TelegramID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	referenceKey := candidate.ExpireAt.Format("2006-01-02")
	if err := s.lifecycleRepo.MarkNotifySent(ctx, candidate.CustomerID, "winback", referenceKey); err != nil {
		slog.Error("lifecycle: mark sent failed", "customer_id", utils.MaskHalfInt64(candidate.CustomerID), "error", err)
	}

	slog.Info("lifecycle: winback sent", "customer_id", utils.MaskHalfInt64(candidate.CustomerID), "percent", finalDiscount.Percent)
	return nil
}

// buildSupportContact возвращает @username из LIFECYCLE_SUPPORT_CONTACT или URL из SUPPORT_URL.
func (s *LifecycleService) buildSupportContact() string {
	contact := config.LifecycleSupportContact()
	if contact != "" {
		return contact
	}
	supportURL := config.SupportURL()
	if supportURL != "" {
		return supportURL
	}
	feedbackURL := config.FeedbackURL()
	if feedbackURL != "" {
		return feedbackURL
	}
	return "@support"
}

func (s *LifecycleService) buildNoConnectKeyboard(lang string) models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton

	// 1. Подключиться
	// Если кабинет включён → WebApp на /cabinet/connections, иначе → callback в главное меню
	cabinetURL := handler.BuildCabinetWebAppURL("/cabinet/connections")
	var connectBtn models.InlineKeyboardButton
	if cabinetURL != "" {
		connectBtn = s.tm.WithButton(lang, "lifecycle_btn_connect", models.InlineKeyboardButton{
			WebApp: &models.WebAppInfo{URL: cabinetURL},
		})
	} else {
		connectBtn = s.tm.WithButton(lang, "lifecycle_btn_connect", models.InlineKeyboardButton{
			CallbackData: handler.CallbackConnect,
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{connectBtn})

	// 2. Видео инструкция (если задан URL)
	videoURL := config.LifecycleVideoGuideURL()
	if videoURL != "" {
		videoBtn := s.tm.WithButton(lang, "lifecycle_btn_video", models.InlineKeyboardButton{
			URL: videoURL,
		})
		rows = append(rows, []models.InlineKeyboardButton{videoBtn})
	}

	// 3. Поддержка
	supportURL := config.SupportURL()
	if supportURL == "" {
		supportURL = config.FeedbackURL()
	}
	if supportURL != "" {
		supportBtn := s.tm.WithButton(lang, "lifecycle_btn_support", models.InlineKeyboardButton{
			URL: supportURL,
		})
		rows = append(rows, []models.InlineKeyboardButton{supportBtn})
	}

	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (s *LifecycleService) buildWinbackKeyboard(lang string, discountPercent int) models.InlineKeyboardMarkup {
	// Кнопка «Продлить со скидкой −X%» → callback buy / webapp tariffs (как SubscriptionExpiringRenewInlineButton)
	btnText := fmt.Sprintf(s.tm.GetText(lang, "lifecycle_btn_winback_renew"), discountPercent)

	if handler.IsCabinetTelegramMinimalismActive() {
		return models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{
					Text:   btnText,
					WebApp: &models.WebAppInfo{URL: handler.BuildCabinetWebAppURL("/cabinet/tariffs")},
				},
			}},
		}
	}

	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{
				Text:         btnText,
				CallbackData: handler.CallbackBuy,
			},
		}},
	}
}

func (s *LifecycleService) formatTimeLeft(expiresAt *time.Time, lang string) string {
	if expiresAt == nil {
		return s.tm.GetText(lang, "lifecycle_time_unlimited")
	}

	left := time.Until(*expiresAt)
	if left < 0 {
		return s.tm.GetText(lang, "lifecycle_time_expired")
	}

	hours := int(left.Hours())
	if hours < 24 {
		return fmt.Sprintf(s.tm.GetText(lang, "lifecycle_time_hours"), hours)
	}
	days := hours / 24
	return fmt.Sprintf(s.tm.GetText(lang, "lifecycle_time_days"), days)
}
