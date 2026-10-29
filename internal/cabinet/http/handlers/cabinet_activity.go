package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// CabinetActivityHandler — GET /cabinet/api/me/referrals и /cabinet/api/me/purchases.
type CabinetActivityHandler struct {
	links     *repository.AccountCustomerLinkRepo
	ids       *repository.IdentityRepo
	customers *database.CustomerRepository
	referrals *database.ReferralRepository
	purchases *database.PurchaseRepository
	publicURL string
}

// NewCabinetActivity — конструктор.
func NewCabinetActivity(
	links *repository.AccountCustomerLinkRepo,
	ids *repository.IdentityRepo,
	customers *database.CustomerRepository,
	referrals *database.ReferralRepository,
	purchases *database.PurchaseRepository,
	publicURL string,
) *CabinetActivityHandler {
	return &CabinetActivityHandler{
		links: links, ids: ids, customers: customers, referrals: referrals, purchases: purchases,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func (h *CabinetActivityHandler) resolveCustomerID(r *http.Request, accountID int64) (int64, bool) {
	if h.links == nil {
		return 0, false
	}
	link, err := h.links.FindByAccountID(r.Context(), accountID)
	if err != nil || link == nil {
		return 0, false
	}
	return link.CustomerID, true
}

type referralStatsDTO struct {
	Total              int `json:"total"`
	Paid               int `json:"paid"`
	Active             int `json:"active"`
	ConversionPct      int `json:"conversion_pct"`
	EarnedDaysTotal    int `json:"earned_days_total"`
	EarnedDaysLastMo   int `json:"earned_days_last_month"`
	ReferralDaysPerPay int `json:"referral_days_per_paid_default"`
}

type refereeRowDTO struct {
	TelegramIDMasked string `json:"telegram_id_masked"`
	// Готовая к показу подпись: «@i***n», «d***y@mail.ru» или маскированный id.
	// Раньше здесь ехали полные username и email, а звёздочки рисовал браузер —
	// то есть настоящие значения лежали в JSON и читались через devtools.
	Name string `json:"name"`
	// Сколько дней принёс этот человек. Список без этой цифры отвечал на
	// «кто у меня есть», но не на «а что мне с них».
	EarnedDays int  `json:"earned_days"`
	Active     bool `json:"active"`
}

// Сколько последних начислений отдаём в ленту. Лента отвечает на вопрос
// «откуда взялись дни», а не заменяет выгрузку: за пределами десятка строк её
// уже не читают, а вес ответа растёт у каждого пользователя.
const referralLedgerLimit = 12

// referralLedgerRowDTO — строка ленты начислений.
type referralLedgerRowDTO struct {
	// Кто принёс начисление — в том же замаскированном виде, что и в списке.
	Actor     string `json:"actor"`
	Days      int    `json:"days"`
	Months    int    `json:"months,omitempty"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// maskReferee — как приглашённый показывается пригласившему.
//
// Порядок тот же, что был на клиенте: username, иначе почта, иначе
// маскированный telegram id. Отличие одно — username теперь тоже маскируется:
// показывать чужой ник целиком незачем, узнаваемости «@i***k» для своего
// списка достаточно.
func maskReferee(username, email *string, telegramID int64) string {
	if username != nil {
		if u := strings.TrimSpace(*username); u != "" {
			// Пробел означает, что это имя, а не @username: собаку не приписываем.
			if strings.Contains(u, " ") {
				return utils.MaskEdges(u)
			}
			return "@" + utils.MaskEdges(u)
		}
	}
	if email != nil {
		if e := strings.TrimSpace(*email); e != "" {
			return utils.MaskEmail(e)
		}
	}
	return utils.MaskHalfInt64(telegramID)
}

type referralsResp struct {
	ReferrerTelegramID int64            `json:"referrer_telegram_id"`
	Stats              referralStatsDTO `json:"stats"`
	Referees           []refereeRowDTO  `json:"referees"`
	// Лента начислений: откуда взялись дни из stats.earned_days_total.
	Ledger              []referralLedgerRowDTO `json:"ledger"`
	BotStartLink        string                 `json:"bot_start_link,omitempty"`
	CabinetRegisterLink string                 `json:"cabinet_register_link,omitempty"`
	ReferralMode        string                 `json:"referral_mode"`
	// Параметры бонусов как в боте (для подробного UI).
	ReferralBonusDaysDefault   int `json:"referral_bonus_days_default"`
	ReferralFirstReferrerDays  int `json:"referral_first_referrer_days,omitempty"`
	ReferralFirstRefereeDays   int `json:"referral_first_referee_days,omitempty"`
	ReferralRepeatReferrerDays int `json:"referral_repeat_referrer_days,omitempty"`
	// Помесячное начисление бонуса пригласившему. Без этого флага страница
	// обещала бы фиксированные дни там, где за годовую оплату начислят кратно
	// больше.
	ReferralScaleByMonths bool `json:"referral_scale_by_months"`
}

// GetReferrals — GET /cabinet/api/me/referrals.
func (h *CabinetActivityHandler) GetReferrals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, ok := h.resolveCustomerID(r, claims.AccountID)
	if !ok || h.customers == nil || h.referrals == nil {
		writeJSON(w, http.StatusOK, referralsResp{
			Referees:                 []refereeRowDTO{},
			Ledger:                   []referralLedgerRowDTO{},
			ReferralMode:             config.ReferralMode(),
			ReferralBonusDaysDefault: config.GetReferralDays(),
		})
		return
	}
	cust, err := h.customers.FindById(r.Context(), cid)
	if err != nil || cust == nil {
		slog.Error("cabinet_activity: find customer", "customer_id", cid, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tg := cust.TelegramID
	// Если к cabinet-аккаунту привязан Telegram identity, для рефералки используем
	// реальный TG id identity (а не synthetic telegram_id web-only customer).
	if h.ids != nil {
		if ids, ierr := h.ids.ListByAccount(r.Context(), claims.AccountID); ierr == nil {
			for _, id := range ids {
				if id.Provider != repository.ProviderTelegram {
					continue
				}
				pid := strings.TrimSpace(id.ProviderUserID)
				if pid == "" {
					continue
				}
				if v, perr := strconv.ParseInt(pid, 10, 64); perr == nil && v > 0 {
					tg = v
					break
				}
			}
		}
	}

	stats, err := h.referrals.GetStats(r.Context(), tg)
	if err != nil {
		slog.Error("cabinet_activity: referral stats", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	summaries, err := h.referrals.FindRefereeSummariesByReferrer(r.Context(), tg)
	if err != nil {
		slog.Error("cabinet_activity: referral list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Дни по каждому приглашённому — одним запросом на весь список, а не
	// запросом на строку: иначе список из полусотни рефералов дал бы полсотни
	// походов в базу на одно открытие страницы.
	daysByReferee, err := h.referrals.EarnedDaysByReferee(r.Context(), tg)
	if err != nil {
		slog.Error("cabinet_activity: referral days by referee", "error", err.Error())
		daysByReferee = nil
	}

	referees := make([]refereeRowDTO, 0, len(summaries))
	for _, s := range summaries {
		referees = append(referees, refereeRowDTO{
			TelegramIDMasked: utils.MaskHalfInt64(s.TelegramID),
			Name:             maskReferee(s.TelegramUsername, s.Email, s.TelegramID),
			EarnedDays:       daysByReferee[s.TelegramID],
			Active:           s.Active,
		})
	}

	// Лента — дополнение к странице, а не её суть: если журнал не ответил,
	// показываем остальное, а не пятисотим весь экран.
	ledger := make([]referralLedgerRowDTO, 0, referralLedgerLimit)
	feed, err := h.referrals.RecentEarnedBonuses(r.Context(), tg, referralLedgerLimit)
	if err != nil {
		slog.Error("cabinet_activity: referral ledger", "error", err.Error())
	} else {
		for _, e := range feed {
			ledger = append(ledger, referralLedgerRowDTO{
				Actor:     maskReferee(e.RefereeUsername, e.RefereeEmail, e.RefereeTelegramID),
				Days:      e.Days,
				Months:    e.Months,
				Kind:      e.Kind,
				CreatedAt: e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	}

	dto := referralStatsDTO{
		Total:              stats.Total,
		Paid:               stats.Paid,
		Active:             stats.Active,
		ConversionPct:      stats.Conversion,
		EarnedDaysTotal:    stats.EarnedTotal,
		EarnedDaysLastMo:   stats.EarnedLastMonth,
		ReferralDaysPerPay: config.GetReferralDays(),
	}

	resp := referralsResp{
		ReferrerTelegramID:       tg,
		Stats:                    dto,
		Referees:                 referees,
		Ledger:                   ledger,
		ReferralMode:             config.ReferralMode(),
		ReferralBonusDaysDefault: config.GetReferralDays(),
	}
	if config.ReferralMode() == "progressive" {
		resp.ReferralFirstReferrerDays = config.ReferralFirstReferrerDays()
		resp.ReferralFirstRefereeDays = config.ReferralFirstRefereeDays()
		resp.ReferralRepeatReferrerDays = config.ReferralRepeatReferrerDays()
		resp.ReferralScaleByMonths = config.ReferralScaleByMonths()
	}

	if u := utils.ReferralStartURL(config.BotURL(), tg); u != "" {
		resp.BotStartLink = u
	}
	if h.publicURL != "" {
		resp.CabinetRegisterLink = h.publicURL + "/cabinet/register?ref=ref_" + strconv.FormatInt(tg, 10)
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

type purchaseRowDTO struct {
	ID           int64   `json:"id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Status       string  `json:"status"`
	InvoiceType  string  `json:"invoice_type"`
	PurchaseKind string  `json:"purchase_kind"`
	Month        int     `json:"month"`
	ExtraHwid    int     `json:"extra_hwid"`
	PaidAt       *string `json:"paid_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type purchasesResp struct {
	Items []purchaseRowDTO `json:"items"`
}

// GetPurchases — GET /cabinet/api/me/purchases?limit=50&offset=0.
func (h *CabinetActivityHandler) GetPurchases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims := middleware.AuthClaims(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, ok := h.resolveCustomerID(r, claims.AccountID)
	if !ok || h.purchases == nil {
		writeJSON(w, http.StatusOK, purchasesResp{Items: []purchaseRowDTO{}})
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	list, err := h.purchases.FindPaidByCustomer(r.Context(), cid, limit, offset)
	if err != nil {
		slog.Error("cabinet_activity: purchases list", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]purchaseRowDTO, 0, len(list))
	for _, p := range list {
		row := purchaseRowDTO{
			ID:           p.ID,
			Amount:       p.Amount,
			Currency:     p.Currency,
			Status:       string(p.Status),
			InvoiceType:  string(p.InvoiceType),
			PurchaseKind: string(p.PurchaseKind),
			Month:        p.Month,
			ExtraHwid:    p.ExtraHwid,
			CreatedAt:    p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if p.PaidAt != nil {
			s := p.PaidAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			row.PaidAt = &s
		}
		items = append(items, row)
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, purchasesResp{Items: items})
}
