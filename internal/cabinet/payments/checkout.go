// Package payments — HTTP-независимый слой web-кабинета для создания счетов
// и чтения их статуса.
//
// Сервис переиспользует бизнес-логику существующего internal/payment:
//
//   - расчёт суммы (classic vs tariffs + upgrade/downgrade) идёт через
//     payment.ResolveTariffPurchase и config.Price — здесь мы _не дублируем_
//     никаких формул, только выбираем правильную ветку;
//   - создание purchase / получение payment_url идёт через обычный
//     PaymentService.CreatePurchase, но контекст доукомплектовывается
//     return_url / receipt_email / paid_btn_url — так в вебе не появляется
//     редиректа в Telegram после оплаты и чек летит на email пользователя;
//   - обработка самой оплаты (перевод purchase → paid, активация подписки)
//     уже работает фоновым worker'ом (checkYookasaInvoice /
//     checkCryptoPayInvoice) — он подхватит purchase, созданный из кабинета.
//
// cabinet_checkout играет роль «идемпотентной обёртки» над purchase:
// повторный POST с тем же Idempotency-Key не создаёт новый счёт.
package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	cabmetrics "remnawave-tg-shop-bot/internal/cabinet/metrics"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/heleket"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/platega"
	promosvc "remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/yookasa"
	"remnawave-tg-shop-bot/utils"
)

// Sentinel-ошибки, на которые HTTP-слой маппит конкретные статусы.
var (
	// ErrInvalidInput — 400: period/provider/tariff_id не прошли валидацию.
	ErrInvalidInput = errors.New("payments: invalid input")
	// ErrProviderDisabled — 400: выбранный провайдер отключён в env.
	ErrProviderDisabled = errors.New("payments: provider disabled")
	// ErrCheckoutNotFound — 404 в GET /status.
	ErrCheckoutNotFound = errors.New("payments: checkout not found")
	// ErrForbidden — 403: чужой checkout.
	ErrForbidden = errors.New("payments: forbidden")
)

// supportedMonths — те же значения, что в витрине тарифов. Храним дубликат
// константы (а не импорт из service), чтобы не тянуть сюда HTTP-пакет.
var supportedMonths = map[int]bool{1: true, 3: true, 6: true, 12: true}

// Config задаёт внешние параметры, которые нельзя вывести из env внутри сервиса
// (в частности — CABINET_PUBLIC_URL живёт в cabinet/config, а этот пакет не
// должен тянуть весь cabinet/config, чтобы unit-тесты могли его инстанцировать).
type Config struct {
	// PublicURL без завершающего слэша. Пример: "https://cabinet.example.com".
	PublicURL string
	// MinIdempotencyKeyLen — минимальная длина Idempotency-Key (защита от
	// пустых/коротких ключей). Рекомендуется 16.
	MinIdempotencyKeyLen int
	// MaxIdempotencyKeyLen — максимальная длина (UNIQUE-колонка VARCHAR(64)).
	MaxIdempotencyKeyLen int
}

// DefaultConfig возвращает sane defaults. PublicURL заполняется caller'ом.
func DefaultConfig() Config {
	return Config{MinIdempotencyKeyLen: 16, MaxIdempotencyKeyLen: 64}
}

// CheckoutService — фасад над PaymentService для web-кабинета.
//
// Зависимости передаются через конструктор (чистая инъекция). Ни один из
// репозиториев не может быть nil — конструктор это проверяет.
type CheckoutService struct {
	cfg Config

	checkouts *repository.CheckoutRepo
	links     *repository.AccountCustomerLinkRepo
	customers *database.CustomerRepository
	purchases *database.PurchaseRepository
	tariffs   *database.TariffRepository
	accounts  *repository.AccountRepo
	bootstrap *bootstrap.CustomerBootstrap
	payments  *payment.PaymentService
	loyalty   *database.LoyaltyTierRepository
	promo     *promosvc.Service
	rw        *remnawave.Client
}

// NewCheckoutService — конструктор.
func NewCheckoutService(
	cfg Config,
	checkouts *repository.CheckoutRepo,
	links *repository.AccountCustomerLinkRepo,
	customers *database.CustomerRepository,
	purchases *database.PurchaseRepository,
	tariffs *database.TariffRepository,
	accounts *repository.AccountRepo,
	boot *bootstrap.CustomerBootstrap,
	payments *payment.PaymentService,
	loyaltyRepo *database.LoyaltyTierRepository,
	promo *promosvc.Service,
	rw *remnawave.Client,
) *CheckoutService {
	if cfg.MinIdempotencyKeyLen <= 0 {
		cfg.MinIdempotencyKeyLen = 16
	}
	if cfg.MaxIdempotencyKeyLen <= 0 || cfg.MaxIdempotencyKeyLen > 64 {
		cfg.MaxIdempotencyKeyLen = 64
	}
	return &CheckoutService{
		cfg:       cfg,
		checkouts: checkouts,
		links:     links,
		customers: customers,
		purchases: purchases,
		tariffs:   tariffs,
		accounts:  accounts,
		bootstrap: boot,
		payments:  payments,
		loyalty:   loyaltyRepo,
		promo:     promo,
		rw:        rw,
	}
}

// CreateRequest — входной DTO HTTP-слоя.
//
// Extra-поля (hwid, promo) намеренно не реализуются в MVP Этапа 5, чтобы не
// разбалтывать расчёт цены. Добавим отдельным этапом, когда появится UI под них.
type CreateRequest struct {
	// Period — 1|3|6|12 месяцев.
	Period int
	// TariffID — обязательный в tariffs-режиме, игнорируется в classic.
	TariffID *int64
	// Provider — "yookassa" | "cryptopay".
	Provider string
	// IdempotencyKey — содержимое заголовка Idempotency-Key.
	IdempotencyKey string
	// RenewExtraHwid — если true, при наличии активных extra_hwid добавляем их продление к счёту (как в Telegram-боте).
	RenewExtraHwid bool
	// LocationID — площадка тарифа со списком локаций. Пусто, если у тарифа локаций нет.
	LocationID *int64
}

