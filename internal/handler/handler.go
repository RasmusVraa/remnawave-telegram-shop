package handler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/yookasa"
)

type Handler struct {
	customerRepository     *database.CustomerRepository
	purchaseRepository     *database.PurchaseRepository
	tariffRepository       *database.TariffRepository
	cryptoPayClient        *cryptopay.Client
	yookasaClient          *yookasa.Client
	translation            *translation.Manager
	paymentService         *payment.PaymentService
	syncService            *sync.SyncService
	referralRepository     *database.ReferralRepository
	partnerRepository      *database.PartnerRepository
	cache                  *cache.Cache
	promoRepository        *database.PromoRepository
	promoService           *promo.Service
	remnawaveClient        *remnawave.Client
	statsRepository        *database.StatsRepository
	infraBillingRepository *database.InfraBillingRepository
	loyaltyTierRepository  *database.LoyaltyTierRepository
	broadcastSender        *broadcast.Sender
}

func NewHandler(
	syncService *sync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	tariffRepository *database.TariffRepository,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client,
	referralRepository *database.ReferralRepository,
	partnerRepository *database.PartnerRepository,
	cache *cache.Cache,
	promoRepository *database.PromoRepository,
	promoService *promo.Service,
	remnawaveClient *remnawave.Client,
	statsRepository *database.StatsRepository,
	infraBillingRepository *database.InfraBillingRepository,
	loyaltyTierRepository *database.LoyaltyTierRepository,
	broadcastSender *broadcast.Sender,
) *Handler {
	return &Handler{
		syncService:            syncService,
		paymentService:         paymentService,
		customerRepository:     customerRepository,
		purchaseRepository:     purchaseRepository,
		tariffRepository:       tariffRepository,
		cryptoPayClient:        cryptoPayClient,
		yookasaClient:          yookasaClient,
		translation:            translation,
		referralRepository:     referralRepository,
		partnerRepository:      partnerRepository,
		cache:                  cache,
		promoRepository:        promoRepository,
		promoService:           promoService,
		remnawaveClient:        remnawaveClient,
		statsRepository:        statsRepository,
		infraBillingRepository: infraBillingRepository,
		loyaltyTierRepository:  loyaltyTierRepository,
		broadcastSender:        broadcastSender,
	}
}

// ForwardUserMessageToAdmin пересылает все не-командные сообщения пользователей админу
func (h Handler) ForwardUserMessageToAdmin(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !config.ForwardUserMessagesToAdmin() {
		return
	}
	if update.Message == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	user := update.Message.From
	text := update.Message.Text
	if user.ID == adminID {
		return // не пересылать админу его же сообщения
	}
	msgText := ""
	if strings.HasPrefix(text, "/") {
		msgText = fmt.Sprintf("@%s (ID: %d):\n%s", user.Username, user.ID, text)
	} else {
		msgText = fmt.Sprintf("@%s (ID: %d):\n%s", user.Username, user.ID, text)
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminID,
		Text:   msgText,
	})
	if err != nil {
		slog.Error("send message to admin", "error", err)
	}
}

// ForwardUserMessageToAdminMiddleware - middleware версия для отслеживания сообщений
func (h Handler) ForwardUserMessageToAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		// Сначала пересылаем сообщение админу
		h.ForwardUserMessageToAdmin(ctx, b, update)
		// Затем вызываем следующий обработчик
		next(ctx, b, update)
	}
}

// AdminReplyToUser отправляет ответ админа пользователю, если это reply на пересланное сообщение
func (h Handler) AdminReplyToUser(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.ReplyToMessage == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	if update.Message.From.ID != adminID {
		return
	}
	// Не отвечать на свои же сообщения
	if update.Message.ReplyToMessage.From != nil && update.Message.ReplyToMessage.From.ID == adminID {
		return
	}
	// Пытаемся извлечь ID пользователя из текста пересланного сообщения через regexp
	origText := update.Message.ReplyToMessage.Text
	re := regexp.MustCompile(`\(ID: (\d+)\)`)
	matches := re.FindStringSubmatch(origText)
	if len(matches) != 2 {
		return
	}
	userID, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || userID == 0 {
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   update.Message.Text,
	})
}
