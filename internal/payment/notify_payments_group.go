package payment

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// Должен совпадать с handler.CallbackPaymentsNotifyUserOpenPrefix.
const paymentsNotifyUserOpenCallbackPrefix = "pnu"

// htmlCode — фрагмент для Telegram ParseMode HTML: удобное копирование одним тапом.
func htmlCode(s string) string {
	return "<code>" + html.EscapeString(s) + "</code>"
}

func invoiceTypeTitle(t database.InvoiceType) string {
	switch t {
	case database.InvoiceTypeYookasa:
		return "YooKassa"
	case database.InvoiceTypeCrypto:
		return "Crypto Pay"
	case database.InvoiceTypeTelegram:
		return "Telegram Stars"
	case database.InvoiceTypeTribute:
		return "Tribute"
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		return "Platega"
	case database.InvoiceTypeHeleket:
		return "Heleket"
	default:
		return string(t)
	}
}

func ptrTimeIfValid(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func formatDateDMYUTC(t *time.Time) string {
	if t == nil {
		return "—"
	}
	u := t.UTC()
	return fmt.Sprintf("%02d.%02d.%04d", u.Day(), u.Month(), u.Year())
}

func formatClockUTC(t time.Time) string {
	u := t.UTC()
	return fmt.Sprintf("%02d:%02d:%02d UTC", u.Hour(), u.Minute(), u.Second())
}

func telegramUsernameRaw(c *database.Customer) string {
	if c == nil || c.TelegramUsername == nil {
		return ""
	}
	u := strings.TrimSpace(*c.TelegramUsername)
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "@") {
		return "@" + u
	}
	return u
}

func paymentLinkBlock(p *database.Purchase) string {
	if p == nil {
		return "🔗 платёж: —"
	}
	switch p.InvoiceType {
	case database.InvoiceTypeYookasa:
		if p.YookasaID != nil && *p.YookasaID != uuid.Nil {
			return "🔗 платёж: " + htmlCode(p.YookasaID.String())
		}
	case database.InvoiceTypeCrypto:
		if p.CryptoInvoiceID != nil {
			line := "🔗 платёж: " + htmlCode(strconv.FormatInt(*p.CryptoInvoiceID, 10))
			if p.CryptoInvoiceLink != nil && strings.TrimSpace(*p.CryptoInvoiceLink) != "" {
				line += "\n   " + htmlCode(strings.TrimSpace(*p.CryptoInvoiceLink))
			}
			return line
		}
	case database.InvoiceTypeTelegram:
		if p.CryptoInvoiceLink != nil && strings.TrimSpace(*p.CryptoInvoiceLink) != "" {
			return "🔗 счёт Stars:\n   " + htmlCode(strings.TrimSpace(*p.CryptoInvoiceLink))
		}
	case database.InvoiceTypePlategaSBP, database.InvoiceTypePlategaCards, database.InvoiceTypePlategaAcquiring,
		database.InvoiceTypePlategaWorldwide, database.InvoiceTypePlategaCrypto:
		if p.PlategaID != nil && strings.TrimSpace(*p.PlategaID) != "" {
			return "🔗 платёж Platega: " + htmlCode(strings.TrimSpace(*p.PlategaID))
		}
	case database.InvoiceTypeHeleket:
		if p.HeleketID != nil && strings.TrimSpace(*p.HeleketID) != "" {
			return "🔗 платёж Heleket: " + htmlCode(strings.TrimSpace(*p.HeleketID))
		}
	}
	return "🔗 платёж: —"
}

func cabinetWebHint(cabinet bool, c *database.Customer) string {
	src := "бот"
	if cabinet {
		src = "кабинет"
	}
	var tags []string
	if c != nil && c.IsWebOnly {
		tags = append(tags, "web-only")
	}
	if c != nil && utils.IsSyntheticTelegramID(c.TelegramID) {
		tags = append(tags, "synthetic tg")
	}
	if len(tags) == 0 {
		return "🌐 " + src
	}
	return "🌐 " + src + " · " + strings.Join(tags, " · ")
}

