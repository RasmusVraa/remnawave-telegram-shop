package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// BroadcastType определяет тип рассылки
type BroadcastType string

const (
	BroadcastTypeAll           BroadcastType = "all"          // Всем пользователям
	BroadcastTypeActivePaid    BroadcastType = "active_paid"  // Активные платные
	BroadcastTypeActiveTrial   BroadcastType = "active_trial" // Активный триал
	BroadcastTypeActiveAll     BroadcastType = "active_all"   // Все активные
	BroadcastTypeInactivePaid  BroadcastType = "inactive_paid"
	BroadcastTypeInactiveTrial BroadcastType = "inactive_trial"
	BroadcastTypeInactiveAll   BroadcastType = "inactive_all"
)

// BroadcastRecipientButtons — какие inline-кнопки прикрепить к рассылке.
type BroadcastRecipientButtons = broadcast.RecipientButtons

// broadcastDraftMedia — вложение черновика рассылки: фото, видео или файл.
type broadcastDraftMedia = broadcast.Media

// BroadcastState хранит состояние рассылки для каждого админа
type BroadcastState struct {
	mu                   sync.Mutex
	pendingText          map[int64]string
	pendingEntities      map[int64][]models.MessageEntity
	pendingMedia         map[int64]*broadcastDraftMedia
	pendingButtons       map[int64]BroadcastRecipientButtons
	waitingForInput      map[int64]bool
	waitingForButtonPick map[int64]bool
	selectedType         map[int64]BroadcastType
	// Фильтр тарифа для платных сегментов в SALES_MODE=tariffs; nil — все платники сегмента; не в map — то же после classic.
	selectedTariffID         map[int64]*int64
	broadcastOpenedFromAdmin map[int64]bool
	promptMessageID          map[int64]int
	pendingPreviewMsgID      map[int64]int
}

func newBroadcastState() *BroadcastState {
	return &BroadcastState{
		pendingText:              make(map[int64]string),
		pendingEntities:          make(map[int64][]models.MessageEntity),
		pendingMedia:             make(map[int64]*broadcastDraftMedia),
		pendingButtons:           make(map[int64]BroadcastRecipientButtons),
		waitingForInput:          make(map[int64]bool),
		waitingForButtonPick:     make(map[int64]bool),
		selectedType:             make(map[int64]BroadcastType),
		selectedTariffID:         make(map[int64]*int64),
		broadcastOpenedFromAdmin: make(map[int64]bool),
		promptMessageID:          make(map[int64]int),
		pendingPreviewMsgID:      make(map[int64]int),
	}
}

func clearBroadcastState(adminID int64) {
	broadcastState.mu.Lock()
	delete(broadcastState.pendingText, adminID)
	delete(broadcastState.pendingEntities, adminID)
	delete(broadcastState.pendingMedia, adminID)
	delete(broadcastState.pendingButtons, adminID)
	delete(broadcastState.waitingForInput, adminID)
	delete(broadcastState.waitingForButtonPick, adminID)
	delete(broadcastState.selectedType, adminID)
	delete(broadcastState.selectedTariffID, adminID)
	delete(broadcastState.broadcastOpenedFromAdmin, adminID)
	delete(broadcastState.promptMessageID, adminID)
	delete(broadcastState.pendingPreviewMsgID, adminID)
	broadcastState.mu.Unlock()
}

func resetBroadcastDraft(adminID int64) {
	broadcastState.mu.Lock()
	delete(broadcastState.pendingText, adminID)
	delete(broadcastState.pendingEntities, adminID)
	delete(broadcastState.pendingMedia, adminID)
	delete(broadcastState.pendingButtons, adminID)
	delete(broadcastState.waitingForButtonPick, adminID)
	delete(broadcastState.promptMessageID, adminID)
	delete(broadcastState.pendingPreviewMsgID, adminID)
	delete(broadcastState.selectedTariffID, adminID)
	broadcastState.mu.Unlock()
}

var broadcastState = newBroadcastState()

const broadcastTariffBtnMaxRunes = 52

func broadcastTruncateButtonLabel(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes-1]) + "…"
	}
	return s
}

