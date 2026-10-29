package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/heleket"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/internal/platega"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/receipt"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/yookasa"
	"remnawave-tg-shop-bot/utils"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// skipTelegramCustomerDM — для web-only / synthetic telegram_id нет TG-чата;
// любой SendMessage/DeleteMessage по ChatID=telegram_id даст «chat not found».
// См. docs/cabinet/audit-telegram-id.md §1.3.
func skipTelegramCustomerDM(c *database.Customer) bool {
	if c == nil {
		return true
	}
	return c.IsWebOnly || utils.IsSyntheticTelegramID(c.TelegramID)
}

func emailLocalPartForDescription(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return ""
	}
	local := email[:at]
	if plus := strings.Index(local, "+"); plus >= 0 {
		local = local[:plus]
	}
	return strings.TrimSpace(local)
}

func (s PaymentService) withRemnawavePanelUsername(ctx context.Context, customer *database.Customer) context.Context {
	if customer == nil || (!customer.IsWebOnly && !utils.IsSyntheticTelegramID(customer.TelegramID)) {
		return ctx
	}
	if s.customerRepository == nil {
		return ctx
	}
	emails, err := s.customerRepository.CabinetAccountEmailsByCustomerIDs(ctx, []int64{customer.ID})
	if err != nil {
		slog.Warn("payment: load cabinet email for panel username", "customer_id", customer.ID, "error", err)
		return ctx
	}
	email := strings.TrimSpace(emails[customer.ID])
	panelUsername := utils.PanelLoginForCustomer(customer.ID, email)
	ctx = context.WithValue(ctx, remnawave.CtxKeyPanelUsername, panelUsername)
	if desc := emailLocalPartForDescription(email); desc != "" {
		// Для web-only профилей храним local-part email в Description (без домена):
		// это упрощает идентификацию в админке при отсутствии реального TG username.
		ctx = context.WithValue(ctx, remnawave.CtxKeyUsername, desc)
	}
	return ctx
}

func (s PaymentService) ctxWithTelegramUsernameIfMissing(ctx context.Context, customer *database.Customer) context.Context {
	if customer == nil {
		return ctx
	}
	if v, ok := ctx.Value(remnawave.CtxKeyUsername).(string); ok && strings.TrimSpace(v) != "" {
		return ctx
	}
	if customer.TelegramUsername != nil {
		u := strings.TrimSpace(*customer.TelegramUsername)
		u = strings.TrimPrefix(u, "@")
		if u != "" {
			return context.WithValue(ctx, remnawave.CtxKeyUsername, u)
		}
	}
	return ctx
}

type PaymentService struct {
	purchaseRepository    *database.PurchaseRepository
	tariffRepository      *database.TariffRepository
	remnawaveClient       *remnawave.Client
	customerRepository    *database.CustomerRepository
	telegramBot           *bot.Bot
	translation           *translation.Manager
	cryptoPayClient       *cryptopay.Client
	yookasaClient         *yookasa.Client
	plategaClient         *platega.Client
	heleketClient         *heleket.Client
	referralRepository    *database.ReferralRepository
	referralLedger        *database.ReferralBonusLedgerRepository
	partnerRepository     *database.PartnerRepository
	cache                 *cache.Cache
	receiptQueue          *receipt.Service
	promoService          *promo.Service
	loyaltyTierRepository *database.LoyaltyTierRepository
}

// PromoMeta attaches an activated percent discount to a new purchase row (optional).
type PromoMeta struct {
	PromoCodeID            *int64
	DiscountPercentApplied *int
}

func NewPaymentService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	tariffRepository *database.TariffRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client,
	plategaClient *platega.Client,
	heleketClient *heleket.Client,
	referralRepository *database.ReferralRepository,
	referralLedger *database.ReferralBonusLedgerRepository,
	partnerRepository *database.PartnerRepository,
	cache *cache.Cache,
	receiptQueue *receipt.Service,
	promoService *promo.Service,
	loyaltyTierRepository *database.LoyaltyTierRepository,
) *PaymentService {
	return &PaymentService{
		purchaseRepository:    purchaseRepository,
		tariffRepository:      tariffRepository,
		remnawaveClient:       remnawaveClient,
		customerRepository:    customerRepository,
		telegramBot:           telegramBot,
		translation:           translation,
		cryptoPayClient:       cryptoPayClient,
		yookasaClient:         yookasaClient,
		plategaClient:         plategaClient,
		heleketClient:         heleketClient,
		referralRepository:    referralRepository,
		referralLedger:        referralLedger,
		partnerRepository:     partnerRepository,
		cache:                 cache,
		receiptQueue:          receiptQueue,
		promoService:          promoService,
		loyaltyTierRepository: loyaltyTierRepository,
	}
}