func customerNotifyLineHTML(c *database.Customer) string {
	if c == nil {
		return ""
	}
	tgID := htmlCode(strconv.FormatInt(c.TelegramID, 10))
	userPart := "(нет username)"
	if u := telegramUsernameRaw(c); u != "" {
		userPart = htmlCode(u)
	}
	return fmt.Sprintf("👤 customer #%d · tg %s · %s\n", c.ID, tgID, userPart)
}

func amountPeriodLine(p *database.Purchase) string {
	if p == nil {
		return "💳 —"
	}
	cur := strings.TrimSpace(p.Currency)
	if cur == "" {
		cur = "RUB"
	}
	amt := p.Amount
	if p.InvoiceType == database.InvoiceTypeTelegram {
		if cur == "" {
			cur = "XTR"
		}
		return fmt.Sprintf("💳 %.0f ⭐ · %s", amt, cur)
	}
	if p.PurchaseKind == database.PurchaseKindExtraHwid || p.ExtraHwid > 0 && p.Month <= 0 {
		return fmt.Sprintf("💳 %.2f %s · доп. устройства", amt, cur)
	}
	if p.Month > 0 {
		return fmt.Sprintf("💳 %.2f %s · %d мес", amt, cur, p.Month)
	}
	return fmt.Sprintf("💳 %.2f %s", amt, cur)
}

func promoLine(p *database.Purchase) string {
	if p == nil {
		return ""
	}
	hasPromo := p.PromoCodeID != nil && *p.PromoCodeID > 0
	hasDisc := p.DiscountPercentApplied != nil && *p.DiscountPercentApplied > 0
	if !hasPromo && !hasDisc {
		return ""
	}
	var b strings.Builder
	b.WriteString("🏷 промо:")
	if hasPromo {
		b.WriteString(fmt.Sprintf(" #%d", *p.PromoCodeID))
	}
	if hasDisc {
		b.WriteString(fmt.Sprintf(" · −%d%%", *p.DiscountPercentApplied))
	}
	return b.String()
}

func loyaltyLine(loyaltyPct int) string {
	if loyaltyPct <= 0 {
		return ""
	}
	return fmt.Sprintf("🎯 лояльность: −%d%%", loyaltyPct)
}

func tariffDisplayName(ctx context.Context, repo *database.TariffRepository, tariffID int64) string {
	if repo == nil || tariffID <= 0 {
		return ""
	}
	t, err := repo.GetByID(ctx, tariffID)
	if err != nil || t == nil {
		return ""
	}
	if t.Name == nil {
		return ""
	}
	name := strings.TrimSpace(*t.Name)
	if name == "" {
		return ""
	}
	return name
}

func tariffLine(ctx context.Context, p *database.Purchase, repo *database.TariffRepository) string {
	if p == nil || p.TariffID == nil || *p.TariffID <= 0 {
		return "📦 " + string(p.PurchaseKind) + fmt.Sprintf(" · HWID +%d", p.ExtraHwid)
	}
	if name := tariffDisplayName(ctx, repo, *p.TariffID); name != "" {
		return fmt.Sprintf("📦 %s · HWID +%d · тариф «%s»", p.PurchaseKind, p.ExtraHwid, html.EscapeString(name))
	}
	return fmt.Sprintf("📦 %s · HWID +%d · тариф #%d", p.PurchaseKind, p.ExtraHwid, *p.TariffID)
}

func durationPaidLine(p *database.Purchase) string {
	if p == nil {
		return "⏱️ счёт: — → оплачен: —"
	}
	if p.PaidAt == nil {
		return "⏱️ счёт: " + formatClockUTC(p.CreatedAt) + " → оплачен: —"
	}
	d := p.PaidAt.Sub(p.CreatedAt)
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("⏱️ счёт: %s → оплачен: %s · %d с в пути",
		formatClockUTC(p.CreatedAt),
		formatClockUTC(*p.PaidAt),
		int(d.Round(time.Second).Seconds()),
	)
}

