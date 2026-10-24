package handler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

const (
	tariffCallbackView   = "tf_v"
	tariffCallbackToggle = "tf_t"
	tariffCallbackDel    = "tf_d"
	tariffCallbackDelYes = "tf_y"
	tariffCallbackSrv    = "tf_s"
	tariffCallbackSq     = "tf_q"
	tariffCallbackClr    = "tf_sc"
	tariffCallbackAll    = "tf_sa"
	tariffCallbackNm     = "tf_nm"
	tariffCallbackEp     = "tf_ep"
	tariffCallbackTT     = "tf_tt"
	tariffCallbackTD     = "tf_td"
	tariffCallbackTL     = "tf_tl"
	tariffCallbackCancel = "tf_ca"
	tariffCallbackWizCan = "tf_wc"
	tariffCallbackDs     = "tf_ds"
	tariffCallbackLoc    = "tf_loc"
	tariffCallbackLocTog = "tf_lt"
	tariffCallbackLocDel = "tf_ld"
	tariffCallbackLocAdd = "tf_la"
)

type tariffWizardDraft struct {
	Name      string
	TrafficGB int64
	Devices   int
	Tier      int
	Rub       [4]int
}

var adminTariffCtx = struct {
	mu sync.Mutex
	// mode: "wiz" | "edit"
	mode      map[int64]string
	wizStep   map[int64]string
	draft     map[int64]*tariffWizardDraft
	editID    map[int64]int64
	editField map[int64]string
}{
	mode:      make(map[int64]string),
	wizStep:   make(map[int64]string),
	draft:     make(map[int64]*tariffWizardDraft),
	editID:    make(map[int64]int64),
	editField: make(map[int64]string),
}

func adminTariffReset(adminID int64) {
	adminTariffCtx.mu.Lock()
	defer adminTariffCtx.mu.Unlock()
	delete(adminTariffCtx.mode, adminID)
	delete(adminTariffCtx.wizStep, adminID)
	delete(adminTariffCtx.draft, adminID)
	delete(adminTariffCtx.editID, adminID)
	delete(adminTariffCtx.editField, adminID)
}

// AdminTariffWizardWaiting — мастер создания тарифа.
func AdminTariffWizardWaiting(adminID int64) bool {
	adminTariffCtx.mu.Lock()
	defer adminTariffCtx.mu.Unlock()
	return adminTariffCtx.mode[adminID] == "wiz"
}

// AdminTariffEditWaiting — ввод при редактировании поля.
func AdminTariffEditWaiting(adminID int64) bool {
	adminTariffCtx.mu.Lock()
	defer adminTariffCtx.mu.Unlock()
	return adminTariffCtx.mode[adminID] == "edit"
}

func bytesInGB() int64 { return 1073741824 }

func slugifyTariffName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.Is(unicode.Cyrillic, r):
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteRune('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "tariff"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func parseFourInts(s string) ([4]int, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return [4]int{}, fmt.Errorf("need 4 numbers")
	}
	var out [4]int
	for i := range 4 {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 {
			return [4]int{}, fmt.Errorf("bad number")
		}
		out[i] = n
	}
	return out, nil
}

func starsFromRub(rub [4]int) [4]*int {
	rate := config.RubPerStar()
	if rate <= 0 {
		return [4]*int{nil, nil, nil, nil}
	}
	var out [4]*int
	for i := range 4 {
		if rub[i] == 0 {
			z := 0
			out[i] = &z
			continue
		}
		s := int(math.Max(1, math.Round(float64(rub[i])/rate)))
		out[i] = &s
	}
	return out
}