func (s PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	// Вебхук, его повторы и поллер приходят с одной покупкой одновременно; без
	// блокировки каждый увидел бы pending и продлил подписку заново. Если покупку
	// уже обрабатывают, отдаём database.ErrPurchaseLocked: касса повторит вебхук,
	// поллер — следующий проход, а к тому времени статус будет paid.
	unlock, err := s.purchaseRepository.LockForProcessing(ctx, purchaseId)
	if err != nil {
		return err
	}
	defer unlock()

	// Статус читаем уже под блокировкой: предыдущий держатель мог успеть записать paid.
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}
	if purchase.Status == database.PurchaseStatusPaid {
		// Защита от повторной обработки paid-покупки (поллеры, повторы webhook'ов).
		return nil
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	ctx = s.ctxWithTelegramUsernameIfMissing(ctx, customer)

	if !config.HwidExtraDevicesEnabled() && purchase.ExtraHwid > 0 {
		if purchase.Month <= 0 {
			return fmt.Errorf("extra hwid purchase is disabled")
		}
		purchase.ExtraHwid = 0
	}

	if messageId, b := s.cache.Get(purchase.ID); b && !skipTelegramCustomerDM(customer) {
		_, err = s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    customer.TelegramID,
			MessageID: messageId,
		})
		if err != nil {
			slog.Error("Error deleting message", "error", err)
		}
	}

	if purchase.Month <= 0 && purchase.ExtraHwid > 0 {
		return s.processDevicePurchase(ctx, purchase, customer)
	}

	daysToAdd := purchase.Month * config.DaysInMonth()
	if config.SalesMode() == "tariffs" && purchase.TariffID != nil && *purchase.TariffID > 0 && s.tariffRepository != nil {
		tariff, err := s.tariffRepository.GetByID(ctx, *purchase.TariffID)
		if err != nil {
			return err
		}
		if tariff == nil {
			return fmt.Errorf("tariff %d not found", *purchase.TariffID)
		}
		profile := BuildRemnawaveTariffProfile(tariff)

		// Апгрейд и досрочный даунгрейд: срок от момента оплаты. Остаток старого тарифа уже учтён в bonus
		// (пересчёт «дневной» стоимости); нельзя прибавлять дни к текущему expire_at — иначе остаток считается дважды.
		if purchase.PurchaseKind == database.PurchaseKindTariffUpgrade || purchase.IsEarlyDowngrade {
			now := time.Now().UTC()
			tpNew, err := s.tariffRepository.GetPrice(ctx, *purchase.TariffID, purchase.Month)
			if err != nil {
				return err
			}
			if tpNew == nil {
				return fmt.Errorf("no tariff_price for tariff %d months %d", *purchase.TariffID, purchase.Month)
			}
			dim := config.DaysInMonth()
			if dim <= 0 {
				dim = 30
			}
			var bonus int
			if customer.CurrentTariffID != nil && *customer.CurrentTariffID > 0 {
				tpOld, err := s.tariffRepository.GetPrice(ctx, *customer.CurrentTariffID, purchase.Month)
				if err == nil && tpOld != nil {
					bonus = ComputeUpgradeBonusDays(customer, tpOld, tpNew, purchase.Month, now)
				}
			}
			daysSwitch := purchase.Month*dim + bonus
			rwCtx := s.withRemnawavePanelUsername(ctx, customer)
			user, err := s.remnawaveClient.CreateOrUpdateUserWithTariffProfileFromNow(rwCtx, customer.ID, customer.TelegramID, daysSwitch, profile)
			if err != nil {
				return err
			}
			if err := s.finalizePurchase(ctx, purchase, customer, user); err != nil {
				return err
			}
			if err := s.applyExtraAfterSubscription(ctx, customer, user, purchase); err != nil {
				return err
			}
			return s.resetTrafficAfterSubscriptionPayment(ctx, user)
		}

		// Первый платёж «поверх» триала без current_tariff_id: при trialAddsToPaid=false
		// срок от момента оплаты (не стакаем к expire_at). Если тариф уже привязан —
		// это продление/оплата с тарифом, даже при paidCount==0 (промо/админ/расхождение
		// счётчиков) продлеваем от expire в Remnawave, как в боте.
		useFromNow := !config.TrialAddsToPaid() && customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())
		if useFromNow {
			paidCount, err := s.purchaseRepository.CountPaidSubscriptionsByCustomer(ctx, customer.ID)
			if err != nil {
				return err
			}
			noPaidTariffYet := customer.CurrentTariffID == nil || *customer.CurrentTariffID == 0
			if paidCount == 0 && noPaidTariffYet {
				rwCtx := s.withRemnawavePanelUsername(ctx, customer)
				user, err := s.remnawaveClient.CreateOrUpdateUserWithTariffProfileFromNow(rwCtx, customer.ID, customer.TelegramID, daysToAdd, profile)
				if err != nil {
					return err
				}
				if err := s.finalizePurchase(ctx, purchase, customer, user); err != nil {
					return err
				}
				if err := s.applyExtraAfterSubscription(ctx, customer, user, purchase); err != nil {
					return err
				}
				return s.resetTrafficAfterSubscriptionPayment(ctx, user)
			}
		}
		rwCtx := s.withRemnawavePanelUsername(ctx, customer)
		user, err := s.remnawaveClient.CreateOrUpdateUserWithTariffProfile(rwCtx, customer.ID, customer.TelegramID, daysToAdd, profile)
		if err != nil {
			return err
		}
		if err := s.finalizePurchase(ctx, purchase, customer, user); err != nil {
			return err
		}
		if err := s.applyExtraAfterSubscription(ctx, customer, user, purchase); err != nil {
			return err
		}
		return s.resetTrafficAfterSubscriptionPayment(ctx, user)
	}

	useFromNow := !config.TrialAddsToPaid() && customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())
	if useFromNow {
		paidCount, err := s.purchaseRepository.CountPaidSubscriptionsByCustomer(ctx, customer.ID)
		if err != nil {
			return err
		}
		noPaidTariffYet := customer.CurrentTariffID == nil || *customer.CurrentTariffID == 0
		if paidCount == 0 && noPaidTariffYet {
			rwCtx := s.withRemnawavePanelUsername(ctx, customer)
			user, err := s.remnawaveClient.CreateOrUpdateUserFromNow(rwCtx, customer.ID, customer.TelegramID, config.TrafficLimit(), daysToAdd, false)
			if err != nil {
				return err
			}
			if err := s.finalizePurchase(ctx, purchase, customer, user); err != nil {
				return err
			}
			if err := s.applyExtraAfterSubscription(ctx, customer, user, purchase); err != nil {
				return err
			}
			return s.resetTrafficAfterSubscriptionPayment(ctx, user)
		}
	}
	rwCtx := s.withRemnawavePanelUsername(ctx, customer)
	user, err := s.remnawaveClient.CreateOrUpdateUser(rwCtx, customer.ID, customer.TelegramID, config.TrafficLimit(), daysToAdd, false)
	if err != nil {
		return err
	}
	if err := s.finalizePurchase(ctx, purchase, customer, user); err != nil {
		return err
	}
	if err := s.applyExtraAfterSubscription(ctx, customer, user, purchase); err != nil {
		return err
	}
	return s.resetTrafficAfterSubscriptionPayment(ctx, user)
}

// panelUserForCustomer находит пользователя панели клиента так же, как кабинет и
// админка. Поиск только по telegram_id не находит web-only клиентов кабинета —
// в панели у них нет telegram_id, и вся обработка после оплаты (лимит устройств,
// докупка HWID, сброс трафика) падала с ErrUserNotFound уже после продления.
func (s PaymentService) panelUserForCustomer(ctx context.Context, customer *database.Customer) (*remnawave.User, error) {
	rwCtx := s.withRemnawavePanelUsername(ctx, customer)
	return s.remnawaveClient.FindUserForAdminCustomer(rwCtx, customer.ID, customer.TelegramID, customer.SubscriptionLink, customer.IsWebOnly)
}

// setDeviceLimit меняет лимит устройств с тем же поиском пользователя, что и panelUserForCustomer.
func (s PaymentService) setDeviceLimit(ctx context.Context, customer *database.Customer, limit int) (*remnawave.User, error) {
	rwCtx := s.withRemnawavePanelUsername(ctx, customer)
	return s.remnawaveClient.UpdateUserDeviceLimitByCustomer(rwCtx, customer.ID, customer.TelegramID, limit)
}

func (s PaymentService) resetTrafficAfterSubscriptionPayment(ctx context.Context, user *remnawave.User) error {
	if user == nil || user.ID <= 0 {
		return nil
	}
	if err := s.remnawaveClient.ResetUserTraffic(ctx, user.ID); err != nil {
		slog.Error("failed to reset user traffic after subscription payment", "error", err)
		return err
	}
	return nil
}

func (s PaymentService) processDevicePurchase(ctx context.Context, purchase *database.Purchase, customer *database.Customer) error {
	err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}
	paidNow := time.Now().UTC()
	purchase.Status = database.PurchaseStatusPaid
	purchase.PaidAt = &paidNow
	expireBefore := customer.ExpireAt

	userInfo, err := s.panelUserForCustomer(ctx, customer)
	if err != nil {
		return err
	}

	currentExtra := 0
	if customer.ExtraHwid > 0 && customer.ExtraHwidExpiresAt != nil && customer.ExtraHwidExpiresAt.After(time.Now()) {
		currentExtra = customer.ExtraHwid
	} else if customer.ExtraHwid > 0 && customer.ExtraHwidExpiresAt != nil && customer.ExtraHwidExpiresAt.Before(time.Now()) {
		newLimit := config.GetHwidFallbackDeviceLimit()
		if newLimit < 1 {
			newLimit = 1
		}
		if _, err := s.setDeviceLimit(ctx, customer, newLimit); err != nil {
			return err
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"extra_hwid":            0,
			"extra_hwid_expires_at": nil,
		}); err != nil {
			return err
		}
		currentExtra = 0
	}

	currentLimit := resolveDeviceLimit(userInfo)
	newLimit := currentLimit + purchase.ExtraHwid
	maxLimit := config.HwidMaxDevices()
	if maxLimit > 0 && newLimit > maxLimit {
		newLimit = maxLimit
	}

	updatedUser, err := s.setDeviceLimit(ctx, customer, newLimit)
	if err != nil {
		return err
	}

	newExtra := currentExtra + purchase.ExtraHwid
	if customer.ExpireAt == nil {
		return fmt.Errorf("subscription expire_at is not set")
	}
	if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"extra_hwid":            newExtra,
		"extra_hwid_expires_at": customer.ExpireAt,
	}); err != nil {
		return err
	}

	if s.receiptQueue.Enabled() && invoiceUsesMoynalogReceipt(purchase) {
		description := buildRubReceiptDescription(0, purchase.ExtraHwid, nil, purchase.InvoiceType)
		slog.Info("Sending receipt to moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", purchase.Amount, "description", description)
		// Очередь сама сообщит админу и переотправит, если ФНС недоступна.
		s.receiptQueue.Submit(ctx, purchase, description)
	}

	if updatedUser != nil {
		customerFilesToUpdate := map[string]interface{}{
			"subscription_link": updatedUser.SubscriptionUrl,
			"expire_at":         updatedUser.ExpireAt,
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
			return err
		}
	}

	if !skipTelegramCustomerDM(customer) {
		successText := fmt.Sprintf(s.translation.GetText(customer.Language, "hwid_change_success_paid"), currentLimit, newLimit, int(math.Ceil(purchase.Amount)))
		_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: customer.TelegramID,
			Text:   successText,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: s.createConnectKeyboard(customer),
			},
		})
		if err != nil {
			// Покупка уже оплачена и выдана: недоставленное сообщение (например,
			// бот заблокирован) не должно отменять промокод, опыт и уведомления.
			slog.Warn("payment DM not delivered", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}
	s.clearPromoDiscountIfUsed(ctx, purchase, customer)
	s.applyLoyaltyXPAfterPayment(ctx, purchase, customer)

	var expireAfter *time.Time
	if updatedUser != nil {
		expireAfter = ptrTimeIfValid(updatedUser.ExpireAt)
	}
	s.tryNotifyPurchasePaid(ctx, purchase, customer, expireBefore, expireAfter)

	return nil
}