// broadcastAudienceSummaryLine — текст сегмента для подсказки и подтверждения рассылки.
func (h Handler) broadcastAudienceSummaryLine(ctx context.Context, lang string, t BroadcastType, tariffID *int64) string {
	switch t {
	case BroadcastTypeActivePaid:
		if tariffID != nil && h.tariffRepository != nil {
			tr, err := h.tariffRepository.GetByID(ctx, *tariffID)
			if err == nil && tr != nil {
				return fmt.Sprintf(h.translation.GetText(lang, "broadcast_summary_active_paid_tariff"), escapeHTML(displayTariffName(tr)))
			}
		}
		return h.translation.GetText(lang, "broadcast_summary_active_paid")
	case BroadcastTypeInactivePaid:
		if tariffID != nil && h.tariffRepository != nil {
			tr, err := h.tariffRepository.GetByID(ctx, *tariffID)
			if err == nil && tr != nil {
				return fmt.Sprintf(h.translation.GetText(lang, "broadcast_summary_inactive_paid_tariff"), escapeHTML(displayTariffName(tr)))
			}
		}
		return h.translation.GetText(lang, "broadcast_summary_inactive_paid")
	case BroadcastTypeActiveTrial:
		return h.translation.GetText(lang, "broadcast_summary_active_trial")
	case BroadcastTypeActiveAll:
		return h.translation.GetText(lang, "broadcast_summary_active_all_seg")
	case BroadcastTypeInactiveTrial:
		return h.translation.GetText(lang, "broadcast_summary_inactive_trial")
	case BroadcastTypeInactiveAll:
		return h.translation.GetText(lang, "broadcast_summary_inactive_all_seg")
	case BroadcastTypeAll:
		return h.translation.GetText(lang, "broadcast_summary_all")
	default:
		return h.translation.GetText(lang, "broadcast_summary_all")
	}
}

func (h Handler) broadcastPaidTariffPickKeyboard(lang string, activePaidSegment bool, tariffs []database.Tariff) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	for i := range tariffs {
		t := tariffs[i]
		label := broadcastTruncateButtonLabel(displayTariffName(&t), broadcastTariffBtnMaxRunes)
		var cb string
		if activePaidSegment {
			cb = CallbackBroadcastPaidTariffActivePrefix + strconv.FormatInt(t.ID, 10)
		} else {
			cb = CallbackBroadcastPaidTariffInactivePrefix + strconv.FormatInt(t.ID, 10)
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: label, CallbackData: cb}})
	}
	if activePaidSegment {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "broadcast_aud_paid_all_tariffs", models.InlineKeyboardButton{CallbackData: CallbackBroadcastPaidTariffActiveAll}),
		})
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "broadcast_aud_back_segment", models.InlineKeyboardButton{CallbackData: CallbackBroadcastPaidTariffBackActiveSeg}),
		})
	} else {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "broadcast_aud_paid_all_tariffs", models.InlineKeyboardButton{CallbackData: CallbackBroadcastPaidTariffInactiveAll}),
		})
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "broadcast_aud_back_segment", models.InlineKeyboardButton{CallbackData: CallbackBroadcastPaidTariffBackInactiveSeg}),
		})
	}
	return rows
}

// extractBroadcastImageFromMessage — фото как картинка или документ JPEG/PNG/WebP (отправка файлом).
func extractBroadcastImageFromMessage(m *models.Message) (fileID string, kind broadcast.MediaKind, ok bool) {
	if m == nil {
		return "", "", false
	}
	if len(m.Photo) > 0 {
		last := m.Photo[len(m.Photo)-1]
		return last.FileID, broadcast.MediaPhoto, true
	}
	// Видео Telegram отдаёт отдельным полем, а не документом — и переслать его
	// надо тоже видео, иначе в чате получателя оно не проиграется.
	if m.Video != nil {
		return m.Video.FileID, broadcast.MediaVideo, true
	}
	if m.Document != nil {
		mime := strings.ToLower(strings.TrimSpace(m.Document.MimeType))
		switch mime {
		case "image/jpeg", "image/jpg", "image/png", "image/webp":
			return m.Document.FileID, broadcast.MediaDocument, true
		case "video/mp4", "video/quicktime", "video/webm":
			// Видео, отправленное файлом (без сжатия), Telegram кладёт в
			// Document. Получателю всё равно шлём его как видео.
			return m.Document.FileID, broadcast.MediaVideo, true
		}
	}
	return "", "", false
}

// BroadcastAwaitingMessageInput — админ выбрал аудиторию и бот ждёт текст/картинку черновика.
func BroadcastAwaitingMessageInput(adminID int64) bool {
	broadcastState.mu.Lock()
	defer broadcastState.mu.Unlock()
	isWaiting, wok := broadcastState.waitingForInput[adminID]
	_, tok := broadcastState.selectedType[adminID]
	return wok && isWaiting && tok
}