func durationCancelLine(p *database.Purchase) string {
	if p == nil {
		return "⏱️ счёт: —"
	}
	return "⏱️ счёт: " + formatClockUTC(p.CreatedAt) + " (отмена, без оплаты)"
}

func expireChangeLine(before, after *time.Time) string {
	return fmt.Sprintf("📅 подписка до: было %s → стало %s", formatDateDMYUTC(before), formatDateDMYUTC(after))
}

// providerExtrasFromCtx — доп. строки из SuccessfulPayment (Stars) или ответа CryptoPay, если провайдер включён и метаданные переданы в ctx.
func providerExtrasFromCtx(ctx context.Context, p *database.Purchase) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	switch p.InvoiceType {
	case database.InvoiceTypeTelegram:
		if !config.IsTelegramStarsEnabled() {
			return ""
		}
		m, ok := StarsNotifyMetaFromCtx(ctx)
		if !ok {
			return ""
		}
		b.WriteString("\n\n⭐ Stars (Telegram)\n")
		if id := strings.TrimSpace(m.TelegramPaymentChargeID); id != "" {
			b.WriteString("tg_charge_id: " + htmlCode(id) + "\n")
		}
		if id := strings.TrimSpace(m.ProviderPaymentChargeID); id != "" {
			b.WriteString("provider_charge_id: " + htmlCode(id) + "\n")
		}
		b.WriteString(fmt.Sprintf("successful_payment: %d %s", m.TotalAmount, html.EscapeString(strings.TrimSpace(m.Currency))))
	case database.InvoiceTypeCrypto:
		if !config.IsCryptoPayEnabled() {
			return ""
		}
		m, ok := CryptoNotifyMetaFromCtx(ctx)
		if !ok {
			return ""
		}
		b.WriteString("\n\n₿ CryptoPay\n")
		if h := strings.TrimSpace(m.Hash); h != "" {
			b.WriteString("hash: " + htmlCode(h) + "\n")
		}
		if st := strings.TrimSpace(m.Status); st != "" {
			b.WriteString("status: " + html.EscapeString(st) + "\n")
		}
		if strings.TrimSpace(m.CurrencyType) != "" || strings.TrimSpace(m.Asset) != "" {
			b.WriteString(fmt.Sprintf("currency: %s · asset: %s\n", html.EscapeString(strings.TrimSpace(m.CurrencyType)), html.EscapeString(strings.TrimSpace(m.Asset))))
		}
		if strings.TrimSpace(m.PaidAsset) != "" || strings.TrimSpace(m.PaidAmount) != "" {
			b.WriteString(fmt.Sprintf("оплачено: %s %s\n", html.EscapeString(strings.TrimSpace(m.PaidAmount)), html.EscapeString(strings.TrimSpace(m.PaidAsset))))
		}
		if strings.TrimSpace(m.FeeAmount) != "" {
			b.WriteString("комиссия: " + html.EscapeString(strings.TrimSpace(m.FeeAmount)) + "\n")
		}
		if u := strings.TrimSpace(m.PayUrl); u != "" {
			b.WriteString("pay_url: " + htmlCode(u) + "\n")
		}
		if u := strings.TrimSpace(m.BotInvoiceUrl); u != "" && u != strings.TrimSpace(m.PayUrl) {
			b.WriteString("bot_invoice_url: " + htmlCode(u) + "\n")
		}
	default:
		return ""
	}
	return b.String()
}