// CreateResult — ответ для POST /cabinet/api/payments/checkout.
type CreateResult struct {
	CheckoutID int64  `json:"checkout_id"`
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	PaymentURL string `json:"payment_url"`
	// Reused==true, если клиент повторно прислал тот же Idempotency-Key,
	// и мы вернули ранее выданный payment_url.
	Reused bool `json:"reused,omitempty"`
}

// StatusResult — ответ для GET /cabinet/api/payments/:id/status.
//
// SubscriptionLink и ExpireAt — ненулевые только в статусе "paid". UI поллит
// этот эндпоинт после редиректа с YooKassa/CryptoPay.
type StatusResult struct {
	CheckoutID       int64      `json:"checkout_id"`
	Status           string     `json:"status"`
	PaymentURL       string     `json:"payment_url,omitempty"`
	SubscriptionLink *string    `json:"subscription_link,omitempty"`
	ExpireAt         *time.Time `json:"expire_at,omitempty"`
	// PurchaseKind — при привязанном purchase (в т.ч. paid): для текста на странице статуса (подписка vs доп. HWID).
	PurchaseKind string `json:"purchase_kind,omitempty"`

	// Реквизиты чека на экране успешной оплаты. Всё читается из уже
	// загруженного purchase — дополнительных запросов в БД не добавляют.
	PaymentID   int64   `json:"payment_id,omitempty"`
	Amount      float64 `json:"amount,omitempty"`
	Currency    string  `json:"currency,omitempty"`
	InvoiceType string  `json:"invoice_type,omitempty"`
	// PaidAt — момент оплаты; если провайдер его не проставил, отдаём время
	// создания счёта: на чеке пустая строка времени выглядит как сбой.
	PaidAt *time.Time `json:"paid_at,omitempty"`
	// Month и ExtraHwid нужны фронту, чтобы назвать покупку тем же
	// purchaseKindLabel(), что и в истории оплат.
	Month     int `json:"month,omitempty"`
	ExtraHwid int `json:"extra_hwid,omitempty"`
}

// PreviewResult — ответ GET /payments/preview: сумма с учётом upgrade/downgrade (как у бота).
type PreviewResult struct {
	Amount           int    `json:"amount"`
	Currency         string `json:"currency"`
	AmountRub        int    `json:"amount_rub"`
	SalesMode        string `json:"sales_mode"`
	Scenario         string `json:"scenario"` // new | renew | upgrade | downgrade | classic_new | classic_renew
	PurchaseKind     string `json:"purchase_kind,omitempty"`
	IsEarlyDowngrade bool   `json:"is_early_downgrade,omitempty"`
	ListPriceRub     int    `json:"list_price_rub,omitempty"` // полная цена прайса за период (для сравнения в UI)
	// BaseAmountRub — сумма до скидок лояльности и pending-промокода (как в боте).
	BaseAmountRub      int `json:"base_amount_rub,omitempty"`
	LoyaltyDiscountPct int `json:"loyalty_discount_pct,omitempty"`
	PromoDiscountPct   int `json:"promo_discount_pct,omitempty"`
	TotalDiscountPct   int `json:"total_discount_pct,omitempty"`
	// TariffSwitch* — только при scenario upgrade/downgrade (активная подписка, смена тарифа).
	TariffSwitchRemainingDays int `json:"tariff_switch_remaining_days,omitempty"` // календарных дней до expire (ceil)
	TariffSwitchBonusDays     int `json:"tariff_switch_bonus_days,omitempty"`     // остаток в днях по дневной цене нового тарифа
	TariffSwitchPeriodDays    int `json:"tariff_switch_period_days,omitempty"`    // месяцы × DaysInMonth
	TariffSwitchTotalDays     int `json:"tariff_switch_total_days,omitempty"`     // период + бонус
	// Extra HWID renew (как в Telegram sell-flow): при наличии активных слотов можно включить/выключить продление.
	ExtraHwidActive    int  `json:"extra_hwid_active,omitempty"`
	ExtraHwidIncluded  bool `json:"extra_hwid_included,omitempty"`
	ExtraHwidAmountRub int  `json:"extra_hwid_amount_rub,omitempty"`
}

