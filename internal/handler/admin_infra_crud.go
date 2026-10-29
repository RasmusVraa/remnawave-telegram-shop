package handler

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/remnawave"
)

// Callback-префиксы CRUD (не пересекаются с ib_*): ifn* — ноды, ifp* — провайдеры, ifh* — история.
const (
	cbInfraNodeDetail   = "ifnd" // + billingUuid (36)
	cbInfraNodeEditDate = "ifne"
	cbInfraNodeDelete   = "ifnk"
	cbInfraNodeCreate   = "ifnc"
	cbInfraNodePickProv = "ifnt" // + providerUuid (4+36)
	cbInfraNodePickNode = "ifnx" // + nodeUuid

	cbInfraProvCreate    = "ifpc"
	cbInfraProvDetail    = "ifpd"
	cbInfraProvEditMenu  = "ifpe"
	cbInfraProvDelete    = "ifpk" // + uuid — запрос подтверждения удаления
	cbInfraProvDeleteYes = "ifpy" // + uuid — подтвердить удаление
	cbInfraProvDeleteNo  = "ifpx" // + uuid — отмена
	cbInfraProvWizName   = "ifpm" // + providerUuid — правка имени
	cbInfraProvWizIcon   = "ifpv" // + providerUuid — favicon
	cbInfraProvWizLogin  = "ifpl" // + providerUuid — loginUrl

	cbInfraHistCreate     = "ifhc"
	cbInfraHistPickProv   = "ifht" // + providerUuid — создание записи истории
	cbInfraHistDelete     = "ifhk" // + recordUuid — удалить выбранную запись
	cbInfraHistDeleteMenu = "ifhm" // меню выбора записи для удаления
	cbInfraWizBackProv    = "ifbp" // отмена мастера → список провайдеров
	cbInfraWizBackNodes   = "ifbn" // → список нод
	cbInfraWizBackHist    = "ifbh" // → история (текущая страница)
)

const infraUUIDCallbackLen = 4 + 36

var infraHistLastPage sync.Map // admin telegram id -> последняя страница истории

func setInfraHistLastPage(adminID int64, page int) {
	infraHistLastPage.Store(adminID, page)
}

func getInfraHistLastPage(adminID int64) int {
	if v, ok := infraHistLastPage.Load(adminID); ok {
		if p, ok := v.(int); ok && p > 0 {
			return p
		}
	}
	return 1
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 2 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func parseInfraUUIDCallback(prefix, data string) (uuid.UUID, bool) {
	if !strings.HasPrefix(data, prefix) || len(data) != infraUUIDCallbackLen {
		return uuid.Nil, false
	}
	u, err := uuid.Parse(strings.TrimPrefix(data, prefix))
	return u, err == nil
}

func (h Handler) infraProviderLoginAnchor(lang, loginURL string) string {
	u := strings.TrimSpace(loginURL)
	if u == "" {
		return ""
	}
	lbl := html.EscapeString(h.translation.GetText(lang, "admin_infra_prov_login_link_text"))
	return " · " + fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(u), lbl)
}

func (h Handler) infraProvCardHTML(lang string, p *remnawave.InfraBillingProviderItem) string {
	return fmt.Sprintf(h.translation.GetText(lang, "admin_infra_prov_card"),
		html.EscapeString(p.Name),
		formatInfraAmount(p.BillingHistory.TotalAmount),
		p.BillingHistory.TotalBills,
		h.infraProviderLoginAnchor(lang, p.LoginURL),
	)
}

func (h Handler) infraProvDetailKeyboard(lang string, pu uuid.UUID) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_infra_prov_edit_menu", models.InlineKeyboardButton{CallbackData: cbInfraProvEditMenu + pu.String()}),
			h.translation.WithButton(lang, "admin_infra_prov_delete", models.InlineKeyboardButton{CallbackData: cbInfraProvDelete + pu.String()}),
		},
		{h.translation.WithButton(lang, "admin_infra_prov_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraProv})},
		h.adminInfraBackRootRow(lang)[0],
	}
}

func parseInfraAdminDateInput(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	loc := adminInfraTZ()
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("02.01.2006 15:04", s, loc); err == nil {
		return t.UTC(), nil
	}
	now := time.Now().In(loc)
	for _, lay := range []string{"02.01.2006", "02.01.06"} {
		d, err := time.ParseInLocation(lay, s, loc)
		if err != nil {
			continue
		}
		combined := time.Date(d.Year(), d.Month(), d.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), loc)
		return combined.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("parse date")
}