func buildPaidGroupMessage(ctx context.Context, p *database.Purchase, c *database.Customer, cabinet bool, expireBefore, expireAfter *time.Time, tariffRepo *database.TariffRepository, loyaltyPct int) string {
	var b strings.Builder
	b.WriteString("✅ Оплата\n\n")
	b.WriteString(fmt.Sprintf("🧾 #%d · paid · %s\n", p.ID, invoiceTypeTitle(p.InvoiceType)))
	b.WriteString(amountPeriodLine(p) + "\n")
	b.WriteString(tariffLine(ctx, p, tariffRepo) + "\n")
	if ll := loyaltyLine(loyaltyPct); ll != "" {
		b.WriteString(ll + "\n")
	}
	if pl := promoLine(p); pl != "" {
		b.WriteString(pl + "\n")
	}
	b.WriteString("\n")
	b.WriteString(durationPaidLine(p) + "\n")
	b.WriteString(paymentLinkBlock(p) + "\n\n")
	if c != nil {
		b.WriteString(customerNotifyLineHTML(c))
	}
	b.WriteString(cabinetWebHint(cabinet, c) + "\n")
	b.WriteString(expireChangeLine(expireBefore, expireAfter))
	b.WriteString(providerExtrasFromCtx(ctx, p))
	return b.String()
}

func buildCancelGroupMessage(ctx context.Context, p *database.Purchase, c *database.Customer, cabinet bool, tariffRepo *database.TariffRepository, loyaltyPct int) string {
	var b strings.Builder
	b.WriteString("❌ Счёт отменён\n\n")
	b.WriteString(fmt.Sprintf("🧾 #%d · cancel · %s\n", p.ID, invoiceTypeTitle(p.InvoiceType)))
	b.WriteString(amountPeriodLine(p) + "\n")
	b.WriteString(tariffLine(ctx, p, tariffRepo) + "\n")
	if ll := loyaltyLine(loyaltyPct); ll != "" {
		b.WriteString(ll + "\n")
	}
	if pl := promoLine(p); pl != "" {
		b.WriteString(pl + "\n")
	}
	b.WriteString("\n")
	b.WriteString(durationCancelLine(p) + "\n")
	b.WriteString(paymentLinkBlock(p) + "\n\n")
	if c != nil {
		b.WriteString(customerNotifyLineHTML(c))
	}
	b.WriteString(cabinetWebHint(cabinet, c))
	return b.String()
}

// NotifyHeleketNeedsAttention зовёт админа на счёт Heleket, который нельзя
// закрыть автоматически: деньги в крипте необратимы, поэтому вместо догадок
// (зачесть? отменить?) показываем ситуацию человеку.
func (s *PaymentService) NotifyHeleketNeedsAttention(ctx context.Context, purchase *database.Purchase, heleketUUID, status, reason string) {
	if s == nil || purchase == nil {
		return
	}
	var b strings.Builder
	b.WriteString("⚠️ <b>Heleket: нужен ручной разбор</b>\n\n")
	b.WriteString(reason)
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("🧾 покупка #%d, статус в базе: %s\n", purchase.ID, htmlCode(string(purchase.Status))))
	b.WriteString("💠 статус в Heleket: " + htmlCode(status) + "\n")
	if strings.TrimSpace(heleketUUID) != "" {
		b.WriteString("🔗 платёж Heleket: " + htmlCode(strings.TrimSpace(heleketUUID)) + "\n")
	}
	b.WriteString(amountPeriodLine(purchase))
	b.WriteString("\n\nАвтозачисления не было — проверьте и закройте вручную.")

	s.sendPaymentsGroupHTML(ctx, b.String(), s.paymentsNotifyToUserReplyMarkup(purchase.CustomerID))
}

func (s *PaymentService) sendPaymentsGroupHTML(ctx context.Context, text string, replyMarkup models.ReplyMarkup) {
	if s.telegramBot == nil || !config.PaymentsNotifyEnabled() {
		return
	}
	chatID := config.PaymentsNotifyChatID()
	if chatID == 0 {
		slog.Warn("payments notify: PAYMENTS_NOTIFY_CHAT_ID пуст — пропуск отправки")
		return
	}
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if replyMarkup != nil {
		params.ReplyMarkup = replyMarkup
	}
	if tid := config.PaymentsNotifyMessageThreadID(); tid > 0 {
		params.MessageThreadID = tid
	}
	if _, err := s.telegramBot.SendMessage(ctx, params); err != nil {
		slog.Warn("payments notify: не удалось отправить в группу", "error", err)
	}
}