func (h Handler) ensureStandardTariff(ctx context.Context) error {
	n, err := h.tariffRepository.CountAll(ctx)
	if err != nil || n > 0 {
		return err
	}
	// slug standard — если занят, не создаём
	ok, err := h.tariffRepository.SlugExists(ctx, "standard")
	if err != nil || ok {
		return err
	}
	parts := make([]string, 0, len(config.SquadUUIDs()))
	for u := range config.SquadUUIDs() {
		parts = append(parts, u.String())
	}
	squadStr := strings.Join(parts, ",")
	ext := config.ExternalSquadUUID()
	var extPtr *uuid.UUID
	if ext != uuid.Nil {
		extPtr = &ext
	}
	tag := config.RemnawaveTag()
	var tagPtr *string
	if tag != "" {
		tagPtr = &tag
	}
	name := "Стандарт"
	tier := 1
	dev := config.PaidHwidLimit()
	if dev <= 0 {
		dev = config.GetHwidFallbackDeviceLimit()
	}
	if dev < 1 {
		dev = 1
	}
	tb := int64(config.TrafficLimit())
	t := &database.Tariff{
		Slug:                      "standard",
		Name:                      &name,
		SortOrder:                 1,
		IsActive:                  true,
		DeviceLimit:               dev,
		TrafficLimitBytes:         tb,
		TrafficLimitResetStrategy: config.TrafficLimitResetStrategy(),
		ActiveInternalSquadUUIDs:  squadStr,
		ExternalSquadUUID:         extPtr,
		RemnawaveTag:              tagPtr,
		TierLevel:                 &tier,
	}
	rub := [4]int{config.Price1(), config.Price3(), config.Price6(), config.Price12()}
	stars := [4]*int{}
	if config.IsTelegramStarsEnabled() {
		s1 := config.StarsPrice(1)
		s3 := config.StarsPrice(3)
		s6 := config.StarsPrice(6)
		s12 := config.StarsPrice(12)
		stars[0], stars[1], stars[2], stars[3] = &s1, &s3, &s6, &s12
	}
	_, err = h.tariffRepository.CreateWithPrices(ctx, t, rub, stars)
	if err != nil {
		return fmt.Errorf("ensure standard tariff: %w", err)
	}
	slog.Info("auto-created standard tariff")
	return nil
}

// editAdminTariffsMessage перерисовывает список тарифов (без answerCallbackQuery).
func (h Handler) editAdminTariffsMessage(ctx context.Context, b *bot.Bot, chatID int64, msgID int, lang string) error {
	_ = h.ensureStandardTariff(ctx)
	all, err := h.tariffRepository.ListAll(ctx)
	if err != nil {
		slog.Error("tariff list all", "error", err)
		return err
	}
	var active, total int64
	for _, t := range all {
		total++
		if t.IsActive {
			active++
		}
	}
	paidN, _ := h.tariffRepository.CountPaidPurchasesWithTariff(ctx)
	text := fmt.Sprintf(h.translation.GetText(lang, "admin_tariffs_title"), total, active, paidN)
	var rows [][]models.InlineKeyboardButton
	for _, t := range all {
		label := t.Slug
		if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
			label = fmt.Sprintf("%s (%s)", *t.Name, t.Slug)
		}
		if !t.IsActive {
			label = "⏸ " + label
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackView, t.ID)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_tariffs_create", models.InlineKeyboardButton{CallbackData: CallbackTariffNew}),
	})
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel}),
	})
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	if err != nil {
		slog.Error("admin tariffs edit", "error", err)
	}
	return err
}

