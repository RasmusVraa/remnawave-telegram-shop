package handler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

const (
	adminUsersListPageSize      = 16
	adminUserPayPageSize        = 8
	adminUsersPagePickChunkSize = 12
)

var (
	adminUsersSearchMu      sync.Mutex
	adminUsersSearchWaiting map[int64]bool
)

func adminUsersSearchSet(adminID int64, waiting bool) {
	adminUsersSearchMu.Lock()
	defer adminUsersSearchMu.Unlock()
	if adminUsersSearchWaiting == nil {
		adminUsersSearchWaiting = make(map[int64]bool)
	}
	if waiting {
		adminUsersSearchWaiting[adminID] = true
	} else {
		delete(adminUsersSearchWaiting, adminID)
	}
}

var (
	adminUsersDMMu     sync.Mutex
	adminUsersDMTarget map[int64]int64 // admin telegram_id -> получатель (telegram_id)
)

func adminUsersDMSet(adminID int64, targetTelegramID int64) {
	adminUsersDMMu.Lock()
	defer adminUsersDMMu.Unlock()
	if adminUsersDMTarget == nil {
		adminUsersDMTarget = make(map[int64]int64)
	}
	adminUsersDMTarget[adminID] = targetTelegramID
}

func adminUsersDMClear(adminID int64) {
	adminUsersDMMu.Lock()
	defer adminUsersDMMu.Unlock()
	delete(adminUsersDMTarget, adminID)
}

// AdminUsersDMWaiting — админ после «Отправить сообщение» должен ввести текст следующим сообщением.
func AdminUsersDMWaiting(adminID int64) bool {
	adminUsersDMMu.Lock()
	defer adminUsersDMMu.Unlock()
	_, ok := adminUsersDMTarget[adminID]
	return ok
}

func adminUsersDMRecipient(adminID int64) (int64, bool) {
	adminUsersDMMu.Lock()
	defer adminUsersDMMu.Unlock()
	tg, ok := adminUsersDMTarget[adminID]
	return tg, ok
}

// AdminUsersSearchWaiting — админ ждёт ввод Telegram ID для поиска пользователя.
func AdminUsersSearchWaiting(adminID int64) bool {
	adminUsersSearchMu.Lock()
	defer adminUsersSearchMu.Unlock()
	if adminUsersSearchWaiting == nil {
		return false
	}
	return adminUsersSearchWaiting[adminID]
}

var (
	adminTrafficLimitMu      sync.Mutex
	adminTrafficLimitCust    map[int64]int64 // admin telegram id → customer id (ввод лимита ГБ)
	adminExpireDateMu        sync.Mutex
	adminExpireDateCust      map[int64]int64 // admin telegram id → customer id (ввод даты окончания)
	adminUserDescriptionMu   sync.Mutex
	adminUserDescriptionCust map[int64]int64 // admin id → customer id (ввод описания Remnawave)
)

// AdminUserTrafficLimitWaiting — админ после «Свой лимит» ждёт число ГБ сообщением.
func AdminUserTrafficLimitWaiting(adminID int64) bool {
	adminTrafficLimitMu.Lock()
	defer adminTrafficLimitMu.Unlock()
	if adminTrafficLimitCust == nil {
		return false
	}
	_, ok := adminTrafficLimitCust[adminID]
	return ok
}

func adminTrafficLimitSet(adminID, customerID int64) {
	adminTrafficLimitMu.Lock()
	defer adminTrafficLimitMu.Unlock()
	if adminTrafficLimitCust == nil {
		adminTrafficLimitCust = make(map[int64]int64)
	}
	adminTrafficLimitCust[adminID] = customerID
}

func adminTrafficLimitClear(adminID int64) {
	adminTrafficLimitMu.Lock()
	defer adminTrafficLimitMu.Unlock()
	delete(adminTrafficLimitCust, adminID)
}

func adminTrafficLimitCustomer(adminID int64) (int64, bool) {
	adminTrafficLimitMu.Lock()
	defer adminTrafficLimitMu.Unlock()
	if adminTrafficLimitCust == nil {
		return 0, false
	}
	cid, ok := adminTrafficLimitCust[adminID]
	return cid, ok
}

// AdminUserExpireDateWaiting — админ после «Ввести дату» ждёт дату сообщением.
func AdminUserExpireDateWaiting(adminID int64) bool {
	adminExpireDateMu.Lock()
	defer adminExpireDateMu.Unlock()
	if adminExpireDateCust == nil {
		return false
	}
	_, ok := adminExpireDateCust[adminID]
	return ok
}

func adminExpireDateSet(adminID, customerID int64) {
	adminExpireDateMu.Lock()
	defer adminExpireDateMu.Unlock()
	if adminExpireDateCust == nil {
		adminExpireDateCust = make(map[int64]int64)
	}
	adminExpireDateCust[adminID] = customerID
}

func adminExpireDateClear(adminID int64) {
	adminExpireDateMu.Lock()
	defer adminExpireDateMu.Unlock()
	delete(adminExpireDateCust, adminID)
}

func adminExpireDateCustomer(adminID int64) (int64, bool) {
	adminExpireDateMu.Lock()
	defer adminExpireDateMu.Unlock()
	if adminExpireDateCust == nil {
		return 0, false
	}
	cid, ok := adminExpireDateCust[adminID]
	return cid, ok
}

// AdminUserDescriptionWaiting — админ после «Изменить описание» ждёт текст сообщением.
func AdminUserDescriptionWaiting(adminID int64) bool {
	adminUserDescriptionMu.Lock()
	defer adminUserDescriptionMu.Unlock()
	if adminUserDescriptionCust == nil {
		return false
	}
	_, ok := adminUserDescriptionCust[adminID]
	return ok
}

func adminUserDescriptionSet(adminID, customerID int64) {
	adminUserDescriptionMu.Lock()
	defer adminUserDescriptionMu.Unlock()
	if adminUserDescriptionCust == nil {
		adminUserDescriptionCust = make(map[int64]int64)
	}
	adminUserDescriptionCust[adminID] = customerID
}

func adminUserDescriptionClear(adminID int64) {
	adminUserDescriptionMu.Lock()
	defer adminUserDescriptionMu.Unlock()
	delete(adminUserDescriptionCust, adminID)
}

func adminUserDescriptionCustomer(adminID int64) (int64, bool) {
	adminUserDescriptionMu.Lock()
	defer adminUserDescriptionMu.Unlock()
	if adminUserDescriptionCust == nil {
		return 0, false
	}
	cid, ok := adminUserDescriptionCust[adminID]
	return cid, ok
}

func isAdmin(cb *models.CallbackQuery) bool {
	return cb != nil && cb.From.ID == config.GetAdminTelegramId()
}

// --- Подменю «Юзеры и подписки» -------------------------------------------------

func (h Handler) AdminUsersSubmenuHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	adminUsersSearchSet(cb.From.ID, false)
	adminUsersDMClear(cb.From.ID)
	text := h.translation.GetText(lang, "admin_users_submenu_title")
	kb := [][]models.InlineKeyboardButton{
		{h.translation.WithButton(lang, "admin_users_submenu_btn_users", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot})},
		{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel})},
	}
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin users submenu", "error", err)
	}
}

// --- Корень «Пользователи» ------------------------------------------------------

func (h Handler) AdminUsersRootHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	adminUsersSearchSet(cb.From.ID, false)
	adminUsersDMClear(cb.From.ID)
	var summary string
	if h.statsRepository != nil {
		snap, err := h.statsRepository.FetchAdminStatsSnapshot(ctx)
		if err != nil {
			slog.Error("admin users root stats", "error", err)
			summary = h.translation.GetText(lang, "admin_users_summary_error")
		} else {
			summary = fmt.Sprintf(h.translation.GetText(lang, "admin_users_summary_fmt"),
				snap.TotalCustomers,
				snap.ActiveSubscriptions,
				snap.Inactive,
				snap.NewToday,
				snap.NewWeek,
				snap.NewMonth,
			)
		}
	} else {
		summary = h.translation.GetText(lang, "admin_users_summary_error")
	}
	text := h.translation.GetText(lang, "admin_users_menu_title") + "\n\n" + summary
	kb := adminUsersMainKeyboard(h, lang)
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin users root", "error", err)
	}
}