// BroadcastIncomingDraftMessage — подходит ли сообщение как черновик рассылки (текст, подпись к медиа или картинка).
func BroadcastIncomingDraftMessage(m *models.Message) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.Text) != "" && !strings.HasPrefix(m.Text, "/") {
		return true
	}
	if m.Caption != "" {
		return true
	}
	_, _, ok := extractBroadcastImageFromMessage(m)
	return ok
}

// BroadcastAudienceKeyboard выбор аудитории; withBackToAdmin — кнопка «Назад» в админ-меню (экран из админки).
func (h Handler) BroadcastAudienceKeyboard(lang string, withBackToAdmin bool) [][]models.InlineKeyboardButton {
	rows := [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "broadcast_audience_all", models.InlineKeyboardButton{CallbackData: CallbackBroadcastAll})},
		{h.translation.WithButton(lang, "broadcast_audience_active", models.InlineKeyboardButton{CallbackData: CallbackBroadcastActive})},
		{h.translation.WithButton(lang, "broadcast_audience_inactive", models.InlineKeyboardButton{CallbackData: CallbackBroadcastInactive})},
	}
	if withBackToAdmin {
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackBroadcastBackAdmin}),
		})
	}
	return rows
}

// SendBroadcastTypeSelection sends the broadcast audience keyboard (команда /broadcast — отдельное сообщение, без «Назад» в админку).
func (h Handler) SendBroadcastTypeSelection(ctx context.Context, b *bot.Bot, adminChatID int64, lang string) error {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminChatID,
		ParseMode:   models.ParseModeHTML,
		Text:        h.translation.GetText(lang, "broadcast_choose_audience"),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.BroadcastAudienceKeyboard(lang, false)},
	})
	if err != nil {
		slog.Error("error sending broadcast type selection", "error", err)
	}
	return err
}

// BroadcastCommandHandler обрабатывает команду /broadcast от админа
func (h Handler) BroadcastCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	adminID := config.GetAdminTelegramId()
	if update.Message.From.ID != adminID {
		return
	}

	clearBroadcastState(adminID)

	lang := update.Message.From.LanguageCode
	_ = h.SendBroadcastTypeSelection(ctx, b, adminID, lang)
}

func broadcastTypeToAudience(t BroadcastType) string {
	switch t {
	case BroadcastTypeAll:
		return database.BroadcastAudienceAll
	case BroadcastTypeActivePaid:
		return database.BroadcastAudienceActivePaid
	case BroadcastTypeActiveTrial:
		return database.BroadcastAudienceActiveTrial
	case BroadcastTypeActiveAll:
		return database.BroadcastAudienceActiveAll
	case BroadcastTypeInactivePaid:
		return database.BroadcastAudienceInactivePaid
	case BroadcastTypeInactiveTrial:
		return database.BroadcastAudienceInactiveTrial
	case BroadcastTypeInactiveAll:
		return database.BroadcastAudienceInactiveAll
	default:
		return database.BroadcastAudienceAll
	}
}

func (h Handler) broadcastAudienceRootKeyboard(lang string, adminID int64) [][]models.InlineKeyboardButton {
	fromAdmin := false
	broadcastState.mu.Lock()
	fromAdmin = broadcastState.broadcastOpenedFromAdmin[adminID]
	broadcastState.mu.Unlock()
	return h.BroadcastAudienceKeyboard(lang, fromAdmin)
}

func (h Handler) broadcastActiveSegmentKeyboard(lang string) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "broadcast_aud_active_paid", models.InlineKeyboardButton{CallbackData: CallbackBroadcastActivePaid})},
		{h.translation.WithButton(lang, "broadcast_aud_active_trial", models.InlineKeyboardButton{CallbackData: CallbackBroadcastActiveTrial})},
		{h.translation.WithButton(lang, "broadcast_aud_active_all", models.InlineKeyboardButton{CallbackData: CallbackBroadcastActiveAllSeg})},
		{
			h.translation.WithButton(lang, "broadcast_aud_back_audience", models.InlineKeyboardButton{CallbackData: CallbackBroadcastBackAudience}),
		},
	}
}