// iso3166Alpha2ToFlagEmoji — флаг из двух латинских букв (UK → GB).
func iso3166Alpha2ToFlagEmoji(alpha2 string) string {
	a := strings.ToUpper(strings.TrimSpace(alpha2))
	if len(a) != 2 || a[0] < 'A' || a[0] > 'Z' || a[1] < 'A' || a[1] > 'Z' {
		return ""
	}
	if a == "UK" {
		a = "GB"
	}
	const base = 0x1F1E6 // Regional Indicator Symbol Letter A
	return string([]rune{base + rune(a[0]-'A'), base + rune(a[1]-'A')})
}

// infraNodeCountryParenSuffix — суффикс после имени провайдера в скобках: пробел + флаг или « XX».
func (h Handler) infraProvEditDoneReplyMarkup(lang string, providerUUID uuid.UUID) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "admin_infra_prov_back_card", models.InlineKeyboardButton{CallbackData: cbInfraProvDetail + providerUUID.String()})},
		{h.translation.WithButton(lang, "admin_infra_back_root", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraRoot})},
	}}
}

func infraNodeCountryParenSuffix(countryCode string) string {
	raw := strings.TrimSpace(countryCode)
	if raw == "" {
		return ""
	}
	c := strings.ToUpper(raw)
	if c == "XX" {
		return " XX"
	}
	if len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' {
		return " " + iso3166Alpha2ToFlagEmoji(c)
	}
	return " " + html.EscapeString(raw)
}

func infraNodesBackKeyboard(lang string, h Handler) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "admin_infra_nodes_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraNodes})},
		h.adminInfraBackRootRow(lang)[0],
	}
}

// infraNodesScreen — текст и клавиатура списка оплачиваемых нод (с CRUD).
func (h Handler) infraNodesScreen(lang string, body *remnawave.InfraBillingNodesBody) (string, [][]models.InlineKeyboardButton) {
	nodes := append([]remnawave.InfraBillingBillingNode(nil), body.BillingNodes...)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NextBillingAt.Before(nodes[j].NextBillingAt)
	})
	loc := adminInfraTZ()
	var sb strings.Builder
	sb.WriteString(h.translation.GetText(lang, "admin_infra_nodes_title"))
	sb.WriteString("\n\n")
	if len(nodes) == 0 {
		sb.WriteString(h.translation.GetText(lang, "admin_infra_nodes_empty"))
	} else {
		for i := range nodes {
			n := &nodes[i]
			when := n.NextBillingAt.In(loc).Format("02.01.2006 15:04")
			line := fmt.Sprintf(h.translation.GetText(lang, "admin_infra_nodes_line"),
				html.EscapeString(n.Node.Name),
				html.EscapeString(n.Provider.Name),
				infraNodeCountryParenSuffix(n.Node.CountryCode),
				when,
			)
			if sb.Len()+len(line)+2 > 3200 {
				sb.WriteString("\n… ")
				sb.WriteString(h.translation.GetText(lang, "admin_infra_list_truncated"))
				break
			}
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(h.translation.GetText(lang, "admin_infra_nodes_pick_hint"))

	var kb [][]models.InlineKeyboardButton
	if len(body.AvailableBillingNodes) > 0 {
		kb = append(kb, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_infra_nodes_add", models.InlineKeyboardButton{CallbackData: cbInfraNodeCreate}),
		})
	}
	maxPick := 15
	for i := range nodes {
		if i >= maxPick {
			break
		}
		n := &nodes[i]
		kb = append(kb, []models.InlineKeyboardButton{{
			Text:         truncateRunes(n.Node.Name, 30),
			CallbackData: cbInfraNodeDetail + n.UUID.String(),
		}})
	}
	if len(nodes) > maxPick {
		kb = append(kb, []models.InlineKeyboardButton{
			{Text: "…", CallbackData: CallbackAdminInfraNodes},
		})
	}
	kb = append(kb, h.adminInfraBackRootRow(lang)...)
	return sb.String(), kb
}

func (h Handler) renderAdminInfraNodesScreen(ctx context.Context, b *bot.Bot, chatID int64, msgID int, lang string) error {
	body, err := h.remnawaveClient.GetInfraBillingNodes(ctx)
	if err != nil {
		return err
	}
	text, kb := h.infraNodesScreen(lang, body)
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
	return err
}