// Create создаёт новый web-checkout или возвращает ранее выданный payment_url
// при повторе с тем же Idempotency-Key.
//
// Шаги:
//  1. Валидация входа.
//  2. Lookup по (account_id, idempotency_key). Если есть — возвращаем.
//  3. Bootstrap: гарантируем, что у аккаунта есть customer (EnsureForAccount).
//  4. Расчёт суммы (classic: config.Price / tariffs: ResolveTariffPurchase).
//  5. INSERT cabinet_checkout(status='new'). При конфликте по idempotency_key
//     — перечитываем и возвращаем.
//  6. Проставляем ctx-override'ы и вызываем PaymentService.CreatePurchase.
//  7. UPDATE cabinet_checkout SET purchase_id, return_url, status='pending'.
func (s *CheckoutService) Create(ctx context.Context, accountID int64, req CreateRequest) (*CreateResult, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}
	provider := req.Provider
	invoiceType, err := mapProviderToInvoiceType(provider)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProviderEnabled(provider); err != nil {
		return nil, err
	}

	if existing, err := s.checkouts.FindByIdempotencyKey(ctx, accountID, req.IdempotencyKey); err == nil {
		return s.reuseExisting(ctx, existing)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("payments: find existing: %w", err)
	}

	// Гарантируем customer — создаём web-only, если link'а ещё нет.
	link, err := s.bootstrap.EnsureForAccount(ctx, accountID, "")
	if err != nil {
		return nil, fmt.Errorf("payments: bootstrap customer: %w", err)
	}
	customer, err := s.customers.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("payments: load customer: %w", err)
	}
	if customer == nil {
		return nil, fmt.Errorf("payments: customer %d not found after bootstrap", link.CustomerID)
	}

	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("payments: load account: %w", err)
	}

	amount, tariffID, extras, extraHwid, err := s.resolveAmount(ctx, customer, invoiceType, req)
	if err != nil {
		return nil, err
	}
	location, err := s.resolveTariffLocation(ctx, tariffID, req.LocationID)
	if err != nil {
		return nil, err
	}

	amount, _, _, meta := s.applyCheckoutDiscounts(ctx, customer, invoiceType, amount)

	checkout, err := s.checkouts.Create(ctx, accountID, req.IdempotencyKey, provider)
	if err != nil {
		// Гонка: параллельный POST с тем же ключом уже успел вставить строку.
		if errors.Is(err, repository.ErrCheckoutConflict) {
			existing, ferr := s.checkouts.FindByIdempotencyKey(ctx, accountID, req.IdempotencyKey)
			if ferr != nil {
				return nil, fmt.Errorf("payments: read after conflict: %w", ferr)
			}
			return s.reuseExisting(ctx, existing)
		}
		return nil, fmt.Errorf("payments: create checkout: %w", err)
	}

	returnURL := s.buildReturnURL(checkout.ID)
	providerCtx := s.withProviderOverrides(ctx, provider, returnURL, acc)

	var paymentURL string
	var purchaseID int64
	if extraHwid > 0 {
		paymentURL, purchaseID, err = s.payments.CreatePurchaseWithExtra(
			providerCtx,
			float64(amount),
			req.Period,
			extraHwid,
			customer,
			invoiceType,
			meta,
			tariffID,
			extras,
		)
	} else {
		paymentURL, purchaseID, err = s.payments.CreatePurchase(
			providerCtx,
			float64(amount),
			req.Period,
			customer,
			invoiceType,
			meta,
			tariffID,
			extras,
		)
	}
	if err != nil {
		// Checkout остался в 'new'. Можно попытаться отметить failed, но в MVP
		// оставляем как есть: клиент может прислать тот же ключ ещё раз, а
		// ErrCheckoutConflict → reuseExisting проверит purchase_id==nil и
		// вернёт ошибку «невалидный checkout, смените Idempotency-Key».
		return nil, fmt.Errorf("payments: create purchase: %w", err)
	}

	if location != nil && purchaseID > 0 {
		if err := s.purchases.SetTariffLocation(ctx, purchaseID, location.ID, location.Name); err != nil {
			return nil, fmt.Errorf("payments: save location: %w", err)
		}
	}

	if err := s.checkouts.AttachPurchase(ctx, checkout.ID, purchaseID, returnURL); err != nil {
		// Purchase создан, но link не обновился — это легко лечится: GetStatus
		// увидит cabinet_checkout со status='new' и purchase_id=nil, и в этот
		// момент статус «провиснет». На MVP логируем и возвращаем успех.
		slog.Error("payments: attach purchase failed",
			"checkout_id", checkout.ID,
			"purchase_id", purchaseID,
			"error", err,
		)
	}

	return &CreateResult{
		CheckoutID: checkout.ID,
		Provider:   provider,
		Status:     repository.CheckoutStatusPending,
		PaymentURL: paymentURL,
	}, nil
}

// resolveTariffLocation проверяет выбор площадки, если у тарифа есть локации.
// nil, nil — у тарифа локаций нет, выбор не нужен.
func (s *CheckoutService) resolveTariffLocation(ctx context.Context, tariffID, locationID *int64) (*database.TariffLocation, error) {
	if tariffID == nil || *tariffID <= 0 || s.tariffs == nil {
		return nil, nil
	}
	locs, err := s.tariffs.ListLocations(ctx, *tariffID)
	if err != nil {
		return nil, fmt.Errorf("payments: list locations: %w", err)
	}
	if len(locs) == 0 {
		return nil, nil
	}
	available := 0
	for _, loc := range locs {
		if !loc.SoldOut {
			available++
		}
	}
	if available == 0 {
		return nil, fmt.Errorf("%w: location_sold_out", ErrInvalidInput)
	}
	if locationID == nil || *locationID <= 0 {
		return nil, fmt.Errorf("%w: location_required", ErrInvalidInput)
	}
	for i := range locs {
		if locs[i].ID != *locationID {
			continue
		}
		if locs[i].SoldOut {
			return nil, fmt.Errorf("%w: location_sold_out", ErrInvalidInput)
		}
		return &locs[i], nil
	}
	return nil, fmt.Errorf("%w: location_invalid", ErrInvalidInput)
}