func (h Handler) broadcastInactiveSegmentKeyboard(lang string) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "broadcast_aud_inactive_paid", models.InlineKeyboardButton{CallbackData: CallbackBroadcastInactivePaid})},
		{h.translation.WithButton(lang, "broadcast_aud_inactive_trial", models.InlineKeyboardButton{CallbackData: CallbackBroadcastInactiveTrial})},
		{h.translation.WithButton(lang, "broadcast_aud_inactive_all", models.InlineKeyboardButton{CallbackData: CallbackBroadcastInactiveAllSeg})},
		{
			h.translation.WithButton(lang, "broadcast_aud_back_audience", models.InlineKeyboardButton{CallbackData: CallbackBroadcastBackAudience}),
		},
	}
}

// BroadcastActiveMenuHandler — подменю выбора сегмента среди активных подписок.
func (h Handler) BroadcastActiveMenuHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}
	cb := update.CallbackQuery
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_active_segment"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastActiveSegmentKeyboard(lang)}, nil)
	if err != nil {
		slog.Error("broadcast active submenu", "error", err)
	}
}

// BroadcastInactiveMenuHandler — подменю выбора сегмента среди неактивных.
func (h Handler) BroadcastInactiveMenuHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}
	cb := update.CallbackQuery
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_inactive_segment"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastInactiveSegmentKeyboard(lang)}, nil)
	if err != nil {
		slog.Error("broadcast inactive submenu", "error", err)
	}
}

// BroadcastBackToAudienceHandler — назад к выбору «всем / активным / неактивным».
func (h Handler) BroadcastBackToAudienceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}
	cb := update.CallbackQuery
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_audience"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastAudienceRootKeyboard(lang, adminID)}, nil)
	if err != nil {
		slog.Error("broadcast back audience", "error", err)
	}
}

func segmentPickBroadcastType(cbData string) BroadcastType {
	switch cbData {
	case CallbackBroadcastActivePaid:
		return BroadcastTypeActivePaid
	case CallbackBroadcastActiveTrial:
		return BroadcastTypeActiveTrial
	case CallbackBroadcastActiveAllSeg:
		return BroadcastTypeActiveAll
	case CallbackBroadcastInactivePaid:
		return BroadcastTypeInactivePaid
	case CallbackBroadcastInactiveTrial:
		return BroadcastTypeInactiveTrial
	case CallbackBroadcastInactiveAllSeg:
		return BroadcastTypeInactiveAll
	default:
		return BroadcastTypeAll
	}
}

// broadcastBeginDraftPrompt — аудитория выбрана, запрашиваем текст/медиа черновика.
func (h Handler) broadcastBeginDraftPrompt(ctx context.Context, b *bot.Bot, cb *models.CallbackQuery, broadcastType BroadcastType, tariffFilter *int64) {
	adminID := config.GetAdminTelegramId()
	if cb.From.ID != adminID {
		return
	}
	callbackMessage := cb.Message.Message
	if callbackMessage == nil {
		return
	}
	lang := cb.From.LanguageCode

	resetBroadcastDraft(adminID)

	broadcastState.mu.Lock()
	broadcastState.selectedType[adminID] = broadcastType
	if tariffFilter != nil {
		tid := *tariffFilter
		broadcastState.selectedTariffID[adminID] = &tid
	} else {
		delete(broadcastState.selectedTariffID, adminID)
	}
	broadcastState.waitingForInput[adminID] = true
	delete(broadcastState.promptMessageID, adminID)
	broadcastState.mu.Unlock()

	typeText := h.broadcastAudienceSummaryLine(ctx, lang, broadcastType, tariffFilter)
	prompt := fmt.Sprintf(h.translation.GetText(lang, "broadcast_enter_message"), typeText)
	newMsg, err := editCallbackOriginToHTMLText(ctx, b, callbackMessage, prompt, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{},
	}, nil)
	if err != nil {
		slog.Error("broadcast draft prompt", "error", err)
		return
	}
	if newMsg != nil {
		broadcastState.mu.Lock()
		broadcastState.promptMessageID[adminID] = newMsg.ID
		broadcastState.mu.Unlock()
	}
}