// AdminInfraNodeCRUDRouter — префикс ifn* (ноды).
func (h Handler) AdminInfraNodeCRUDRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	data := cb.Data
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode

	switch {
	case data == cbInfraNodeCreate:
		infraWizClear(cb.From.ID)
		body, err := h.remnawaveClient.GetInfraBillingNodes(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		if len(body.AvailableBillingNodes) == 0 {
			_ = h.renderAdminInfraNodesScreen(ctx, b, msg.Chat.ID, msg.ID, lang)
			return
		}
		providers, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var sb strings.Builder
		sb.WriteString(h.translation.GetText(lang, "admin_infra_node_create_pick_provider"))
		var rows [][]models.InlineKeyboardButton
		n := 0
		for i := range providers.Providers {
			p := &providers.Providers[i]
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         truncateRunes(p.Name, 36),
				CallbackData: cbInfraNodePickProv + p.UUID.String(),
			}})
			n++
			if n >= 20 {
				break
			}
		}
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_infra_nodes_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraNodes}),
		})
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        sb.String(),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra node create pick prov", err)

	case strings.HasPrefix(data, cbInfraNodePickProv):
		pu, ok := parseInfraUUIDCallback(cbInfraNodePickProv, data)
		if !ok {
			return
		}
		body, err := h.remnawaveClient.GetInfraBillingNodes(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizNodeAwaitNode, ProviderUUID: pu})
		var sb strings.Builder
		sb.WriteString(h.translation.GetText(lang, "admin_infra_node_create_pick_node"))
		var rows [][]models.InlineKeyboardButton
		for i := range body.AvailableBillingNodes {
			an := &body.AvailableBillingNodes[i]
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         truncateRunes(an.Name, 36),
				CallbackData: cbInfraNodePickNode + an.UUID.String(),
			}})
		}
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_infra_nodes_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraNodes}),
		})
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        sb.String(),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra node create pick node", err)

	case strings.HasPrefix(data, cbInfraNodePickNode):
		nu, ok := parseInfraUUIDCallback(cbInfraNodePickNode, data)
		if !ok {
			return
		}
		st, wok := infraWizGet(cb.From.ID)
		if !wok || st.Kind != infraWizNodeAwaitNode {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{
			Kind:         infraWizNodeCreateDate,
			ProviderUUID: st.ProviderUUID,
			NodeUUID:     nu,
		})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_node_create_date_prompt"), cbInfraWizBackNodes)

	case strings.HasPrefix(data, cbInfraNodeDetail):
		bu, ok := parseInfraUUIDCallback(cbInfraNodeDetail, data)
		if !ok {
			return
		}
		body, err := h.remnawaveClient.GetInfraBillingNodes(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var node *remnawave.InfraBillingBillingNode
		for i := range body.BillingNodes {
			if body.BillingNodes[i].UUID == bu {
				node = &body.BillingNodes[i]
				break
			}
		}
		if node == nil {
			_ = h.renderAdminInfraNodesScreen(ctx, b, msg.Chat.ID, msg.ID, lang)
			return
		}
		loc := adminInfraTZ()
		when := node.NextBillingAt.In(loc).Format("02.01.2006 15:04")
		text := fmt.Sprintf(h.translation.GetText(lang, "admin_infra_node_card"),
			html.EscapeString(node.Node.Name),
			html.EscapeString(node.Provider.Name),
			infraNodeCountryParenSuffix(node.Node.CountryCode),
			when,
		)
		rows := [][]models.InlineKeyboardButton{
			{
				h.translation.WithButton(lang, "admin_infra_node_edit_date", models.InlineKeyboardButton{CallbackData: cbInfraNodeEditDate + bu.String()}),
				h.translation.WithButton(lang, "admin_infra_node_delete", models.InlineKeyboardButton{CallbackData: cbInfraNodeDelete + bu.String()}),
			},
			infraNodesBackKeyboard(lang, h)[0],
		}
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        text,
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra node detail", err)

	case strings.HasPrefix(data, cbInfraNodeEditDate):
		bu, ok := parseInfraUUIDCallback(cbInfraNodeEditDate, data)
		if !ok {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizNodeNextDate, BillingUUID: bu})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_node_date_prompt"), cbInfraWizBackNodes)

	case strings.HasPrefix(data, cbInfraNodeDelete):
		bu, ok := parseInfraUUIDCallback(cbInfraNodeDelete, data)
		if !ok {
			return
		}
		if err := h.remnawaveClient.DeleteInfraBillingNode(ctx, bu); err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		if err := h.renderAdminInfraNodesScreen(ctx, b, msg.Chat.ID, msg.ID, lang); err != nil {
			logEditError("infra node delete refresh", err)
		}
	}
}