func adminUsersMainKeyboard(h Handler, lang string) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_users_all", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersListAllPrefix + "0"}),
			h.translation.WithButton(lang, "admin_users_inactive", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersInactiveMenu}),
		},
		{
			h.translation.WithButton(lang, "admin_users_search", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersSearch}),
			h.translation.WithButton(lang, "admin_users_stats_section", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersStatsSection}),
		},
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminPanel}),
		},
	}
}

func (h Handler) AdminUsersInactiveJumpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeInactive, 0, "", CallbackAdminUsersRoot)
}

func (h Handler) AdminUsersListAllRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, CallbackAdminUsersListAllPrefix) {
		return
	}
	page, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, CallbackAdminUsersListAllPrefix))
	if err != nil || page < 0 {
		page = 0
	}
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeAll, page, "", CallbackAdminUsersRoot)
}

func (h Handler) AdminUsersListInactiveRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, CallbackAdminUsersListInactivePrefix) {
		return
	}
	page, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, CallbackAdminUsersListInactivePrefix))
	if err != nil || page < 0 {
		page = 0
	}
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeInactive, page, "", CallbackAdminUsersRoot)
}

// listPaginationPrefix — префикс callback для ◀/▶/прямого номера страницы (не путать с «все юзеры» vs «подписки»).
func listPaginationPrefix(scope database.CustomerListScope, listParentBack string) string {
	switch {
	case scope == database.CustomerListScopeInactive:
		return CallbackAdminUsersListInactivePrefix
	case scope == database.CustomerListScopeExpiringSoon && listParentBack == CallbackAdminSubsRoot:
		return CallbackAdminSubsExpiringListPrefix
	case listParentBack == CallbackAdminSubsRoot && scope == database.CustomerListScopeAll:
		return CallbackAdminSubsListPrefix
	default:
		return CallbackAdminUsersListAllPrefix
	}
}

func adminUsersListMode(scope database.CustomerListScope, listParentBack string) string {
	switch {
	case scope == database.CustomerListScopeInactive:
		return "ai"
	case scope == database.CustomerListScopeExpiringSoon && listParentBack == CallbackAdminSubsRoot:
		return "se"
	case listParentBack == CallbackAdminSubsRoot && scope == database.CustomerListScopeAll:
		return "sa"
	default:
		return "au"
	}
}

func adminUsersListFromMode(mode string) (scope database.CustomerListScope, titleKey string, listParentBack string, ok bool) {
	switch mode {
	case "ai":
		return database.CustomerListScopeInactive, "", CallbackAdminUsersRoot, true
	case "se":
		return database.CustomerListScopeExpiringSoon, "admin_subs_expiring_list_title", CallbackAdminSubsRoot, true
	case "sa":
		return database.CustomerListScopeAll, "admin_subs_list_title", CallbackAdminSubsRoot, true
	case "au":
		return database.CustomerListScopeAll, "", CallbackAdminUsersRoot, true
	default:
		return database.CustomerListScopeAll, "", CallbackAdminUsersRoot, false
	}
}

func adminUsersPagePickerMarkup(lang string, h Handler, mode string, chunk, returnPg int, totalPages int) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	start := chunk * adminUsersPagePickChunkSize
	end := start + adminUsersPagePickChunkSize
	if end > totalPages {
		end = totalPages
	}
	if start >= totalPages {
		start = 0
		end = min(adminUsersPagePickChunkSize, totalPages)
	}
	const cols = 6
	var cur []models.InlineKeyboardButton
	for p := start; p < end; p++ {
		cur = append(cur, models.InlineKeyboardButton{
			Text:         strconv.Itoa(p + 1),
			CallbackData: fmt.Sprintf("%s%s%d", CallbackAdminUsersListPagePickJumpPrefix, mode, p),
		})
		if len(cur) >= cols {
			rows = append(rows, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	var nav []models.InlineKeyboardButton
	if chunk > 0 {
		nav = append(nav, models.InlineKeyboardButton{
			Text:         "◀️",
			CallbackData: fmt.Sprintf("%s%s%03d%04d", CallbackAdminUsersListPagePickOpenPrefix, mode, chunk-1, returnPg),
		})
	}
	if end < totalPages {
		nav = append(nav, models.InlineKeyboardButton{
			Text:         "▶️",
			CallbackData: fmt.Sprintf("%s%s%03d%04d", CallbackAdminUsersListPagePickOpenPrefix, mode, chunk+1, returnPg),
		})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_users_page_picker_back", models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s%s%d", CallbackAdminUsersListPagePickJumpPrefix, mode, returnPg),
		}),
	})
	return rows
}

// AdminUsersListPagePickerOpenHandler — сетка выбора страницы по нажатию «N/M» в списке клиентов.
func (h Handler) AdminUsersListPagePickerOpenHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	data := cb.Data
	if !strings.HasPrefix(data, CallbackAdminUsersListPagePickOpenPrefix) {
		return
	}
	rest := strings.TrimPrefix(data, CallbackAdminUsersListPagePickOpenPrefix)
	if len(rest) < 9 {
		return
	}
	mode := rest[:2]
	chunk, err1 := strconv.Atoi(rest[2:5])
	returnPg, err2 := strconv.Atoi(rest[5:9])
	if err1 != nil || err2 != nil || chunk < 0 || returnPg < 0 {
		return
	}
	scope, _, _, ok := adminUsersListFromMode(mode)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode
	total, err := h.customerRepository.CountByListScope(ctx, scope)
	if err != nil {
		slog.Error("admin users page picker count", "error", err)
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(adminUsersListPageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	maxChunk := (totalPages - 1) / adminUsersPagePickChunkSize
	if maxChunk < 0 {
		maxChunk = 0
	}
	if chunk > maxChunk {
		chunk = maxChunk
	}
	start := chunk * adminUsersPagePickChunkSize
	end := start + adminUsersPagePickChunkSize
	if end > totalPages {
		end = totalPages
	}
	first := start + 1
	last := end
	if first > last {
		first, last = 1, 1
	}
	text := fmt.Sprintf(h.translation.GetText(lang, "admin_users_page_picker_body"), first, last, totalPages)
	rows := adminUsersPagePickerMarkup(lang, h, mode, chunk, returnPg, totalPages)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: rows}, nil)
	if err != nil {
		slog.Error("admin users page picker edit", "error", err)
	}
}

// AdminUsersListPagePickerJumpHandler — переход на выбранную страницу из сетки.
func (h Handler) AdminUsersListPagePickerJumpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	data := update.CallbackQuery.Data
	if !strings.HasPrefix(data, CallbackAdminUsersListPagePickJumpPrefix) {
		return
	}
	rest := strings.TrimPrefix(data, CallbackAdminUsersListPagePickJumpPrefix)
	if len(rest) < 3 {
		return
	}
	mode := rest[:2]
	pg, err := strconv.Atoi(rest[2:])
	if err != nil || pg < 0 {
		return
	}
	scope, titleKey, listParent, ok := adminUsersListFromMode(mode)
	if !ok {
		return
	}
	h.adminUsersListPage(ctx, b, update, scope, pg, titleKey, listParent)
}

