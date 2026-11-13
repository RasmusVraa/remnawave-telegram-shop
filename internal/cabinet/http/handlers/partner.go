package handlers

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/adminevents"
	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/utils"
)

// PartnerHandler — кабинет партнёра: /cabinet/api/me/partner/*.
//
// Все ручки работают от аккаунта кабинета: партнёрство — свойство клиента, а
// клиент резолвится из аккаунта через bootstrap, как в остальных «me»-ручках.
type PartnerHandler struct {
	boot      *bootstrap.CustomerBootstrap
	customers *database.CustomerRepository
	partners  *database.PartnerRepository
	publicURL string
	botURL    string
	// notify может быть nil — см. AdminPartnersHandler.notify.
	notify *notification.PartnerNotifier
}

func NewPartner(
	boot *bootstrap.CustomerBootstrap,
	customers *database.CustomerRepository,
	partners *database.PartnerRepository,
	publicURL string,
	botURL string,
	notify *notification.PartnerNotifier,
) *PartnerHandler {
	return &PartnerHandler{
		boot:      boot,
		customers: customers,
		partners:  partners,
		publicURL: strings.TrimRight(publicURL, "/"),
		botURL:    strings.TrimSpace(botURL),
		notify:    notify,
	}
}

// adminLabel — подпись партнёра для админского уведомления, без маскирования.
//
// Читаем админскую строку, а не Customer: почта живёт в аккаунте кабинета, и у
// web-only партнёра подпись по одному лишь Customer выродилась бы в прочерк.
// Пустая строка означает «подпись не получилась» — тогда уведомление лучше не
// слать вовсе, чем слать безымянное.
func (h *PartnerHandler) adminLabel(ctx context.Context, partnerID int64) string {
	row, err := h.partners.GetPartnerAdminRow(ctx, partnerID)
	if err != nil || row == nil {
		slog.Error("partner: admin label", "error", err, "partner_id", partnerID)
		return ""
	}
	return adminCustomerLabel(row.CustomerUsername, row.CustomerEmail,
		row.CustomerTelegramID, row.CustomerIsWebOnly)
}

// --- DTO ---

type partnerTermsDTO struct {
	FirstPercent       float64 `json:"first_percent"`
	RenewalPercent     float64 `json:"renewal_percent"`
	HoldDays           int     `json:"hold_days"`
	MinPayout          float64 `json:"min_payout"`
	PayoutCooldownDays int     `json:"payout_cooldown_days"`
	MaxLinks           int     `json:"max_links"`
	CountExtraHwid     bool    `json:"count_extra_hwid"`
}

type partnerApplicationDTO struct {
	About       string `json:"about,omitempty"`
	Channels    string `json:"channels,omitempty"`
	Expected    string `json:"expected,omitempty"`
	SubmittedAt string `json:"submitted_at,omitempty"`
	AdminNote   string `json:"admin_note,omitempty"`
}