// AdminInfraProviderCRUDRouter — префикс ifp*.
func (h Handler) AdminInfraProviderCRUDRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	data := cb.Data
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode

	switch {
	case data == cbInfraProvCreate:
		infraWizClear(cb.From.ID)
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizProvCreateName})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_name_prompt"), cbInfraWizBackProv)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})

	case strings.HasPrefix(data, cbInfraProvDetail):
		pu, ok := parseInfraUUIDCallback(cbInfraProvDetail, data)
		if !ok {
			return
		}
		body, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var p *remnawave.InfraBillingProviderItem
		for i := range body.Providers {
			if body.Providers[i].UUID == pu {
				p = &body.Providers[i]
				break
			}
		}
		if p == nil {
			_ = h.renderAdminInfraProvidersScreen(ctx, b, msg.Chat.ID, msg.ID, lang)
			return
		}
		text := h.infraProvCardHTML(lang, p)
		rows := h.infraProvDetailKeyboard(lang, pu)
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:             msg.Chat.ID,
			MessageID:          msg.ID,
			ParseMode:          models.ParseModeHTML,
			Text:               text,
			LinkPreviewOptions: infraNoLinkPreview(),
			ReplyMarkup:        models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra prov detail", err)

	case strings.HasPrefix(data, cbInfraProvEditMenu):
		pu, ok := parseInfraUUIDCallback(cbInfraProvEditMenu, data)
		if !ok {
			return
		}
		rows := [][]models.InlineKeyboardButton{
			{
				h.translation.WithButton(lang, "admin_infra_prov_edit_name", models.InlineKeyboardButton{CallbackData: cbInfraProvWizName + pu.String()}),
				h.translation.WithButton(lang, "admin_infra_prov_edit_icon", models.InlineKeyboardButton{CallbackData: cbInfraProvWizIcon + pu.String()}),
			},
			{
				h.translation.WithButton(lang, "admin_infra_prov_edit_login", models.InlineKeyboardButton{CallbackData: cbInfraProvWizLogin + pu.String()}),
			},
			{h.translation.WithButton(lang, "admin_infra_prov_back_card", models.InlineKeyboardButton{CallbackData: cbInfraProvDetail + pu.String()})},
		}
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:             msg.Chat.ID,
			MessageID:          msg.ID,
			ParseMode:          models.ParseModeHTML,
			Text:               h.translation.GetText(lang, "admin_infra_prov_edit_title"),
			LinkPreviewOptions: infraNoLinkPreview(),
			ReplyMarkup:        models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra prov edit menu", err)

	case strings.HasPrefix(data, cbInfraProvDeleteYes):
		pu, ok := parseInfraUUIDCallback(cbInfraProvDeleteYes, data)
		if !ok {
			return
		}
		if err := h.remnawaveClient.DeleteInfraProvider(ctx, pu); err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		_ = h.renderAdminInfraProvidersScreen(ctx, b, msg.Chat.ID, msg.ID, lang)

	case strings.HasPrefix(data, cbInfraProvDeleteNo):
		pu, ok := parseInfraUUIDCallback(cbInfraProvDeleteNo, data)
		if !ok {
			return
		}
		body, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var p *remnawave.InfraBillingProviderItem
		for i := range body.Providers {
			if body.Providers[i].UUID == pu {
				p = &body.Providers[i]
				break
			}
		}
		if p == nil {
			_ = h.renderAdminInfraProvidersScreen(ctx, b, msg.Chat.ID, msg.ID, lang)
			return
		}
		text := h.infraProvCardHTML(lang, p)
		rows := h.infraProvDetailKeyboard(lang, pu)
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:             msg.Chat.ID,
			MessageID:          msg.ID,
			ParseMode:          models.ParseModeHTML,
			Text:               text,
			LinkPreviewOptions: infraNoLinkPreview(),
			ReplyMarkup:        models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra prov delete cancel", err)

	case strings.HasPrefix(data, cbInfraProvDelete):
		pu, ok := parseInfraUUIDCallback(cbInfraProvDelete, data)
		if !ok {
			return
		}
		body, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var p *remnawave.InfraBillingProviderItem
		for i := range body.Providers {
			if body.Providers[i].UUID == pu {
				p = &body.Providers[i]
				break
			}
		}
		if p == nil {
			_ = h.renderAdminInfraProvidersScreen(ctx, b, msg.Chat.ID, msg.ID, lang)
			return
		}
		confirmText := fmt.Sprintf("%s\n\n<b>%s</b>",
			h.translation.GetText(lang, "admin_infra_prov_delete_confirm"),
			html.EscapeString(p.Name),
		)
		rows := [][]models.InlineKeyboardButton{
			{
				h.translation.WithButton(lang, "admin_infra_confirm", models.InlineKeyboardButton{CallbackData: cbInfraProvDeleteYes + pu.String()}),
				h.translation.WithButton(lang, "admin_infra_cancel", models.InlineKeyboardButton{CallbackData: cbInfraProvDeleteNo + pu.String()}),
			},
			{h.translation.WithButton(lang, "admin_infra_prov_back_card", models.InlineKeyboardButton{CallbackData: cbInfraProvDetail + pu.String()})},
		}
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:             msg.Chat.ID,
			MessageID:          msg.ID,
			ParseMode:          models.ParseModeHTML,
			Text:               confirmText,
			LinkPreviewOptions: infraNoLinkPreview(),
			ReplyMarkup:        models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra prov delete confirm", err)

	case strings.HasPrefix(data, cbInfraProvWizName):
		pu, ok := parseInfraUUIDCallback(cbInfraProvWizName, data)
		if !ok {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizProvEditName, ProvEditUUID: pu})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_edit_name_prompt"), cbInfraProvEditMenu+pu.String())

	case strings.HasPrefix(data, cbInfraProvWizIcon):
		pu, ok := parseInfraUUIDCallback(cbInfraProvWizIcon, data)
		if !ok {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizProvEditFavicon, ProvEditUUID: pu})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_edit_icon_prompt"), cbInfraProvEditMenu+pu.String())

	case strings.HasPrefix(data, cbInfraProvWizLogin):
		pu, ok := parseInfraUUIDCallback(cbInfraProvWizLogin, data)
		if !ok {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizProvEditLogin, ProvEditUUID: pu})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_edit_login_prompt"), cbInfraProvEditMenu+pu.String())
	}
}

