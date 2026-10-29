package promo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"
)

// Service applies promo codes and manages pending discounts.
type Service struct {
	PromoRepo    *database.PromoRepository
	CustomerRepo *database.CustomerRepository
	PurchaseRepo *database.PurchaseRepository
	Remnawave    *remnawave.Client
}

func NewService(promo *database.PromoRepository, customers *database.CustomerRepository, purchases *database.PurchaseRepository, rw *remnawave.Client) *Service {
	return &Service{
		PromoRepo:    promo,
		CustomerRepo: customers,
		PurchaseRepo: purchases,
		Remnawave:    rw,
	}
}

// NormalizeCode uppercases and trims (DB stores uppercase).
func NormalizeCode(s string) string {
	return strings.TrimSpace(strings.ToUpper(s))
}

// ValidCodePattern: Latin letters and digits only (admin wizard).
func ValidCodePattern(s string) bool {
	s = NormalizeCode(s)
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			if r > unicode.MaxASCII || (r < 'A' || r > 'Z') {
				return false
			}
			continue
		}
		if unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

// Activate validates and applies a promo code for the customer. Customer must be non-nil.
// One-time discount: pending row is cleared after the next successful subscription payment (see payment flow).
func (s *Service) Activate(ctx context.Context, telegramID int64, username string, customer *database.Customer, rawCode string) (*ActivateResult, error) {
	if customer == nil {
		return nil, database.ValidationErrorf("no customer")
	}
	code := NormalizeCode(rawCode)
	if code == "" {
		return nil, database.ValidationErrorf("empty code")
	}

	tx, err := s.PromoRepo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := s.PromoRepo.FindByCodeForUpdate(ctx, tx, code)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, database.ValidationErrorf("not found")
	}

	if err := s.validatePromoRow(ctx, p, customer); err != nil {
		return nil, err
	}

	if err := s.PromoRepo.InsertRedemption(ctx, tx, p.ID, customer.ID); err != nil {
		return nil, err
	}
	if err := s.PromoRepo.IncrementUses(ctx, tx, p.ID); err != nil {
		return nil, err
	}

	if p.Type == database.PromoTypeDiscount {
		var exp *time.Time
		untilFirst := false
		isMulti := p.DiscountMaxSubscriptionPaymentsPerCustomer != 1
		if p.DiscountTTLHours != nil && *p.DiscountTTLHours == 0 {
			if isMulti {
				// Многоразовая: 0 ч = бессрочно по времени (лимит только числом оплат).
				exp = nil
				untilFirst = false
			} else {
				// Одноразовая: 0 ч = до первой оплаты подписки.
				untilFirst = true
			}
		} else if p.DiscountTTLHours != nil && *p.DiscountTTLHours > 0 {
			t := time.Now().UTC().Add(time.Duration(*p.DiscountTTLHours) * time.Hour)
			exp = &t
		}
		var remaining int
		if p.DiscountMaxSubscriptionPaymentsPerCustomer == 0 {
			remaining = database.PendingDiscountUnlimitedPayments
		} else {
			remaining = p.DiscountMaxSubscriptionPaymentsPerCustomer
		}
		if err := s.PromoRepo.UpsertPendingDiscount(ctx, tx, customer.ID, p.ID, *p.DiscountPercent, exp, untilFirst, remaining); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if p.Type == database.PromoTypeDiscount {
		slog.Info("promo activated", "promo_id", p.ID, "type", p.Type, "customer_id", utils.MaskHalfInt64(customer.ID))
		return &ActivateResult{Type: database.PromoTypeDiscount, DiscountPercent: *p.DiscountPercent}, nil
	}

	ctxUser := context.WithValue(ctx, remnawave.CtxKeyUsername, username)
	res, err := s.applyEffect(ctxUser, telegramID, customer, p)
	if err != nil {
		s.compensate(ctx, p.ID, customer.ID)
		slog.Error("promo effect failed, compensated", "error", err, "promo_id", p.ID, "customer_id", utils.MaskHalfInt64(customer.ID))
		return nil, err
	}

	slog.Info("promo activated", "promo_id", p.ID, "type", p.Type, "customer_id", utils.MaskHalfInt64(customer.ID))
	return res, nil
}