func (h Handler) adminUsersListPage(ctx context.Context, b *bot.Bot, update *models.Update, scope database.CustomerListScope, page int, titleKeyOverride string, listParentBack string) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	if listParentBack == "" {
		listParentBack = CallbackAdminUsersRoot
	}
	total, err := h.customerRepository.CountByListScope(ctx, scope)
	if err != nil {
		slog.Error("admin users list count", "error", err)
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(adminUsersListPageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	if page < 0 {
		page = 0
	}
	offset := page * adminUsersListPageSize
	customers, err := h.customerRepository.ListPaged(ctx, scope, offset, adminUsersListPageSize)
	if err != nil {
		slog.Error("admin users list page", "error", err)
		return
	}
	panelHints := h.enrichPanelUsernameHints(ctx, b, customers)
	prefix := listPaginationPrefix(scope, listParentBack)
	mode := adminUsersListMode(scope, listParentBack)
	titleKey := titleKeyOverride
	if titleKey == "" {
		switch scope {
		case database.CustomerListScopeInactive:
			titleKey = "admin_users_list_title_inactive"
		case database.CustomerListScopeExpiringSoon:
			titleKey = "admin_users_expiring_list_title"
		default:
			titleKey = "admin_users_list_title_all"
		}
	}
	header := h.translation.GetText(lang, titleKey)
	text := fmt.Sprintf("%s\n\n<b>%s</b>\n%s\n\n%s",
		header,
		h.translation.GetText(lang, "admin_users_list_total"),
		strconv.FormatInt(total, 10),
		h.translation.GetText(lang, "admin_users_list_hint"),
	)
	if len(customers) == 0 {
		text += "\n\n<i>" + h.translation.GetText(lang, "admin_users_list_empty") + "</i>"
	}
	var rows [][]models.InlineKeyboardButton
	now := time.Now()
	for i := 0; i < len(customers); i += 2 {
		c0 := customers[i]
		row := []models.InlineKeyboardButton{
			{Text: adminUserListButtonLabel(h, lang, &c0, now, panelHints[i]), CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, c0.ID)},
		}
		if i+1 < len(customers) {
			c1 := customers[i+1]
			row = append(row, models.InlineKeyboardButton{
				Text:         adminUserListButtonLabel(h, lang, &c1, now, panelHints[i+1]),
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, c1.ID),
			})
		}
		rows = append(rows, row)
	}
	if totalPages > 1 {
		var nav []models.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, models.InlineKeyboardButton{
				Text:         "◀️",
				CallbackData: fmt.Sprintf("%s%d", prefix, page-1),
			})
		}
		chunk := page / adminUsersPagePickChunkSize
		nav = append(nav, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d/%d", page+1, totalPages),
			CallbackData: fmt.Sprintf("%s%s%03d%04d", CallbackAdminUsersListPagePickOpenPrefix, mode, chunk, page),
		})
		if page < totalPages-1 {
			nav = append(nav, models.InlineKeyboardButton{
				Text:         "▶️",
				CallbackData: fmt.Sprintf("%s%d", prefix, page+1),
			})
		}
		rows = append(rows, nav)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: listParentBack}),
	})
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: rows}, nil)
	if err != nil {
		slog.Error("admin users list edit", "error", err)
	}
}

// adminCustomerWebCabinetDisplay — web-кабинет / synthetic telegram_id: показываем id_email без домена.
func adminCustomerWebCabinetDisplay(c *database.Customer) bool {
	return c != nil && (c.IsWebOnly || utils.IsSyntheticTelegramID(c.TelegramID))
}

// sanitizeEmailLocalForAdminLabel — локальная часть email: нижний регистр, без +tag, точки убраны, только [a-z0-9_].
func sanitizeEmailLocalForAdminLabel(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return ""
	}
	local := email[:at]
	if plus := strings.Index(local, "+"); plus >= 0 {
		local = local[:plus]
	}
	local = strings.ReplaceAll(local, ".", "")
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune('_')
		}
	}
	return b.String()
}

func formatCabinetWebAdminLabel(customerID int64, accountEmail string) string {
	local := sanitizeEmailLocalForAdminLabel(accountEmail)
	if local == "" {
		return fmt.Sprintf("%d_web", customerID)
	}
	return fmt.Sprintf("%d_%s", customerID, local)
}

// enrichPanelUsernameHints — для строки списка, если в БД нет @username: сначала Telegram (GetChat), иначе логин панели Remnawave.
func (h Handler) enrichPanelUsernameHints(ctx context.Context, b *bot.Bot, customers []database.Customer) []string {
	out := make([]string, len(customers))
	var webIDs []int64
	for i := range customers {
		if adminCustomerWebCabinetDisplay(&customers[i]) {
			webIDs = append(webIDs, customers[i].ID)
		}
	}
	emailByCustomer, err := h.customerRepository.CabinetAccountEmailsByCustomerIDs(ctx, webIDs)
	if err != nil {
		slog.Warn("admin: cabinet emails for web labels", "error", err)
		emailByCustomer = nil
	}
	for i := range customers {
		c := &customers[i]
		if adminCustomerWebCabinetDisplay(c) {
			if em, ok := emailByCustomer[c.ID]; ok && strings.TrimSpace(em) != "" {
				out[i] = formatCabinetWebAdminLabel(c.ID, em)
				continue
			}
		}
		if c.TelegramUsername != nil && strings.TrimSpace(*c.TelegramUsername) != "" {
			continue
		}
		if b != nil && !utils.IsSyntheticTelegramID(c.TelegramID) {
			if chat, err := b.GetChat(ctx, &bot.GetChatParams{ChatID: c.TelegramID}); err == nil && chat != nil {
				if un := strings.TrimSpace(chat.Username); un != "" {
					out[i] = "@" + strings.TrimPrefix(un, "@")
					continue
				}
				fn := strings.TrimSpace(chat.FirstName)
				ln := strings.TrimSpace(chat.LastName)
				if nick := strings.TrimSpace(fn + " " + ln); nick != "" {
					out[i] = nick
					continue
				}
			}
		}
		u, err := h.remnawaveClient.GetUserTrafficInfo(ctx, c.TelegramID)
		if err != nil || u == nil || strings.TrimSpace(u.Username) == "" {
			continue
		}
		out[i] = strings.TrimSpace(u.Username)
	}
	return out
}

func adminUserListButtonLabel(h Handler, lang string, c *database.Customer, now time.Time, panelUsernameHint string) string {
	idStr := strconv.FormatInt(c.TelegramID, 10)
	idTail := idStr
	if len(idTail) > 9 {
		idTail = "…" + idTail[len(idTail)-6:]
	}
	var statusEmoji string
	switch {
	case c.ExpireAt == nil:
		statusEmoji = "⚪️"
	case c.ExpireAt.After(now):
		statusEmoji = "🟢"
	default:
		statusEmoji = "🔴"
	}
	short := idStr
	if len(short) > 18 {
		short = short[:15] + "…"
	}
	base := fmt.Sprintf("%s · %s", statusEmoji, short)
	// Web / synthetic: подпись из кабинета (customer_id + локальная часть email), без домена.
	if adminCustomerWebCabinetDisplay(c) && panelUsernameHint != "" {
		pu := strings.TrimSpace(panelUsernameHint)
		if len([]rune(pu)) > 22 {
			pu = string([]rune(pu)[:20]) + "…"
		}
		base = fmt.Sprintf("%s %s · %s", statusEmoji, pu, idTail)
	} else if c.TelegramUsername != nil && strings.TrimSpace(*c.TelegramUsername) != "" {
		u := strings.TrimSpace(*c.TelegramUsername)
		if len([]rune(u)) > 14 {
			u = string([]rune(u)[:12]) + "…"
		}
		base = fmt.Sprintf("%s @%s · %s", statusEmoji, u, idTail)
	} else if panelUsernameHint != "" {
		pu := strings.TrimSpace(panelUsernameHint)
		if strings.HasPrefix(pu, "@") {
			u := strings.TrimPrefix(pu, "@")
			if len([]rune(u)) > 14 {
				u = string([]rune(u)[:12]) + "…"
			}
			base = fmt.Sprintf("%s @%s · %s", statusEmoji, u, idTail)
		} else {
			if len([]rune(pu)) > 18 {
				pu = string([]rune(pu)[:16]) + "…"
			}
			base = fmt.Sprintf("%s %s · %s", statusEmoji, pu, idTail)
		}
	}
	return base
}