// BroadcastPaidTariffCallbacksHandler — выбор тарифа для «платников» в режиме tariffs (callback префикс bc_pt_).
func (h Handler) BroadcastPaidTariffCallbacksHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	cb := update.CallbackQuery
	adminID := config.GetAdminTelegramId()
	if cb.From.ID != adminID || cb.Message.Message == nil {
		return
	}
	data := cb.Data
	lang := cb.From.LanguageCode
	msg := cb.Message.Message

	switch data {
	case CallbackBroadcastPaidTariffBackActiveSeg:
		_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_active_segment"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastActiveSegmentKeyboard(lang)}, nil)
		if err != nil {
			slog.Error("broadcast tariff back active", "error", err)
		}
		return
	case CallbackBroadcastPaidTariffBackInactiveSeg:
		_, err := editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "broadcast_choose_inactive_segment"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastInactiveSegmentKeyboard(lang)}, nil)
		if err != nil {
			slog.Error("broadcast tariff back inactive", "error", err)
		}
		return
	}

	var broadcastType BroadcastType
	var tariffFilter *int64

	switch {
	case data == CallbackBroadcastPaidTariffActiveAll:
		broadcastType = BroadcastTypeActivePaid
	case data == CallbackBroadcastPaidTariffInactiveAll:
		broadcastType = BroadcastTypeInactivePaid
	case strings.HasPrefix(data, CallbackBroadcastPaidTariffActivePrefix):
		idStr := strings.TrimPrefix(data, CallbackBroadcastPaidTariffActivePrefix)
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			return
		}
		broadcastType = BroadcastTypeActivePaid
		tariffFilter = &id
	case strings.HasPrefix(data, CallbackBroadcastPaidTariffInactivePrefix):
		idStr := strings.TrimPrefix(data, CallbackBroadcastPaidTariffInactivePrefix)
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			return
		}
		broadcastType = BroadcastTypeInactivePaid
		tariffFilter = &id
	default:
		return
	}

	h.broadcastBeginDraftPrompt(ctx, b, cb, broadcastType, tariffFilter)
}

// BroadcastSegmentPickHandler — выбор платные/триал/все для активных или неактивных.
func (h Handler) BroadcastSegmentPickHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}
	cb := update.CallbackQuery
	callbackMessage := cb.Message.Message
	if callbackMessage == nil {
		return
	}
	data := cb.Data
	broadcastType := segmentPickBroadcastType(data)
	if broadcastType == BroadcastTypeAll {
		return
	}

	if (data == CallbackBroadcastActivePaid || data == CallbackBroadcastInactivePaid) &&
		config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		tariffs, err := h.tariffRepository.ListActive(ctx)
		if err != nil {
			slog.Error("broadcast list tariffs", "error", err)
		} else if len(tariffs) > 0 {
			lang := cb.From.LanguageCode
			activePaidSeg := data == CallbackBroadcastActivePaid
			_, err := editCallbackOriginToHTMLText(ctx, b, callbackMessage, h.translation.GetText(lang, "broadcast_choose_paid_tariff"), models.ParseModeHTML, models.InlineKeyboardMarkup{
				InlineKeyboard: h.broadcastPaidTariffPickKeyboard(lang, activePaidSeg, tariffs),
			}, nil)
			if err != nil {
				slog.Error("broadcast tariff submenu", "error", err)
			}
			return
		}
	}

	h.broadcastBeginDraftPrompt(ctx, b, cb, broadcastType, nil)
}

func (h Handler) broadcastToggleRow(lang string, on bool, buttonKey, callbackData string) models.InlineKeyboardButton {
	data := h.translation.GetButton(lang, buttonKey)
	prefix := "☐ "
	if on {
		prefix = "✅ "
	}
	return models.InlineKeyboardButton{Text: prefix + data.Text, CallbackData: callbackData}
}

func (h Handler) broadcastButtonPickerKeyboard(lang string, flags BroadcastRecipientButtons) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.broadcastToggleRow(lang, flags.Buy, "buy_button", CallbackBroadcastToggleBuy)},
		{h.broadcastToggleRow(lang, flags.Connect, "connect_button", CallbackBroadcastToggleVPN)},
		{h.broadcastToggleRow(lang, flags.Promo, "promo_code_button", CallbackBroadcastTogglePromo)},
		{h.broadcastToggleRow(lang, flags.ShareReferral, "broadcast_share_referral_button", CallbackBroadcastToggleShare)},
		{h.broadcastToggleRow(lang, flags.MainMenu, "broadcast_inline_main", CallbackBroadcastToggleMain)},
		{
			{Text: h.translation.GetText(lang, "broadcast_buttons_next"), CallbackData: CallbackBroadcastButtonsNext},
			{Text: h.translation.GetText(lang, "broadcast_confirm_no"), CallbackData: CallbackBroadcastCancel},
		},
	}
}