func (s *Service) validatePromoRow(ctx context.Context, p *database.PromoCode, customer *database.Customer) error {
	if !p.Active {
		return database.ValidationErrorf("inactive")
	}
	if p.ValidUntil != nil && time.Now().After(*p.ValidUntil) {
		return database.ValidationErrorf("expired")
	}
	max := p.MaxUses
	unlimited := max == nil || *max == 0
	if !unlimited && p.UsesCount >= *max {
		return database.ValidationErrorf("max uses")
	}
	has, err := s.PromoRepo.HasRedemption(ctx, p.ID, customer.ID)
	if err != nil {
		return err
	}
	if has {
		return database.ValidationErrorf("already redeemed")
	}
	// RequireCustomerInDB: строка customer к этому моменту уже есть — отдельная
	// проверка не нужна.
	if p.FirstPurchaseOnly {
		n, err := s.PurchaseRepo.CountPaidSubscriptionsByCustomer(ctx, customer.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return database.ValidationErrorf("first purchase only")
		}
	}

	switch p.Type {
	case database.PromoTypeSubscriptionDays:
		if p.SubscriptionDays == nil || *p.SubscriptionDays <= 0 {
			return database.ValidationErrorf("bad subscription_days")
		}
		if p.TariffID != nil && *p.TariffID > 0 {
			if customer.CurrentTariffID == nil {
				return database.ValidationErrorf("tariff mismatch")
			}
			if *customer.CurrentTariffID != *p.TariffID {
				return database.ValidationErrorf("tariff mismatch")
			}
		}
	case database.PromoTypeTrial:
		if p.TrialDays == nil || *p.TrialDays <= 0 {
			return database.ValidationErrorf("bad trial_days")
		}
	case database.PromoTypeExtraHwid:
		if p.ExtraHwidDelta == nil || *p.ExtraHwidDelta <= 0 {
			return database.ValidationErrorf("bad extra_hwid")
		}
		if customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now()) {
			return database.ValidationErrorf("extra_hwid needs active sub")
		}
		// Лимит закреплён админом: устройства из промокода ничего бы не
		// добавили, а код сгорел бы. Проверка до записи активации — код цел.
		if customer.HwidLimitOverride != nil {
			return database.ValidationErrorf("hwid pinned")
		}
	case database.PromoTypeDiscount:
		if p.DiscountPercent == nil || *p.DiscountPercent < 1 || *p.DiscountPercent > 100 {
			return database.ValidationErrorf("bad discount percent")
		}
		// TTL не обязателен: пустое поле = скидка без срока по времени.
		// Это совпадает с соглашением остальной формы промокода, где пусто
		// означает «безлимит», и с тем, как nil уже обрабатывается ниже при
		// создании pending-скидки (exp остаётся nil). Раньше здесь стоял отказ,
		// из-за чего админка позволяла сохранить промокод, который потом
		// никогда не активировался — с неинформативным "promo_validation_failed".
		// Отрицательный TTL — это уже ошибка ввода, а не «без срока».
		if p.DiscountTTLHours != nil && *p.DiscountTTLHours < 0 {
			return database.ValidationErrorf("bad discount ttl")
		}
		pd, err := s.PromoRepo.GetPendingDiscountByCustomerID(ctx, customer.ID)
		if err != nil {
			return err
		}
		if pd != nil {
			return database.ValidationErrorf("pending discount")
		}
	default:
		return database.ValidationErrorf("unknown type")
	}
	return nil
}

func (s *Service) compensate(ctx context.Context, promoID, customerID int64) {
	tx, err := s.PromoRepo.Pool().Begin(ctx)
	if err != nil {
		slog.Error("promo compensate begin failed", "error", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.PromoRepo.DeleteRedemption(ctx, tx, promoID, customerID); err != nil {
		slog.Error("promo compensate delete redemption", "error", err)
		return
	}
	if err := s.PromoRepo.DecrementUses(ctx, tx, promoID); err != nil {
		slog.Error("promo compensate decrement", "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("promo compensate commit", "error", err)
	}
}

func (s *Service) applyEffect(ctx context.Context, telegramID int64, customer *database.Customer, p *database.PromoCode) (*ActivateResult, error) {
	switch p.Type {
	case database.PromoTypeDiscount:
		return nil, nil
	case database.PromoTypeSubscriptionDays:
		if err := s.applySubscriptionDays(ctx, telegramID, customer, *p.SubscriptionDays); err != nil {
			return nil, err
		}
		return &ActivateResult{Type: database.PromoTypeSubscriptionDays, SubscriptionDays: *p.SubscriptionDays}, nil
	case database.PromoTypeTrial:
		skipped, err := s.applyTrial(ctx, telegramID, customer, *p.TrialDays, p.AllowTrialWithoutPayment)
		if err != nil {
			return nil, err
		}
		return &ActivateResult{Type: database.PromoTypeTrial, TrialDays: *p.TrialDays, TrialSkippedActiveSub: skipped}, nil
	case database.PromoTypeExtraHwid:
		if err := s.applyExtraHwid(ctx, telegramID, customer, *p.ExtraHwidDelta); err != nil {
			return nil, err
		}
		return &ActivateResult{Type: database.PromoTypeExtraHwid, ExtraHwidDelta: *p.ExtraHwidDelta}, nil
	default:
		return nil, fmt.Errorf("unknown promo type %s", p.Type)
	}
}

func (s *Service) applySubscriptionDays(ctx context.Context, telegramID int64, customer *database.Customer, days int) error {
	hasActive := customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())
	var user *remnawave.User
	var err error
	if hasActive {
		user, err = s.Remnawave.ExtendSubscriptionByDaysPreserveSquads(ctx, customer.ID, telegramID, days)
	} else {
		user, err = s.Remnawave.CreateOrUpdateUserFromNow(ctx, customer.ID, telegramID, config.TrafficLimit(), days, false)
	}
	if err != nil {
		return err
	}
	return s.CustomerRepo.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	})
}