func (s PaymentService) applyExtraAfterSubscription(ctx context.Context, customer *database.Customer, user *remnawave.User, purchase *database.Purchase) error {
	if customer == nil || user == nil || purchase == nil {
		return nil
	}
	if purchase.Month <= 0 {
		return nil
	}

	userInfo, err := s.panelUserForCustomer(ctx, customer)
	if err != nil {
		return err
	}

	currentLimit := resolveDeviceLimit(userInfo)
	storedExtra := 0
	if customer.ExtraHwid > 0 && customer.ExtraHwidExpiresAt != nil && customer.ExtraHwidExpiresAt.After(time.Now()) {
		storedExtra = customer.ExtraHwid
	}
	// SALES_MODE=tariffs: при смене тарифа слоты «база старого + extra» могут целиком войти в базу нового — не копить лишний extra в БД.
	carriedExtra := s.tariffEffectiveCarriedExtraHwid(ctx, customer, purchase, storedExtra)

	newExtra := purchase.ExtraHwid
	if newExtra < 0 {
		newExtra = 0
	}

	paidBaseLimit := s.paidHwidBaseLimit(ctx, purchase)

	// В tariffs при оплате подписки без доп. HWID в счёте: лимит = только база выбранного тарифа;
	// не переносим «виртуальные» слоты между тарифами и не сохраняем старый paid extra в БД.
	if newExtra == 0 && config.SalesMode() == "tariffs" && purchase.TariffID != nil && *purchase.TariffID > 0 && s.tariffRepository != nil {
		limit := paidBaseLimit
		if limit < 1 {
			limit = 1
		}
		if maxL := config.HwidMaxDevices(); maxL > 0 && limit > maxL {
			limit = maxL
		}
		if _, err := s.setDeviceLimit(ctx, customer, limit); err != nil {
			return err
		}
		return s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"extra_hwid":            0,
			"extra_hwid_expires_at": nil,
		})
	}

	// SALES_MODE=tariffs и в счёте есть доп. HWID: лимит = база тарифа из счёта + newExtra.
	// Учитываем и смену тарифа, и продление того же тарифа: после CreateOrUpdateUserWithTariffProfile
	// панель часто отдаёт currentLimit = только база тарифа; тогда currentLimit−carriedExtra+newExtra занижает лимит
	// (например 2−2+2=2 вместо 2+2=4, с min base 1 → 3).
	if config.SalesMode() == "tariffs" && purchase.TariffID != nil && *purchase.TariffID > 0 && s.tariffRepository != nil && newExtra > 0 {
		newLimit := paidBaseLimit + newExtra
		if maxL := config.HwidMaxDevices(); maxL > 0 && newLimit > maxL {
			newLimit = maxL
		}
		if _, err := s.setDeviceLimit(ctx, customer, newLimit); err != nil {
			return err
		}
		return s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"extra_hwid":            newExtra,
			"extra_hwid_expires_at": user.ExpireAt,
		})
	}

	if carriedExtra == 0 && newExtra == 0 && paidBaseLimit > 0 && currentLimit > 0 && currentLimit < paidBaseLimit {
		if _, err := s.setDeviceLimit(ctx, customer, paidBaseLimit); err != nil {
			return err
		}
		currentLimit = paidBaseLimit
	}
	baseLimit := currentLimit - carriedExtra
	if baseLimit < 1 {
		baseLimit = 1
	}
	newLimit := baseLimit + newExtra
	maxLimit := config.HwidMaxDevices()
	if maxLimit > 0 && newLimit > maxLimit {
		newLimit = maxLimit
	}
	if carriedExtra > 0 && newExtra == 0 {
		// Продление подписки без новой докупки HWID: база = лимит тарифа (или env), не fallback из конфига.
		newLimit = paidBaseLimit + carriedExtra
		if newLimit < 1 {
			newLimit = 1
		}
		if maxLimit > 0 && newLimit > maxLimit {
			newLimit = maxLimit
		}
	}

	if carriedExtra > 0 || newExtra > 0 {
		if _, err := s.setDeviceLimit(ctx, customer, newLimit); err != nil {
			return err
		}
	}

	persistedExtra := newExtra
	if config.SalesMode() == "tariffs" && purchase.TariffID != nil && *purchase.TariffID > 0 && s.tariffRepository != nil {
		var oldTID int64
		if customer != nil && customer.CurrentTariffID != nil {
			oldTID = *customer.CurrentTariffID
		}
		if oldTID != *purchase.TariffID {
			persistedExtra = carriedExtra + newExtra
		}
	}
	updates := map[string]interface{}{
		"extra_hwid":            persistedExtra,
		"extra_hwid_expires_at": nil,
	}
	if persistedExtra > 0 {
		updates["extra_hwid_expires_at"] = user.ExpireAt
	}
	return s.customerRepository.UpdateFields(ctx, customer.ID, updates)
}

// tariffEffectiveCarriedExtraHwid — для classic / без смены тарифа возвращает stored как есть.
// В tariffs при смене тарифа: сколько слотов остаётся «сверх новой базы» (старая база тарифа + extra_hwid минус база нового тарифа); stored может быть 0, если все слоты были в базе дорогого тарифа.
// Нет current_tariff_id (триал, первый платный тариф): не подставлять PAID_HWID_LIMIT из env как «старую базу» — лимит задаёт только выбранный тариф и явный extra_hwid.
func (s PaymentService) tariffEffectiveCarriedExtraHwid(ctx context.Context, customer *database.Customer, purchase *database.Purchase, storedActiveExtra int) int {
	if config.SalesMode() != "tariffs" || s.tariffRepository == nil || purchase == nil || purchase.TariffID == nil || *purchase.TariffID <= 0 {
		return storedActiveExtra
	}
	newBase := s.paidHwidBaseLimit(ctx, purchase)
	if newBase < 1 {
		newBase = 1
	}
	newTariffID := *purchase.TariffID
	var oldTariffID int64
	if customer != nil && customer.CurrentTariffID != nil {
		oldTariffID = *customer.CurrentTariffID
	}
	if oldTariffID > 0 && oldTariffID == newTariffID {
		return storedActiveExtra
	}
	if oldTariffID == 0 {
		return storedActiveExtra
	}
	oldBase := config.PaidHwidLimit()
	if oldBase <= 0 {
		oldBase = config.GetHwidFallbackDeviceLimit()
	}
	if oldBase < 1 {
		oldBase = 1
	}
	ot, err := s.tariffRepository.GetByID(ctx, oldTariffID)
	if err == nil && ot != nil && ot.DeviceLimit > 0 {
		oldBase = ot.DeviceLimit
	}
	totalHad := oldBase + storedActiveExtra
	if totalHad <= newBase {
		return 0
	}
	return totalHad - newBase
}