// --- Статистика в разделе пользователей -----------------------------------------

func (h Handler) AdminUsersStatsSectionHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil || h.statsRepository == nil {
		return
	}
	snap, err := h.statsRepository.FetchAdminStatsSnapshot(ctx)
	if err != nil {
		slog.Error("admin users section stats", "error", err)
		return
	}
	actPct := pctStr(snap.ActiveSubscriptions, snap.TotalCustomers)
	body := fmt.Sprintf(h.translation.GetText(lang, "admin_users_stats_section_body"),
		snap.TotalCustomers,
		snap.ActiveSubscriptions,
		actPct,
		snap.Inactive,
		snap.NewToday,
		snap.NewWeek,
		snap.NewMonth,
	)
	text := h.translation.GetText(lang, "admin_users_stats_section_title") + "\n\n" + body + "\n\n" + h.formatStatsUpdated(lang, snap.CapturedAt)
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_stats_refresh", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersStatsSection}),
		},
		{
			h.translation.WithButton(lang, "admin_users_stats_open_full", models.InlineKeyboardButton{CallbackData: CallbackAdminStatsRoot}),
		},
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin users stats section", "error", err)
	}
}

// --- Поиск по Telegram ID -------------------------------------------------------

func (h Handler) AdminUsersSearchHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	adminUsersDMClear(cb.From.ID)
	adminTrafficLimitClear(cb.From.ID)
	adminUsersSearchSet(cb.From.ID, true)
	text := h.translation.GetText(lang, "admin_users_search_prompt")
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_users_search_cancel", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
		},
	}
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin users search prompt", "error", err)
	}
}

// AdminUsersSearchMessageHandler — поиск по Telegram ID, @username в боте и совпадениям в панели (описание, логин, тег).
func (h Handler) AdminUsersSearchMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	adminID := update.Message.From.ID
	if adminID != config.GetAdminTelegramId() || update.Message.ReplyToMessage != nil {
		return
	}
	if !AdminUsersSearchWaiting(adminID) {
		return
	}
	lang := update.Message.From.LanguageCode
	raw := normalizeAdminSearchQuery(update.Message.Text)
	if raw == "" || len(raw) > 80 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(lang, "admin_users_search_bad_query"),
		})
		return
	}

	adminUsersSearchSet(adminID, false)

	// Точное совпадение по числовому Telegram ID (только цифры).
	if isAdminSearchDigitsOnly(raw) && len(raw) <= 15 {
		if tgID, err := strconv.ParseInt(raw, 10, 64); err == nil && tgID > 0 {
			if cust, err := h.customerRepository.FindByTelegramId(ctx, tgID); err == nil && cust != nil {
				h.adminSearchSendCardAndDeletePrompt(ctx, b, update, cust)
				return
			}
		}
	}

	dbRows, dbErr := h.customerRepository.SearchForAdmin(ctx, raw, database.AdminSearchMaxResults)
	if dbErr != nil {
		slog.Error("admin search db", "error", dbErr)
		dbRows = nil
	}
	var rwRows []remnawave.User
	rwRows, rwErr := h.remnawaveClient.FindUsersMatchingAdminSearch(ctx, raw)
	if rwErr != nil {
		slog.Error("admin search panel", "error", rwErr)
		rwRows = nil
	}

	merged := h.adminUsersMergeSearchResults(ctx, dbRows, rwRows, database.AdminSearchMaxResults)
	if len(merged) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(lang, "admin_users_search_not_found"),
		})
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    update.Message.Chat.ID,
			MessageID: update.Message.ID,
		})
		return
	}
	if len(merged) == 1 {
		h.adminSearchSendCardAndDeletePrompt(ctx, b, update, &merged[0])
		return
	}

	if err := h.adminUsersSendSearchMulti(ctx, b, update.Message.Chat.ID, lang, merged); err != nil {
		slog.Error("admin search multi send", "error", err)
		return
	}
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: update.Message.ID,
	})
}

func normalizeAdminSearchQuery(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	return strings.TrimSpace(s)
}

func isAdminSearchDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (h Handler) adminUsersMergeSearchResults(ctx context.Context, dbRows []database.Customer, rwRows []remnawave.User, limit int) []database.Customer {
	if limit <= 0 {
		limit = database.AdminSearchMaxResults
	}
	seen := make(map[int64]struct{})
	out := make([]database.Customer, 0, limit)
	add := func(c *database.Customer) {
		if c == nil {
			return
		}
		if _, ok := seen[c.ID]; ok {
			return
		}
		if len(out) >= limit {
			return
		}
		seen[c.ID] = struct{}{}
		out = append(out, *c)
	}
	for i := range dbRows {
		add(&dbRows[i])
		if len(out) >= limit {
			return out
		}
	}
	for _, u := range rwRows {
		cust, err := remnawave.CustomerFromAdminSearchUser(ctx, h.customerRepository, u)
		if err != nil || cust == nil {
			continue
		}
		add(cust)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (h Handler) adminSearchSendCardAndDeletePrompt(ctx context.Context, b *bot.Bot, update *models.Update, cust *database.Customer) {
	text, markup := h.adminUserManageContent(ctx, b, update.Message.From.LanguageCode, cust)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		slog.Error("admin search send card", "error", err)
		return
	}
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: update.Message.ID,
	})
}

func (h Handler) adminUsersSendSearchMulti(ctx context.Context, b *bot.Bot, chatID int64, lang string, customers []database.Customer) error {
	header := fmt.Sprintf(h.translation.GetText(lang, "admin_users_search_multi_title"), len(customers))
	text := header + "\n\n" + h.translation.GetText(lang, "admin_users_search_multi_hint")
	hints := h.enrichPanelUsernameHints(ctx, b, customers)
	var rows [][]models.InlineKeyboardButton
	now := time.Now()
	for i := 0; i < len(customers); i += 2 {
		c0 := customers[i]
		row := []models.InlineKeyboardButton{
			{Text: adminUserListButtonLabel(h, lang, &c0, now, hints[i]), CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, c0.ID)},
		}
		if i+1 < len(customers) {
			c1 := customers[i+1]
			row = append(row, models.InlineKeyboardButton{
				Text:         adminUserListButtonLabel(h, lang, &c1, now, hints[i+1]),
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, c1.ID),
			})
		}
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_users_search_multi_close", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
	})
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		ParseMode:   models.ParseModeHTML,
		Text:        text,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

// --- Карточка пользователя -------------------------------------------------------

func parseCustomerIDFromPrefix(data, prefix string) (int64, bool) {
	if !strings.HasPrefix(data, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(data, prefix)
	if rest == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (h Handler) AdminUserManageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserManagePrefix)
	if !ok {
		return
	}
	adminUsersDMClear(cb.From.ID)
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		slog.Warn("admin user manage not found", "id", id)
		return
	}
	lang := cb.From.LanguageCode
	text, markup := h.adminUserManageContent(ctx, b, lang, cust)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, markup, nil)
	if err != nil {
		slog.Error("admin user manage edit", "error", err)
	}
}