// AdminInfraHistoryCRUDRouter — префикс ifh*.
func (h Handler) AdminInfraHistoryCRUDRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}
	cb := update.CallbackQuery
	data := cb.Data
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode

	if data == cbInfraHistDeleteMenu {
		page := getInfraHistLastPage(cb.From.ID)
		if page < 1 {
			page = 1
		}
		start := (page - 1) * infraHistoryPageSize
		body, err := h.remnawaveClient.GetInfraBillingHistory(ctx, start, infraHistoryPageSize)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		text, kb := h.infraHistDeletePicker(lang, body, page)
		isDisabled := true
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			ParseMode: models.ParseModeHTML,
			Text:      text,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: &isDisabled,
			},
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
		})
		logEditError("infra hist delete menu", err)
		return
	}

	if data == cbInfraHistCreate {
		infraWizClear(cb.From.ID)
		body, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
		if err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		var rows [][]models.InlineKeyboardButton
		for i := range body.Providers {
			if i >= 20 {
				break
			}
			p := &body.Providers[i]
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         truncateRunes(p.Name, 36),
				CallbackData: cbInfraHistPickProv + p.UUID.String(),
			}})
		}
		rows = append(rows, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_infra_hist_back", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraHist}),
		})
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        h.translation.GetText(lang, "admin_infra_hist_create_pick_provider"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		logEditError("infra hist pick prov", err)
		return
	}

	if strings.HasPrefix(data, cbInfraHistPickProv) {
		pu, ok := parseInfraUUIDCallback(cbInfraHistPickProv, data)
		if !ok {
			return
		}
		infraWizSet(cb.From.ID, infraWizState{Kind: infraWizHistAmount, ProviderUUID: pu})
		_ = h.sendInfraWizardPrompt(ctx, b, cb.From.ID, lang, h.translation.GetText(lang, "admin_infra_wiz_hist_amount_prompt"), cbInfraWizBackHist)
		return
	}

	if strings.HasPrefix(data, cbInfraHistDelete) {
		ru, ok := parseInfraUUIDCallback(cbInfraHistDelete, data)
		if !ok {
			return
		}
		page := getInfraHistLastPage(cb.From.ID)
		if err := h.remnawaveClient.DeleteInfraBillingHistory(ctx, ru); err != nil {
			h.editAdminInfraAPIError(ctx, b, msg, lang, err)
			return
		}
		if err := h.renderInfraHistoryPage(ctx, b, msg, lang, page); err != nil {
			logEditError("infra hist after delete", err)
		}
	}
}