func (s *PaymentService) paidHwidBaseLimit(ctx context.Context, purchase *database.Purchase) int {
	if s.tariffRepository != nil && purchase != nil && purchase.TariffID != nil && *purchase.TariffID > 0 {
		t, err := s.tariffRepository.GetByID(ctx, *purchase.TariffID)
		if err == nil && t != nil && t.DeviceLimit > 0 {
			return t.DeviceLimit
		}
	}
	paidBaseLimit := config.PaidHwidLimit()
	if paidBaseLimit <= 0 {
		paidBaseLimit = config.GetHwidFallbackDeviceLimit()
	}
	return paidBaseLimit
}

func resolveDeviceLimit(userInfo *remnawave.User) int {
	if userInfo != nil && userInfo.HwidDeviceLimit != nil && *userInfo.HwidDeviceLimit > 0 {
		return *userInfo.HwidDeviceLimit
	}
	fallback := config.GetHwidFallbackDeviceLimit()
	if fallback <= 0 {
		return 1
	}
	return fallback
}

func (s PaymentService) finalizePurchase(ctx context.Context, purchase *database.Purchase, customer *database.Customer, user *remnawave.User) error {
	err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}
	paidNow := time.Now().UTC()
	purchase.Status = database.PurchaseStatusPaid
	purchase.PaidAt = &paidNow
	expireBefore := customer.ExpireAt

	// Отправка дохода в «Мой налог» для способов из MOYNALOG_RECEIPT_FOR (сразу после подтверждения оплаты).
	if s.receiptQueue.Enabled() && invoiceUsesMoynalogReceipt(purchase) {
		slog.Debug("Attempting to send moynalog receipt", "invoice_type", purchase.InvoiceType, "purchase_id", utils.MaskHalfInt64(purchase.ID))

		rcExtras := &TariffPurchaseExtras{Kind: purchase.PurchaseKind, IsEarlyDowngrade: purchase.IsEarlyDowngrade}
		description := buildRubReceiptDescription(purchase.Month, purchase.ExtraHwid, rcExtras, purchase.InvoiceType)

		slog.Info("Sending receipt to moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", purchase.Amount, "description", description)

		// Чек ставится в очередь до отправки: при недоступности ФНС он не теряется,
		// а переотправляется воркером датой оплаты. Обработку покупки не прерываем.
		s.receiptQueue.Submit(ctx, purchase, description)
	} else {
		if !s.receiptQueue.Enabled() {
			slog.Debug("Moynalog receipt queue not available, skipping receipt", "purchase_id", utils.MaskHalfInt64(purchase.ID))
		} else if !invoiceUsesMoynalogReceipt(purchase) {
			slog.Debug("Invoice type skips moynalog receipt (config or currency)", "invoice_type", purchase.InvoiceType, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	}
	if purchase.TariffID != nil && *purchase.TariffID > 0 {
		customerFilesToUpdate["current_tariff_id"] = *purchase.TariffID
		now := time.Now().UTC()
		customerFilesToUpdate["subscription_period_start"] = now
		customerFilesToUpdate["subscription_period_months"] = purchase.Month
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return err
	}

	if !skipTelegramCustomerDM(customer) {
		_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: customer.TelegramID,
			Text:   s.translation.GetText(customer.Language, "subscription_activated"),
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: s.createConnectKeyboard(customer),
			},
		})
		if err != nil {
			// Покупка уже помечена оплаченной (MarkAsPaid выше), повтор вебхука её
			// не обработает. Раньше недоставленное сообщение — чаще всего бот
			// заблокирован — обрывало всё ниже: реферальный бонус, партнёрскую
			// комиссию, промокод, опыт лояльности и уведомление админу.
			slog.Warn("payment DM not delivered", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}

	if err := s.applyReferralBonus(ctx, purchase, customer); err != nil {
		// По той же причине ошибка бонуса не должна отменять партнёрку и остальное.
		slog.Error("referral bonus after payment", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
	}
	s.applyPartnerCommission(ctx, purchase, customer)
	s.clearPromoDiscountIfUsed(ctx, purchase, customer)
	s.applyLoyaltyXPAfterPayment(ctx, purchase, customer)
	slog.Info("purchase processed", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	s.tryNotifyPurchasePaid(ctx, purchase, customer, expireBefore, ptrTimeIfValid(user.ExpireAt))

	return nil
}

func (s PaymentService) applyLoyaltyXPAfterPayment(ctx context.Context, purchase *database.Purchase, customer *database.Customer) {
	if !config.LoyaltyEnabled() || purchase == nil || customer == nil {
		return
	}
	gain := loyalty.XPRubEquivalentForPurchase(purchase)
	if gain <= 0 {
		return
	}
	oldXP := customer.LoyaltyXP
	if err := s.customerRepository.IncrementLoyaltyXP(ctx, customer.ID, gain); err != nil {
		slog.Error("loyalty xp increment", "error", err, "customer_id", utils.MaskHalfInt64(customer.ID), "purchase_id", utils.MaskHalfInt64(purchase.ID))
		return
	}
	s.maybeNotifyLoyaltyLevelUp(ctx, customer, oldXP, oldXP+gain)
}

// maybeNotifyLoyaltyLevelUp отправляет поздравление при повышении уровня (sort_order текущего tier).
func (s PaymentService) maybeNotifyLoyaltyLevelUp(ctx context.Context, customer *database.Customer, oldXP, newXP int64) {
	if s.telegramBot == nil || s.loyaltyTierRepository == nil || customer == nil {
		return
	}
	lang := customer.Language
	oldProg, err := s.loyaltyTierRepository.ProgressForXP(ctx, oldXP)
	if err != nil {
		slog.Error("loyalty progress before level-up notify", "error", err)
		return
	}
	newProg, err := s.loyaltyTierRepository.ProgressForXP(ctx, newXP)
	if err != nil {
		slog.Error("loyalty progress after level-up notify", "error", err)
		return
	}
	if newProg.CurrentTier.SortOrder <= oldProg.CurrentTier.SortOrder {
		return
	}
	tm := s.translation
	var b strings.Builder
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_level_up_title"), newProg.CurrentTier.SortOrder))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_level_up_discount"), newProg.CurrentTier.DiscountPercent))
	if newProg.NextTier != nil {
		need := newProg.NextTier.XpMin - newXP
		if need < 0 {
			need = 0
		}
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf(tm.GetText(lang, "loyalty_level_up_until_next"), newProg.NextTier.SortOrder, need))
	}
	b.WriteString("\n\n")
	b.WriteString(tm.GetText(lang, "loyalty_level_up_footer"))
	if skipTelegramCustomerDM(customer) {
		return
	}
	isDisabled := true
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      b.String(),
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{tm.WithButton(lang, "loyalty_level_up_main", models.InlineKeyboardButton{CallbackData: "start"})},
			},
		},
	})
	if err != nil {
		slog.Error("loyalty level-up notify send", "error", err, "customer_id", utils.MaskHalfInt64(customer.ID))
	}
}

