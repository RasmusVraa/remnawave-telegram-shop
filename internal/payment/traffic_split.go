package payment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/trafficquota"
	"remnawave-tg-shop-bot/utils"
)

// trafficSplit — зависимости раздельного трафика. nil — фича не подключена
// (тесты, режим classic): все хуки тогда ничего не делают.
type trafficSplit struct {
	repo  *database.TrafficSplitRepository
	quota *trafficquota.Service
}

// SetTrafficSplit подключает раздельный трафик к платежам. Вызывается один раз при старте.
func (s *PaymentService) SetTrafficSplit(repo *database.TrafficSplitRepository, quota *trafficquota.Service) {
	s.traffic = &trafficSplit{repo: repo, quota: quota}
}

// adjustProfileForTrafficSplit дополняет профиль тарифа перед обновлением панели:
//   - к лимиту обычного трафика прибавляются действующие пакеты (иначе продление их затрёт);
//   - сквад обхода, снятый за исчерпание лимита, не возвращается продлением — это делает воркер.
func (s PaymentService) adjustProfileForTrafficSplit(ctx context.Context, customer *database.Customer, tariff *database.Tariff, profile remnawave.TariffPaidProfile) remnawave.TariffPaidProfile {
	if s.traffic == nil || customer == nil || tariff == nil {
		return profile
	}
	if profile.TrafficLimitBytes > 0 {
		extra, err := s.traffic.repo.ActiveExtraBytes(ctx, customer.ID, database.TrafficKindMain, time.Now())
		if err != nil {
			slog.Error("traffic split: active extra", "customer_id", customer.ID, "error", err)
		} else {
			profile.TrafficLimitBytes += extra
		}
	}
	b, err := s.traffic.repo.GetTariffBypass(ctx, tariff.ID)
	if err != nil || !b.Enabled() {
		return profile
	}
	st, err := s.traffic.repo.GetState(ctx, customer.ID)
	if err != nil || st == nil || !st.SquadRemoved {
		return profile
	}
	if squads, _ := database.ParseSquadUUIDList(b.SquadUUIDs); len(squads) > 0 {
		profile.ExcludeSquadUUIDs = squads
	}
	return profile
}

// snapshotTrafficSplit запоминает у клиента настройки обхода тарифа, который он только что
// купил или продлил. Изменение тарифа в админке касается только таких будущих покупок:
// у тех, кто уже на тарифе, остаётся то, что было при их последней оплате.
func (s PaymentService) snapshotTrafficSplit(ctx context.Context, customerID int64, tariffID *int64, user *remnawave.User) {
	if s.traffic == nil || tariffID == nil || *tariffID <= 0 {
		return
	}
	// Воркер ищет клиентов по id профиля в панели, а он сохраняется лениво (при заходе в кабинет):
	// запоминаем его сразу, пока профиль у нас в руках.
	if user != nil && user.ID > 0 && s.customerRepository != nil {
		if err := s.customerRepository.SetRemnawaveIdentity(ctx, customerID, user.ID, user.ShortUUID); err != nil {
			slog.Warn("traffic split: remember panel identity", "customer_id", customerID, "error", err)
		}
	}
	b, err := s.traffic.repo.GetTariffBypass(ctx, *tariffID)
	if err != nil {
		slog.Error("traffic split: read tariff bypass", "tariff_id", *tariffID, "error", err)
		return
	}
	if err := s.traffic.repo.SaveSnapshot(ctx, customerID, tariffID, b, time.Now()); err != nil {
		slog.Error("traffic split: save snapshot", "customer_id", customerID, "error", err)
		return
	}
}