// Preview считает сумму к оплате без создания checkout (та же логика, что Create → resolveAmount).
func (s *CheckoutService) Preview(ctx context.Context, accountID int64, period int, tariffID *int64, provider string) (*PreviewResult, error) {
	if !supportedMonths[period] {
		return nil, fmt.Errorf("%w: period must be one of 1/3/6/12", ErrInvalidInput)
	}
	if provider == "" {
		provider = repository.CheckoutProviderYookassa
	}
	invoiceType, err := mapProviderToInvoiceType(provider)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProviderEnabled(provider); err != nil {
		return nil, err
	}
	link, err := s.bootstrap.EnsureForAccount(ctx, accountID, "")
	if err != nil {
		return nil, fmt.Errorf("payments: bootstrap customer: %w", err)
	}
	customer, err := s.customers.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("payments: load customer: %w", err)
	}
	if customer == nil {
		return nil, fmt.Errorf("payments: customer %d not found after bootstrap", link.CustomerID)
	}

	mode := config.SalesMode()
	out := &PreviewResult{SalesMode: mode}
	if invoiceType == database.InvoiceTypeTelegram {
		out.Currency = "STARS"
	} else {
		out.Currency = "RUB"
	}

	activeExtra := cabsvc.ActiveExtraSlots(customer)
	out.ExtraHwidActive = activeExtra
	out.ExtraHwidIncluded = reqBoolFalse(reqRenewExtraHwidFromContext(ctx))

	if mode == "tariffs" {
		if tariffID == nil || *tariffID <= 0 {
			return nil, fmt.Errorf("%w: tariff_id required in tariffs mode", ErrInvalidInput)
		}
		kind, amt, pk, early, rerr := payment.ResolveTariffPurchase(
			ctx, s.tariffs, customer, *tariffID, period, invoiceType == database.InvoiceTypeTelegram,
		)
		if rerr != nil {
			return nil, fmt.Errorf("payments: resolve tariff: %w", rerr)
		}
		out.Amount = amt
		out.AmountRub = amt
		out.PurchaseKind = string(pk)
		out.IsEarlyDowngrade = early
		out.Scenario = tariffCheckoutScenario(kind)
		if tp, err := s.tariffs.GetPrice(ctx, *tariffID, period); err == nil && tp != nil {
			out.ListPriceRub = tp.AmountRub
		}
		if activeExtra > 0 && reqBoolFalse(reqRenewExtraHwidFromContext(ctx)) {
			extra := s.subscriptionExtraAmount(period, invoiceType, activeExtra)
			out.ExtraHwidAmountRub = extra
			out.Amount += extra
			out.AmountRub += extra
			out.ExtraHwidIncluded = true
			if out.ListPriceRub > 0 {
				out.ListPriceRub += extra
			}
		}
		if kind == payment.TariffCheckoutUpgrade || kind == payment.TariffCheckoutDowngrade {
			if customer.CurrentTariffID != nil && *customer.CurrentTariffID > 0 {
				tpOld, e1 := s.tariffs.GetPrice(ctx, *customer.CurrentTariffID, period)
				tpNew, e2 := s.tariffs.GetPrice(ctx, *tariffID, period)
				if e1 == nil && e2 == nil && tpOld != nil && tpNew != nil {
					now := time.Now().UTC()
					out.TariffSwitchRemainingDays = payment.UpgradeCalendarDaysRemainingCeil(customer, now)
					out.TariffSwitchBonusDays = payment.ComputeUpgradeBonusDays(customer, tpOld, tpNew, period, now)
					dim := config.DaysInMonth()
					if dim <= 0 {
						dim = 30
					}
					out.TariffSwitchPeriodDays = period * dim
					out.TariffSwitchTotalDays = out.TariffSwitchPeriodDays + out.TariffSwitchBonusDays
				}
			}
		}
		s.applyPreviewDiscounts(ctx, customer, invoiceType, out)
		return out, nil
	}

	price := config.Price(period)
	if invoiceType == database.InvoiceTypeTelegram {
		price = config.StarsPrice(period)
	}
	if price <= 0 {
		return nil, fmt.Errorf("payments: no price configured for %d months", period)
	}
	out.Amount = price
	out.AmountRub = price
	out.PurchaseKind = string(database.PurchaseKindSubscription)
	now := time.Now().UTC()
	if customer.ExpireAt != nil && customer.ExpireAt.After(now) {
		out.Scenario = "classic_renew"
	} else {
		out.Scenario = "classic_new"
	}
	if activeExtra > 0 && reqBoolFalse(reqRenewExtraHwidFromContext(ctx)) {
		extra := s.subscriptionExtraAmount(period, invoiceType, activeExtra)
		out.ExtraHwidAmountRub = extra
		out.Amount += extra
		out.AmountRub += extra
		out.ExtraHwidIncluded = true
	}
	s.applyPreviewDiscounts(ctx, customer, invoiceType, out)
	return out, nil
}

func tariffCheckoutScenario(kind payment.TariffCheckoutKind) string {
	switch kind {
	case payment.TariffCheckoutNew:
		return "new"
	case payment.TariffCheckoutRenewSame:
		return "renew"
	case payment.TariffCheckoutUpgrade:
		return "upgrade"
	case payment.TariffCheckoutDowngrade:
		return "downgrade"
	default:
		return "unknown"
	}
}

// GetStatus возвращает текущий статус checkout'а + ссылку на подписку, если
// оплата прошла. Попутно лениво синхронизирует cabinet_checkout.status с
// purchase.status — первый же поллинг после webhook'а бот приведёт в
// консистентное состояние.
func (s *CheckoutService) GetStatus(ctx context.Context, accountID, checkoutID int64) (*StatusResult, error) {
	checkout, err := s.checkouts.FindByID(ctx, checkoutID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrCheckoutNotFound
		}
		return nil, fmt.Errorf("payments: find checkout: %w", err)
	}
	if checkout.AccountID != accountID {
		return nil, ErrForbidden
	}

	result := &StatusResult{
		CheckoutID: checkout.ID,
		Status:     checkout.Status,
	}

	if checkout.PurchaseID == nil {
		// Счёт ещё не прикреплён к purchase — это возможно только в узкое
		// окно между INSERT checkout и AttachPurchase. UI должен перезапросить.
		return result, nil
	}

	purchase, err := s.purchases.FindById(ctx, *checkout.PurchaseID)
	if err != nil {
		return nil, fmt.Errorf("payments: load purchase: %w", err)
	}
	if purchase == nil {
		return nil, fmt.Errorf("payments: purchase %d missing", *checkout.PurchaseID)
	}

	kind := string(purchase.PurchaseKind)
	if purchase.Month <= 0 && purchase.ExtraHwid > 0 {
		kind = string(database.PurchaseKindExtraHwid)
	}
	result.PurchaseKind = kind

	result.PaymentID = purchase.ID
	result.Amount = purchase.Amount
	result.Currency = purchase.Currency
	result.InvoiceType = string(purchase.InvoiceType)
	result.Month = purchase.Month
	result.ExtraHwid = purchase.ExtraHwid
	if purchase.PaidAt != nil {
		result.PaidAt = purchase.PaidAt
	} else {
		paidAt := checkout.CreatedAt
		result.PaidAt = &paidAt
	}

	desired := checkoutStatusForPurchase(purchase.Status)
	if desired != "" && desired != checkout.Status {
		if err := s.checkouts.UpdateStatus(ctx, checkout.ID, desired); err != nil {
			slog.Warn("payments: sync status failed", "checkout_id", checkout.ID, "error", err)
		} else if desired == repository.CheckoutStatusPaid {
			sec := time.Since(checkout.CreatedAt).Seconds()
			cabmetrics.RecordCheckoutPaidDuration(checkout.Provider, sec)
		}
		result.Status = desired
	}

	switch result.Status {
	case repository.CheckoutStatusPending:
		result.PaymentURL = paymentURLFromPurchase(purchase)
	case repository.CheckoutStatusPaid:
		link, err := s.links.FindByAccountID(ctx, accountID)
		if err == nil {
			if cust, err2 := s.customers.FindById(ctx, link.CustomerID); err2 == nil && cust != nil {
				result.SubscriptionLink = cust.SubscriptionLink
				result.ExpireAt = cust.ExpireAt
			}
		}
	}

	return result, nil
}