// AdminPaymentsNotifyOpenUserHandler — callback «К пользователю» из уведомления об оплате в группе: новое сообщение с той же карточкой, что в разделе «Пользователи».
func (h Handler) AdminPaymentsNotifyOpenUserHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackPaymentsNotifyUserOpenPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		return
	}
	lang := strings.TrimSpace(cb.From.LanguageCode)
	if lang == "" {
		lang = "ru"
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(lang, "payments_notify_user_not_found"),
			ShowAlert:       true,
		})
		return
	}
	adminUsersDMClear(cb.From.ID)
	text, markup := h.adminUserManageContent(ctx, b, lang, cust)
	params := &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	}
	if msg.MessageThreadID != 0 {
		params.MessageThreadID = msg.MessageThreadID
	}
	if _, err := b.SendMessage(ctx, params); err != nil {
		slog.Error("payments notify open user send", "error", err, "customer_id", id)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// adminUserCardTitleHTML — заголовок карточки: @username из БД, иначе имя из Telegram API, иначе числовой ID.
func (h Handler) adminUserCardTitleHTML(ctx context.Context, b *bot.Bot, cust *database.Customer) string {
	if adminCustomerWebCabinetDisplay(cust) {
		emails, err := h.customerRepository.CabinetAccountEmailsByCustomerIDs(ctx, []int64{cust.ID})
		if err == nil {
			if em, ok := emails[cust.ID]; ok && strings.TrimSpace(em) != "" {
				return escapeHTML(formatCabinetWebAdminLabel(cust.ID, em))
			}
		} else {
			slog.Warn("admin user card: cabinet email lookup", "customer_id", cust.ID, "error", err)
		}
	}
	if cust.TelegramUsername != nil {
		u := strings.TrimSpace(*cust.TelegramUsername)
		if u != "" {
			u = strings.TrimPrefix(u, "@")
			return "@" + escapeHTML(u)
		}
	}
	if b != nil && !utils.IsSyntheticTelegramID(cust.TelegramID) {
		chat, err := b.GetChat(ctx, &bot.GetChatParams{ChatID: cust.TelegramID})
		if err == nil && chat != nil {
			if un := strings.TrimSpace(chat.Username); un != "" {
				un = strings.TrimPrefix(un, "@")
				return "@" + escapeHTML(un)
			}
			fn := strings.TrimSpace(chat.FirstName)
			ln := strings.TrimSpace(chat.LastName)
			if nick := strings.TrimSpace(fn + " " + ln); nick != "" {
				return escapeHTML(nick)
			}
		}
	}
	if adminCustomerWebCabinetDisplay(cust) {
		return escapeHTML(fmt.Sprintf("%d_web", cust.ID))
	}
	return escapeHTML(strconv.FormatInt(cust.TelegramID, 10))
}

// adminUserCardBodyHTML — карточка пользователя (как в главном меню управления), без кнопок.
func (h Handler) adminUserCardBodyHTML(ctx context.Context, b *bot.Bot, lang string, cust *database.Customer) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>👤 %s</b>\n\n", h.adminUserCardTitleHTML(ctx, b, cust)))
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_telegram"), cust.TelegramID))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_lang"), escapeHTML(cust.Language)))
	sb.WriteString("\n")

	var rw *remnawave.User
	if u, err := h.adminFindRWUserByCustomer(ctx, cust); err == nil {
		rw = u
	}

	if rw != nil {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_status_line"), escapeHTML(rw.Status)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if config.SalesMode() == "tariffs" && cust.CurrentTariffID != nil && h.tariffRepository != nil {
		if tr, err := h.tariffRepository.GetByID(ctx, *cust.CurrentTariffID); err == nil && tr != nil {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_tariff"), escapeHTML(displayTariffName(tr))))
			sb.WriteString("\n")
		}
	}
	if rw != nil && !rw.ExpireAt.IsZero() {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_expire_panel"),
			rw.ExpireAt.Format("02.01.2006 15:04")))
		sb.WriteString("\n")
	} else if rw == nil && cust.ExpireAt != nil {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_expire_db"),
			cust.ExpireAt.In(time.Now().Location()).Format("02.01.2006 15:04")))
		sb.WriteString("\n")
	}
	if config.LoyaltyEnabled() && h.loyaltyTierRepository != nil {
		if prog, err := h.loyaltyTierRepository.ProgressForXP(ctx, cust.LoyaltyXP); err == nil {
			lvlLabel := strconv.Itoa(prog.CurrentTier.DiscountPercent) + "%"
			if prog.CurrentTier.DisplayName != nil && strings.TrimSpace(*prog.CurrentTier.DisplayName) != "" {
				lvlLabel = escapeHTML(strings.TrimSpace(*prog.CurrentTier.DisplayName))
			}
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_loyalty_level"), cust.LoyaltyXP, lvlLabel))
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")

	if rw == nil {
		sb.WriteString("<i>")
		sb.WriteString(h.translation.GetText(lang, "admin_user_card_panel_error"))
		sb.WriteString("</i>\n")
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_extra_hwid"), cust.ExtraHwid))
		sb.WriteString("\n")
	} else {
		usedGB := rw.UserTraffic.UsedTrafficBytes / (1024 * 1024 * 1024)
		var limGB float64
		if rw.TrafficLimitBytes > 0 {
			limGB = float64(rw.TrafficLimitBytes) / (1024 * 1024 * 1024)
		}
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_traffic"), usedGB, limGB))
		sb.WriteString("\n")
		if strings.TrimSpace(rw.TrafficLimitStrategy) != "" {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_traffic_strategy"), escapeHTML(rw.TrafficLimitStrategy)))
			sb.WriteString("\n")
		}
		if rw.LastTrafficResetAt != nil && !rw.LastTrafficResetAt.IsZero() {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_last_traffic_reset"),
				rw.LastTrafficResetAt.Format("02.01.2006 15:04")))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")

		if rw.HwidDeviceLimit != nil {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_hwid_limit"), *rw.HwidDeviceLimit))
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_extra_hwid"), cust.ExtraHwid))
		sb.WriteString("\n\n")

		if strings.TrimSpace(rw.Username) != "" {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_panel_username"), escapeHTML(strings.TrimSpace(rw.Username))))
			sb.WriteString("\n")
		}
		if rw.Description != nil && strings.TrimSpace(*rw.Description) != "" {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_description_line"), escapeHTML(strings.TrimSpace(*rw.Description))))
			sb.WriteString("\n")
		}
		if rw.Tag != nil && strings.TrimSpace(*rw.Tag) != "" {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_tag_line"), escapeHTML(strings.TrimSpace(*rw.Tag))))
			sb.WriteString("\n")
		}
		if len(rw.ActiveInternalSquads) > 0 {
			var names []string
			for _, s := range rw.ActiveInternalSquads {
				names = append(names, escapeHTML(s.Name))
			}
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_squads_line"), strings.Join(names, ", ")))
			sb.WriteString("\n")
		}
		if rw.UserTraffic.OnlineAt != nil && !rw.UserTraffic.OnlineAt.IsZero() {
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_online_line"),
				rw.UserTraffic.OnlineAt.Format("02.01.2006 15:04")))
			sb.WriteString("\n")
		}
		if u := strings.TrimSpace(rw.SubscriptionUrl); u != "" {
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_card_subscription_url"), escapeHTML(u)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (h Handler) adminUserSubscriptionScreenHTML(ctx context.Context, b *bot.Bot, lang string, cust *database.Customer) string {
	return h.adminUserCardBodyHTML(ctx, b, lang, cust) + "\n\n" + h.adminUserSubscriptionSubmenuText(lang, cust)
}

func (h Handler) adminUserManageContent(ctx context.Context, b *bot.Bot, lang string, cust *database.Customer) (string, models.ReplyMarkup) {
	text := h.adminUserCardBodyHTML(ctx, b, lang, cust)
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_user_subscription_settings", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSubscriptionPrefix, cust.ID),
			}),
		},
		{
			h.translation.WithButton(lang, "admin_user_referrals_btn", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserReferralsPrefix, cust.ID),
			}),
			h.translation.WithButton(lang, "admin_user_stats_btn", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSpendPrefix, cust.ID),
			}),
		},
		{
			h.translation.WithButton(lang, "admin_user_payments_btn", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%dp%d", CallbackAdminUserPaymentsPrefix, cust.ID, 1),
			}),
			h.translation.WithButton(lang, "admin_user_send_message_btn", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserMsgHintPrefix, cust.ID),
			}),
		},
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersListAllPrefix + "0"}),
		},
	}
	return text, models.InlineKeyboardMarkup{InlineKeyboard: kb}
}