// buildBroadcastReplyMarkup — клавиатура предпросмотра. В «Пригласить друга»
// попадает реферальная ссылка самого админа: так видно, что кнопка рабочая,
// а получатели получат каждый свою.
func (h Handler) buildBroadcastReplyMarkup(lang string, flags BroadcastRecipientButtons, adminID int64) models.ReplyMarkup {
	return broadcast.BuildReplyMarkup(h.translation, lang, flags, adminID)
}

func (h Handler) broadcastButtonsSummaryLine(lang string, flags BroadcastRecipientButtons) string {
	if flags.IsEmpty() {
		return h.translation.GetText(lang, "broadcast_buttons_none")
	}
	var parts []string
	if flags.Buy {
		parts = append(parts, h.translation.GetButton(lang, "buy_button").Text)
	}
	if flags.Connect {
		parts = append(parts, h.translation.GetButton(lang, "connect_button").Text)
	}
	if flags.Promo {
		parts = append(parts, h.translation.GetButton(lang, "promo_code_button").Text)
	}
	if flags.ShareReferral {
		parts = append(parts, h.translation.GetButton(lang, "broadcast_share_referral_button").Text)
	}
	if flags.MainMenu {
		parts = append(parts, h.translation.GetButton(lang, "broadcast_inline_main").Text)
	}
	return strings.Join(parts, ", ")
}

// BroadcastTypeSelectHandler обрабатывает выбор типа рассылки
func (h Handler) BroadcastTypeSelectHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}

	if update.CallbackQuery.Data != CallbackBroadcastAll {
		return
	}

	h.broadcastBeginDraftPrompt(ctx, b, update.CallbackQuery, BroadcastTypeAll, nil)
}

// BroadcastMessageHandler обрабатывает сообщение от админа с текстом или подписью к медиа после выбора аудитории
func (h Handler) BroadcastMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	adminID := config.GetAdminTelegramId()
	if update.Message.From.ID != adminID {
		return
	}

	broadcastState.mu.Lock()
	isWaiting, exists := broadcastState.waitingForInput[adminID]
	_, typeExists := broadcastState.selectedType[adminID]
	if !exists || !isWaiting || !typeExists {
		broadcastState.mu.Unlock()
		return
	}
	broadcastState.mu.Unlock()

	fileID, mediaKind, hasMedia := extractBroadcastImageFromMessage(update.Message)
	var messageText string
	var entities []models.MessageEntity
	if hasMedia {
		messageText = update.Message.Caption
		if len(update.Message.CaptionEntities) > 0 {
			entities = append([]models.MessageEntity(nil), update.Message.CaptionEntities...)
		}
	} else {
		messageText = update.Message.Text
		if len(update.Message.Entities) > 0 {
			entities = append([]models.MessageEntity(nil), update.Message.Entities...)
		}
	}
	if strings.TrimSpace(messageText) == "" && !hasMedia {
		return
	}
	if !hasMedia && len(messageText) > 0 && messageText[0] == '/' {
		return
	}

	var entCopy []models.MessageEntity
	if len(entities) > 0 {
		entCopy = append([]models.MessageEntity(nil), entities...)
	}

	lang := update.Message.From.LanguageCode

	broadcastState.mu.Lock()
	broadcastState.pendingText[adminID] = messageText
	if len(entCopy) > 0 {
		broadcastState.pendingEntities[adminID] = entCopy
	} else {
		delete(broadcastState.pendingEntities, adminID)
	}
	if hasMedia {
		broadcastState.pendingMedia[adminID] = &broadcastDraftMedia{FileID: fileID, Kind: mediaKind}
	} else {
		delete(broadcastState.pendingMedia, adminID)
	}
	broadcastState.pendingButtons[adminID] = BroadcastRecipientButtons{}
	broadcastState.waitingForInput[adminID] = false
	broadcastState.waitingForButtonPick[adminID] = true
	promptMid := broadcastState.promptMessageID[adminID]
	delete(broadcastState.promptMessageID, adminID)
	broadcastState.mu.Unlock()

	if promptMid != 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    adminID,
			MessageID: promptMid,
		})
	}

	flags := BroadcastRecipientButtons{}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminID,
		ParseMode:   models.ParseModeHTML,
		Text:        h.translation.GetText(lang, "broadcast_pick_buttons"),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastButtonPickerKeyboard(lang, flags)},
	})
	if err != nil {
		slog.Error("error sending broadcast button picker", "error", err)
	}
}