// HwidPreviewResult — ответ GET /cabinet/api/payments/hwid/preview.
type HwidPreviewResult struct {
	CurrentLimit       int    `json:"current_limit"`
	TargetLimit        int    `json:"target_limit"`
	Delta              int    `json:"delta"`
	DaysLeft           int    `json:"days_left"`
	AmountRub          int    `json:"amount_rub"`
	BaseAmountRub      int    `json:"base_amount_rub"`
	Currency           string `json:"currency"`
	LoyaltyDiscountPct int    `json:"loyalty_discount_pct,omitempty"`
	PromoDiscountPct   int    `json:"promo_discount_pct,omitempty"`
	TotalDiscountPct   int    `json:"total_discount_pct,omitempty"`
}

// HwidCreateRequest — тело POST /cabinet/api/payments/hwid/checkout.
type HwidCreateRequest struct {
	TargetLimit    int
	Provider       string
	IdempotencyKey string
}

func (s *CheckoutService) validateHwidIdempotencyKey(key string) error {
	l := len(key)
	if l < s.cfg.MinIdempotencyKeyLen || l > s.cfg.MaxIdempotencyKeyLen {
		return fmt.Errorf(
			"%w: idempotency key must be between %d and %d chars",
			ErrInvalidInput, s.cfg.MinIdempotencyKeyLen, s.cfg.MaxIdempotencyKeyLen,
		)
	}
	return nil
}

func (s *CheckoutService) hwidPrecheck(ctx context.Context, accountID int64) (*database.Customer, cabsvc.HwidExtraLimits, error) {
	if s.rw == nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("%w: remnawave unavailable", ErrInvalidInput)
	}
	if !config.HwidExtraDevicesEnabled() {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("%w: extra devices disabled", ErrInvalidInput)
	}
	link, err := s.bootstrap.EnsureForAccount(ctx, accountID, "")
	if err != nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("payments: bootstrap customer: %w", err)
	}
	customer, err := s.customers.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("payments: load customer: %w", err)
	}
	if customer == nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("payments: customer missing")
	}
	if customer.IsWebOnly || utils.IsSyntheticTelegramID(customer.TelegramID) {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("%w: not available for this account", ErrForbidden)
	}
	paid, err := s.purchases.HasPaidSubscription(ctx, customer.ID)
	if err != nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("payments: paid check: %w", err)
	}
	if !paid {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("%w: paid subscription required", ErrInvalidInput)
	}
	now := time.Now()
	if customer.ExpireAt == nil || !customer.ExpireAt.After(now) {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("%w: subscription inactive", ErrInvalidInput)
	}
	if err := cabsvc.CleanupExpiredExtraHwid(ctx, s.rw, s.customers, customer); err != nil {
		return nil, cabsvc.HwidExtraLimits{}, err
	}
	if fresh, ferr := s.customers.FindById(ctx, customer.ID); ferr == nil && fresh != nil {
		customer = fresh
	}
	u, err := s.rw.GetUserTrafficInfo(ctx, customer.TelegramID)
	if err != nil {
		return nil, cabsvc.HwidExtraLimits{}, fmt.Errorf("remnawave: %w", err)
	}
	lim := cabsvc.BuildHwidExtraLimits(customer, u)
	return customer, lim, nil
}