// AdminTariffsHandler — корень раздела тарифов.
func (h Handler) AdminTariffsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	adminTariffReset(cb.From.ID)
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	_ = h.editAdminTariffsMessage(ctx, b, msg.Chat.ID, msg.ID, lang)
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminTariffNewHandler — старт мастера создания.
func (h Handler) AdminTariffNewHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	adminID := cb.From.ID
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	adminTariffReset(adminID)
	adminTariffCtx.mu.Lock()
	adminTariffCtx.mode[adminID] = "wiz"
	adminTariffCtx.wizStep[adminID] = "name"
	adminTariffCtx.draft[adminID] = &tariffWizardDraft{}
	adminTariffCtx.mu.Unlock()
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		ParseMode: models.ParseModeHTML,
		Text:      h.translation.GetText(lang, "tariff_wizard_name"),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminTariffs})},
		}},
	})
	if err != nil {
		slog.Error("tariff new", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminTariffCallbackRouter — callback с префиксом tf_ (кроме tf_new — отдельная регистрация).
func (h Handler) AdminTariffCallbackRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	data := update.CallbackQuery.Data
	if data == CallbackTariffNew {
		return
	}
	prefix := strings.SplitN(data, "?", 2)[0]
	switch prefix {
	case tariffCallbackView:
		h.AdminTariffViewHandler(ctx, b, update)
	case tariffCallbackToggle:
		h.AdminTariffToggleHandler(ctx, b, update)
	case tariffCallbackDel:
		h.AdminTariffDeleteAskHandler(ctx, b, update)
	case tariffCallbackDelYes:
		h.AdminTariffDeleteYesHandler(ctx, b, update)
	case tariffCallbackSrv:
		h.AdminTariffServersHandler(ctx, b, update)
	case tariffCallbackSq:
		h.AdminTariffSquadToggleHandler(ctx, b, update)
	case tariffCallbackClr:
		h.AdminTariffSquadClearHandler(ctx, b, update)
	case tariffCallbackAll:
		h.AdminTariffSquadAllHandler(ctx, b, update)
	case tariffCallbackNm, tariffCallbackTT, tariffCallbackTD, tariffCallbackTL, tariffCallbackEp, tariffCallbackDs:
		h.AdminTariffEditAskHandler(ctx, b, update)
	case tariffCallbackCancel:
		h.AdminTariffEditCancelHandler(ctx, b, update)
	case tariffCallbackWizCan:
		h.AdminTariffWizardCancelHandler(ctx, b, update)
	case tariffCallbackLoc, tariffCallbackLocTog, tariffCallbackLocDel, tariffCallbackLocAdd:
		h.AdminTariffLocationsHandler(ctx, b, update)
	default:
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
	}
}

// AdminTariffTextHandler — ввод в мастере и при редактировании.
func (h Handler) AdminTariffTextHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From.ID != config.GetAdminTelegramId() {
		return
	}
	adminID := update.Message.From.ID
	lang := update.Message.From.LanguageCode
	msgText := update.Message.Text
	if strings.TrimSpace(msgText) == "" {
		return
	}
	text := strings.TrimSpace(msgText)

	adminTariffCtx.mu.Lock()
	mode := adminTariffCtx.mode[adminID]
	step := adminTariffCtx.wizStep[adminID]
	d := adminTariffCtx.draft[adminID]
	editID := adminTariffCtx.editID[adminID]
	editField := adminTariffCtx.editField[adminID]
	adminTariffCtx.mu.Unlock()

	if mode == "wiz" && d != nil {
		switch step {
		case "name":
			if len(text) < 1 || len(text) > 200 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_name")})
				return
			}
			d.Name = text
			adminTariffCtx.mu.Lock()
			adminTariffCtx.wizStep[adminID] = "traffic"
			adminTariffCtx.mu.Unlock()
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_wizard_traffic"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.wizardCancelKeyboard(lang)},
			})
			return
		case "traffic":
			gb, err := strconv.ParseInt(text, 10, 64)
			if err != nil || gb < 0 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_number")})
				return
			}
			d.TrafficGB = gb
			adminTariffCtx.mu.Lock()
			adminTariffCtx.wizStep[adminID] = "devices"
			adminTariffCtx.mu.Unlock()
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_wizard_devices"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.wizardCancelKeyboard(lang)},
			})
			return
		case "devices":
			dev, err := strconv.Atoi(text)
			if err != nil || dev < 1 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_number")})
				return
			}
			d.Devices = dev
			adminTariffCtx.mu.Lock()
			adminTariffCtx.wizStep[adminID] = "tier"
			adminTariffCtx.mu.Unlock()
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_wizard_tier"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.wizardCancelKeyboard(lang)},
			})
			return
		case "tier":
			tier, err := strconv.Atoi(text)
			if err != nil || tier < 1 || tier > 10 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_tier")})
				return
			}
			d.Tier = tier
			adminTariffCtx.mu.Lock()
			adminTariffCtx.wizStep[adminID] = "rub"
			adminTariffCtx.mu.Unlock()
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_wizard_rub"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.wizardCancelKeyboard(lang)},
			})
			return
		case "rub":
			rub, err := parseFourInts(text)
			if err != nil {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_prices")})
				return
			}
			d.Rub = rub
			adminTariffCtx.mu.Lock()
			adminTariffCtx.wizStep[adminID] = "stars"
			adminTariffCtx.mu.Unlock()
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_wizard_stars"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.wizardCancelKeyboard(lang)},
			})
			return
		case "stars":
			var stars [4]*int
			low := strings.ToLower(text)
			if low == "-" || low == "—" || low == "auto" {
				if config.RubPerStar() <= 0 {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_stars_auto")})
					return
				}
				stars = starsFromRub(d.Rub)
			} else {
				s, err := parseFourStars(text)
				if err != nil {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_prices")})
					return
				}
				stars = s
			}
			newID, err := h.finalizeTariffWizard(ctx, adminID, d, stars)
			if err != nil {
				slog.Error("finalize tariff wizard", "error", err)
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_save")})
				return
			}
			adminTariffReset(adminID)
			kb := append(h.tariffSavedKeyboard(lang, newID), []models.InlineKeyboardButton{
				h.translation.WithButton(lang, "admin_tariffs", models.InlineKeyboardButton{CallbackData: CallbackAdminTariffs}),
			})
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminID,
				Text:        h.translation.GetText(lang, "tariff_created_ok"),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
			})
			return
		}
	}

	if mode == "edit" && editID > 0 {
		switch editField {
		case "name":
			if len(text) < 1 || len(text) > 200 {
				return
			}
			_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"name": text})
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_name"))
			return
		case "traffic":
			gb, err := strconv.ParseInt(text, 10, 64)
			if err != nil || gb < 0 {
				return
			}
			var tb int64
			if gb == 0 {
				tb = 0
			} else {
				tb = gb * bytesInGB()
			}
			_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"traffic_limit_bytes": tb})
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_traffic"))
			return
		case "devices":
			dev, err := strconv.Atoi(text)
			if err != nil || dev < 1 {
				return
			}
			_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"device_limit": dev})
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_devices"))
			return
		case "tier":
			tier, err := strconv.Atoi(text)
			if err != nil || tier < 1 || tier > 10 {
				return
			}
			_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"tier_level": tier, "sort_order": tier})
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_tier"))
			return
		case "prices":
			parts := strings.SplitN(text, "|", 2)
			if len(parts) != 2 {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_prices_edit")})
				return
			}
			rub, err := parseFourInts(strings.TrimSpace(parts[0]))
			if err != nil {
				return
			}
			var stars [4]*int
			starsPart := strings.TrimSpace(strings.ToLower(parts[1]))
			if starsPart == "auto" {
				if config.RubPerStar() <= 0 {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_stars_auto")})
					return
				}
				stars = starsFromRub(rub)
			} else {
				var parseErr error
				stars, parseErr = parseFourStars(strings.TrimSpace(parts[1]))
				if parseErr != nil {
					return
				}
			}
			if err := h.tariffRepository.ReplaceAllPrices(ctx, editID, rub, stars); err != nil {
				slog.Error("replace prices", "error", err)
				return
			}
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_prices"))
			return
		case "description":
			low := strings.ToLower(text)
			if low == "-" || low == "—" {
				_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"description": nil})
			} else {
				if utf16UnitsLen(msgText) > 1024 {
					_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_err_description_len")})
					return
				}
				ents := append([]models.MessageEntity(nil), update.Message.Entities...)
				toStore := strings.TrimSpace(messageEntitiesToHTML(msgText, ents))
				_ = h.tariffRepository.UpdateTariff(ctx, editID, map[string]interface{}{"description": toStore})
			}
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffFullCard(ctx, b, adminID, lang, tid, h.translation.GetText(lang, "tariff_edit_saved_description"))
			return
		case "location":
			name, code, ok := parseTariffLocationInput(text)
			if !ok {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_locations_bad")})
				return
			}
			if err := h.tariffRepository.InsertLocation(ctx, editID, name, code); err != nil {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "tariff_locations_bad")})
				return
			}
			tid := editID
			adminTariffReset(adminID)
			h.sendAdminTariffLocations(ctx, b, adminID, 0, lang, tid)
			return
		}
	}
}