func (s *PaymentService) clearPromoDiscountIfUsed(ctx context.Context, purchase *database.Purchase, customer *database.Customer) {
	if s.promoService == nil || purchase == nil || customer == nil {
		return
	}
	if purchase.PromoCodeID != nil && *purchase.PromoCodeID > 0 {
		_ = s.promoService.OnSuccessfulSubscriptionDiscountPayment(ctx, purchase, customer.ID)
	}
}

func (s PaymentService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	var connectRow []models.InlineKeyboardButton
	if u := cabcfg.MiniAppEntryURL(); u != "" {
		connectRow = []models.InlineKeyboardButton{
			s.translation.WithButton(customer.Language, "connect_button", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: u},
			}),
		}
	} else {
		connectRow = []models.InlineKeyboardButton{
			s.translation.WithButton(customer.Language, "connect_button", models.InlineKeyboardButton{CallbackData: "connect"}),
		}
	}
	inlineCustomerKeyboard = append(inlineCustomerKeyboard, connectRow)

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		s.translation.WithButton(customer.Language, "back_button", models.InlineKeyboardButton{CallbackData: "start"}),
	})
	return inlineCustomerKeyboard
}

func (s PaymentService) applyReferralBonus(ctx context.Context, purchase *database.Purchase, customer *database.Customer) error {
	ctxReferee := context.Background()
	referral, err := s.referralRepository.FindByReferee(ctxReferee, customer.TelegramID)
	if err != nil || referral == nil {
		return err
	}
	if referral.ReferrerID == customer.TelegramID {
		slog.Warn("Skip referral bonus: self-referral detected",
			"referrer_tg_id", utils.MaskHalfInt64(referral.ReferrerID),
			"referee_tg_id", utils.MaskHalfInt64(customer.TelegramID),
		)
		return nil
	}

	mode := config.ReferralMode()
	if mode == "progressive" {
		return s.applyProgressiveReferralBonus(ctxReferee, referral, purchase, customer)
	}
	return s.applyDefaultReferralBonus(ctxReferee, referral, purchase)
}

func (s PaymentService) applyDefaultReferralBonus(ctx context.Context, referral *database.Referral, purchase *database.Purchase) error {
	if referral.BonusGranted {
		return nil
	}

	referrerCustomer, err := s.customerRepository.FindByTelegramId(ctx, referral.ReferrerID)
	if err != nil || referrerCustomer == nil {
		return err
	}

	// Разовый бонус режима default по длине периода не масштабируется: он
	// начисляется один раз за само появление платящего реферала, а не за каждую
	// его оплату. Прогрессия — предмет режима progressive.
	bonusDays := config.GetReferralDays()
	if err := s.grantReferralDays(ctx, referrerCustomer, bonusDays); err != nil {
		return err
	}
	s.recordReferralBonus(ctx, referral, purchase, referrerCustomer, bonusDays, 0, 0, database.ReferralBonusKindDefault)
	if err := s.referralRepository.MarkBonusGranted(ctx, referral.ID); err != nil {
		return err
	}

	slog.Info("Granted referral bonus", "customer_id", utils.MaskHalfInt64(referrerCustomer.ID))
	err = s.sendReferralBonusMessage(ctx, referrerCustomer, bonusDays)
	return err
}

func (s PaymentService) applyProgressiveReferralBonus(ctx context.Context, referral *database.Referral, purchase *database.Purchase, customer *database.Customer) error {
	if purchase.Month < 1 {
		return nil
	}

	paidCount, err := s.purchaseRepository.CountPaidSubscriptionsByCustomer(ctx, customer.ID)
	if err != nil {
		return err
	}

	referrerCustomer, err := s.customerRepository.FindByTelegramId(ctx, referral.ReferrerID)
	if err != nil || referrerCustomer == nil {
		return err
	}

	// Вся политика начисления живёт в config.ReferralBonusForPayment: сколько
	// получает пригласивший (помесячно, с повышенной ставкой за первый месяц) и
	// сколько приглашённый (фиксированный приветственный бонус).
	months := purchase.Month
	isFirstPayment := paidCount == 1
	bonus := config.ReferralBonusForPayment(months, isFirstPayment)

	referrerKind := database.ReferralBonusKindRepeatReferrer
	if isFirstPayment {
		referrerKind = database.ReferralBonusKindFirstReferrer

		// Приглашённому начисляем первым: если продление ему не удастся,
		// пригласивший не получит своё за оплату, которая до реферала не доехала.
		if err := s.grantReferralDays(ctx, customer, bonus.RefereeDays); err != nil {
			return err
		}
		// Помесячных ставок нет: бонус приглашённого плоский по замыслу.
		s.recordReferralBonus(ctx, referral, purchase, customer, bonus.RefereeDays, 0, 0, database.ReferralBonusKindFirstReferee)
		if err := s.sendReferralFirstBonusMessage(ctx, customer, bonus.RefereeDays); err != nil {
			return err
		}
	}

	bonusDays := bonus.ReferrerDays
	if err := s.grantReferralDays(ctx, referrerCustomer, bonusDays); err != nil {
		return err
	}
	s.recordReferralBonus(ctx, referral, purchase, referrerCustomer, bonusDays, bonus.FirstMonthDays, bonus.PerMonthDays, referrerKind)
	if !referral.BonusGranted {
		if err := s.referralRepository.MarkBonusGranted(ctx, referral.ID); err != nil {
			return err
		}
	}

	slog.Info("Granted referral bonus",
		"customer_id", utils.MaskHalfInt64(referrerCustomer.ID),
		"days", bonusDays, "months", months,
		"first_month_days", bonus.FirstMonthDays, "per_month_days", bonus.PerMonthDays,
		"kind", referrerKind)
	if bonusDays <= 0 {
		return nil
	}
	err = s.sendReferralBonusMessage(ctx, referrerCustomer, bonusDays)
	return err
}

// recordReferralBonus фиксирует в журнале уже выданные дни.
//
// Вызывается ТОЛЬКО после успешного grantReferralDays: журнал описывает
// выданное, а не задуманное, иначе статистика начнёт считать дни, до подписки
// не доехавшие.
//
// Ошибка записи намеренно не роняет обработку платежа. Подписка к этому моменту
// уже продлена во внешней панели, откатить это нельзя, и падение здесь привело
// бы к повторной обработке платежа — то есть к ПОВТОРНОМУ продлению. Потерянная
// строка статистики — несопоставимо меньшая беда, чем задвоенные дни, поэтому
// сбой уходит в лог и на этом всё.
func (s PaymentService) recordReferralBonus(
	ctx context.Context,
	referral *database.Referral,
	purchase *database.Purchase,
	recipient *database.Customer,
	days int,
	firstMonthDays int,
	perMonthDays int,
	kind string,
) {
	if s.referralLedger == nil || days <= 0 || referral == nil || recipient == nil {
		return
	}
	entry := database.ReferralBonusEntry{
		ReferralID:          &referral.ID,
		ReferrerTelegramID:  referral.ReferrerID,
		RefereeTelegramID:   referral.RefereeID,
		RecipientTelegramID: recipient.TelegramID,
		RecipientCustomerID: &recipient.ID,
		Days:                days,
		FirstMonthDays:      firstMonthDays,
		PerMonthDays:        perMonthDays,
		Kind:                kind,
		CreatedAt:           time.Now().UTC(),
	}
	if purchase != nil {
		entry.PurchaseID = &purchase.ID
		entry.Months = purchase.Month
	}
	if err := s.referralLedger.Insert(ctx, entry); err != nil {
		slog.Error("referral bonus ledger write failed",
			"error", err,
			"kind", kind,
			"days", days,
			"recipient_id", utils.MaskHalfInt64(recipient.ID),
		)
	}
}