// --- Подменю подписки ------------------------------------------------------------

func (h Handler) adminUserSubscriptionSubmenuText(lang string, cust *database.Customer) string {
	return h.translation.GetText(lang, "admin_user_sub_menu_title") + "\n\n" +
		fmt.Sprintf(h.translation.GetText(lang, "admin_user_sub_menu_hint"), strconv.FormatInt(cust.TelegramID, 10))
}

func (h Handler) adminUserSubscriptionSubmenuKeyboard(ctx context.Context, lang string, cust *database.Customer) [][]models.InlineKeyboardButton {
	id := cust.ID
	st := ""
	if u, err := h.adminFindRWUserByCustomer(ctx, cust); err == nil && u != nil {
		st = strings.ToUpper(strings.TrimSpace(u.Status))
	}
	var disableOrEnable models.InlineKeyboardButton
	if st == "DISABLED" {
		disableOrEnable = h.translation.WithButton(lang, "admin_user_panel_enable", models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserEnableAskPrefix, id),
		})
	} else {
		disableOrEnable = h.translation.WithButton(lang, "admin_user_panel_adv_disable", models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserDisableAskPrefix, id),
		})
	}

	out := [][]models.InlineKeyboardButton{
		{
			models.InlineKeyboardButton{Text: h.translation.GetText(lang, "admin_user_cal_open_btn"), CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserCalOpenPrefix, id)},
		},
		{
			h.translation.WithButton(lang, "admin_user_devices_btn", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserDevicesPrefix, id)}),
			h.translation.WithButton(lang, "admin_user_hw_limit_menu_btn", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserHwPresetMenuPrefix, id)}),
		},
		{
			h.translation.WithButton(lang, "admin_user_extra_hwid_minus", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserExtraHwidDecPrefix, id),
			}),
			h.translation.WithButton(lang, "admin_user_extra_hwid_plus", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserExtraHwidIncPrefix, id),
			}),
		},
		{
			h.translation.WithButton(lang, "admin_user_reset_traffic_btn", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserResetTrafficAskPrefix, id)}),
		},
		{
			h.translation.WithButton(lang, "admin_user_panel_adv_squad", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSquadMenuPrefix, id)}),
		},
		{
			h.translation.WithButton(lang, "admin_user_panel_adv_strategy", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserStrategyMenuPrefix, id)}),
			h.translation.WithButton(lang, "admin_user_panel_adv_traffic", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserTrafficMenuPrefix, id)}),
		},
		{
			disableOrEnable,
			h.translation.WithButton(lang, "admin_user_panel_adv_delete", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserDeleteAskPrefix, id)}),
		},
	}
	if config.SalesMode() == "tariffs" && h.tariffRepository != nil {
		out = append(out, []models.InlineKeyboardButton{
			h.translation.WithButton(lang, "admin_user_change_tariff_btn", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserTariffMenuPrefix, id),
			}),
		})
	}
	out = append(out, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "admin_user_desc_change_btn", models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserDescAskPrefix, id),
		}),
	})
	out = append(out, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, id)}),
	})
	return out
}

func (h Handler) AdminUserSubscriptionHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserSubscriptionPrefix)
	if !ok {
		return
	}
	adminTrafficLimitClear(cb.From.ID)
	adminUserDescriptionClear(cb.From.ID)
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	text := h.adminUserSubscriptionScreenHTML(ctx, b, lang, cust)
	kb := h.adminUserSubscriptionSubmenuKeyboard(ctx, lang, cust)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin user subscription menu", "error", err)
	}
}

func parseAxeExtend(data string) (days int, customerID int64, ok bool) {
	if !strings.HasPrefix(data, CallbackAdminUserExtendPrefix) {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(data, CallbackAdminUserExtendPrefix)
	idx := strings.Index(rest, "t")
	if idx <= 0 || idx >= len(rest)-1 {
		return 0, 0, false
	}
	d, err1 := strconv.Atoi(rest[:idx])
	cid, err2 := strconv.ParseInt(rest[idx+1:], 10, 64)
	if err1 != nil || err2 != nil || d <= 0 || cid <= 0 {
		return 0, 0, false
	}
	return d, cid, true
}

func (h Handler) AdminUserExtendHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	days, cid, ok := parseAxeExtend(cb.Data)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, cid)
	if err != nil || cust == nil {
		return
	}
	if err := h.adminExtendCustomerDays(ctx, cust, days); err != nil {
		slog.Error("admin extend days", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	cust, _ = h.customerRepository.FindById(ctx, cid)
	lang := cb.From.LanguageCode
	text, markup := h.adminUserManageContent(ctx, b, lang, cust)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, markup, nil)
	if err != nil {
		slog.Error("admin extend edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func (h Handler) adminExtendCustomerDays(ctx context.Context, cust *database.Customer, days int) error {
	if days <= 0 {
		return fmt.Errorf("invalid days")
	}
	hasActive := cust.ExpireAt != nil && cust.ExpireAt.After(time.Now())
	var user *remnawave.User
	var err error
	if hasActive {
		user, err = h.remnawaveClient.ExtendSubscriptionByDaysPreserveSquads(ctx, cust.ID, cust.TelegramID, days)
	} else {
		user, err = h.remnawaveClient.CreateOrUpdateUserFromNow(ctx, cust.ID, cust.TelegramID, config.TrafficLimit(), days, false)
	}
	if err != nil {
		return err
	}
	return h.customerRepository.UpdateFields(ctx, cust.ID, map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
	})
}

func (h Handler) AdminUserResetTrafficAskHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserResetTrafficAskPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	text := h.translation.GetText(lang, "admin_user_reset_traffic_confirm_text")
	if u, errRW := h.adminFindRWUserByCustomer(ctx, cust); errRW == nil && u != nil && strings.TrimSpace(u.TrafficLimitStrategy) != "" {
		text += fmt.Sprintf(h.translation.GetText(lang, "admin_user_reset_traffic_current_strategy"), u.TrafficLimitStrategy)
	}
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_user_reset_traffic_confirm_yes", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserResetTrafficConfirmPrefix, cust.ID),
			}),
			h.translation.WithButton(lang, "admin_user_reset_traffic_confirm_no", models.InlineKeyboardButton{
				CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSubscriptionPrefix, cust.ID),
			}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin reset traffic ask edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func (h Handler) AdminUserResetTrafficConfirmHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserResetTrafficConfirmPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	u, err := h.adminFindRWUserByCustomer(ctx, cust)
	if err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	if err := h.remnawaveClient.ResetUserTraffic(ctx, u.ID); err != nil {
		slog.Error("admin reset traffic", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	lang := cb.From.LanguageCode
	text, markup := h.adminUserManageContent(ctx, b, lang, cust)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, markup, nil)
	if err != nil {
		slog.Error("admin reset traffic edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            h.translation.GetText(lang, "admin_user_reset_traffic_ok"),
	})
}

func (h Handler) AdminUserHwPresetMenuHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserHwPresetMenuPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	text := h.translation.GetText(lang, "admin_user_hw_limit_menu_text")
	row1 := []int{1, 2, 3, 4, 5}
	row2 := []int{6, 7, 8, 9, 10}
	var kb [][]models.InlineKeyboardButton
	for _, presets := range [][]int{row1, row2} {
		var row []models.InlineKeyboardButton
		for _, n := range presets {
			row = append(row, models.InlineKeyboardButton{
				Text:         strconv.Itoa(n),
				CallbackData: fmt.Sprintf("%s%d_%d", CallbackAdminUserHwPresetSetPrefix, cust.ID, n),
			})
		}
		kb = append(kb, row)
	}
	kb = append(kb, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSubscriptionPrefix, cust.ID)}),
	})
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin hw preset menu", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

func (h Handler) AdminUserHwPresetSetHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	if !strings.HasPrefix(cb.Data, CallbackAdminUserHwPresetSetPrefix) {
		return
	}
	rest := strings.TrimPrefix(cb.Data, CallbackAdminUserHwPresetSetPrefix)
	lastUnderscore := strings.LastIndex(rest, "_")
	if lastUnderscore <= 0 || lastUnderscore >= len(rest)-1 {
		return
	}
	cid, err := strconv.ParseInt(rest[:lastUnderscore], 10, 64)
	if err != nil || cid <= 0 {
		return
	}
	limit, err := strconv.Atoi(rest[lastUnderscore+1:])
	if err != nil || limit <= 0 {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, cid)
	if err != nil || cust == nil {
		return
	}
	// Лимит закреплён в кабинете: пресет меняет само закреплённое число,
	// иначе клиент Remnawave тут же подставил бы прежнее.
	if cust.HwidLimitOverride != nil {
		if err := h.customerRepository.SetHwidLimitOverride(ctx, cust.ID, &limit); err != nil {
			slog.Error("admin hw preset set pin", "error", err)
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: cb.ID,
				Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
				ShowAlert:       true,
			})
			return
		}
	}
	if _, err := h.remnawaveClient.UpdateUserDeviceLimitByCustomer(ctx, cust.ID, cust.TelegramID, limit); err != nil {
		slog.Error("admin hw preset set", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	lang := cb.From.LanguageCode
	text, markup := h.adminUserManageContent(ctx, b, lang, cust)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, markup, nil)
	if err != nil {
		slog.Error("admin hw preset edit card", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            h.translation.GetText(lang, "admin_user_hw_limit_saved"),
	})
}

// --- Устройства (админ по customer id) -------------------------------------------

// adminUserDevicesMessageAndKeyboard строит текст и клавиатуру экрана устройств (для adv и после adx).
func (h Handler) adminUserDevicesMessageAndKeyboard(ctx context.Context, lang string, cust *database.Customer) (messageText string, keyboard [][]models.InlineKeyboardButton, err error) {
	rwUser, e := h.adminFindRWUserByCustomer(ctx, cust)
	if e != nil || rwUser == nil {
		return "", nil, e
	}
	userID := rwUser.ID
	deviceLimit := 0
	if rwUser.HwidDeviceLimit != nil {
		deviceLimit = *rwUser.HwidDeviceLimit
	}
	if deviceLimit == 0 {
		deviceLimit = config.GetHwidFallbackDeviceLimit()
	}
	devices, e := h.remnawaveClient.GetUserDevices(ctx, userID)
	if e != nil {
		return "", nil, e
	}
	messageText = h.translation.GetText(lang, "devices_title")
	messageText += fmt.Sprintf(h.translation.GetText(lang, "device_limit"), len(devices), deviceLimit)
	if len(devices) == 0 {
		messageText += h.translation.GetText(lang, "no_devices")
	} else {
		for i, device := range devices {
			deviceName := h.getDeviceDisplayName(device, i+1)
			addedAt := device.CreatedAt.Format("02.01.2006 15:04")
			messageText += fmt.Sprintf("\n• %s — %s", deviceName, addedAt)
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: "🗑 " + deviceName, CallbackData: fmt.Sprintf("%s%d_%d", CallbackAdminUserDevDelPrefix, cust.ID, i)},
			})
		}
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSubscriptionPrefix, cust.ID)}),
	})
	return messageText, keyboard, nil
}

func (h Handler) AdminUserDevicesHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserDevicesPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	messageText, keyboard, err := h.adminUserDevicesMessageAndKeyboard(ctx, lang, cust)
	if err != nil {
		_, err = editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(lang, "admin_user_devices_error"), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserSubscriptionPrefix, cust.ID)})},
		}}, nil)
		if err != nil {
			slog.Error("admin devices error edit", "error", err)
		}
		return
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, messageText, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: keyboard}, nil)
	if err != nil {
		slog.Error("admin devices edit", "error", err)
	}
}

func (h Handler) AdminUserDevDelHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	rest := strings.TrimPrefix(cb.Data, CallbackAdminUserDevDelPrefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return
	}
	cid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil || idx < 0 {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, cid)
	if err != nil || cust == nil {
		return
	}
	rwUser, err := h.adminFindRWUserByCustomer(ctx, cust)
	if err != nil || rwUser == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	userID := rwUser.ID
	devices, err := h.remnawaveClient.GetUserDevices(ctx, userID)
	if err != nil || idx >= len(devices) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	if err := h.remnawaveClient.DeleteUserDevice(ctx, userID, devices[idx].Hwid); err != nil {
		slog.Error("admin delete device", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(cb.From.LanguageCode, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	lang := cb.From.LanguageCode
	messageText, keyboard, err := h.adminUserDevicesMessageAndKeyboard(ctx, lang, cust)
	if err != nil {
		slog.Error("admin delete device refresh", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            h.translation.GetText(lang, "admin_user_action_error"),
			ShowAlert:       true,
		})
		return
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, messageText, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: keyboard}, nil)
	if err != nil {
		slog.Error("admin delete device edit", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            h.translation.GetText(lang, "admin_user_panel_saved"),
	})
}

// --- Рефералы / статистика / оплаты ---------------------------------------------

func (h Handler) AdminUserReferralsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserReferralsPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	refByReferee, err := h.referralRepository.FindByReferee(ctx, cust.TelegramID)
	if err != nil {
		slog.Error("admin user ref by referee", "error", err)
		return
	}
	var sb strings.Builder
	sb.WriteString(h.translation.GetText(lang, "admin_user_ref_title"))
	sb.WriteString("\n\n")
	if refByReferee != nil {
		sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_ref_referrer_line"),
			refByReferee.ReferrerID))
		sb.WriteString("\n")
	} else {
		sb.WriteString(h.translation.GetText(lang, "admin_user_ref_no_inviter"))
		sb.WriteString("\n")
	}
	stats, err := h.referralRepository.GetStats(ctx, cust.TelegramID)
	if err != nil {
		slog.Error("admin user ref stats", "error", err)
		return
	}
	earned, err := h.referralRepository.CalculateEarnedDays(ctx, cust.TelegramID)
	if err != nil {
		earned = 0
	}
	sb.WriteString(fmt.Sprintf(h.translation.GetText(lang, "admin_user_ref_body"),
		stats.Total,
		stats.Paid,
		stats.Active,
		earned,
	))
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, cust.ID)}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, sb.String(), models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin user ref edit", "error", err)
	}
}

func (h Handler) AdminUserSpendHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserSpendPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	rubN, sumRub, starsN, sumStars, err := h.purchaseRepository.SumPaidSpendBreakdown(ctx, cust.ID)
	if err != nil {
		slog.Error("admin user spend", "error", err)
		return
	}
	text := h.translation.GetText(lang, "admin_user_spend_title") + "\n\n" +
		fmt.Sprintf(h.translation.GetText(lang, "admin_user_spend_body"), rubN, sumRub, starsN, sumStars)
	if rate := config.RubPerStar(); rate > 0 && sumStars > 0 {
		text += "\n" + fmt.Sprintf(h.translation.GetText(lang, "admin_user_spend_stars_rub_equiv"), sumStars*rate)
	}
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, cust.ID)}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin user spend edit", "error", err)
	}
}