// applyTrial returns skipped=true if user already has an active subscription (no Remnawave change; promo still redeemed).
func (s *Service) applyTrial(ctx context.Context, telegramID int64, customer *database.Customer, days int, allowTrialWithoutPayment bool) (skipped bool, err error) {
	hasActiveSub := customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())
	if hasActiveSub {
		return true, nil
	}
	if !allowTrialWithoutPayment {
		if customer.SubscriptionLink != nil {
			return false, database.ValidationErrorf("trial not allowed")
		}
	}
	user, err := s.Remnawave.CreateOrUpdateUser(ctx, customer.ID, telegramID, config.TrialTrafficLimit(), days, true)
	if err != nil {
		return false, err
	}
	err = s.CustomerRepo.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	})
	if err != nil {
		return false, err
	}
	return false, nil
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

func (s *Service) applyExtraHwid(ctx context.Context, telegramID int64, customer *database.Customer, delta int) error {
	userInfo, err := s.Remnawave.GetUserTrafficInfo(ctx, telegramID)
	if err != nil {
		return err
	}
	current := resolveDeviceLimit(userInfo)
	newLimit := current + delta
	maxLimit := config.HwidMaxDevices()
	if maxLimit > 0 && newLimit > maxLimit {
		newLimit = maxLimit
	}
	updated, err := s.Remnawave.UpdateUserDeviceLimit(ctx, telegramID, newLimit)
	if err != nil {
		return err
	}
	newExtra := customer.ExtraHwid + delta
	if customer.ExpireAt == nil {
		return database.ValidationErrorf("no expire for extra")
	}
	updates := map[string]interface{}{
		"extra_hwid":            newExtra,
		"extra_hwid_expires_at": customer.ExpireAt,
	}
	if updated != nil {
		updates["subscription_link"] = updated.SubscriptionUrl
		updates["expire_at"] = updated.ExpireAt
	}
	return s.CustomerRepo.UpdateFields(ctx, customer.ID, updates)
}

// getActivePendingDiscount loads pending discount and deletes the row if time-limited discount has expired.
func (s *Service) getActivePendingDiscount(ctx context.Context, customerID int64) (*database.PendingDiscount, error) {
	if s == nil {
		return nil, nil
	}
	d, err := s.PromoRepo.GetPendingDiscountByCustomerID(ctx, customerID)
	if err != nil || d == nil {
		return nil, err
	}
	if !d.UntilFirstPurchase && d.ExpiresAt != nil && time.Now().After(*d.ExpiresAt) {
		if delErr := s.PromoRepo.DeletePendingDiscount(ctx, customerID); delErr != nil {
			slog.Error("clear expired discount", "error", delErr)
		}
		return nil, nil
	}
	return d, nil
}

// PendingDiscountForPayment returns percent 0-100 if a discount applies; clears expired pending discounts.
//
// Скидка гасится только при оплате, поэтому каждый неоплаченный счёт со
// скидкой резервирует одно использование. Раньше скидку «на одну оплату»
// можно было получить на любом числе заранее созданных счетов и оплатить их все.
func (s *Service) PendingDiscountForPayment(ctx context.Context, customerID int64) (percent int, promoCodeID int64, err error) {
	d, err := s.getActivePendingDiscount(ctx, customerID)
	if err != nil || d == nil {
		return 0, 0, err
	}
	if d.SubscriptionPaymentsRemaining != database.PendingDiscountUnlimitedPayments {
		open, err := s.PromoRepo.CountOpenPurchasesWithPromo(ctx, customerID, d.PromoCodeID)
		if err != nil {
			return 0, 0, err
		}
		if open >= d.SubscriptionPaymentsRemaining {
			// Брошенные счета Stars не истекают — освобождаем их и считаем заново.
			if n, cerr := s.PromoRepo.CancelOpenTelegramPurchasesWithPromo(ctx, customerID, d.PromoCodeID); cerr != nil {
				slog.Error("cancel stale stars invoices with promo", "error", cerr)
			} else if n > 0 {
				open, err = s.PromoRepo.CountOpenPurchasesWithPromo(ctx, customerID, d.PromoCodeID)
				if err != nil {
					return 0, 0, err
				}
			}
		}
		if open >= d.SubscriptionPaymentsRemaining {
			return 0, 0, nil
		}
	}
	return d.Percent, d.PromoCodeID, nil
}