// processTrafficPackPurchase выдаёт оплаченный пакет трафика: запись о пакете (до 1-го числа
// следующего месяца), мгновенное применение в панели, чек и уведомления.
//
// Порядок важен: пакет записывается до MarkAsPaid и привязан к purchase_id уникальным индексом.
// Упадёт MarkAsPaid — повтор не выдаст пакет дважды; упадёт запись — покупка не станет оплаченной.
func (s PaymentService) processTrafficPackPurchase(ctx context.Context, purchase *database.Purchase, customer *database.Customer) error {
	if s.traffic == nil {
		return fmt.Errorf("traffic packs are not configured")
	}
	if purchase.TrafficPackKind == nil || purchase.TrafficPackBytes <= 0 {
		return fmt.Errorf("traffic pack purchase %d has no pack data", purchase.ID)
	}
	kind := *purchase.TrafficPackKind
	if err := s.trafficPackApplicable(ctx, customer, kind); err != nil {
		// Деньги уже получены, поэтому пакет всё равно выдаётся; пишем в лог, чтобы админ мог разобраться.
		slog.Warn("traffic pack paid but no longer applicable", "customer_id", customer.ID, "kind", kind, "purchase_id", purchase.ID, "reason", err.Error())
	}
	now := time.Now().UTC()
	pid := purchase.ID
	if _, err := s.traffic.repo.AddExtra(ctx, database.TrafficExtra{
		CustomerID: customer.ID, Kind: kind, Bytes: purchase.TrafficPackBytes, PurchaseID: &pid,
		ValidUntil: database.TrafficPeriodEnd(now),
	}); err != nil {
		return err
	}
	if err := s.purchaseRepository.MarkAsPaid(ctx, purchase.ID); err != nil {
		return err
	}
	purchase.Status = database.PurchaseStatusPaid
	purchase.PaidAt = &now

	// Применить в панели сразу: обычный трафик — поднять лимит, обход — вернуть сквад.
	// Покупка уже оплачена, поэтому ошибку не возвращаем (повтор вебхука её не обработает), а повторяем.
	utils.Go("traffic-pack-apply", func() { s.applyTrafficPackWithRetry(customer.ID, kind) })

	if s.receiptQueue.Enabled() && invoiceUsesMoynalogReceipt(purchase) {
		desc := "Докупка трафика"
		if !database.InvoiceTypeIsPlategaCryptoOrCryptopay(purchase.InvoiceType) {
			desc += invoiceDescriptionPlategaSuffix(purchase.InvoiceType)
		}
		s.receiptQueue.Submit(ctx, purchase, desc)
	}

	lang := customer.Language
	if lang == "" {
		lang = "ru"
	}
	label := s.translation.GetText(lang, "traffic_kind_main")
	if kind == database.TrafficKindBypass {
		label = config.TrafficBypassLabel()
	}
	size := trafficquota.FormatGB(purchase.TrafficPackBytes, lang)
	until := cabinetnotify.Date(database.TrafficPeriodEnd(now))

	cabinetnotify.Notify(ctx, customer.ID, customer.Language, cabinetnotify.Event{
		Kind:      database.CabinetNotificationPayment,
		TitleKey:  "traffic_pack_paid_title",
		BodyKey:   "traffic_pack_paid_body",
		Args:      []any{size, label, until},
		DedupeKey: fmt.Sprintf("traffic-pack:%d", purchase.ID),
	})
	if !skipTelegramCustomerDM(customer) {
		text := fmt.Sprintf(s.translation.GetText(lang, "traffic_pack_paid_dm"), size, label, until)
		if _, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: customer.TelegramID, Text: text, ParseMode: models.ParseModeHTML}); err != nil {
			slog.Warn("payment DM not delivered", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}
	s.clearPromoDiscountIfUsed(ctx, purchase, customer)
	s.applyLoyaltyXPAfterPayment(ctx, purchase, customer)
	s.tryNotifyPurchasePaid(ctx, purchase, customer, customer.ExpireAt, customer.ExpireAt)
	return nil
}

func (s PaymentService) applyTrafficPackWithRetry(customerID int64, kind string) {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = s.traffic.quota.OnPackGranted(ctx, customerID, kind)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(time.Duration(attempt*3) * time.Second)
	}
	slog.Error("traffic pack: apply to panel failed, воркер подхватит при ближайшем проходе (обход) или нужна ручная правка лимита (обычный)",
		"customer_id", customerID, "kind", kind, "error", err)
}

type trafficPackCtxKey struct{}

// receiptDescriptionCtx — описание счёта для ЮKassa, Platega и «Моего налога»: для пакета трафика
// (он едет в контексте) это «Докупка трафика», для остального — обычное описание подписки или устройств.
func receiptDescriptionCtx(ctx context.Context, months, extraHwid int, extras *TariffPurchaseExtras, invoiceType database.InvoiceType) string {
	if _, ok := ctx.Value(trafficPackCtxKey{}).(trafficPackCtx); ok {
		desc := "Докупка трафика"
		if !database.InvoiceTypeIsPlategaCryptoOrCryptopay(invoiceType) {
			desc += invoiceDescriptionPlategaSuffix(invoiceType)
		}
		return desc
	}
	return buildRubReceiptDescription(months, extraHwid, extras, invoiceType)
}

// trafficPackCtx — пакет, который покупается через общий конвейер счетов.
type trafficPackCtx struct {
	ID    int64
	Kind  string
	Bytes int64
}

// applyTrafficPackFromCtx помечает покупку как пакет трафика, если пакет передан в контексте.
// Конвейер создания счетов у всех способов оплаты общий и знает только про подписку и места
// устройств, поэтому пакет едет контекстом, а не новым параметром у каждого создателя счёта.
func applyTrafficPackFromCtx(ctx context.Context, pur *database.Purchase) {
	pack, ok := ctx.Value(trafficPackCtxKey{}).(trafficPackCtx)
	if !ok || pur == nil {
		return
	}
	id, kind := pack.ID, pack.Kind
	pur.PurchaseKind = database.PurchaseKindExtraTraffic
	pur.TrafficPackID = &id
	pur.TrafficPackKind = &kind
	pur.TrafficPackBytes = pack.Bytes
}

// TrafficPackStars — цена пакета в звёздах: введённая админом, а если поле пустое — из рублей по
// курсу RUB_PER_STAR (та же формула, что у цен тарифов). 0 — посчитать нечем.
func TrafficPackStars(rub int, stars *int) int {
	if stars != nil && *stars > 0 {
		return *stars
	}
	if rate := config.RubPerStar(); rate > 0 && rub > 0 {
		return int(math.Ceil(float64(rub) / rate))
	}
	return 0
}