func (s *PaymentService) paymentsNotifyToUserReplyMarkup(customerID int64) models.ReplyMarkup {
	if s == nil || s.translation == nil || customerID <= 0 {
		return nil
	}
	// Тексты уведомлений в группе на русском — подпись кнопки из admin_ru.
	lang := "ru"
	btnText := strings.TrimSpace(s.translation.GetText(lang, "payments_notify_to_user_btn"))
	if btnText == "" {
		btnText = "К пользователю"
	}
	data := paymentsNotifyUserOpenCallbackPrefix + strconv.FormatInt(customerID, 10)
	if len(data) > 64 {
		return nil
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: btnText, CallbackData: data}},
	}}
}

// loyaltyDiscountPercentForNotify — процент скидки по текущему уровню лояльности на момент оплаты.
// Использует c.LoyaltyXP до инкремента XP за эту покупку (в памяти customer не обновляется в applyLoyaltyXPAfterPayment).
// Не зависит от промо: discount_percent_applied в purchase — только про промокод.
func (s *PaymentService) loyaltyDiscountPercentForNotify(ctx context.Context, _ *database.Purchase, c *database.Customer) int {
	if s == nil || s.loyaltyTierRepository == nil || c == nil || !config.LoyaltyEnabled() {
		return 0
	}
	prog, err := s.loyaltyTierRepository.ProgressForXP(ctx, c.LoyaltyXP)
	if err != nil {
		return 0
	}
	pct := prog.CurrentTier.DiscountPercent
	if pct <= 0 {
		return 0
	}
	return pct
}

// tryNotifyPurchasePaid — уведомление в группу после успешной оплаты (не влияет на результат покупки).
func (s *PaymentService) tryNotifyPurchasePaid(ctx context.Context, p *database.Purchase, c *database.Customer, expireBefore, expireAfter *time.Time) {
	if p == nil || c == nil || !config.PaymentsNotifySendPaid() {
		return
	}
	cabinet, err := s.purchaseRepository.HasCabinetCheckoutForPurchase(ctx, p.ID)
	if err != nil {
		slog.Warn("payments notify: cabinet_checkout lookup", "error", err)
		cabinet = false
	}
	loyaltyPct := s.loyaltyDiscountPercentForNotify(ctx, p, c)
	msg := buildPaidGroupMessage(ctx, p, c, cabinet, expireBefore, expireAfter, s.tariffRepository, loyaltyPct)
	s.sendPaymentsGroupHTML(ctx, msg, s.paymentsNotifyToUserReplyMarkup(c.ID))
}

// tryNotifyPurchaseCancel — уведомление об отмене счёта (YooKassa worker, Tribute cancel и т.д.).
func (s *PaymentService) tryNotifyPurchaseCancel(ctx context.Context, p *database.Purchase, c *database.Customer) {
	if p == nil || !config.PaymentsNotifySendCancel() {
		return
	}
	if c == nil {
		var err error
		c, err = s.customerRepository.FindById(ctx, p.CustomerID)
		if err != nil || c == nil {
			slog.Warn("payments notify cancel: customer not loaded", "customer_id", p.CustomerID, "error", err)
		}
	}
	cabinet, err := s.purchaseRepository.HasCabinetCheckoutForPurchase(ctx, p.ID)
	if err != nil {
		slog.Warn("payments notify cancel: cabinet_checkout lookup", "error", err)
		cabinet = false
	}
	loyaltyPct := s.loyaltyDiscountPercentForNotify(ctx, p, c)
	msg := buildCancelGroupMessage(ctx, p, c, cabinet, s.tariffRepository, loyaltyPct)
	var markup models.ReplyMarkup
	if c != nil {
		markup = s.paymentsNotifyToUserReplyMarkup(c.ID)
	}
	s.sendPaymentsGroupHTML(ctx, msg, markup)
}