func parseFourStars(s string) ([4]*int, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return [4]*int{}, fmt.Errorf("bad")
	}
	var out [4]*int
	for i := range 4 {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 {
			return [4]*int{}, fmt.Errorf("bad")
		}
		v := n
		out[i] = &v
	}
	return out, nil
}

func (h Handler) finalizeTariffWizard(ctx context.Context, adminID int64, d *tariffWizardDraft, stars [4]*int) (int64, error) {
	base := slugifyTariffName(d.Name)
	slug := base
	for i := 0; i < 50; i++ {
		if i > 0 {
			slug = fmt.Sprintf("%s_%d", base, i)
		}
		ok, err := h.tariffRepository.SlugExists(ctx, slug)
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		if i == 49 {
			return 0, fmt.Errorf("slug unique")
		}
	}
	var tb int64
	if d.TrafficGB == 0 {
		tb = 0
	} else {
		tb = d.TrafficGB * bytesInGB()
	}
	tier := d.Tier
	name := d.Name
	t := &database.Tariff{
		Slug:                      slug,
		Name:                      &name,
		SortOrder:                 d.Tier,
		IsActive:                  true,
		DeviceLimit:               d.Devices,
		TrafficLimitBytes:         tb,
		TrafficLimitResetStrategy: config.TrafficLimitResetStrategy(),
		ActiveInternalSquadUUIDs:  "",
		ExternalSquadUUID:         nil,
		RemnawaveTag:              nil,
		TierLevel:                 &tier,
	}
	if tag := config.RemnawaveTag(); tag != "" {
		t.RemnawaveTag = &tag
	}
	ext := config.ExternalSquadUUID()
	if ext != uuid.Nil {
		t.ExternalSquadUUID = &ext
	}
	id, err := h.tariffRepository.CreateWithPrices(ctx, t, d.Rub, stars)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// AdminTariffViewHandler — карточка тарифа (prefix tf_v?i=).
func (h Handler) AdminTariffViewHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	q := parseCallbackData(cb.Data)
	id, _ := strconv.ParseInt(q["i"], 10, 64)
	if id <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	t, err := h.tariffRepository.GetByID(ctx, id)
	if err != nil || t == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	prices, _ := h.tariffRepository.ListPricesForTariff(ctx, id)
	locs, _ := h.tariffRepository.ListLocations(ctx, id)
	nPur, _ := h.tariffRepository.CountPurchasesForTariff(ctx, id)
	text := h.formatTariffCard(lang, t, prices, locs, nPur)
	kb := h.tariffAdminCardKeyboard(lang, id, t)
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
	if err != nil {
		slog.Error("tariff view", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func (h Handler) tariffAdminCardKeyboard(lang string, id int64, t *database.Tariff) [][]models.InlineKeyboardButton {
	activeLabel := "tariff_btn_toggle_off"
	if !t.IsActive {
		activeLabel = "tariff_btn_toggle_on"
	}
	return [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "tariff_btn_rename", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackNm, id)}),
			h.translation.WithButton(lang, "tariff_btn_description", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackDs, id)}),
		},
		{
			h.translation.WithButton(lang, "tariff_btn_devices", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackTD, id)}),
			h.translation.WithButton(lang, "tariff_btn_traffic", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackTT, id)}),
		},
		{h.translation.WithButton(lang, "tariff_btn_tier", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackTL, id)})},
		{h.translation.WithButton(lang, "tariff_btn_prices", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackEp, id)})},
		{h.translation.WithButton(lang, "tariff_btn_servers", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackSrv, id)})},
		{h.translation.WithButton(lang, "tariff_btn_locations", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackLoc, id)})},
		{
			h.translation.WithButton(lang, activeLabel, models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackToggle, id)}),
			h.translation.WithButton(lang, "tariff_btn_delete", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackDel, id)}),
		},
		{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminTariffs})},
	}
}