// renderInfraHistoryPage — перерисовать экран истории (после удаления и т.п.).
func (h Handler) renderInfraHistoryPage(ctx context.Context, b *bot.Bot, msg *models.Message, lang string, page int) error {
	if page < 1 {
		page = 1
	}
	start := (page - 1) * infraHistoryPageSize
	body, err := h.remnawaveClient.GetInfraBillingHistory(ctx, start, infraHistoryPageSize)
	if err != nil {
		return err
	}
	total := body.Total
	if total < 0 {
		total = 0
	}
	totalPages := int(math.Ceil(float64(total) / float64(infraHistoryPageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
		start = (page - 1) * infraHistoryPageSize
		body, err = h.remnawaveClient.GetInfraBillingHistory(ctx, start, infraHistoryPageSize)
		if err != nil {
			return err
		}
	}
	setInfraHistLastPage(msg.Chat.ID, page)
	text, kb := h.infraHistoryScreen(lang, body, page, totalPages)
	isDisabled := true
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
	return err
}

func (h Handler) infraHistDeletePicker(lang string, body *remnawave.InfraBillingHistoryBody, page int) (string, [][]models.InlineKeyboardButton) {
	loc := adminInfraTZ()
	var sb strings.Builder
	sb.WriteString(h.translation.GetText(lang, "admin_infra_hist_delete_pick_title"))
	sb.WriteString("\n\n")
	if len(body.Records) == 0 {
		sb.WriteString(h.translation.GetText(lang, "admin_infra_hist_empty"))
	} else {
		for i := range body.Records {
			r := &body.Records[i]
			when := r.BilledAt.In(loc).Format("02.01.2006")
			line := fmt.Sprintf(h.translation.GetText(lang, "admin_infra_hist_line"),
				when,
				html.EscapeString(r.Provider.Name),
				formatInfraAmount(r.Amount),
			)
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
		}
	}
	var kb [][]models.InlineKeyboardButton
	for i := range body.Records {
		r := &body.Records[i]
		when := r.BilledAt.In(loc).Format("02.01.2006")
		btnText := fmt.Sprintf("%s · %s · %s$", when, truncateRunes(r.Provider.Name, 20), formatInfraAmount(r.Amount))
		kb = append(kb, []models.InlineKeyboardButton{{
			Text:         truncateRunes(btnText, 64),
			CallbackData: cbInfraHistDelete + r.UUID.String(),
		}})
	}
	kb = append(kb, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_infra_hist_back_page", models.InlineKeyboardButton{CallbackData: infraHistoryCallback(page)}),
	})
	kb = append(kb, h.adminInfraBackRootRow(lang)...)
	return sb.String(), kb
}

func (h Handler) infraHistoryScreen(lang string, body *remnawave.InfraBillingHistoryBody, page, totalPages int) (string, [][]models.InlineKeyboardButton) {
	loc := adminInfraTZ()
	var sb strings.Builder
	title := h.translation.GetText(lang, "admin_infra_hist_title")
	if totalPages > 1 {
		title = fmt.Sprintf("%s (%d/%d)", title, page, totalPages)
	}
	sb.WriteString(title)
	sb.WriteString("\n\n")
	if len(body.Records) == 0 {
		sb.WriteString(h.translation.GetText(lang, "admin_infra_hist_empty"))
	} else {
		for i := range body.Records {
			r := &body.Records[i]
			when := r.BilledAt.In(loc).Format("02.01.2006")
			line := fmt.Sprintf(h.translation.GetText(lang, "admin_infra_hist_line"),
				when,
				html.EscapeString(r.Provider.Name),
				formatInfraAmount(r.Amount),
			)
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(h.translation.GetText(lang, "admin_infra_hist_actions_hint"))

	var kb [][]models.InlineKeyboardButton
	kb = append(kb, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_infra_hist_add", models.InlineKeyboardButton{CallbackData: cbInfraHistCreate}),
	})
	if len(body.Records) > 0 {
		kb = append(kb, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_infra_hist_delete", models.InlineKeyboardButton{CallbackData: cbInfraHistDeleteMenu}),
		})
	}
	var nav []models.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, h.translation.WithButton(lang, "admin_infra_hist_prev", models.InlineKeyboardButton{CallbackData: infraHistoryCallback(page - 1)}))
	}
	if page < totalPages {
		nav = append(nav, h.translation.WithButton(lang, "admin_infra_hist_next", models.InlineKeyboardButton{CallbackData: infraHistoryCallback(page + 1)}))
	}
	if len(nav) > 0 {
		kb = append(kb, nav)
	}
	kb = append(kb, h.adminInfraBackRootRow(lang)...)
	return sb.String(), kb
}

