package promo

import (
	"context"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/database"
)

// emitPromoEvent — промокод для «Мониторинга»: удачная активация — важное
// событие, неудачная — мелочь (видна с «Нажатиями в боте» и в истории клиента).
// По неудачным попыткам видно перебор кодов.
func emitPromoEvent(ctx context.Context, customer *database.Customer, rawCode string, res *ActivateResult, err error) {
	if customer == nil {
		return
	}
	code := NormalizeCode(rawCode)
	if len(code) > 64 {
		code = code[:64]
	}
	source := adminevents.SourceFrom(ctx, adminevents.SourceBot)
	if err != nil || res == nil {
		if !IsPromoValidationErr(err) {
			return
		}
		adminevents.Emit(adminevents.Event{
			Type: adminevents.PromoInvalid, Level: adminevents.LevelBot, CustomerID: customer.ID,
			Source: source, Payload: map[string]any{"code": code},
		})
		return
	}
	payload := map[string]any{"code": code, "promo_type": res.Type}
	if res.SubscriptionDays > 0 {
		payload["days"] = res.SubscriptionDays
	}
	if res.TrialDays > 0 {
		payload["trial_days"] = res.TrialDays
	}
	if res.ExtraHwidDelta > 0 {
		payload["devices"] = res.ExtraHwidDelta
	}
	if res.DiscountPercent > 0 {
		payload["discount"] = res.DiscountPercent
	}
	adminevents.Emit(adminevents.Event{
		Type: adminevents.PromoActivated, CustomerID: customer.ID, Source: source, Payload: payload,
	})
}