func (h Handler) sendAdminTariffFullCard(ctx context.Context, b *bot.Bot, adminID int64, lang string, tid int64, topBanner string) {
	t, err := h.tariffRepository.GetByID(ctx, tid)
	if err != nil || t == nil {
		return
	}
	prices, _ := h.tariffRepository.ListPricesForTariff(ctx, tid)
	locs, _ := h.tariffRepository.ListLocations(ctx, tid)
	nPur, _ := h.tariffRepository.CountPurchasesForTariff(ctx, tid)
	body := h.formatTariffCard(lang, t, prices, locs, nPur)
	text := body
	if strings.TrimSpace(topBanner) != "" {
		text = topBanner + "\n\n" + body
	}
	kb := h.tariffAdminCardKeyboard(lang, tid, t)
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
}

func (h Handler) formatTariffCard(lang string, t *database.Tariff, prices []database.TariffPrice, locs []database.TariffLocation, nPurch int64) string {
	dim := config.DaysInMonth()
	var sb strings.Builder
	name := escapeHTML(displayTariffName(t))
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_tariff"), name))
	if t.IsActive {
		sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_active"))
	} else {
		sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_inactive"))
	}
	if t.TierLevel != nil {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_tier"), *t.TierLevel))
	}
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_sort"), t.SortOrder))
	sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_params_header"))
	var trafficVal string
	if t.TrafficLimitBytes > 0 {
		gb := t.TrafficLimitBytes / bytesInGB()
		trafficVal = fmt.Sprintf(h.translation.GetText(lang, "payment_tariff_traffic_gb"), gb)
	} else {
		trafficVal = h.translation.GetText(lang, "payment_tariff_traffic_unlim")
	}
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_traffic_bullet"), trafficVal))
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_devices_bullet"), t.DeviceLimit))
	if len(locs) == 0 {
		sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_locations_empty"))
	} else {
		sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_locations_header"))
		for _, loc := range locs {
			mark := ""
			if loc.SoldOut {
				mark = h.translation.GetText(lang, "tariff_admin_card_location_sold")
			}
			sb.WriteString(fmt.Sprintf("• %s%s\n", escapeHTML(loc.Name), mark))
		}
	}
	sb.WriteString(h.translation.GetText(lang, "tariff_admin_card_prices_header"))
	for _, p := range prices {
		days := p.Months * dim
		starPart := ""
		if p.AmountStars != nil {
			starPart = fmt.Sprintf(h.translation.GetText(lang, "tariff_price_stars_suffix"), *p.AmountStars)
		}
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_price_line"), days, p.Months, p.AmountRub, starPart))
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_purchases"), nPurch))
	sq := strings.TrimSpace(t.ActiveInternalSquadUUIDs)
	if sq == "" {
		sb.WriteString(h.translation.GetText(lang, "tariff_card_servers_all"))
	} else {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_card_servers_some"), len(strings.Split(sq, ","))))
	}
	if t.Description != nil && strings.TrimSpace(*t.Description) != "" {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "tariff_admin_card_description"), strings.TrimSpace(*t.Description)))
	}
	return sb.String()
}