// AdminInfraBillingTextHandler — ответы текстом в мастерах инфра-биллинга.
func (h Handler) AdminInfraBillingTextHandler(_ context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	adminID := update.Message.From.ID
	if adminID != config.GetAdminTelegramId() {
		return
	}
	st, ok := infraWizGet(adminID)
	if !ok {
		return
	}
	lang := update.Message.From.LanguageCode
	text := strings.TrimSpace(update.Message.Text)
	ctx := context.Background()

	switch st.Kind {
	case infraWizNodeNextDate:
		t, err := parseInfraAdminDateInput(text)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_bad_date")})
			return
		}
		_, err = h.remnawaveClient.PatchInfraBillingNodes(ctx, remnawave.UpdateInfraBillingNodeRequest{
			UUIDs:         []uuid.UUID{st.BillingUUID},
			NextBillingAt: t,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "admin_infra_wiz_done"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{h.translation.WithButton(lang, "admin_infra_nodes_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraNodes})},
			}},
		})

	case infraWizNodeCreateDate:
		t, err := parseInfraAdminDateInput(text)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_bad_date")})
			return
		}
		nu := st.NodeUUID
		if nu == uuid.Nil {
			infraWizClear(adminID)
			return
		}
		_, err = h.remnawaveClient.CreateInfraBillingNode(ctx, remnawave.CreateInfraBillingNodeRequest{
			ProviderUUID:  st.ProviderUUID,
			NodeUUID:      nu,
			NextBillingAt: &t,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "admin_infra_wiz_node_created"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{h.translation.WithButton(lang, "admin_infra_nodes_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraNodes})},
			}},
		})

	case infraWizHistAmount:
		s := strings.ReplaceAll(text, ",", ".")
		amt, err := strconv.ParseFloat(s, 64)
		if err != nil || amt < 0 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_bad_amount")})
			return
		}
		infraWizSet(adminID, infraWizState{
			Kind:             infraWizHistDate,
			ProviderUUID:     st.ProviderUUID,
			HistDraftAmount:  amt,
			HistAmountFilled: true,
		})
		_ = h.sendInfraWizardPrompt(ctx, b, adminID, lang, h.translation.GetText(lang, "admin_infra_wiz_hist_date_prompt"), cbInfraWizBackHist)

	case infraWizHistDate:
		if !st.HistAmountFilled {
			infraWizClear(adminID)
			return
		}
		t, err := parseInfraAdminDateInput(text)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_bad_date")})
			return
		}
		_, err = h.remnawaveClient.CreateInfraBillingHistory(ctx, remnawave.CreateInfraBillingHistoryRequest{
			ProviderUUID: st.ProviderUUID,
			Amount:       st.HistDraftAmount,
			BilledAt:     t,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "admin_infra_wiz_hist_created"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{h.translation.WithButton(lang, "admin_infra_hist_back", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraHist})},
			}},
		})

	case infraWizProvCreateName:
		if len([]rune(text)) < 2 || len([]rune(text)) > 30 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_prov_name_bad")})
			return
		}
		infraWizSet(adminID, infraWizState{Kind: infraWizProvCreateFavicon, DraftName: text})
		_ = h.sendInfraWizardPrompt(ctx, b, adminID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_favicon_prompt"), cbInfraWizBackProv)

	case infraWizProvCreateFavicon:
		fav := text
		if fav == "-" || fav == "" {
			fav = ""
		}
		infraWizSet(adminID, infraWizState{Kind: infraWizProvCreateLogin, DraftName: st.DraftName, DraftFavicon: fav})
		_ = h.sendInfraWizardPrompt(ctx, b, adminID, lang, h.translation.GetText(lang, "admin_infra_wiz_prov_login_prompt"), cbInfraWizBackProv)

	case infraWizProvCreateLogin:
		req := remnawave.CreateInfraProviderRequest{Name: st.DraftName}
		if st.DraftFavicon != "" {
			req.FaviconLink = &st.DraftFavicon
		}
		if text != "-" && text != "" {
			u := text
			req.LoginURL = &u
		}
		_, err := h.remnawaveClient.CreateInfraProvider(ctx, req)
		infraWizClear(adminID)
		if err != nil {
			slog.Error("create infra provider", "error", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   h.translation.GetText(lang, "admin_infra_wiz_prov_created"),
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{h.translation.WithButton(lang, "admin_infra_prov_back_list", models.InlineKeyboardButton{CallbackData: CallbackAdminInfraProv})},
			}},
		})

	case infraWizProvEditName:
		if len([]rune(text)) < 2 || len([]rune(text)) > 30 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_wiz_prov_name_bad")})
			return
		}
		pu := st.ProvEditUUID
		_, err := h.remnawaveClient.PatchInfraProvider(ctx, remnawave.UpdateInfraProviderRequest{
			UUID: pu,
			Name: &text,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      adminID,
			Text:        h.translation.GetText(lang, "admin_infra_wiz_done"),
			ReplyMarkup: h.infraProvEditDoneReplyMarkup(lang, pu),
		})

	case infraWizProvEditFavicon:
		var fav *string
		if text != "-" && text != "" {
			fav = &text
		}
		pu := st.ProvEditUUID
		_, err := h.remnawaveClient.PatchInfraProvider(ctx, remnawave.UpdateInfraProviderRequest{
			UUID:        pu,
			FaviconLink: fav,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      adminID,
			Text:        h.translation.GetText(lang, "admin_infra_wiz_done"),
			ReplyMarkup: h.infraProvEditDoneReplyMarkup(lang, pu),
		})

	case infraWizProvEditLogin:
		var login *string
		if text != "-" && text != "" {
			login = &text
		}
		pu := st.ProvEditUUID
		_, err := h.remnawaveClient.PatchInfraProvider(ctx, remnawave.UpdateInfraProviderRequest{
			UUID:     pu,
			LoginURL: login,
		})
		infraWizClear(adminID)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: h.translation.GetText(lang, "admin_infra_api_error")})
			return
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      adminID,
			Text:        h.translation.GetText(lang, "admin_infra_wiz_done"),
			ReplyMarkup: h.infraProvEditDoneReplyMarkup(lang, pu),
		})
	}
}