// BroadcastButtonToggleHandler переключает выбор кнопок под рассылкой
func (h Handler) BroadcastButtonToggleHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	cb := update.CallbackQuery
	adminID := config.GetAdminTelegramId()
	if cb.From.ID != adminID || cb.Message.Message == nil {
		return
	}

	lang := cb.From.LanguageCode

	broadcastState.mu.Lock()
	if !broadcastState.waitingForButtonPick[adminID] {
		broadcastState.mu.Unlock()
		return
	}
	flags := broadcastState.pendingButtons[adminID]
	switch cb.Data {
	case CallbackBroadcastToggleBuy:
		flags.Buy = !flags.Buy
	case CallbackBroadcastToggleMain:
		flags.MainMenu = !flags.MainMenu
	case CallbackBroadcastTogglePromo:
		flags.Promo = !flags.Promo
	case CallbackBroadcastToggleVPN:
		flags.Connect = !flags.Connect
	case CallbackBroadcastToggleShare:
		flags.ShareReferral = !flags.ShareReferral
	default:
		broadcastState.mu.Unlock()
		return
	}
	broadcastState.pendingButtons[adminID] = flags
	broadcastState.mu.Unlock()

	pickerMsg := cb.Message.Message
	_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      pickerMsg.Chat.ID,
		MessageID:   pickerMsg.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.broadcastButtonPickerKeyboard(lang, flags)},
	})
	if err != nil {
		slog.Error("broadcast toggle edit markup", "error", err)
	}
}

// BroadcastButtonsNextHandler переходит от выбора кнопок к предпросмотру и подтверждению
func (h Handler) BroadcastButtonsNextHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	cb := update.CallbackQuery
	adminID := config.GetAdminTelegramId()
	if cb.From.ID != adminID || cb.Message.Message == nil {
		return
	}

	lang := cb.From.LanguageCode

	broadcastState.mu.Lock()
	if !broadcastState.waitingForButtonPick[adminID] {
		broadcastState.mu.Unlock()
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "broadcast_session_expired"),
		})
		return
	}
	messageText := broadcastState.pendingText[adminID]
	entities := broadcastState.pendingEntities[adminID]
	flags := broadcastState.pendingButtons[adminID]
	broadcastType := broadcastState.selectedType[adminID]
	var draftMedia *broadcastDraftMedia
	if m, ok := broadcastState.pendingMedia[adminID]; ok {
		cp := *m
		draftMedia = &cp
	}
	broadcastState.waitingForButtonPick[adminID] = false
	var tfSummary *int64
	if ptr, ok := broadcastState.selectedTariffID[adminID]; ok && ptr != nil {
		tfSummary = ptr
	}
	broadcastState.mu.Unlock()

	targetText := h.broadcastAudienceSummaryLine(ctx, lang, broadcastType, tfSummary)
	buttonsLine := h.broadcastButtonsSummaryLine(lang, flags)

	pickerMsg := cb.Message.Message
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    pickerMsg.Chat.ID,
		MessageID: pickerMsg.ID,
	})

	// Предпросмотр рисуется тем же кодом, что и сама рассылка: иначе админ
	// подтверждает одно, а получатели видят другое.
	markup := h.buildBroadcastReplyMarkup(lang, flags, adminID)
	previewMsg, previewErr := broadcast.Deliver(ctx, b, adminID, broadcast.Message{
		Text:     messageText,
		Entities: entities,
		Media:    draftMedia,
		Buttons:  flags,
	}, markup)
	if previewErr != nil {
		slog.Error("broadcast preview send", "error", previewErr)
	}

	if previewMsg != nil {
		broadcastState.mu.Lock()
		broadcastState.pendingPreviewMsgID[adminID] = previewMsg.ID
		broadcastState.mu.Unlock()
	}

	confirmText := fmt.Sprintf(h.translation.GetText(lang, "broadcast_confirm_question"), targetText, buttonsLine)
	inlineKeyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.translation.GetText(lang, "broadcast_confirm_yes"), CallbackData: CallbackBroadcastConfirm},
				{Text: h.translation.GetText(lang, "broadcast_confirm_no"), CallbackData: CallbackBroadcastCancel},
			},
		},
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminID,
		ParseMode:   models.ParseModeHTML,
		Text:        confirmText,
		ReplyMarkup: inlineKeyboard,
	})
	if err != nil {
		slog.Error("error sending broadcast confirmation", "error", err)
	}
}