func (s PaymentService) grantReferralDays(ctx context.Context, customer *database.Customer, days int) error {
	if days <= 0 {
		return nil
	}
	rwCtx := s.withRemnawavePanelUsername(ctx, customer)
	user, err := s.remnawaveClient.ExtendSubscriptionByDaysPreserveSquads(rwCtx, customer.ID, customer.TelegramID, days)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	}
	return s.customerRepository.UpdateFields(ctx, customer.ID, updates)
}

func (s PaymentService) sendReferralBonusMessage(ctx context.Context, customer *database.Customer, days int) error {
	if days <= 0 {
		return nil
	}
	if skipTelegramCustomerDM(customer) {
		return nil
	}
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      fmt.Sprintf(s.translation.GetText(customer.Language, "referral_bonus_granted"), days),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createReferralBonusKeyboard(customer),
		},
	})
	return err
}

func (s PaymentService) sendReferralFirstBonusMessage(ctx context.Context, customer *database.Customer, days int) error {
	if days <= 0 {
		return nil
	}
	if skipTelegramCustomerDM(customer) {
		return nil
	}
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      fmt.Sprintf(s.translation.GetText(customer.Language, "referral_first_bonus_granted"), days),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createReferralBonusKeyboard(customer),
		},
	})
	return err
}

func (s PaymentService) createReferralBonusKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	lang := customer.Language
	connectBtn := models.InlineKeyboardButton{CallbackData: "connect"}
	if u := cabcfg.MiniAppEntryURL(); u != "" {
		connectBtn = models.InlineKeyboardButton{WebApp: &models.WebAppInfo{URL: u}}
	}
	return [][]models.InlineKeyboardButton{
		{
			s.translation.WithButton(lang, "referral_button", models.InlineKeyboardButton{CallbackData: "referral"}),
		},
		{
			s.translation.WithButton(lang, "connect_button", connectBtn),
		},
	}
}

func applyTariffPurchaseExtras(pur *database.Purchase, extras *TariffPurchaseExtras) {
	if extras == nil {
		return
	}
	if extras.Kind != "" {
		pur.PurchaseKind = extras.Kind
	}
	pur.IsEarlyDowngrade = extras.IsEarlyDowngrade
}

// ensurePurchaseKindExtraHwidOnly помечает покупку «только доп. HWID» (month==0, extra>0),
// иначе в БД через Create подставлялся subscription и кабинет/отчёты путали с подпиской.
func ensurePurchaseKindExtraHwidOnly(pur *database.Purchase) {
	if pur == nil {
		return
	}
	if pur.Month > 0 {
		return
	}
	if pur.ExtraHwid > 0 {
		pur.PurchaseKind = database.PurchaseKindExtraHwid
	}
}

// invoiceUsesMoynalogReceipt — учитывает MOYNALOG_RECEIPT_FOR и валюту (в «Мой налог» только рублёвый эквивалент из purchase.amount).
func invoiceUsesMoynalogReceipt(p *database.Purchase) bool {
	if p == nil || !purchaseCurrencyRubForMoynalog(p) {
		return false
	}
	switch p.InvoiceType {
	case database.InvoiceTypeYookasa:
		return config.MoynalogReceiptForYookasa()
	case database.InvoiceTypeCrypto:
		return config.MoynalogReceiptForCrypto()
	default:
		if database.InvoiceTypeIsPlatega(p.InvoiceType) {
			return config.MoynalogReceiptForPlatega()
		}
	}
	return false
}

func purchaseCurrencyRubForMoynalog(p *database.Purchase) bool {
	c := strings.ToUpper(strings.TrimSpace(p.Currency))
	return c == "" || c == "RUB" || c == "RUR"
}

func rubMonthWord(months int) string {
	switch months {
	case 1:
		return "месяц"
	case 2, 3, 4:
		return "месяца"
	default:
		return "месяцев"
	}
}

// buildRubReceiptDescription одна строка для описания в ЮKassa/Platega и комментария дохода в «Мой налог» (рус.).
// invoiceType добавляет нейтральный суффикс канала для Platega (без слов про «крипто» — в т.ч. для plt_crypto и счетов, уходящих в «Мой налог»).
func buildRubReceiptDescription(months, extraHwid int, extras *TariffPurchaseExtras, invoiceType database.InvoiceType) string {
	ms := rubMonthWord(months)
	var base string
	switch {
	case months > 0 && extraHwid > 0:
		base = fmt.Sprintf("Подписка на %d %s + %d устр.", months, ms, extraHwid)
	case extraHwid > 0 && months <= 0:
		base = fmt.Sprintf("Оплата дополнительного устройства +%d", extraHwid)
	case extras != nil && extras.Kind == database.PurchaseKindTariffUpgrade && months > 0:
		base = fmt.Sprintf("Улучшение тарифа (полная оплата периода на %d %s + пересчёт остатка в дни)", months, ms)
	case extras != nil && extras.IsEarlyDowngrade && months > 0:
		base = fmt.Sprintf("Понижение тарифа (полная оплата периода на %d %s + пересчёт остатка в дни)", months, ms)
	case months > 0:
		base = fmt.Sprintf("Подписка на %d %s", months, ms)
	default:
		base = "Оплата"
	}
	if database.InvoiceTypeIsPlategaCryptoOrCryptopay(invoiceType) {
		return base
	}
	if suf := invoiceDescriptionPlategaSuffix(invoiceType); suf != "" {
		return base + suf
	}
	return base
}

// invoiceDescriptionPlategaSuffix — уточнение способа Platega без упоминания криптовалюты (в т.ч. в текстах для «Мой налог»).
func invoiceDescriptionPlategaSuffix(t database.InvoiceType) string {
	switch t {
	case database.InvoiceTypePlategaSBP:
		return " — Platega, СБП"
	case database.InvoiceTypePlategaCards:
		return " — Platega, карты"
	case database.InvoiceTypePlategaAcquiring:
		return " — Platega, эквайринг"
	case database.InvoiceTypePlategaWorldwide:
		return " — Platega, worldwide"
	case database.InvoiceTypePlategaCrypto:
		return ""
	default:
		return ""
	}
}

func (s PaymentService) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, months, 0, customer, meta, tariffID, extras)
	case database.InvoiceTypeYookasa:
		return s.createYookasaInvoice(ctx, amount, months, 0, customer, meta, tariffID, extras)
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		return s.createPlategaInvoice(ctx, amount, months, 0, customer, meta, tariffID, extras, invoiceType)
	case database.InvoiceTypeHeleket:
		return s.createHeleketInvoice(ctx, amount, months, 0, customer, meta, tariffID, extras)
	case database.InvoiceTypeTelegram:
		return s.createTelegramInvoice(ctx, amount, months, 0, customer, meta, tariffID, extras)
	case database.InvoiceTypeTribute:
		return s.createTributeInvoice(ctx, amount, months, 0, customer, nil, tariffID, extras)
	default:
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

// ErrHwidLimitPinned — лимит устройств закреплён админом: доп. устройства в
// счёт не берём, оплата ничего бы не изменила. Интерфейсы кнопку докупки таким
// клиентам не показывают; это последняя линия на случай подделанного запроса.
var ErrHwidLimitPinned = errors.New("hwid limit is pinned by admin")