// PendingDiscountForConnectUI returns pending discount for the My VPN screen; expired rows are cleared.
func (s *Service) PendingDiscountForConnectUI(ctx context.Context, customerID int64) (percent int, untilFirst bool, expiresAt *time.Time, ok bool, err error) {
	d, err := s.getActivePendingDiscount(ctx, customerID)
	if err != nil || d == nil {
		return 0, false, nil, false, err
	}
	return d.Percent, d.UntilFirstPurchase, d.ExpiresAt, true, nil
}

// OnSuccessfulSubscriptionDiscountPayment вызывается после успешной оплаты с применённой промо-скидкой.
// Уменьшает счётчик многоразовой скидки или удаляет pending после последней оплаты; безлимитная скидка сохраняется до TTL.
// Важно: учитываем любые типы оплат, где скидка реально была применена (подписка, доп. HWID и т.д.).
func (s *Service) OnSuccessfulSubscriptionDiscountPayment(ctx context.Context, purchase *database.Purchase, customerID int64) error {
	if s == nil || purchase == nil || purchase.PromoCodeID == nil || *purchase.PromoCodeID == 0 {
		return nil
	}
	tx, err := s.PromoRepo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pd, err := s.PromoRepo.GetPendingDiscountByCustomerIDForUpdate(ctx, tx, customerID)
	if err != nil {
		return err
	}
	if pd == nil {
		return tx.Commit(ctx)
	}
	if pd.PromoCodeID != *purchase.PromoCodeID {
		return tx.Commit(ctx)
	}

	rem := pd.SubscriptionPaymentsRemaining
	if rem == database.PendingDiscountUnlimitedPayments {
		return tx.Commit(ctx)
	}
	if rem <= 1 {
		if err := s.PromoRepo.DeletePendingDiscountTx(ctx, tx, customerID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := s.PromoRepo.UpdatePendingDiscountRemainingTx(ctx, tx, customerID, rem-1); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ClearPendingDiscountAfterSuccessfulSubscriptionPayment удаляет pending без учёта многоразовой логики (совместимость).
func (s *Service) ClearPendingDiscountAfterSuccessfulSubscriptionPayment(ctx context.Context, customerID int64) error {
	return s.PromoRepo.DeletePendingDiscount(ctx, customerID)
}

// IsPromoValidationErr is true for failed validation / effect (user sees neutral message).
func IsPromoValidationErr(err error) bool {
	return err != nil && errors.Is(err, database.ErrPromoValidation)
}

// GrantStackedPendingDiscount создаёт или складывает системную pending discount (lifecycle win-back).
// Если у customer уже есть pending discount — складывает проценты (cap 100%), продлевает TTL.
// promoCodeID — FK на системный promo_code (например __lifecycle_winback__).
func (s *Service) GrantStackedPendingDiscount(ctx context.Context, customerID, promoCodeID int64, percentAdd int, ttlHours int) error {
	if s == nil || s.PromoRepo == nil {
		return fmt.Errorf("promo service not initialized")
	}

	tx, err := s.PromoRepo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := s.PromoRepo.GetPendingDiscountByCustomerIDForUpdate(ctx, tx, customerID)
	if err != nil {
		return err
	}

	newPercent := percentAdd
	var newExpiry *time.Time
	untilFirst := true // системные скидки — одноразовые (до первой оплаты)
	remaining := 1

	if existing != nil {
		// Складываем проценты, cap 100
		newPercent = existing.Percent + percentAdd
		if newPercent > 100 {
			newPercent = 100
		}

		// TTL: max из существующей и новой
		now := time.Now().UTC()
		newDeadline := now.Add(time.Duration(ttlHours) * time.Hour)
		if existing.ExpiresAt != nil && existing.ExpiresAt.After(newDeadline) {
			newExpiry = existing.ExpiresAt
		} else {
			newExpiry = &newDeadline
		}

		// Если existing была многоразовой или без дедлайна — сохраняем её настройки
		if existing.SubscriptionPaymentsRemaining != 1 {
			remaining = existing.SubscriptionPaymentsRemaining
		}
		if !existing.UntilFirstPurchase {
			untilFirst = false
		}
	} else {
		// Новая скидка с TTL
		if ttlHours > 0 {
			exp := time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour)
			newExpiry = &exp
		}
	}

	err = s.PromoRepo.UpsertPendingDiscount(ctx, tx, customerID, promoCodeID, newPercent, newExpiry, untilFirst, remaining)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