func (h Handler) renderAdminInfraProvidersScreen(ctx context.Context, b *bot.Bot, chatID int64, msgID int, lang string) error {
	body, err := h.remnawaveClient.GetInfraBillingProviders(ctx)
	if err != nil {
		return err
	}
	text, kb := h.infraProvidersScreen(lang, body)
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:             chatID,
		MessageID:          msgID,
		ParseMode:          models.ParseModeHTML,
		Text:               text,
		LinkPreviewOptions: infraNoLinkPreview(),
		ReplyMarkup:        models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
	return err
}

func (h Handler) infraProvidersScreen(lang string, body *remnawave.InfraBillingProvidersBody) (string, [][]models.InlineKeyboardButton) {
	var sb strings.Builder
	sb.WriteString(h.translation.GetText(lang, "admin_infra_prov_title"))
	sb.WriteString("\n\n")
	items := append([]remnawave.InfraBillingProviderItem(nil), body.Providers...)
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	if len(items) == 0 {
		sb.WriteString(h.translation.GetText(lang, "admin_infra_prov_empty"))
	} else {
		for i := range items {
			p := &items[i]
			line := fmt.Sprintf(h.translation.GetText(lang, "admin_infra_prov_line"),
				html.EscapeString(p.Name),
				formatInfraAmount(p.BillingHistory.TotalAmount),
				p.BillingHistory.TotalBills,
				h.infraProviderLoginAnchor(lang, p.LoginURL),
			)
			if sb.Len()+len(line)+2 > 3000 {
				sb.WriteString("\n… ")
				sb.WriteString(h.translation.GetText(lang, "admin_infra_list_truncated"))
				break
			}
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
		}
	}
	var kb [][]models.InlineKeyboardButton
	kb = append(kb, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_infra_prov_add", models.InlineKeyboardButton{CallbackData: cbInfraProvCreate}),
	})
	for i := range items {
		if i >= 18 {
			break
		}
		p := &items[i]
		kb = append(kb, []models.InlineKeyboardButton{{
			Text:         truncateRunes(p.Name, 34),
			CallbackData: cbInfraProvDetail + p.UUID.String(),
		}})
	}
	kb = append(kb, h.adminInfraBackRootRow(lang)...)
	return sb.String(), kb
}