func (s PaymentService) CreatePurchaseWithExtra(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, invoiceType database.InvoiceType, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	if extraHwid < 0 {
		return "", 0, fmt.Errorf("invalid extra hwid: %d", extraHwid)
	}
	if !config.HwidExtraDevicesEnabled() && extraHwid > 0 {
		return "", 0, fmt.Errorf("extra hwid purchases are disabled")
	}
	if extraHwid > 0 && customer != nil && customer.HwidLimitOverride != nil {
		return "", 0, ErrHwidLimitPinned
	}
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, months, extraHwid, customer, meta, tariffID, extras)
	case database.InvoiceTypeYookasa:
		return s.createYookasaInvoice(ctx, amount, months, extraHwid, customer, meta, tariffID, extras)
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		return s.createPlategaInvoice(ctx, amount, months, extraHwid, customer, meta, tariffID, extras, invoiceType)
	case database.InvoiceTypeHeleket:
		return s.createHeleketInvoice(ctx, amount, months, extraHwid, customer, meta, tariffID, extras)
	case database.InvoiceTypeTelegram:
		return s.createTelegramInvoice(ctx, amount, months, extraHwid, customer, meta, tariffID, extras)
	case database.InvoiceTypeTribute:
		return s.createTributeInvoice(ctx, amount, months, extraHwid, customer, nil, tariffID, extras)
	default:
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

func (s PaymentService) CreateHwidPurchase(ctx context.Context, amount float64, extraHwid int, customer *database.Customer, invoiceType database.InvoiceType, meta *PromoMeta) (url string, purchaseId int64, err error) {
	if extraHwid <= 0 {
		return "", 0, fmt.Errorf("invalid extra hwid: %d", extraHwid)
	}
	if !config.HwidExtraDevicesEnabled() {
		return "", 0, fmt.Errorf("extra hwid purchases are disabled")
	}
	if customer != nil && customer.HwidLimitOverride != nil {
		return "", 0, ErrHwidLimitPinned
	}
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, 0, extraHwid, customer, meta, nil, nil)
	case database.InvoiceTypeYookasa:
		return s.createYookasaInvoice(ctx, amount, 0, extraHwid, customer, meta, nil, nil)
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		return s.createPlategaInvoice(ctx, amount, 0, extraHwid, customer, meta, nil, nil, invoiceType)
	case database.InvoiceTypeHeleket:
		return s.createHeleketInvoice(ctx, amount, 0, extraHwid, customer, meta, nil, nil)
	case database.InvoiceTypeTelegram:
		return s.createTelegramInvoice(ctx, amount, 0, extraHwid, customer, meta, nil, nil)
	case database.InvoiceTypeTribute:
		return s.createTributeInvoice(ctx, amount, 0, extraHwid, customer, nil, nil, nil)
	default:
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

var ErrCustomerNotFound = errors.New("customer not found")

func (s PaymentService) CancelTributePurchase(ctx context.Context, telegramId int64) error {
	slog.Info("Canceling tribute purchase", "telegram_id", utils.MaskHalfInt64(telegramId))
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}
	tributePurchase, err := s.purchaseRepository.FindByCustomerIDAndInvoiceTypeLast(ctx, customer.ID, database.InvoiceTypeTribute)
	if err != nil {
		return err
	}
	if tributePurchase == nil {
		return errors.New("tribute purchase not found")
	}
	expireAt, err := s.remnawaveClient.DecreaseSubscription(ctx, telegramId, config.TrafficLimit(), -tributePurchase.Month*config.DaysInMonth())
	if err != nil {
		return err
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"expire_at": expireAt,
	}); err != nil {
		return err
	}

	if err := s.purchaseRepository.UpdateFields(ctx, tributePurchase.ID, map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}); err != nil {
		return err
	}
	tributePurchase.Status = database.PurchaseStatusCancel
	s.tryNotifyPurchaseCancel(ctx, tributePurchase, customer)

	if !utils.IsSyntheticTelegramID(telegramId) && !customer.IsWebOnly {
		_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    telegramId,
			ParseMode: models.ParseModeHTML,
			Text:      s.translation.GetText(customer.Language, "tribute_cancelled"),
		})
		if err != nil {
			slog.Error("Error sending message about tribute cancelled", "error", err, "telegram_id", utils.MaskHalfInt64(telegramId))
		}
	}
	slog.Info("Canceled tribute purchase", "purchase_id", utils.MaskHalfInt64(tributePurchase.ID), "telegram_id", utils.MaskHalfInt64(telegramId))
	return nil
}

func (s PaymentService) createCryptoInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	pur := &database.Purchase{
		InvoiceType: database.InvoiceTypeCrypto,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	if meta != nil {
		pur.PromoCodeID = meta.PromoCodeID
		pur.DiscountPercentApplied = meta.DiscountPercentApplied
	}
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	username, _ := ctx.Value(remnawave.CtxKeyUsername).(string)
	description := buildRubReceiptDescription(months, extraHwid, extras, database.InvoiceTypeCrypto)
	paidBtnURL := cryptopay.PaidBtnURLFromCtx(ctx)
	if paidBtnURL == "" {
		paidBtnURL = config.BotURL()
	}
	invoice, err := s.cryptoPayClient.CreateInvoice(&cryptopay.InvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "RUB",
		Amount:         fmt.Sprintf("%d", int(amount)),
		AcceptedAssets: "USDT",
		Payload:        fmt.Sprintf("purchaseId=%d&username=%s", purchaseId, username),
		Description:    description,
		PaidBtnName:    "callback",
		PaidBtnUrl:     paidBtnURL,
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"crypto_invoice_url": invoice.BotInvoiceUrl,
		"crypto_invoice_id":  invoice.InvoiceID,
		"status":             database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.BotInvoiceUrl, purchaseId, nil
}

func (s PaymentService) createYookasaInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	pur := &database.Purchase{
		InvoiceType: database.InvoiceTypeYookasa,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	if meta != nil {
		pur.PromoCodeID = meta.PromoCodeID
		pur.DiscountPercentApplied = meta.DiscountPercentApplied
	}
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	invDesc := buildRubReceiptDescription(months, extraHwid, extras, database.InvoiceTypeYookasa)
	invoice, err := s.yookasaClient.CreateInvoice(ctx, int(amount), invDesc, customer.ID, purchaseId)
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"yookasa_url": invoice.Confirmation.ConfirmationURL,
		"yookasa_id":  invoice.ID,
		"status":      database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.Confirmation.ConfirmationURL, purchaseId, nil
}

func (s PaymentService) createPlategaInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras, invoiceType database.InvoiceType) (url string, purchaseId int64, err error) {
	if s.plategaClient == nil || !s.plategaClient.IsConfigured() {
		return "", 0, fmt.Errorf("platega client not configured")
	}
	provider, err := platega.ProviderFor(s.plategaClient, invoiceType)
	if err != nil {
		return "", 0, err
	}
	pur := &database.Purchase{
		InvoiceType: invoiceType,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	if meta != nil {
		pur.PromoCodeID = meta.PromoCodeID
		pur.DiscountPercentApplied = meta.DiscountPercentApplied
	}
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	ret := platega.ReturnURLFromCtx(ctx)
	if ret == "" {
		ret = config.BotURL()
	}
	desc := buildRubReceiptDescription(months, extraHwid, extras, invoiceType)
	username := remnawave.UsernameFromCtx(ctx)
	redirectURL, transactionID, err := provider.CreateInvoice(ctx, purchaseId, amount, "RUB", desc, ret, username)
	if err != nil {
		slog.Error("Error creating platega invoice", "error", err, "invoice_type", invoiceType)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"platega_id":  transactionID,
		"platega_url": redirectURL,
		"status":      database.PurchaseStatusPending,
	}
	if err := s.purchaseRepository.UpdateFields(ctx, purchaseId, updates); err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}
	return redirectURL, purchaseId, nil
}