// AdminTariffEditAskHandler — префиксы tf_nm, tf_tt, tf_td, tf_tl, tf_ep, tf_ds.
func (h Handler) AdminTariffEditAskHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	prefix := strings.SplitN(cb.Data, "?", 2)[0]
	q := parseCallbackData(cb.Data)
	id, _ := strconv.ParseInt(q["i"], 10, 64)
	if id <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	t, err := h.tariffRepository.GetByID(ctx, id)
	if err != nil || t == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	lang := cb.From.LanguageCode
	adminID := cb.From.ID
	dn := escapeHTML(displayTariffName(t))
	var field string
	var prompt string
	switch prefix {
	case tariffCallbackNm:
		field = "name"
		prompt = fmt.Sprintf(h.translation.GetText(lang, "tariff_edit_prompt_name"), dn)
	case tariffCallbackTT:
		field = "traffic"
		prompt = fmt.Sprintf(h.translation.GetText(lang, "tariff_edit_prompt_traffic"), dn)
	case tariffCallbackTD:
		field = "devices"
		prompt = fmt.Sprintf(h.translation.GetText(lang, "tariff_edit_prompt_devices"), dn)
	case tariffCallbackTL:
		field = "tier"
		prompt = fmt.Sprintf(h.translation.GetText(lang, "tariff_edit_prompt_tier"), dn)
	case tariffCallbackEp:
		field = "prices"
		prompt = fmt.Sprintf(h.translation.GetText(lang, "tariff_edit_prompt_prices"), dn)
	case tariffCallbackDs:
		field = "description"
		cur := h.translation.GetText(lang, "tariff_edit_description_none")
		if t.Description != nil && strings.TrimSpace(*t.Description) != "" {
			// Сохранённый текст уже в HTML (ParseMode HTML); не экранируем — иначе видны теги и &#34;.
			cur = strings.TrimSpace(*t.Description)
		}
		prompt = strings.Replace(h.translation.GetText(lang, "tariff_edit_description_prompt"), "%s", cur, 1)
	default:
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	adminTariffReset(adminID)
	adminTariffCtx.mu.Lock()
	adminTariffCtx.mode[adminID] = "edit"
	adminTariffCtx.editID[adminID] = id
	adminTariffCtx.editField[adminID] = field
	adminTariffCtx.mu.Unlock()
	cancelRow := []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "tariff_btn_cancel_edit", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackCancel, id)}),
	}
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        prompt,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{cancelRow}},
	})
	if err != nil {
		slog.Error("tariff edit ask", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminTariffEditCancelHandler — отмена ввода при редактировании поля (возврат к карточке).
func (h Handler) AdminTariffEditCancelHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	if id <= 0 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	adminTariffReset(cb.From.ID)
	cb.Data = fmt.Sprintf("%s?i=%d", tariffCallbackView, id)
	h.AdminTariffViewHandler(ctx, b, update)
}

// AdminTariffWizardCancelHandler — отмена мастера создания (сообщение с шагом заменяется на текст + к списку).
func (h Handler) AdminTariffWizardCancelHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	adminTariffReset(cb.From.ID)
	if msg != nil {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(lang, "tariff_wizard_cancelled"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{h.translation.WithButton(lang, "admin_tariffs", models.InlineKeyboardButton{CallbackData: CallbackAdminTariffs})},
			}},
		})
		if err != nil {
			slog.Error("tariff wizard cancel edit", "error", err)
		}
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminTariffToggleHandler — tf_t?i=.
func (h Handler) AdminTariffToggleHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	t, err := h.tariffRepository.GetByID(ctx, id)
	if err != nil || t == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	_ = h.tariffRepository.UpdateTariff(ctx, id, map[string]interface{}{"is_active": !t.IsActive})
	h.AdminTariffViewHandler(ctx, b, update)
}

