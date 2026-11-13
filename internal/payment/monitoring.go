package payment

import (
	"context"
	"log/slog"
	"time"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/database"
)

// События оплат для «Мониторинга». Пишутся всегда — в отличие от сообщений
// в группу платежей, которые админ может выключить.

func paidEventType(p *database.Purchase, expireBefore *time.Time) string {
	switch p.PurchaseKind {
	case database.PurchaseKindTariffUpgrade:
		return adminevents.PaymentTariffUpgrade
	case database.PurchaseKindExtraHwid:
		return adminevents.PaymentExtraHwid
	case database.PurchaseKindExtraTraffic:
		return adminevents.PaymentExtraTraffic
	}
	// Продление — подписка на момент оплаты ещё шла.
	paidAt := time.Now()
	if p.PaidAt != nil {
		paidAt = *p.PaidAt
	}
	if expireBefore != nil && expireBefore.After(paidAt) {
		return adminevents.PaymentSubscriptionRenew
	}
	return adminevents.PaymentSubscriptionNew
}

// purchasePayload — общее для оплаты и отмены: сумма, способ, тариф, срок.
func (s *PaymentService) purchasePayload(ctx context.Context, p *database.Purchase) map[string]any {
	payload := map[string]any{
		"amount":   p.Amount,
		"currency": p.Currency,
		"provider": string(p.InvoiceType),
		"kind":     string(p.PurchaseKind),
	}
	if p.Month > 0 {
		payload["months"] = p.Month
	}
	if p.TariffID != nil && *p.TariffID > 0 {
		payload["tariff_id"] = *p.TariffID
		if name := tariffDisplayName(ctx, s.tariffRepository, *p.TariffID); name != "" {
			payload["tariff"] = name
		}
	}
	if p.ExtraHwid > 0 {
		payload["devices"] = p.ExtraHwid
	}
	if p.TrafficPackBytes > 0 {
		payload["traffic_bytes"] = p.TrafficPackBytes
		if p.TrafficPackKind != nil {
			payload["traffic_kind"] = *p.TrafficPackKind
		}
	}
	if p.DiscountPercentApplied != nil && *p.DiscountPercentApplied > 0 {
		payload["discount"] = *p.DiscountPercentApplied
	}
	if p.IsEarlyDowngrade {
		payload["downgrade"] = true
	}
	return payload
}

// purchaseSource — оплата из кабинета или из бота.
func (s *PaymentService) purchaseSource(ctx context.Context, p *database.Purchase) string {
	if s.purchaseRepository == nil {
		return adminevents.SourceBot
	}
	cabinet, err := s.purchaseRepository.HasCabinetCheckoutForPurchase(ctx, p.ID)
	if err != nil {
		slog.Debug("admin events: cabinet checkout lookup", "purchase_id", p.ID, "error", err)
	}
	if cabinet {
		return adminevents.SourceCabinet
	}
	return adminevents.SourceBot
}

func (s *PaymentService) emitPurchasePaid(ctx context.Context, p *database.Purchase, expireBefore, expireAfter *time.Time) {
	if p == nil {
		return
	}
	payload := s.purchasePayload(ctx, p)
	if expireAfter != nil && !expireAfter.IsZero() {
		payload["expire_at"] = expireAfter.UTC().Format(time.RFC3339)
	}
	adminevents.Emit(adminevents.Event{
		Type: paidEventType(p, expireBefore), CustomerID: p.CustomerID,
		Source: s.purchaseSource(ctx, p), Payload: payload,
	})
}

// emitPurchaseCanceled — счёт закрыт без оплаты: отклонён банком, брошен или
// истёк. Причину провайдеры сюда не передают, поэтому в ленте это «не оплачено».
func (s *PaymentService) emitPurchaseCanceled(ctx context.Context, p *database.Purchase, reason string) {
	if p == nil {
		return
	}
	payload := s.purchasePayload(ctx, p)
	if reason != "" {
		payload["reason"] = reason
	}
	adminevents.Emit(adminevents.Event{
		Type: adminevents.PaymentFailed, CustomerID: p.CustomerID,
		Source: s.purchaseSource(ctx, p), Payload: payload,
	})
}