// trafficPackApplicable проверяет, что пакет данного вида подействует на клиента: докупать безлимит
// или обход, которого у клиента нет, бессмысленно.
func (s PaymentService) trafficPackApplicable(ctx context.Context, customer *database.Customer, kind string) error {
	switch kind {
	case database.TrafficKindMain:
		var tariff *database.Tariff
		if customer.CurrentTariffID != nil && s.tariffRepository != nil {
			t, err := s.tariffRepository.GetByID(ctx, *customer.CurrentTariffID)
			if err != nil {
				return err
			}
			tariff = t
		}
		if trafficquota.MainLimitForTariff(tariff) <= 0 {
			return fmt.Errorf("regular traffic is unlimited")
		}
		return nil
	case database.TrafficKindBypass:
		if !config.TrafficSplitEnabled() {
			return fmt.Errorf("traffic split is disabled")
		}
		st, err := s.traffic.repo.GetState(ctx, customer.ID)
		if err != nil {
			return err
		}
		if st == nil || st.LimitBytes <= 0 {
			return fmt.Errorf("customer has no bypass limit")
		}
		return nil
	}
	return fmt.Errorf("unknown pack kind %q", kind)
}

// ErrTrafficPackUnavailable — пакет нельзя купить (выключен, чужой тариф, продажа отключена).
var ErrTrafficPackUnavailable = fmt.Errorf("traffic pack is not available")

// CreateTrafficPackPurchase создаёт счёт на пакет трафика. Цена берётся из пакета тарифа клиента,
// а не из запроса: клиент передаёт только id пакета.
// TrafficPackQuote возвращает пакет клиента и его цену (рубли или звёзды для Telegram) с теми же проверками,
// что при создании счёта: пакет включён, принадлежит текущему тарифу, подписка активна, пакет применим.
// Скидки сюда не входят: их считает вызывающий (кабинет, бот).
func (s PaymentService) TrafficPackQuote(ctx context.Context, packID int64, customer *database.Customer, invoiceType database.InvoiceType) (*database.TrafficPack, int, error) {
	if s.traffic == nil || !config.TrafficPacksEnabled() || customer == nil {
		return nil, 0, ErrTrafficPackUnavailable
	}
	pack, err := s.traffic.repo.GetPack(ctx, packID)
	if err != nil {
		return nil, 0, err
	}
	if pack == nil || !pack.IsActive || customer.CurrentTariffID == nil || *customer.CurrentTariffID != pack.TariffID {
		return nil, 0, ErrTrafficPackUnavailable
	}
	if customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now()) {
		return nil, 0, ErrTrafficPackUnavailable
	}
	if pack.AmountRub <= 0 {
		return nil, 0, ErrTrafficPackUnavailable
	}
	if err := s.trafficPackApplicable(ctx, customer, pack.Kind); err != nil {
		return nil, 0, ErrTrafficPackUnavailable
	}
	base := pack.AmountRub
	if invoiceType == database.InvoiceTypeTelegram {
		if base = TrafficPackStars(pack.AmountRub, pack.AmountStars); base <= 0 {
			return nil, 0, ErrTrafficPackUnavailable
		}
	}
	return pack, base, nil
}

// CreateTrafficPackPurchase создаёт счёт на пакет трафика. Цена берётся из пакета тарифа клиента,
// а не из запроса: клиент передаёт только id пакета.
func (s PaymentService) CreateTrafficPackPurchase(ctx context.Context, packID int64, customer *database.Customer, invoiceType database.InvoiceType, meta func(base int) (amount float64, pm *PromoMeta)) (url string, purchaseId int64, err error) {
	pack, base, err := s.TrafficPackQuote(ctx, packID, customer, invoiceType)
	if err != nil {
		return "", 0, err
	}
	amount, pm := float64(base), (*PromoMeta)(nil)
	if meta != nil {
		amount, pm = meta(base)
	}
	ctx = context.WithValue(ctx, trafficPackCtxKey{}, trafficPackCtx{ID: pack.ID, Kind: pack.Kind, Bytes: pack.Bytes})
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, 0, 0, customer, pm, nil, nil)
	case database.InvoiceTypeYookasa:
		return s.createYookasaInvoice(ctx, amount, 0, 0, customer, pm, nil, nil)
	case database.InvoiceTypePlatega, database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		return s.createPlategaInvoice(ctx, amount, 0, 0, customer, pm, nil, nil, invoiceType)
	case database.InvoiceTypeHeleket:
		return s.createHeleketInvoice(ctx, amount, 0, 0, customer, pm, nil, nil)
	case database.InvoiceTypeTelegram:
		return s.createTelegramInvoice(ctx, amount, 0, 0, customer, pm, nil, nil)
	default:
		return "", 0, fmt.Errorf("unsupported invoice type for traffic pack: %s", invoiceType)
	}
}