// PreviewHwid считает сумму докупки устройств без создания счёта.
func (s *CheckoutService) PreviewHwid(ctx context.Context, accountID int64, targetLimit int, provider string) (*HwidPreviewResult, error) {
	if targetLimit <= 0 {
		return nil, fmt.Errorf("%w: bad target_limit", ErrInvalidInput)
	}
	if provider == "" {
		provider = repository.CheckoutProviderYookassa
	}
	invoiceType, err := mapProviderToInvoiceType(provider)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProviderEnabled(provider); err != nil {
		return nil, err
	}
	customer, lim, err := s.hwidPrecheck(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if lim.MaxCap <= 0 || lim.CurrentLimit >= lim.MaxCap {
		return nil, fmt.Errorf("%w: cannot add devices (check HWID_MAX_DEVICE)", ErrInvalidInput)
	}
	if targetLimit <= lim.CurrentLimit {
		return nil, fmt.Errorf("%w: target must exceed current limit", ErrInvalidInput)
	}
	if targetLimit > lim.MaxCap {
		return nil, fmt.Errorf("%w: exceeds max devices", ErrInvalidInput)
	}
	if lim.DaysLeft <= 0 {
		return nil, fmt.Errorf("%w: subscription ended", ErrInvalidInput)
	}
	delta := targetLimit - lim.CurrentLimit
	priceMonth := config.HwidAddPrice()
	if invoiceType == database.InvoiceTypeTelegram {
		priceMonth = config.HwidAddStarsPrice()
	}
	base := cabsvc.CalcHwidProportionalRub(priceMonth, delta, lim.DaysLeft)
	final, loyPct, proPct, _ := s.applyCheckoutDiscounts(ctx, customer, invoiceType, base)
	out := &HwidPreviewResult{
		CurrentLimit:       lim.CurrentLimit,
		TargetLimit:        targetLimit,
		Delta:              delta,
		DaysLeft:           lim.DaysLeft,
		AmountRub:          final,
		BaseAmountRub:      base,
		LoyaltyDiscountPct: loyPct,
		PromoDiscountPct:   proPct,
	}
	cap := config.LoyaltyMaxTotalDiscountPercent()
	out.TotalDiscountPct = loyalty.CombinedDiscountPercent(loyPct, proPct, cap)
	if invoiceType == database.InvoiceTypeTelegram {
		out.Currency = "STARS"
	} else {
		out.Currency = "RUB"
	}
	return out, nil
}

// CreateHwid создаёт счёт на докупку HWID (как Telegram AddDevicePayment).
func (s *CheckoutService) CreateHwid(ctx context.Context, accountID int64, req HwidCreateRequest) (*CreateResult, error) {
	if err := s.validateHwidIdempotencyKey(req.IdempotencyKey); err != nil {
		return nil, err
	}
	if req.TargetLimit <= 0 {
		return nil, fmt.Errorf("%w: bad target_limit", ErrInvalidInput)
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = repository.CheckoutProviderYookassa
	}
	invoiceType, err := mapProviderToInvoiceType(provider)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProviderEnabled(provider); err != nil {
		return nil, err
	}

	if existing, err := s.checkouts.FindByIdempotencyKey(ctx, accountID, req.IdempotencyKey); err == nil {
		return s.reuseExisting(ctx, existing)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("payments: find existing: %w", err)
	}

	customer, lim, err := s.hwidPrecheck(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if lim.MaxCap <= 0 || lim.CurrentLimit >= lim.MaxCap {
		return nil, fmt.Errorf("%w: cannot add devices (check HWID_MAX_DEVICE)", ErrInvalidInput)
	}
	if req.TargetLimit <= lim.CurrentLimit {
		return nil, fmt.Errorf("%w: target must exceed current limit", ErrInvalidInput)
	}
	if req.TargetLimit > lim.MaxCap {
		return nil, fmt.Errorf("%w: exceeds max devices", ErrInvalidInput)
	}
	if lim.DaysLeft <= 0 {
		return nil, fmt.Errorf("%w: subscription ended", ErrInvalidInput)
	}
	delta := req.TargetLimit - lim.CurrentLimit
	priceMonth := config.HwidAddPrice()
	if invoiceType == database.InvoiceTypeTelegram {
		priceMonth = config.HwidAddStarsPrice()
	}
	base := cabsvc.CalcHwidProportionalRub(priceMonth, delta, lim.DaysLeft)
	amount, _, _, meta := s.applyCheckoutDiscounts(ctx, customer, invoiceType, base)

	checkout, err := s.checkouts.Create(ctx, accountID, req.IdempotencyKey, provider)
	if err != nil {
		if errors.Is(err, repository.ErrCheckoutConflict) {
			existing, ferr := s.checkouts.FindByIdempotencyKey(ctx, accountID, req.IdempotencyKey)
			if ferr != nil {
				return nil, fmt.Errorf("payments: read after conflict: %w", ferr)
			}
			return s.reuseExisting(ctx, existing)
		}
		return nil, fmt.Errorf("payments: create checkout: %w", err)
	}

	returnURL := s.buildReturnURL(checkout.ID)
	acc, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("payments: load account: %w", err)
	}
	providerCtx := s.withProviderOverrides(ctx, provider, returnURL, acc)

	paymentURL, purchaseID, err := s.payments.CreateHwidPurchase(providerCtx, float64(amount), delta, customer, invoiceType, meta)
	if err != nil {
		return nil, fmt.Errorf("payments: create hwid purchase: %w", err)
	}

	if err := s.checkouts.AttachPurchase(ctx, checkout.ID, purchaseID, returnURL); err != nil {
		slog.Error("payments: hwid attach purchase failed",
			"checkout_id", checkout.ID,
			"purchase_id", purchaseID,
			"error", err,
		)
	}

	return &CreateResult{
		CheckoutID: checkout.ID,
		Provider:   provider,
		Status:     repository.CheckoutStatusPending,
		PaymentURL: paymentURL,
	}, nil
}

// ============================================================================
// Helpers: валидация, маппинги, URL-сборка
// ============================================================================

func (s *CheckoutService) validateCreate(req CreateRequest) error {
	if !supportedMonths[req.Period] {
		return fmt.Errorf("%w: period must be one of 1/3/6/12", ErrInvalidInput)
	}
	if _, err := mapProviderToInvoiceType(req.Provider); err != nil {
		return err
	}
	l := len(req.IdempotencyKey)
	if l < s.cfg.MinIdempotencyKeyLen || l > s.cfg.MaxIdempotencyKeyLen {
		return fmt.Errorf(
			"%w: idempotency key must be between %d and %d chars",
			ErrInvalidInput, s.cfg.MinIdempotencyKeyLen, s.cfg.MaxIdempotencyKeyLen,
		)
	}
	return nil
}

func (s *CheckoutService) ensureProviderEnabled(provider string) error {
	switch provider {
	case repository.CheckoutProviderYookassa:
		if !config.IsYookasaEnabled() {
			return fmt.Errorf("%w: yookassa disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderCryptoPay:
		if !config.IsCryptoPayEnabled() {
			return fmt.Errorf("%w: cryptopay disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderTelegram:
		if !config.IsTelegramStarsEnabled() {
			return fmt.Errorf("%w: telegram stars disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderPlategaSBP:
		if !config.IsPlategaEnabled() || !config.IsPlategaSBPEnabled() {
			return fmt.Errorf("%w: platega sbp disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderPlategaCards:
		if !config.IsPlategaEnabled() || !config.IsPlategaCardsEnabled() {
			return fmt.Errorf("%w: platega cards disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderPlategaAcquiring:
		if !config.IsPlategaEnabled() || !config.IsPlategaAcquiringEnabled() {
			return fmt.Errorf("%w: platega acquiring disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderPlategaWorldwide:
		if !config.IsPlategaEnabled() || !config.IsPlategaWorldwideEnabled() {
			return fmt.Errorf("%w: platega worldwide disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderPlategaCrypto:
		if !config.IsPlategaEnabled() || !config.IsPlategaCryptoEnabled() {
			return fmt.Errorf("%w: platega crypto disabled", ErrProviderDisabled)
		}
	case repository.CheckoutProviderHeleket:
		if !config.IsHeleketEnabled() {
			return fmt.Errorf("%w: heleket disabled", ErrProviderDisabled)
		}
	default:
		return fmt.Errorf("%w: unknown provider %q", ErrInvalidInput, provider)
	}
	return nil
}

// resolveAmount возвращает сумму (в рублях/stars), tariffID для purchase-строки
// и extras (kind/isEarlyDowngrade). В classic-режиме всегда tariffID==nil, extras==nil.
// applyCheckoutDiscounts — как handler.checkoutPromoMeta: лояльность + pending-промо, Tribute не используется в кабинете.
func (s *CheckoutService) applyCheckoutDiscounts(ctx context.Context, customer *database.Customer, invoiceType database.InvoiceType, base int) (final int, loyaltyPct, promoPct int, meta *payment.PromoMeta) {
	final = base
	if customer == nil || invoiceType == database.InvoiceTypeTribute {
		return final, 0, 0, nil
	}
	loyaltyPct = 0
	if config.LoyaltyEnabled() && s.loyalty != nil {
		var err error
		loyaltyPct, err = s.loyalty.DiscountPercentForXP(ctx, customer.LoyaltyXP)
		if err != nil {
			slog.Error("cabinet checkout loyalty discount", "error", err)
			loyaltyPct = 0
		}
	}
	promoPct = 0
	if s.promo != nil {
		pct, pid, err := s.promo.PendingDiscountForPayment(ctx, customer.ID)
		if err != nil {
			slog.Error("cabinet checkout pending promo", "error", err)
		} else if pct > 0 && pid != 0 {
			promoPct = pct
			pc := pct
			p := pid
			meta = &payment.PromoMeta{PromoCodeID: &p, DiscountPercentApplied: &pc}
		}
	}
	cap := config.LoyaltyMaxTotalDiscountPercent()
	final = loyalty.ApplyCombinedPercentDiscount(base, loyaltyPct, promoPct, cap)
	return final, loyaltyPct, promoPct, meta
}

func (s *CheckoutService) applyPreviewDiscounts(ctx context.Context, customer *database.Customer, invoiceType database.InvoiceType, out *PreviewResult) {
	base := out.AmountRub
	final, loyPct, proPct, _ := s.applyCheckoutDiscounts(ctx, customer, invoiceType, base)
	out.BaseAmountRub = base
	out.AmountRub = final
	out.Amount = final
	out.LoyaltyDiscountPct = loyPct
	out.PromoDiscountPct = proPct
	cap := config.LoyaltyMaxTotalDiscountPercent()
	out.TotalDiscountPct = loyalty.CombinedDiscountPercent(loyPct, proPct, cap)
}

func (s *CheckoutService) resolveAmount(
	ctx context.Context,
	customer *database.Customer,
	invoiceType database.InvoiceType,
	req CreateRequest,
) (amount int, tariffID *int64, extras *payment.TariffPurchaseExtras, extraHwid int, err error) {
	invoiceStars := invoiceType == database.InvoiceTypeTelegram
	activeExtra := cabsvc.ActiveExtraSlots(customer)
	if req.RenewExtraHwid && activeExtra > 0 {
		extraHwid = activeExtra
	}
	if config.SalesMode() == "tariffs" {
		if req.TariffID == nil || *req.TariffID <= 0 {
			return 0, nil, nil, 0, fmt.Errorf("%w: tariff_id required in tariffs mode", ErrInvalidInput)
		}
		_, amt, kind, isEarly, rerr := payment.ResolveTariffPurchase(
			ctx, s.tariffs, customer, *req.TariffID, req.Period, invoiceStars,
		)
		if rerr != nil {
			return 0, nil, nil, 0, fmt.Errorf("payments: resolve tariff: %w", rerr)
		}
		extras = &payment.TariffPurchaseExtras{Kind: kind, IsEarlyDowngrade: isEarly}
		tariffID = req.TariffID
		amount = amt
		amount += s.subscriptionExtraAmount(req.Period, invoiceType, extraHwid)
		return amount, tariffID, extras, extraHwid, nil
	}

	// classic
	if invoiceStars {
		price := config.StarsPrice(req.Period)
		if price <= 0 {
			return 0, nil, nil, 0, fmt.Errorf("payments: no stars price configured for %d months", req.Period)
		}
		return price + s.subscriptionExtraAmount(req.Period, invoiceType, extraHwid), nil, nil, extraHwid, nil
	}
	price := config.Price(req.Period)
	if price <= 0 {
		return 0, nil, nil, 0, fmt.Errorf("payments: no price configured for %d months", req.Period)
	}
	return price + s.subscriptionExtraAmount(req.Period, invoiceType, extraHwid), nil, nil, extraHwid, nil
}

type renewExtraCtxKey struct{}

// WithPreviewRenewExtraHwid прокидывает флаг из HTTP preview-query в сервис расчёта.
func WithPreviewRenewExtraHwid(ctx context.Context, renew bool) context.Context {
	return context.WithValue(ctx, renewExtraCtxKey{}, renew)
}

func reqRenewExtraHwidFromContext(ctx context.Context) *bool {
	v, ok := ctx.Value(renewExtraCtxKey{}).(bool)
	if !ok {
		return nil
	}
	return &v
}

func reqBoolFalse(v *bool) bool {
	return v != nil && *v
}

func (s *CheckoutService) subscriptionExtraAmount(period int, invoiceType database.InvoiceType, extra int) int {
	if period <= 0 || extra <= 0 {
		return 0
	}
	if invoiceType == database.InvoiceTypeTelegram {
		return config.HwidAddStarsPrice() * extra * period
	}
	return config.HwidAddPrice() * extra * period
}

// buildReturnURL собирает URL, на который провайдер вернёт пользователя после
// подтверждения оплаты. SPA на этом URL делает poll GET /payments/:id/status.
func (s *CheckoutService) buildReturnURL(checkoutID int64) string {
	if s.cfg.PublicURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/cabinet/payment/status/%d", s.cfg.PublicURL, checkoutID)
}

// withProviderOverrides прокидывает return_url / receipt_email в контекст
// провайдера. Значения подхватываются yookasa.Client / PaymentService.
func (s *CheckoutService) withProviderOverrides(
	ctx context.Context,
	provider, returnURL string,
	acc *repository.Account,
) context.Context {
	switch provider {
	case repository.CheckoutProviderYookassa:
		if returnURL != "" {
			ctx = context.WithValue(ctx, yookasa.CtxKeyReturnURL, returnURL)
		}
		if email := receiptEmailForAccount(acc); email != "" {
			ctx = context.WithValue(ctx, yookasa.CtxKeyReceiptEmail, email)
		}
	case repository.CheckoutProviderCryptoPay:
		if returnURL != "" {
			ctx = context.WithValue(ctx, cryptopay.CtxKeyPaidBtnURL, returnURL)
		}
	case repository.CheckoutProviderPlategaSBP, repository.CheckoutProviderPlategaCards,
		repository.CheckoutProviderPlategaAcquiring, repository.CheckoutProviderPlategaWorldwide,
		repository.CheckoutProviderPlategaCrypto:
		if returnURL != "" {
			ctx = context.WithValue(ctx, platega.CtxKeyReturnURL, returnURL)
		}
	case repository.CheckoutProviderHeleket:
		if returnURL != "" {
			ctx = context.WithValue(ctx, heleket.CtxKeyReturnURL, returnURL)
		}
	}
	return ctx
}

// receiptEmailForAccount возвращает email пользователя, только если он
// подтверждён; в противном случае возвращает пустую строку — yookasa-клиент
// сам подставит config.YookasaEmail() как fallback.
func receiptEmailForAccount(acc *repository.Account) string {
	if acc == nil || acc.Email == nil {
		return ""
	}
	if !acc.EmailVerified() {
		return ""
	}
	return *acc.Email
}

// reuseExisting отдаёт клиенту ранее созданный checkout. Если purchase уже
// привязан и у него жив payment_url — отдаём его. Иначе 409-подобная ошибка
// (в MVP сигналим ErrInvalidInput — клиент должен выбрать новый Idempotency-Key).
func (s *CheckoutService) reuseExisting(ctx context.Context, checkout *repository.Checkout) (*CreateResult, error) {
	if checkout.PurchaseID == nil {
		return nil, fmt.Errorf("%w: checkout %d is in 'new' state, retry with a new Idempotency-Key", ErrInvalidInput, checkout.ID)
	}
	purchase, err := s.purchases.FindById(ctx, *checkout.PurchaseID)
	if err != nil {
		return nil, fmt.Errorf("payments: reuse load purchase: %w", err)
	}
	if purchase == nil {
		return nil, fmt.Errorf("payments: purchase %d missing", *checkout.PurchaseID)
	}
	url := paymentURLFromPurchase(purchase)
	if url == "" && purchase.Status != database.PurchaseStatusPaid {
		return nil, fmt.Errorf("%w: checkout %d has no payment_url", ErrInvalidInput, checkout.ID)
	}
	return &CreateResult{
		CheckoutID: checkout.ID,
		Provider:   checkout.Provider,
		Status:     checkout.Status,
		PaymentURL: url,
		Reused:     true,
	}, nil
}

// mapProviderToInvoiceType превращает значение из API в InvoiceType purchase-строки.
// Обратите внимание: в API и cabinet_checkout.provider — "yookassa" (официальное
// написание бренда), а в purchase.invoice_type — исторически "yookasa" (одна 's').
// Эта асимметрия закрепилась в миграциях бота и здесь мы её просто переводим.
func mapProviderToInvoiceType(provider string) (database.InvoiceType, error) {
	switch provider {
	case repository.CheckoutProviderYookassa:
		return database.InvoiceTypeYookasa, nil
	case repository.CheckoutProviderCryptoPay:
		return database.InvoiceTypeCrypto, nil
	case repository.CheckoutProviderTelegram:
		return database.InvoiceTypeTelegram, nil
	case repository.CheckoutProviderPlategaSBP:
		return database.InvoiceTypePlategaSBP, nil
	case repository.CheckoutProviderPlategaCards:
		return database.InvoiceTypePlategaCards, nil
	case repository.CheckoutProviderPlategaAcquiring:
		return database.InvoiceTypePlategaAcquiring, nil
	case repository.CheckoutProviderPlategaWorldwide:
		return database.InvoiceTypePlategaWorldwide, nil
	case repository.CheckoutProviderPlategaCrypto:
		return database.InvoiceTypePlategaCrypto, nil
	case repository.CheckoutProviderHeleket:
		return database.InvoiceTypeHeleket, nil
	default:
		return "", fmt.Errorf("%w: unknown provider %q", ErrInvalidInput, provider)
	}
}

// checkoutStatusForPurchase — маппинг purchase.status → cabinet_checkout.status.
// Возвращает пустую строку для статусов, которые не мапятся (например 'new').
func checkoutStatusForPurchase(status database.PurchaseStatus) string {
	switch status {
	case database.PurchaseStatusPaid:
		return repository.CheckoutStatusPaid
	case database.PurchaseStatusPending, database.PurchaseStatusNew:
		return repository.CheckoutStatusPending
	case database.PurchaseStatusCancel:
		return repository.CheckoutStatusFailed
	default:
		return ""
	}
}

// paymentURLFromPurchase возвращает URL оплаты в зависимости от провайдера,
// читая поля purchase, которые пишет PaymentService.createYookasaInvoice /
// createCryptoInvoice.
func paymentURLFromPurchase(p *database.Purchase) string {
	if p == nil {
		return ""
	}
	switch p.InvoiceType {
	case database.InvoiceTypeYookasa:
		if p.YookasaURL != nil {
			return *p.YookasaURL
		}
	case database.InvoiceTypeCrypto:
		if p.CryptoInvoiceLink != nil {
			return *p.CryptoInvoiceLink
		}
	case database.InvoiceTypeTelegram:
		if p.CryptoInvoiceLink != nil {
			return *p.CryptoInvoiceLink
		}
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		if p.PlategaURL != nil {
			return *p.PlategaURL
		}
	case database.InvoiceTypeHeleket:
		if p.HeleketURL != nil {
			return *p.HeleketURL
		}
	}
	return ""
}