// createHeleketInvoice — счёт в Heleket («Другая крипта»).
//
// Конкретную монету и сеть выбирает плательщик на странице Heleket; мы задаём
// только фиатный номинал. order_id счёта — id покупки: он же делает повторный
// запрос идемпотентным и по нему вебхук находит покупку обратно.
func (s PaymentService) createHeleketInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	if s.heleketClient == nil || !s.heleketClient.IsConfigured() {
		return "", 0, fmt.Errorf("heleket client not configured")
	}
	pur := &database.Purchase{
		InvoiceType: database.InvoiceTypeHeleket,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    s.heleketClient.Currency(),
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	if meta != nil {
		pur.PromoCodeID = meta.PromoCodeID
		pur.DiscountPercentApplied = meta.DiscountPercentApplied
	}
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	ret := heleket.ReturnURLFromCtx(ctx)
	if ret == "" {
		ret = config.BotURL()
	}
	desc := buildRubReceiptDescription(months, extraHwid, extras, database.InvoiceTypeHeleket)
	payment, err := s.heleketClient.CreatePayment(
		ctx,
		heleket.FormatOrderID(s.heleketClient.OrderPrefix(), purchaseId),
		heleket.FormatAmount(amount),
		desc,
		ret,
	)
	if err != nil {
		slog.Error("Error creating heleket invoice", "error", err, "purchase_id", purchaseId)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"heleket_id":  payment.UUID,
		"heleket_url": payment.URL,
		"status":      database.PurchaseStatusPending,
	}
	if err := s.purchaseRepository.UpdateFields(ctx, purchaseId, updates); err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}
	return payment.URL, purchaseId, nil
}

func (s PaymentService) createTelegramInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, meta *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	pur := &database.Purchase{
		InvoiceType: database.InvoiceTypeTelegram,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "STARS",
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	if meta != nil {
		pur.PromoCodeID = meta.PromoCodeID
		pur.DiscountPercentApplied = meta.DiscountPercentApplied
	}
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		// Раньше здесь возвращался nil: вызывающий получал пустую ссылку без ошибки.
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	username, _ := ctx.Value(remnawave.CtxKeyUsername).(string)
	invoiceUrl, err := s.telegramBot.CreateInvoiceLink(ctx, &bot.CreateInvoiceLinkParams{
		Title:    s.translation.GetText(customer.Language, "invoice_title"),
		Currency: "XTR",
		Prices: []models.LabeledPrice{
			{
				Label:  s.translation.GetText(customer.Language, "invoice_label"),
				Amount: int(amount),
			},
		},
		Description: s.translation.GetText(customer.Language, "invoice_description"),
		Payload:     fmt.Sprintf("%d&%s", purchaseId, username),
	})
	if err != nil {
		// Без проверки покупка уходила в pending с пустой ссылкой.
		slog.Error("Error creating telegram invoice link", "error", err, "purchaseId", purchaseId)
		if _, cerr := s.purchaseRepository.CancelIfOpen(ctx, purchaseId); cerr != nil {
			slog.Error("Error canceling purchase after invoice link failure", "error", cerr, "purchaseId", purchaseId)
		}
		return "", 0, err
	}

	updates := map[string]interface{}{
		"status":             database.PurchaseStatusPending,
		"crypto_invoice_url": invoiceUrl,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoiceUrl, purchaseId, nil
}

// ErrTrialAlreadyUsed — пробный период уже выдан (или выдаётся параллельным запросом).
var ErrTrialAlreadyUsed = errors.New("trial already used")

func (s PaymentService) ActivateTrial(ctx context.Context, telegramId int64) (string, error) {
	if config.TrialDays() == 0 {
		return "", nil
	}
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	// Проверка «пробный ещё не выдан» — под блокировкой по клиенту и по
	// перечитанной строке: раньше её делали вызывающие без блокировки, и
	// несколько быстрых нажатий складывали пробные дни.
	unlock, err := s.customerRepository.LockTrial(ctx, customer.ID)
	if err != nil {
		if errors.Is(err, database.ErrTrialLocked) {
			return "", ErrTrialAlreadyUsed
		}
		return "", err
	}
	defer unlock()
	customer, err = s.customerRepository.FindById(ctx, customer.ID)
	if err != nil {
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	if customer.SubscriptionLink != nil && strings.TrimSpace(*customer.SubscriptionLink) != "" {
		return "", ErrTrialAlreadyUsed
	}
	rwCtx := s.withRemnawavePanelUsername(ctx, customer)
	rwCtx = s.ctxWithTelegramUsernameIfMissing(rwCtx, customer)
	user, err := s.remnawaveClient.CreateOrUpdateUser(rwCtx, customer.ID, telegramId, config.TrialTrafficLimit(), config.TrialDays(), true)
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	now := time.Now().UTC()
	customerFilesToUpdate := map[string]interface{}{
		"subscription_link":         user.SubscriptionUrl,
		"expire_at":                 user.ExpireAt,
		"subscription_period_start": now,
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return "", err
	}

	return user.SubscriptionUrl, nil

}

func (s PaymentService) CancelYookassaPayment(purchaseId int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	cancelled, err := s.purchaseRepository.CancelIfOpen(ctx, purchaseId)
	if err != nil {
		return err
	}
	if !cancelled {
		slog.Info("yookassa: skip cancel, purchase is already finalized", "purchase_id", purchaseId)
		return nil
	}

	purchase.Status = database.PurchaseStatusCancel
	cust, ferr := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if ferr != nil {
		slog.Warn("payments notify cancel: load customer", "error", ferr)
	}
	s.tryNotifyPurchaseCancel(ctx, purchase, cust)

	return nil
}

func (s PaymentService) CancelPlategaPayment(purchaseId int64) error {
	return s.cancelPurchaseAndNotify(purchaseId)
}

// CancelHeleketPayment закрывает счёт Heleket. Отдельный метод, а не прямая
// запись статуса из пакета heleket: отмену нужно показать и пользователю, и в
// группе платежей — этим занимается cancelPurchaseAndNotify.
func (s PaymentService) CancelHeleketPayment(purchaseId int64) error {
	return s.cancelPurchaseAndNotify(purchaseId)
}

// cancelPurchaseAndNotify переводит покупку в cancel и рассылает уведомления.
// Отменяется только new/pending: вебхук и поллинг работают параллельно, и
// поллер может прийти с отменой по уже оплаченной покупке — тогда подписка
// осталась бы выданной, а покупка ушла бы в cancel вместе с выручкой из статистики.
func (s PaymentService) cancelPurchaseAndNotify(purchaseId int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with id %s not found", utils.MaskHalfInt64(purchaseId))
	}
	cancelled, err := s.purchaseRepository.CancelIfOpen(ctx, purchaseId)
	if err != nil {
		return err
	}
	if !cancelled {
		slog.Info("skip cancel, purchase is already finalized", "purchase_id", purchaseId)
		return nil
	}
	purchase.Status = database.PurchaseStatusCancel
	cust, ferr := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if ferr != nil {
		slog.Warn("payments notify cancel: load customer", "error", ferr)
	}
	s.tryNotifyPurchaseCancel(ctx, purchase, cust)
	return nil
}

func (s PaymentService) createTributeInvoice(ctx context.Context, amount float64, months int, extraHwid int, customer *database.Customer, _ *PromoMeta, tariffID *int64, extras *TariffPurchaseExtras) (url string, purchaseId int64, err error) {
	pur := &database.Purchase{
		InvoiceType: database.InvoiceTypeTribute,
		Status:      database.PurchaseStatusPending,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
		ExtraHwid:   extraHwid,
		TariffID:    tariffID,
	}
	applyTariffPurchaseExtras(pur, extras)
	ensurePurchaseKindExtraHwidOnly(pur)
	purchaseId, err = s.purchaseRepository.Create(ctx, pur)
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	return "", purchaseId, nil
}