func parseAupPayments(data string) (customerID int64, page int, ok bool) {
	if !strings.HasPrefix(data, CallbackAdminUserPaymentsPrefix) {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(data, CallbackAdminUserPaymentsPrefix)
	lastP := strings.LastIndex(rest, "p")
	if lastP <= 0 || lastP >= len(rest)-1 {
		return 0, 0, false
	}
	cid, err := strconv.ParseInt(rest[:lastP], 10, 64)
	pg, err2 := strconv.Atoi(rest[lastP+1:])
	if err != nil || err2 != nil || cid <= 0 || pg < 1 {
		return 0, 0, false
	}
	return cid, pg, true
}

func (h Handler) AdminUserPaymentsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	cid, page, ok := parseAupPayments(cb.Data)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, cid)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	total, err := h.purchaseRepository.CountPaidByCustomer(ctx, cust.ID)
	if err != nil {
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(adminUserPayPageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * adminUserPayPageSize
	purchases, err := h.purchaseRepository.FindPaidByCustomer(ctx, cust.ID, adminUserPayPageSize, offset)
	if err != nil {
		return
	}
	text := h.buildPurchaseHistoryText(ctx, lang, purchases, page, totalPages)
	kb := h.adminUserPurchaseHistoryMarkup(lang, cust.ID, page, totalPages)
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin user payments edit", "error", err)
	}
}

func (h Handler) adminUserPurchaseHistoryMarkup(lang string, customerID int64, page, totalPages int) [][]models.InlineKeyboardButton {
	tm := translation.GetInstance()
	var rows [][]models.InlineKeyboardButton
	var nav []models.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, models.InlineKeyboardButton{
			Text:         tm.GetText(lang, "purchase_history_prev"),
			CallbackData: fmt.Sprintf("%s%dp%d", CallbackAdminUserPaymentsPrefix, customerID, page-1),
		})
	}
	if page < totalPages {
		nav = append(nav, models.InlineKeyboardButton{
			Text:         tm.GetText(lang, "purchase_history_next"),
			CallbackData: fmt.Sprintf("%s%dp%d", CallbackAdminUserPaymentsPrefix, customerID, page+1),
		})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		tm.WithButton(lang, "back_button", models.InlineKeyboardButton{
			CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, customerID),
		}),
	})
	return rows
}

func (h Handler) AdminUserMsgHintHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	id, ok := parseCustomerIDFromPrefix(cb.Data, CallbackAdminUserMsgHintPrefix)
	if !ok {
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	cust, err := h.customerRepository.FindById(ctx, id)
	if err != nil || cust == nil {
		return
	}
	lang := cb.From.LanguageCode
	adminUsersSearchSet(cb.From.ID, false)
	adminTrafficLimitClear(cb.From.ID)
	adminUsersDMSet(cb.From.ID, cust.TelegramID)
	text := h.translation.GetText(lang, "admin_user_send_dm_prompt")
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_users_search_cancel", models.InlineKeyboardButton{CallbackData: fmt.Sprintf("%s%d", CallbackAdminUserManagePrefix, cust.ID)}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin user msg hint", "error", err)
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
}

// AdminUserDMMessageHandler — следующий текст админа уходит пользователю от бота.
func (h Handler) AdminUserDMMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	adminID := update.Message.From.ID
	if adminID != config.GetAdminTelegramId() {
		return
	}
	targetTG, ok := adminUsersDMRecipient(adminID)
	if !ok {
		return
	}
	txt := strings.TrimSpace(update.Message.Text)
	if txt == "" {
		return
	}
	adminUsersDMClear(adminID)
	// TODO(cabinet-web-only): после Этапа 1 добавить перед SendMessage:
	//   if utils.IsSyntheticTelegramID(targetTG) {
	//       // ответить админу "admin_user_dm_unavailable_webonly"
	//       return
	//   }
	// Плюс идеально — прятать саму кнопку «Написать пользователю» при
	// cust.IsWebOnly в adminUsersDMSet/карточке пользователя.
	// См. docs/cabinet/audit-telegram-id.md, раздел 1.4.
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: targetTG,
		Text:   txt,
	})
	langDM := update.Message.From.LanguageCode
	dmBackKB := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(langDM, "admin_users_all", models.InlineKeyboardButton{
				CallbackData: CallbackAdminUsersListAllPrefix + "0",
			}),
		},
	}}
	if err != nil {
		slog.Error("admin dm send", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      update.Message.Chat.ID,
			Text:        h.translation.GetText(langDM, "admin_user_send_dm_failed"),
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: dmBackKB,
		})
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        h.translation.GetText(langDM, "admin_user_send_dm_ok"),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: dmBackKB,
	})
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: update.Message.ID,
	})
}

// --- Ветка «Подписки» ------------------------------------------------------------

func (h Handler) AdminSubsRootHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil {
		return
	}
	text := h.translation.GetText(lang, "admin_subs_root_title")
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_subs_all", models.InlineKeyboardButton{CallbackData: CallbackAdminSubsListPrefix + "0"}),
		},
		{
			h.translation.WithButton(lang, "admin_subs_expiring", models.InlineKeyboardButton{CallbackData: CallbackAdminSubsExpiring}),
		},
		{
			h.translation.WithButton(lang, "admin_subs_stats", models.InlineKeyboardButton{CallbackData: CallbackAdminSubsStatsJump}),
		},
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
		},
	}
	_, err := editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin subs root", "error", err)
	}
}

func (h Handler) AdminSubsListRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, CallbackAdminSubsListPrefix) {
		return
	}
	page, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, CallbackAdminSubsListPrefix))
	if err != nil || page < 0 {
		page = 0
	}
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeAll, page, "admin_subs_list_title", CallbackAdminSubsRoot)
}

func (h Handler) AdminSubsExpiringHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeExpiringSoon, 0, "admin_subs_expiring_list_title", CallbackAdminSubsRoot)
}

func (h Handler) AdminSubsExpiringListRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, CallbackAdminSubsExpiringListPrefix) {
		return
	}
	page, err := strconv.Atoi(strings.TrimPrefix(update.CallbackQuery.Data, CallbackAdminSubsExpiringListPrefix))
	if err != nil || page < 0 {
		page = 0
	}
	h.adminUsersListPage(ctx, b, update, database.CustomerListScopeExpiringSoon, page, "admin_subs_expiring_list_title", CallbackAdminSubsRoot)
}

func (h Handler) AdminSubsStatsJumpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.AdminStatsSubsHandler(ctx, b, update)
}

// --- Рефералы (короткий экран) ----------------------------------------------------

func (h Handler) AdminRefRootHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !isAdmin(update.CallbackQuery) {
		return
	}
	cb := update.CallbackQuery
	lang := cb.From.LanguageCode
	msg := cb.Message.Message
	if msg == nil || h.statsRepository == nil {
		return
	}
	snap, err := h.statsRepository.FetchAdminStatsSnapshot(ctx)
	if err != nil {
		slog.Error("admin ref root", "error", err)
		return
	}
	body := fmt.Sprintf(h.translation.GetText(lang, "admin_ref_root_body"),
		snap.DistinctReferrers,
		snap.ActiveReferrers,
		snap.RefBonusDaysMonth,
	)
	text := h.translation.GetText(lang, "admin_ref_root_title") + "\n\n" + body + "\n\n" + h.formatStatsUpdated(lang, snap.CapturedAt)
	kb := [][]models.InlineKeyboardButton{
		{
			h.translation.WithButton(lang, "admin_ref_open_full", models.InlineKeyboardButton{CallbackData: CallbackAdminStatsRef}),
		},
		{
			h.translation.WithButton(lang, "back_button", models.InlineKeyboardButton{CallbackData: CallbackAdminUsersRoot}),
		},
	}
	_, err = editCallbackOriginToHTMLText(ctx, b, msg, text, models.ParseModeHTML, models.InlineKeyboardMarkup{InlineKeyboard: kb}, nil)
	if err != nil {
		slog.Error("admin ref root edit", "error", err)
	}
}