// AdminTariffDeleteAskHandler — tf_d?
func (h Handler) AdminTariffDeleteAskHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	msg := cb.Message.Message
	kb := [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "tariff_delete_yes", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackDelYes, id)})},
		{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackView, id)})},
	}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        h.translation.GetText(lang, "tariff_delete_confirm"),
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
	if err != nil {
		slog.Error("tariff del ask", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminTariffDeleteYesHandler — tf_y?
func (h Handler) AdminTariffDeleteYesHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	msg := cb.Message.Message
	if err := h.tariffRepository.DeleteTariff(ctx, id); err != nil {
		slog.Error("tariff delete", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID, Text: h.translation.GetText(lang, "tariff_err_delete"), ShowAlert: true})
		return
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            h.translation.GetText(lang, "tariff_deleted_ok"),
		ShowAlert:       true,
	})
	if msg == nil {
		return
	}
	_ = h.editAdminTariffsMessage(ctx, b, msg.Chat.ID, msg.ID, lang)
}

// AdminTariffServersHandler — список squads.
func (h Handler) AdminTariffServersHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	t, err := h.tariffRepository.GetByID(ctx, id)
	if err != nil || t == nil {
		return
	}
	squads, err := h.remnawaveClient.ListInternalSquads(ctx)
	if err != nil {
		slog.Error("list squads", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID, Text: "Remnawave", ShowAlert: true})
		return
	}
	selected, _ := database.ParseSquadUUIDList(t.ActiveInternalSquadUUIDs)
	sel := make(map[uuid.UUID]struct{})
	for _, u := range selected {
		sel[u] = struct{}{}
	}
	var rows [][]models.InlineKeyboardButton
	for _, s := range squads {
		mark := "☐"
		if _, ok := sel[s.UUID]; ok {
			mark = "✅"
		}
		label := fmt.Sprintf("%s %s", mark, s.Name)
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("%s?i=%d&u=%s", tariffCallbackSq, id, s.UUID.String())},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(lang, "tariff_srv_clear"), CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackClr, id)},
		{Text: h.translation.GetText(lang, "tariff_srv_all"), CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackAll, id)},
	})
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackView, id)}),
	})
	text := fmt.Sprintf(h.translation.GetText(lang, "tariff_servers_title"), displayTariffName(t), len(selected), len(squads))
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	if err != nil {
		slog.Error("tariff servers", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func displayTariffName(t *database.Tariff) string {
	if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
		return *t.Name
	}
	return t.Slug
}

func (h Handler) tariffSavedKeyboard(lang string, tariffID int64) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "tariff_back_to_card", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s?i=%d", tariffCallbackView, tariffID)})},
	}
}