// BroadcastConfirmHandler обрабатывает подтверждение рассылки
func (h Handler) BroadcastConfirmHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}

	broadcastState.mu.Lock()
	messageText, exists := broadcastState.pendingText[adminID]
	broadcastType, typeExists := broadcastState.selectedType[adminID]
	entities := broadcastState.pendingEntities[adminID]
	flags := broadcastState.pendingButtons[adminID]
	previewID := broadcastState.pendingPreviewMsgID[adminID]
	var draftMedia *broadcastDraftMedia
	if m, ok := broadcastState.pendingMedia[adminID]; ok {
		cp := *m
		draftMedia = &cp
	}
	if !exists || !typeExists {
		broadcastState.mu.Unlock()
		lang := update.CallbackQuery.From.LanguageCode
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "broadcast_session_expired"),
		})
		return
	}
	entCopy := append([]models.MessageEntity(nil), entities...)
	flagsCopy := flags
	var tariffFilter *int64
	if ptr, ok := broadcastState.selectedTariffID[adminID]; ok && ptr != nil {
		tariffFilter = ptr
	}
	broadcastState.mu.Unlock()
	clearBroadcastState(adminID)

	callbackMessage := update.CallbackQuery.Message.Message
	if previewID != 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    adminID,
			MessageID: previewID,
		})
	}
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
	})

	lang := update.CallbackQuery.From.LanguageCode
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminID,
		Text:   h.translation.GetText(lang, "broadcast_started_wait"),
	})

	utils.Go("bot broadcast", func() {
		h.sendBroadcast(ctx, b, adminID, messageText, entCopy, draftMedia, flagsCopy, broadcastType, tariffFilter)
	})
}

// BroadcastCancelHandler обрабатывает отмену рассылки
func (h Handler) BroadcastCancelHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	adminID := config.GetAdminTelegramId()
	if update.CallbackQuery.From.ID != adminID {
		return
	}

	slog.Info("broadcast cancelled by admin", "adminID", adminID)

	broadcastState.mu.Lock()
	previewID := broadcastState.pendingPreviewMsgID[adminID]
	broadcastState.mu.Unlock()
	clearBroadcastState(adminID)

	lang := update.CallbackQuery.From.LanguageCode

	callbackMessage := update.CallbackQuery.Message.Message
	if previewID != 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    adminID,
			MessageID: previewID,
		})
	}
	if callbackMessage != nil {
		_, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
		})
		if err != nil {
			slog.Warn("error deleting confirmation message", "error", err)
		}
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminID,
		Text:   h.translation.GetText(lang, "broadcast_cancelled_msg"),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel})},
		}},
	})
	if err != nil {
		slog.Error("error sending cancel notification", "error", err)
	}
}

// BroadcastBackToAdminHandler возвращает из экрана выбора аудитории рассылки в админ-меню.
func (h Handler) BroadcastBackToAdminHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	clearBroadcastState(cb.From.ID)
	if err := h.RenderAdminPanel(ctx, b, cb.Message.Message, cb.From.LanguageCode); err != nil {
		slog.Error("broadcast back to admin", "error", err)
	}
}

// sendBroadcast отправляет сообщение пользователям пачками в зависимости от типа рассылки.
func (h Handler) sendBroadcast(ctx context.Context, b *bot.Bot, adminID int64, messageText string, entities []models.MessageEntity, media *broadcastDraftMedia, flags BroadcastRecipientButtons, broadcastType BroadcastType, tariffFilter *int64) {
	audience := broadcastTypeToAudience(broadcastType)
	var tf *int64
	if (broadcastType == BroadcastTypeActivePaid || broadcastType == BroadcastTypeInactivePaid) && tariffFilter != nil {
		tf = tariffFilter
	}
	if h.broadcastSender == nil {
		slog.Error("broadcast sender not configured")
		return
	}
	h.broadcastSender.Send(ctx, b, adminID, []string{audience}, tf, broadcast.Message{
		Text: messageText,
		// Форматирование приходит готовыми entities из исходного сообщения
		// админа, поэтому ParseMode здесь не нужен и вреден: Bot API принимает
		// что-то одно.
		Entities: entities,
		Media:    media,
		Buttons:  flags,
	}, broadcast.SendOptions{Source: database.BroadcastSourceTelegram})
}
