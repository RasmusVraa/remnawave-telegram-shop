package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/loyalty"
	"remnawave-tg-shop-bot/internal/payment"
)

// TrafficPackCreateRequest — тело POST /cabinet/api/payments/traffic/checkout.
type TrafficPackCreateRequest struct {
	PackID         int64
	Provider       string
	YookassaMethod string
	IdempotencyKey string
}

// CreateTrafficPack создаёт счёт на пакет трафика. Цена берётся из пакета тарифа клиента (в запросе
// только id пакета); поддержаны те же способы оплаты, что при докупке устройств.
func (s *CheckoutService) CreateTrafficPack(ctx context.Context, accountID int64, req TrafficPackCreateRequest) (*CreateResult, error) {
	if err := s.validateHwidIdempotencyKey(req.IdempotencyKey); err != nil {
		return nil, err
	}
	if req.PackID <= 0 {
		return nil, fmt.Errorf("%w: bad pack_id", ErrInvalidInput)
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

	link, err := s.bootstrap.EnsureForAccount(ctx, accountID, "")
	if err != nil {
		return nil, fmt.Errorf("payments: bootstrap customer: %w", err)
	}
	customer, err := s.customers.FindById(ctx, link.CustomerID)
	if err != nil || customer == nil {
		return nil, fmt.Errorf("payments: load customer: %w", err)
	}

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
	providerCtx := s.withProviderOverrides(ctx, provider, req.YookassaMethod, returnURL, acc)

	// Скидки (лояльность, промокод) применяются к цене пакета так же, как к докупке устройств.
	discount := func(base int) (float64, *payment.PromoMeta) {
		amount, _, _, meta := s.applyCheckoutDiscounts(ctx, customer, invoiceType, base)
		return float64(amount), meta
	}
	paymentURL, purchaseID, err := s.payments.CreateTrafficPackPurchase(providerCtx, req.PackID, customer, invoiceType, discount)
	if err != nil {
		if errors.Is(err, payment.ErrTrafficPackUnavailable) {
			return nil, fmt.Errorf("%w: traffic pack unavailable", ErrInvalidInput)
		}
		return nil, fmt.Errorf("payments: create traffic pack purchase: %w", err)
	}
	if err := s.checkouts.AttachPurchase(ctx, checkout.ID, purchaseID, returnURL); err != nil {
		slog.Error("payments: traffic pack attach purchase failed", "checkout_id", checkout.ID, "purchase_id", purchaseID, "error", err)
	}
	return &CreateResult{CheckoutID: checkout.ID, Provider: provider, Status: repository.CheckoutStatusPending, PaymentURL: paymentURL}, nil
}

// TrafficPackPreviewResult — цена пакета с учётом скидок: то, что клиент увидит на странице оплаты и заплатит.
type TrafficPackPreviewResult struct {
	PackID             int64  `json:"pack_id"`
	Bytes              int64  `json:"bytes"`
	Kind               string `json:"kind"`
	AmountRub          int    `json:"amount_rub"`
	BaseAmountRub      int    `json:"base_amount_rub"`
	Currency           string `json:"currency"`
	LoyaltyDiscountPct int    `json:"loyalty_discount_pct,omitempty"`
	PromoDiscountPct   int    `json:"promo_discount_pct,omitempty"`
	TotalDiscountPct   int    `json:"total_discount_pct,omitempty"`
}

// PreviewTrafficPack считает цену пакета с теми же скидками (лояльность, промокод), что и счёт: иначе на странице
// оплаты была бы цена без скидки, а счёт выставлялся бы со скидкой. Для Telegram цена в звёздах.
func (s *CheckoutService) PreviewTrafficPack(ctx context.Context, accountID, packID int64, provider string) (*TrafficPackPreviewResult, error) {
	if packID <= 0 {
		return nil, fmt.Errorf("%w: bad pack_id", ErrInvalidInput)
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
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
	if err != nil || customer == nil {
		return nil, fmt.Errorf("payments: load customer: %w", err)
	}
	pack, base, err := s.payments.TrafficPackQuote(ctx, packID, customer, invoiceType)
	if err != nil {
		if errors.Is(err, payment.ErrTrafficPackUnavailable) {
			return nil, fmt.Errorf("%w: traffic pack unavailable", ErrInvalidInput)
		}
		return nil, err
	}
	final, loyPct, proPct, _ := s.applyCheckoutDiscounts(ctx, customer, invoiceType, base)
	out := &TrafficPackPreviewResult{
		PackID: pack.ID, Bytes: pack.Bytes, Kind: pack.Kind,
		AmountRub: final, BaseAmountRub: base, Currency: "RUB",
		LoyaltyDiscountPct: loyPct, PromoDiscountPct: proPct,
		TotalDiscountPct: loyalty.CombinedDiscountPercent(loyPct, proPct, config.LoyaltyMaxTotalDiscountPercent()),
	}
	if invoiceType == database.InvoiceTypeTelegram {
		out.Currency = "STARS"
	}
	return out, nil
}