func (h Handler) wizardCancelKeyboard(lang string) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "tariff_wizard_cancel", models.InlineKeyboardButton{CallbackData: tariffCallbackWizCan})},
	}
}

// AdminTariffSquadToggleHandler — tf_q?
func (h Handler) AdminTariffSquadToggleHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	q := parseCallbackData(cb.Data)
	id, _ := strconv.ParseInt(q["i"], 10, 64)
	uidStr := q["u"]
	u, err := uuid.Parse(uidStr)
	if err != nil {
		return
	}
	t, err := h.tariffRepository.GetByID(ctx, id)
	if err != nil || t == nil {
		return
	}
	list, _ := database.ParseSquadUUIDList(t.ActiveInternalSquadUUIDs)
	sel := make(map[uuid.UUID]struct{})
	for _, x := range list {
		sel[x] = struct{}{}
	}
	if _, ok := sel[u]; ok {
		delete(sel, u)
	} else {
		sel[u] = struct{}{}
	}
	var parts []string
	for x := range sel {
		parts = append(parts, x.String())
	}
	newStr := strings.Join(parts, ",")
	_ = h.tariffRepository.UpdateTariff(ctx, id, map[string]interface{}{"active_internal_squad_uuids": newStr})
	h.AdminTariffServersHandler(ctx, b, update)
}

// AdminTariffSquadClearHandler — tf_sc?
func (h Handler) AdminTariffSquadClearHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	id, _ := strconv.ParseInt(parseCallbackData(update.CallbackQuery.Data)["i"], 10, 64)
	_ = h.tariffRepository.UpdateTariff(ctx, id, map[string]interface{}{"active_internal_squad_uuids": ""})
	h.AdminTariffServersHandler(ctx, b, update)
}

// AdminTariffSquadAllHandler — tf_sa?
func (h Handler) AdminTariffSquadAllHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	id, _ := strconv.ParseInt(parseCallbackData(cb.Data)["i"], 10, 64)
	squads, err := h.remnawaveClient.ListInternalSquads(ctx)
	if err != nil {
		return
	}
	var parts []string
	for _, s := range squads {
		parts = append(parts, s.UUID.String())
	}
	_ = h.tariffRepository.UpdateTariff(ctx, id, map[string]interface{}{"active_internal_squad_uuids": strings.Join(parts, ",")})
	h.AdminTariffServersHandler(ctx, b, update)
}
