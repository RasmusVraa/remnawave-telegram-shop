package payment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// TariffCheckoutKind классификация сценария оплаты тарифа (см. .cursor/promts/tariffs.md §7, §9, §15).
type TariffCheckoutKind int

const (
	// TariffCheckoutNew — нет активной подписки или срок истёк: полная цена нового периода.
	TariffCheckoutNew TariffCheckoutKind = iota
	// TariffCheckoutRenewSame — тот же тариф или равная цена другого тарифа: продление от expire_at.
	TariffCheckoutRenewSame
	// TariffCheckoutUpgrade — более дорогой тариф при активной подписке: полная цена выбранного периода + бонусные дни (остаток старого пересчитан по «дневной» цене нового); срок считается от момента оплаты.
	TariffCheckoutUpgrade
	// TariffCheckoutDowngrade — более дешёвый тариф при активной подписке: полная цена, досрочный переход; остаток дорогого тарифа пересчитывается в дни дешёвого, срок от момента оплаты.
	TariffCheckoutDowngrade
)

// ErrTariffUnavailable — тариф не существует или снят с продажи.
var ErrTariffUnavailable = errors.New("tariff is not available for purchase")

// TariffPurchaseExtras задаёт поля строки purchase для режима tariffs (nil = обычная подписка).
type TariffPurchaseExtras struct {
	Kind             database.PurchaseKind
	IsEarlyDowngrade bool
}

// ResolveTariffPurchase определяет сумму к оплате и вид покупки. invoiceStars — оплата в Telegram Stars.
func ResolveTariffPurchase(
	ctx context.Context,
	tr *database.TariffRepository,
	customer *database.Customer,
	newTariffID int64,
	months int,
	invoiceStars bool,
) (kind TariffCheckoutKind, amount int, purchaseKind database.PurchaseKind, isEarlyDowngrade bool, err error) {
	purchaseKind = database.PurchaseKindSubscription
	// Снятый с продажи тариф купить нельзя: раньше выбор шёл по id из кнопки
	// или запроса без проверки is_active. Продлить свой текущий — можно, иначе
	// подписчики снятого тарифа остались бы без продления.
	if customer.CurrentTariffID == nil || *customer.CurrentTariffID != newTariffID {
		t, err := tr.GetByID(ctx, newTariffID)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		if t == nil || !t.IsActive {
			return 0, 0, purchaseKind, false, fmt.Errorf("%w: tariff %d", ErrTariffUnavailable, newTariffID)
		}
	}
	tpNew, err := tr.GetPrice(ctx, newTariffID, months)
	if err != nil {
		return 0, 0, purchaseKind, false, err
	}
	if tpNew == nil || tpNew.AmountRub <= 0 {
		return 0, 0, purchaseKind, false, fmt.Errorf("no tariff_price for tariff %d months %d", newTariffID, months)
	}

	now := time.Now().UTC()
	active := customer.ExpireAt != nil && customer.ExpireAt.After(now)
	if !active {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutNew, amount, purchaseKind, false, nil
	}

	// Активная подписка
	if customer.CurrentTariffID == nil || *customer.CurrentTariffID == 0 {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutRenewSame, amount, purchaseKind, false, nil
	}

	if *customer.CurrentTariffID == newTariffID {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutRenewSame, amount, purchaseKind, false, nil
	}

	tpOld, err := tr.GetPrice(ctx, *customer.CurrentTariffID, months)
	if err != nil {
		return 0, 0, purchaseKind, false, err
	}
	if tpOld == nil || tpOld.AmountRub <= 0 {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutRenewSame, amount, purchaseKind, false, nil
	}

	if tpNew.AmountRub > tpOld.AmountRub {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutUpgrade, amount, database.PurchaseKindTariffUpgrade, false, nil
	}

	if tpNew.AmountRub < tpOld.AmountRub {
		amount, err = pickTariffAmount(tpNew, invoiceStars)
		if err != nil {
			return 0, 0, purchaseKind, false, err
		}
		return TariffCheckoutDowngrade, amount, purchaseKind, true, nil
	}

	// Равная цена в ₽ — смена тарифа «вбок», продление как при продлении
	amount, err = pickTariffAmount(tpNew, invoiceStars)
	if err != nil {
		return 0, 0, purchaseKind, false, err
	}
	return TariffCheckoutRenewSame, amount, purchaseKind, false, nil
}

func pickTariffAmount(tp *database.TariffPrice, invoiceStars bool) (int, error) {
	if invoiceStars {
		if tp.AmountStars != nil && *tp.AmountStars > 0 {
			return *tp.AmountStars, nil
		}
		if config.RubPerStar() > 0 {
			return int(math.Ceil(float64(tp.AmountRub) / config.RubPerStar())), nil
		}
		return 0, fmt.Errorf("stars price not set for tariff_price and RUB_PER_STAR missing")
	}
	return tp.AmountRub, nil
}

// UpgradeCalendarDaysRemainingCeil — сколько полных календарных дней (вверх) осталось до expire_at.
func UpgradeCalendarDaysRemainingCeil(customer *database.Customer, now time.Time) int {
	if customer == nil || customer.ExpireAt == nil {
		return 0
	}
	dur := customer.ExpireAt.Sub(now)
	if dur <= 0 {
		return 0
	}
	return int(math.Ceil(dur.Seconds() / 86400))
}

// ComputeUpgradeBonusDays считает дни на новом тарифе, эквивалентные остатку по старому: оставшееся время × дневная цена старого пакета
// пересчитывается в дни по дневной цене нового (тот же выбранный период months). Используется и при апгрейде, и при досрочном даунгрейде.
func ComputeUpgradeBonusDays(customer *database.Customer, tpOld, tpNew *database.TariffPrice, months int, now time.Time) int {
	if customer == nil || customer.ExpireAt == nil || !customer.ExpireAt.After(now) {
		return 0
	}
	if tpOld == nil || tpNew == nil || months <= 0 {
		return 0
	}
	if tpOld.AmountRub <= 0 || tpNew.AmountRub <= 0 {
		return 0
	}
	dim := config.DaysInMonth()
	if dim <= 0 {
		dim = 30
	}
	periodDays := months * dim
	if periodDays <= 0 {
		return 0
	}
	oldDaily := float64(tpOld.AmountRub) / float64(periodDays)
	newDaily := float64(tpNew.AmountRub) / float64(periodDays)
	if newDaily <= 0 {
		return 0
	}
	dur := customer.ExpireAt.Sub(now)
	if dur <= 0 {
		return 0
	}
	daysLeft := dur.Seconds() / 86400
	creditRub := daysLeft * oldDaily
	bonus := creditRub / newDaily
	if bonus <= 0 {
		return 0
	}
	return int(math.Round(bonus))
}

