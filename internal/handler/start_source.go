package handler

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// Префиксы аргумента /start. Партнёрский код — буквы и цифры, реферальный —
// telegram_id пригласившего, поэтому перепутать их нельзя.
const (
	startArgPartnerPrefix  = "p_"
	startArgReferralPrefix = "ref_"
)

// parseStartArg достаёт аргумент из текста команды: "/start ref_123" → "ref_123".
// Пусто, если аргумента нет — это обычный заход в бота, а не ошибка.
func parseStartArg(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// attachStartSource закрепляет только что созданного клиента за источником, из
// которого он пришёл.
//
// Партнёрская и реферальная программы взаимоисключающие: клиент, пришедший по
// партнёрской ссылке, НЕ становится рефералом. Иначе за одну и ту же оплату
// магазин заплатил бы дважды — деньгами партнёру и днями пригласившему.
//
// Вызывается только для нового клиента: закрепление задним числом означало бы,
// что партнёр получает процент с чужой, уже наработанной базы.
func (h Handler) attachStartSource(ctx context.Context, text string, customer *database.Customer) {
	arg := parseStartArg(text)
	if arg == "" || customer == nil {
		return
	}

	if strings.HasPrefix(arg, startArgPartnerPrefix) {
		if h.attachPartnerFromStart(ctx, strings.TrimPrefix(arg, startArgPartnerPrefix), customer) {
			return
		}
		// Код не сработал (программа выключена, ссылка в архиве, партнёр не в
		// работе) — заход остаётся обычным, без источника.
		return
	}

	if strings.HasPrefix(arg, startArgReferralPrefix) {
		h.attachReferralFromStart(ctx, strings.TrimPrefix(arg, startArgReferralPrefix), customer)
	}
}

// attachPartnerFromStart закрепляет клиента за партнёром по коду ссылки.
// Возвращает true, если закрепление состоялось.
func (h Handler) attachPartnerFromStart(ctx context.Context, code string, customer *database.Customer) bool {
	if h.partnerRepository == nil || !config.PartnerProgramEnabled() {
		return false
	}

	resolved, err := h.partnerRepository.ResolveLinkCode(ctx, code)
	if err != nil {
		slog.Error("error resolving partner link code", "error", err)
		return false
	}
	if resolved == nil {
		return false
	}

	partner, err := h.partnerRepository.FindByID(ctx, resolved.PartnerID)
	if err != nil {
		slog.Error("error loading partner", "error", err, "partner_id", resolved.PartnerID)
		return false
	}
	// Партнёр не может привести сам себя: иначе достаточно завести второй
	// аккаунт и покупать подписки со скидкой в размере своей же комиссии.
	if partner == nil || partner.CustomerID == customer.ID {
		return false
	}

	linkID := resolved.Link.ID
	attached, err := h.partnerRepository.AttachAttribution(ctx, customer.ID, resolved.PartnerID, &linkID, database.PartnerAttributionSourceTelegram)
	if err != nil {
		slog.Error("error attaching partner attribution", "error", err)
		return false
	}
	if attached {
		slog.Info("partner attribution attached",
			"partner_id", resolved.PartnerID,
			"link_code", resolved.Link.Code,
			"customer_id", utils.MaskHalfInt64(customer.ID))
	}
	return true
}

// attachReferralFromStart — прежнее поведение ref_<telegram_id>, вынесенное из
// StartCommandHandler без изменений по смыслу.
func (h Handler) attachReferralFromStart(ctx context.Context, code string, customer *database.Customer) {
	referrerId, err := strconv.ParseInt(code, 10, 64)
	if err != nil {
		slog.Error("error parsing referrer id", "error", err)
		return
	}
	if referrerId == customer.TelegramID {
		return
	}

	referrer, err := h.customerRepository.FindByTelegramId(ctx, referrerId)
	if err != nil || referrer == nil {
		return
	}
	ref, err := h.referralRepository.Create(ctx, referrerId, customer.TelegramID)
	if err != nil {
		slog.Error("error creating referral", "error", err)
		return
	}
	if ref == nil {
		// Приглашённый уже привязан к рефереру — первая связь остаётся.
		return
	}
	slog.Info("referral created",
		"referrerId", utils.MaskHalfInt64(referrerId),
		"refereeId", utils.MaskHalfInt64(customer.TelegramID))
}