type partnerLinkDTO struct {
	ID         int64   `json:"id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	IsDefault  bool    `json:"is_default"`
	Archived   bool    `json:"archived"`
	BotLink    string  `json:"bot_link,omitempty"`
	WebLink    string  `json:"web_link,omitempty"`
	Customers  int     `json:"customers"`
	Paying     int     `json:"paying"`
	Earned     float64 `json:"earned"`
	CanDelete  bool    `json:"can_delete"`
	CanArchive bool    `json:"can_archive"`
}

type partnerSummaryDTO struct {
	Customers         int     `json:"customers"`
	CustomersLastWeek int     `json:"customers_last_week"`
	Paying            int     `json:"paying"`
	Active            int     `json:"active"`
	ConversionPct     int     `json:"conversion_pct"`
	EarnedTotal       float64 `json:"earned_total"`
	EarnedLastMonth   float64 `json:"earned_last_month"`
}

type partnerMonthDTO struct {
	Month  string  `json:"month"`
	Amount float64 `json:"amount"`
}

type partnerAccountDTO struct {
	Balance           float64           `json:"balance"`
	HoldBalance       float64           `json:"hold_balance"`
	ReservedBalance   float64           `json:"reserved_balance"`
	TotalEarned       float64           `json:"total_earned"`
	TotalPaid         float64           `json:"total_paid"`
	FirstPercent      float64           `json:"first_percent"`
	RenewalPercent    float64           `json:"renewal_percent"`
	NextHoldReleaseAt string            `json:"next_hold_release_at,omitempty"`
	CanWithdraw       bool              `json:"can_withdraw"`
	PayoutAvailableAt string            `json:"payout_available_at,omitempty"`
	HasOpenPayout     bool              `json:"has_open_payout"`
	PayoutMethod      string            `json:"payout_method,omitempty"`
	PayoutDetails     string            `json:"payout_details,omitempty"`
	LinksUsed         int               `json:"links_used"`
	LinksLimit        int               `json:"links_limit"`
	Summary           partnerSummaryDTO `json:"summary"`
	Months            []partnerMonthDTO `json:"months"`
	Links             []partnerLinkDTO  `json:"links"`
}

type partnerStateResp struct {
	Enabled             bool `json:"enabled"`
	ApplicationsEnabled bool `json:"applications_enabled"`

	// none — клиент не подавал заявку; дальше статусы партнёра как в БД.
	Status      string                 `json:"status"`
	Terms       partnerTermsDTO        `json:"terms"`
	Application *partnerApplicationDTO `json:"application,omitempty"`
	Partner     *partnerAccountDTO     `json:"partner,omitempty"`
}

type partnerCustomerDTO struct {
	Label      string  `json:"label"`
	Active     bool    `json:"active"`
	HasPaid    bool    `json:"has_paid"`
	Earned     float64 `json:"earned"`
	LinkName   string  `json:"link_name,omitempty"`
	AttachedAt string  `json:"attached_at"`
}

type partnerEarningDTO struct {
	ID            int64   `json:"id"`
	Amount        float64 `json:"amount"`
	Percent       float64 `json:"percent"`
	BaseAmount    float64 `json:"base_amount"`
	BaseCurrency  string  `json:"base_currency"`
	BaseAmountRub float64 `json:"base_amount_rub"`
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	HoldUntil     string  `json:"hold_until,omitempty"`
	Note          string  `json:"note,omitempty"`
	CustomerLabel string  `json:"customer_label,omitempty"`
	LinkName      string  `json:"link_name,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type partnerPayoutDTO struct {
	ID           int64   `json:"id"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
	Method       string  `json:"method,omitempty"`
	AdminComment string  `json:"admin_comment,omitempty"`
	ExternalRef  string  `json:"external_ref,omitempty"`
	RequestedAt  string  `json:"requested_at"`
	ProcessedAt  string  `json:"processed_at,omitempty"`
}

// --- helpers ---

func formatTimeRFC3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// partnerCustomerLabel — подпись клиента в списках партнёра.
//
// Маскируется ВСЁ: и username, и email, и telegram id. Партнёр должен видеть,
// что клиент живой и платит, но не получать контакт — иначе список приведённых
// превращается в готовую базу для увода к конкуренту. Открытый @username это
// контакт ничуть не меньше, чем почта: по нему пишут напрямую.
//
// Подпись при этом остаётся узнаваемой для самого партнёра: он помнит, кого
// приводил, и «@ca***at» ему хватает, чтобы сопоставить строку с человеком.
func partnerCustomerLabel(username, email *string, telegramID int64, webOnly bool) string {
	if username != nil {
		if masked := maskPartnerHandle(strings.TrimSpace(*username)); masked != "" {
			return masked
		}
	}
	if email != nil {
		if masked := maskPartnerEmail(*email); masked != "" {
			return masked
		}
	}
	if webOnly || telegramID == 0 {
		return "—"
	}
	return utils.MaskHalfInt64(telegramID)
}

// adminCustomerLabel — подпись человека для администратора: без маскирования.
//
// Маска существует ради одного сценария — партнёр не должен получить контакты
// приведённых им людей. Администратору она только мешает: он разбирает споры,
// ищет человека в поддержке и сверяет реквизиты, и «@ca***at» для этого
// бесполезен.
func adminCustomerLabel(username, email *string, telegramID int64, webOnly bool) string {
	if username != nil {
		if u := strings.TrimPrefix(strings.TrimSpace(*username), "@"); u != "" {
			return "@" + u
		}
	}
	if email != nil {
		if e := strings.TrimSpace(*email); e != "" {
			return e
		}
	}
	if webOnly || telegramID == 0 {
		return "—"
	}
	return strconv.FormatInt(telegramID, 10)
}

// maskPartnerHandle прячет середину имени: «cat_tac_cat» → «@ca***at».
// Короткие имена сокращаются сильнее, чтобы из двух символов не восстановить
// целое.
func maskPartnerHandle(handle string) string {
	h := strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if h == "" {
		return ""
	}
	runes := []rune(h)
	switch {
	case len(runes) <= 2:
		return "@" + string(runes[0]) + "***"
	case len(runes) <= 5:
		return "@" + string(runes[0]) + "***" + string(runes[len(runes)-1])
	default:
		return "@" + string(runes[:2]) + "***" + string(runes[len(runes)-2:])
	}
}

func maskPartnerEmail(email string) string {
	v := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(v, "@")
	if at <= 0 || at >= len(v)-1 {
		return ""
	}
	local, domain := v[:at], v[at+1:]
	if len(local) <= 1 {
		return local + "***@" + domain
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + domain
}

// partnerBotLink собирает deeplink вида https://t.me/Bot?start=p_<code>.
// Пусто, если BOT_URL не настроен: показывать битую ссылку хуже, чем ни одной.
func partnerBotLink(botURL, code string) string {
	u := strings.TrimRight(strings.TrimSpace(botURL), "/")
	if u == "" || code == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(u, "https://t.me/"), strings.HasPrefix(u, "http://t.me/"):
		return u + "?start=p_" + code
	case strings.HasPrefix(u, "@"):
		return "https://t.me/" + strings.TrimPrefix(u, "@") + "?start=p_" + code
	default:
		return ""
	}
}

func (h *PartnerHandler) partnerWebLink(code string) string {
	if h.publicURL == "" || code == "" {
		return ""
	}
	return h.publicURL + "/cabinet/register?ref=p_" + code
}

func partnerTerms() partnerTermsDTO {
	return partnerTermsDTO{
		FirstPercent:       config.PartnerFirstPercent(),
		RenewalPercent:     config.PartnerRenewalPercent(),
		HoldDays:           config.PartnerHoldDays(),
		MinPayout:          config.PartnerMinPayout(),
		PayoutCooldownDays: config.PartnerPayoutCooldownDays(),
		MaxLinks:           config.PartnerMaxLinks(),
		CountExtraHwid:     config.PartnerCountExtraHwid(),
	}
}

func effectivePercent(individual *float64, fallback float64) float64 {
	if individual != nil {
		return *individual
	}
	return fallback
}

func (h *PartnerHandler) linksLimit(p *database.Partner) int {
	if p != nil && p.LinksLimit != nil && *p.LinksLimit > 0 {
		return *p.LinksLimit
	}
	return config.PartnerMaxLinks()
}

// resolve достаёт клиента и его партнёрскую запись. Второе значение nil, если
// клиент не партнёр — это нормальное состояние, а не ошибка.
func (h *PartnerHandler) resolve(ctx context.Context, accountID int64) (*database.Customer, *database.Partner, error) {
	if h.boot == nil || h.customers == nil || h.partners == nil {
		return nil, nil, errors.New("partner handler not initialized")
	}
	link, err := h.boot.EnsureForAccount(ctx, accountID, "")
	if err != nil {
		return nil, nil, err
	}
	if link == nil {
		return nil, nil, errors.New("customer link not found")
	}
	customer, err := h.customers.FindById(ctx, link.CustomerID)
	if err != nil || customer == nil {
		return nil, nil, err
	}
	partner, err := h.partners.FindByCustomerID(ctx, customer.ID)
	if err != nil {
		return nil, nil, err
	}
	return customer, partner, nil
}

// requirePartner — общая преамбула мутирующих ручек: авторизация, включённая
// программа и партнёр в работе. Возвращает nil, если ответ уже записан.
func (h *PartnerHandler) requirePartner(w http.ResponseWriter, r *http.Request) *database.Partner {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	if !config.PartnerProgramEnabled() {
		http.Error(w, "partner program is disabled", http.StatusForbidden)
		return nil
	}
	_, partner, err := h.resolve(r.Context(), claims.AccountID)
	if err != nil {
		if handleAccountGone(w, err, "partner", claims.AccountID) {
			return nil
		}
		slog.Error("partner: resolve customer", "error", err, "account_id", claims.AccountID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil
	}
	if partner == nil || !partner.IsActive() {
		http.Error(w, "not a partner", http.StatusForbidden)
		return nil
	}
	return partner
}

// --- GET /cabinet/api/me/partner ---

func (h *PartnerHandler) GetState(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := partnerStateResp{
		Enabled:             config.PartnerProgramEnabled(),
		ApplicationsEnabled: config.PartnerApplicationsEnabled(),
		Status:              "none",
		Terms:               partnerTerms(),
	}
	// Выключенная программа не раскрывает ничего лишнего, но и не врёт: раздел
	// просто скрыт на фронте.
	if !resp.Enabled {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, resp)
		return
	}

	_, partner, err := h.resolve(r.Context(), claims.AccountID)
	if err != nil {
		if handleAccountGone(w, err, "partner.state", claims.AccountID) {
			return
		}
		slog.Error("partner: state resolve", "error", err, "account_id", claims.AccountID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if partner == nil {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Status = partner.Status
	if partner.AppSubmittedAt != nil || partner.AppAbout != nil || partner.AdminNote != nil {
		resp.Application = &partnerApplicationDTO{
			About:       ptrString(partner.AppAbout),
			Channels:    ptrString(partner.AppChannels),
			Expected:    ptrString(partner.AppExpected),
			SubmittedAt: formatTimeRFC3339(partner.AppSubmittedAt),
			AdminNote:   ptrString(partner.AdminNote),
		}
	}

	// Кабинет с цифрами показывается только тому, кто уже в работе: у заявки на
	// рассмотрении ни ссылок, ни начислений быть не может.
	if partner.IsActive() {
		account, err := h.buildAccount(r.Context(), partner)
		if err != nil {
			slog.Error("partner: build account", "error", err, "partner_id", partner.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		resp.Partner = account
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

func (h *PartnerHandler) buildAccount(ctx context.Context, partner *database.Partner) (*partnerAccountDTO, error) {
	summary, err := h.partners.Summary(ctx, partner.ID)
	if err != nil {
		return nil, err
	}
	months, err := h.partners.EarningsByMonth(ctx, partner.ID, 6)
	if err != nil {
		return nil, err
	}
	links, err := h.partners.ListLinksWithStats(ctx, partner.ID)
	if err != nil {
		return nil, err
	}
	nextRelease, err := h.partners.NextHoldReleaseAt(ctx, partner.ID)
	if err != nil {
		return nil, err
	}
	lastPayout, err := h.partners.LastPayoutRequestAt(ctx, partner.ID)
	if err != nil {
		return nil, err
	}
	hasOpenPayout, err := h.partners.HasOpenPayout(ctx, partner.ID)
	if err != nil {
		return nil, err
	}

	dto := &partnerAccountDTO{
		Balance:           partner.Balance,
		HoldBalance:       partner.HoldBalance,
		ReservedBalance:   partner.ReservedBalance,
		TotalEarned:       partner.TotalEarned,
		TotalPaid:         partner.TotalPaid,
		FirstPercent:      effectivePercent(partner.FirstPercent, config.PartnerFirstPercent()),
		RenewalPercent:    effectivePercent(partner.RenewalPercent, config.PartnerRenewalPercent()),
		NextHoldReleaseAt: formatTimeRFC3339(nextRelease),
		CanWithdraw:       partner.CanWithdraw(),
		HasOpenPayout:     hasOpenPayout,
		PayoutMethod:      ptrString(partner.PayoutMethod),
		PayoutDetails:     ptrString(partner.PayoutDetails),
		LinksLimit:        h.linksLimit(partner),
		Summary: partnerSummaryDTO{
			Customers:         summary.Customers,
			CustomersLastWeek: summary.CustomersLastWeek,
			Paying:            summary.PayingCustomers,
			Active:            summary.ActiveCustomers,
			ConversionPct:     summary.ConversionPercent(),
			EarnedTotal:       summary.EarnedTotal,
			EarnedLastMonth:   summary.EarnedLastMonth,
		},
		Months: make([]partnerMonthDTO, 0, len(months)),
		Links:  make([]partnerLinkDTO, 0, len(links)),
	}

	// Дата, с которой снова можно просить вывод: партнёр должен видеть её
	// заранее, а не упираться в отказ после заполнения формы.
	if lastPayout != nil && config.PartnerPayoutCooldownDays() > 0 {
		next := lastPayout.AddDate(0, 0, config.PartnerPayoutCooldownDays())
		if next.After(time.Now().UTC()) {
			dto.PayoutAvailableAt = formatTimeRFC3339(&next)
		}
	}

	for _, m := range months {
		dto.Months = append(dto.Months, partnerMonthDTO{
			Month:  m.Month.UTC().Format("2006-01"),
			Amount: m.Amount,
		})
	}

	for _, l := range links {
		archived := l.Link.ArchivedAt != nil
		if !archived {
			dto.LinksUsed++
		}
		dto.Links = append(dto.Links, partnerLinkDTO{
			ID:        l.Link.ID,
			Code:      l.Link.Code,
			Name:      l.Link.Name,
			IsDefault: l.Link.IsDefault,
			Archived:  archived,
			BotLink:   partnerBotLink(h.botURL, l.Link.Code),
			WebLink:   h.partnerWebLink(l.Link.Code),
			Customers: l.Customers,
			Paying:    l.Paying,
			Earned:    l.Earned,
			// Пустой поток — опечатка, его стирают. Поток с историей
			// архивируют: за ним числятся клиенты и начисления.
			CanDelete:  !l.Link.IsDefault && l.Customers == 0 && l.Earned == 0,
			CanArchive: !l.Link.IsDefault,
		})
	}

	return dto, nil
}

// --- POST /cabinet/api/me/partner/apply ---

type partnerApplyReq struct {
	About    string `json:"about"`
	Channels string `json:"channels"`
	Expected string `json:"expected"`
}

func (h *PartnerHandler) Apply(w http.ResponseWriter, r *http.Request) {
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !config.PartnerProgramEnabled() || !config.PartnerApplicationsEnabled() {
		http.Error(w, "applications are closed", http.StatusForbidden)
		return
	}

	var req partnerApplyReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.About = strings.TrimSpace(req.About)
	if req.About == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "about_required"})
		return
	}
	// Верхняя граница на длину — чтобы анкета не превратилась в способ залить в
	// базу мегабайт текста. decodeJSON ограничивает тело, здесь — поля.
	if len(req.About) > 2000 || len(req.Channels) > 1000 || len(req.Expected) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "too_long"})
		return
	}

	customer, _, err := h.resolve(r.Context(), claims.AccountID)
	if err != nil || customer == nil {
		if handleAccountGone(w, err, "partner.apply", claims.AccountID) {
			return
		}
		slog.Error("partner: apply resolve", "error", err, "account_id", claims.AccountID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	partner, err := h.partners.SubmitApplication(r.Context(), customer.ID,
		req.About, req.Channels, req.Expected, config.PartnerAutoApprove())
	switch {
	case errors.Is(err, database.ErrPartnerAlreadyActive):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "already_partner"})
		return
	case errors.Is(err, database.ErrPartnerApplicationPending):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "already_pending"})
		return
	case err != nil:
		slog.Error("partner: submit application", "error", err, "customer_id", utils.MaskHalfInt64(customer.ID))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("partner application submitted",
		"partner_id", partner.ID, "status", partner.Status,
		"customer_id", utils.MaskHalfInt64(customer.ID))
	adminevents.Emit(adminevents.Event{
		Type: adminevents.PartnerApplied, CustomerID: customer.ID, Source: adminevents.SourceCabinet,
		Payload: map[string]any{"auto_approved": partner.Status == database.PartnerStatusActive},
	})

	if h.notify != nil {
		if label := h.adminLabel(r.Context(), partner.ID); label != "" {
			h.notify.ApplicationSubmitted(r.Context(), notification.PartnerApplication{
				Label:    label,
				About:    req.About,
				Channels: req.Channels,
				Expected: req.Expected,
				// При включённом автоодобрении разбирать нечего: человек уже
				// партнёр, и уведомление должно сообщать именно это.
				AutoApproved: partner.Status == database.PartnerStatusActive,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": partner.Status})
}

// --- GET /cabinet/api/me/partner/customers ---

func paginationParams(r *http.Request, defLimit, maxLimit int) (limit, offset int) {
	limit = defLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

func (h *PartnerHandler) GetCustomers(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}
	limit, offset := paginationParams(r, 25, 100)

	rows, total, err := h.partners.ListCustomers(r.Context(), partner.ID, limit, offset)
	if err != nil {
		slog.Error("partner: list customers", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]partnerCustomerDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, partnerCustomerDTO{
			Label:      partnerCustomerLabel(row.TelegramUsername, row.Email, row.TelegramID, row.IsWebOnly),
			Active:     row.Active,
			HasPaid:    row.HasPaid,
			Earned:     row.Earned,
			LinkName:   ptrString(row.LinkName),
			AttachedAt: row.AttachedAt.UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// --- GET /cabinet/api/me/partner/earnings ---

func (h *PartnerHandler) GetEarnings(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}
	limit, offset := paginationParams(r, 25, 100)

	rows, total, err := h.partners.ListEarnings(r.Context(), partner.ID, limit, offset)
	if err != nil {
		slog.Error("partner: list earnings", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]partnerEarningDTO, 0, len(rows))
	for _, row := range rows {
		label := ""
		if row.CustomerTelegramID != nil {
			label = partnerCustomerLabel(row.CustomerUsername, row.CustomerEmail, *row.CustomerTelegramID, row.CustomerIsWebOnly)
		}
		items = append(items, partnerEarningDTO{
			ID:            row.ID,
			Amount:        row.Amount,
			Percent:       row.Percent,
			BaseAmount:    row.BaseAmount,
			BaseCurrency:  row.BaseCurrency,
			BaseAmountRub: row.BaseAmountRub,
			Kind:          row.Kind,
			Status:        row.Status,
			HoldUntil:     formatTimeRFC3339(row.HoldUntil),
			Note:          ptrString(row.Note),
			CustomerLabel: label,
			LinkName:      ptrString(row.LinkName),
			CreatedAt:     row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// --- POST /cabinet/api/me/partner/links ---

type partnerLinkReq struct {
	Name string `json:"name"`
}

func (h *PartnerHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}
	var req partnerLinkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_name"})
		return
	}

	used, err := h.partners.CountLinks(r.Context(), partner.ID)
	if err != nil {
		slog.Error("partner: count links", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if used >= h.linksLimit(partner) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "links_limit_reached"})
		return
	}

	link, err := h.partners.CreateLink(r.Context(), partner.ID, name)
	if err != nil {
		slog.Error("partner: create link", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, partnerLinkDTO{
		ID:         link.ID,
		Code:       link.Code,
		Name:       link.Name,
		BotLink:    partnerBotLink(h.botURL, link.Code),
		WebLink:    h.partnerWebLink(link.Code),
		CanDelete:  true,
		CanArchive: true,
	})
}

// --- PATCH/DELETE /cabinet/api/me/partner/links/{id} ---

func extractPartnerLinkID(path string) (int64, bool) {
	s := strings.TrimPrefix(path, "/cabinet/api/me/partner/links/")
	s = strings.TrimRight(s, "/")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

type partnerLinkPatchReq struct {
	Name     *string `json:"name,omitempty"`
	Archived *bool   `json:"archived,omitempty"`
}

func (h *PartnerHandler) LinkByID(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}
	linkID, ok := extractPartnerLinkID(r.URL.Path)
	if !ok {
		http.Error(w, "invalid link id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req partnerLinkPatchReq
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" || len(name) > 64 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_name"})
				return
			}
			if err := h.partners.RenameLink(r.Context(), partner.ID, linkID, name); err != nil {
				writePartnerLinkErr(w, err, "partner.link.rename", partner.ID)
				return
			}
		}
		if req.Archived != nil {
			if err := h.partners.SetLinkArchived(r.Context(), partner.ID, linkID, *req.Archived, h.linksLimit(partner)); err != nil {
				writePartnerLinkErr(w, err, "partner.link.archive", partner.ID)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := h.partners.DeleteLink(r.Context(), partner.ID, linkID); err != nil {
			writePartnerLinkErr(w, err, "partner.link.delete", partner.ID)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		w.Header().Set("Allow", "PATCH, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writePartnerLinkErr переводит отказы репозитория в ответы, по которым фронт
// покажет объяснение, а не «что-то пошло не так».
func writePartnerLinkErr(w http.ResponseWriter, err error, op string, partnerID int64) {
	switch {
	case errors.Is(err, database.ErrPartnerLinkNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "link_not_found"})
	case errors.Is(err, database.ErrPartnerLinkIsDefault):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "link_is_default"})
	case errors.Is(err, database.ErrPartnerLinkHasHistory):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "link_has_history"})
	case errors.Is(err, database.ErrPartnerLinkLimitReached):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "links_limit_reached"})
	default:
		slog.Error("partner link operation failed", "op", op, "error", err, "partner_id", partnerID)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// --- PUT /cabinet/api/me/partner/payout-details ---

type partnerPayoutDetailsReq struct {
	Method  string `json:"method"`
	Details string `json:"details"`
}

func (h *PartnerHandler) PutPayoutDetails(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}
	var req partnerPayoutDetailsReq
	if !decodeJSON(w, r, &req) {
		return
	}
	method := strings.TrimSpace(req.Method)
	details := strings.TrimSpace(req.Details)
	if method == "" || details == "" || len(method) > 64 || len(details) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_details"})
		return
	}

	if err := h.partners.UpdatePayoutDetails(r.Context(), partner.ID, method, details); err != nil {
		slog.Error("partner: update payout details", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- GET/POST /cabinet/api/me/partner/payouts ---

type partnerPayoutReq struct {
	Amount float64 `json:"amount"`
}

func (h *PartnerHandler) Payouts(w http.ResponseWriter, r *http.Request) {
	partner := h.requirePartner(w, r)
	if partner == nil {
		return
	}

	switch r.Method {
	case http.MethodGet:
		limit, offset := paginationParams(r, 25, 100)
		payouts, total, err := h.partners.ListPayouts(r.Context(), partner.ID, limit, offset)
		if err != nil {
			slog.Error("partner: list payouts", "error", err, "partner_id", partner.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		items := make([]partnerPayoutDTO, 0, len(payouts))
		for _, p := range payouts {
			items = append(items, partnerPayoutDTO{
				ID:           p.ID,
				Amount:       p.Amount,
				Status:       p.Status,
				Method:       ptrString(p.Method),
				AdminComment: ptrString(p.AdminComment),
				ExternalRef:  ptrString(p.ExternalRef),
				RequestedAt:  p.RequestedAt.UTC().Format(time.RFC3339),
				ProcessedAt:  formatTimeRFC3339(p.ProcessedAt),
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})

	case http.MethodPost:
		h.createPayout(w, r, partner)

	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *PartnerHandler) createPayout(w http.ResponseWriter, r *http.Request, partner *database.Partner) {
	var req partnerPayoutReq
	if !decodeJSON(w, r, &req) {
		return
	}

	// Каждая проверка отвечает своим кодом: партнёр должен понять, что именно
	// мешает выводу, а не гадать по общему отказу.
	if !partner.CanWithdraw() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "withdraw_blocked"})
		return
	}
	if partner.PayoutDetails == nil || strings.TrimSpace(*partner.PayoutDetails) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "details_required"})
		return
	}
	amount := roundMoney(req.Amount)
	if amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_amount"})
		return
	}
	if min := config.PartnerMinPayout(); amount < min {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "below_minimum", "minimum": min})
		return
	}

	open, err := h.partners.HasOpenPayout(r.Context(), partner.ID)
	if err != nil {
		slog.Error("partner: check open payout", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if open {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "payout_pending"})
		return
	}

	if cooldown := config.PartnerPayoutCooldownDays(); cooldown > 0 {
		last, err := h.partners.LastPayoutRequestAt(r.Context(), partner.ID)
		if err != nil {
			slog.Error("partner: last payout", "error", err, "partner_id", partner.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if last != nil {
			next := last.AddDate(0, 0, cooldown)
			if next.After(time.Now().UTC()) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":        "cooldown",
					"available_at": next.UTC().Format(time.RFC3339),
				})
				return
			}
		}
	}

	payout, err := h.partners.CreatePayout(r.Context(), partner.ID, amount, partner.PayoutMethod, partner.PayoutDetails)
	if errors.Is(err, database.ErrPartnerInsufficientBalance) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "insufficient_balance"})
		return
	}
	if err != nil {
		slog.Error("partner: create payout", "error", err, "partner_id", partner.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("partner payout requested",
		"partner_id", partner.ID, "payout_id", payout.ID, "amount", payout.Amount)
	adminevents.Emit(adminevents.Event{
		Type: adminevents.PartnerPayoutRequest, CustomerID: partner.CustomerID, Source: adminevents.SourceCabinet,
		Payload: map[string]any{"amount": payout.Amount, "currency": "RUB"},
	})

	if h.notify != nil {
		if label := h.adminLabel(r.Context(), partner.ID); label != "" {
			h.notify.PayoutRequested(r.Context(), label, payout.Amount,
				ptrString(payout.Method), ptrString(payout.DetailsSnapshot))
		}
	}

	writeJSON(w, http.StatusOK, partnerPayoutDTO{
		ID:          payout.ID,
		Amount:      payout.Amount,
		Status:      payout.Status,
		Method:      ptrString(payout.Method),
		RequestedAt: payout.RequestedAt.UTC().Format(time.RFC3339),
	})
}

// roundMoney округляет до копеек: клиент может прислать 100.999, а сумма
// операции должна совпадать с тем, что ляжет в NUMERIC(12,2).
//
// Именно math.Round, а не приведение к int64: усечение к нулю превращало бы
// списание в −99.99 вместо −100, и ручная отмена начисления никогда не
// обнуляла бы его до копейки.
func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
