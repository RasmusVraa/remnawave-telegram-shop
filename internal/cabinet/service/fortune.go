package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/bootstrap"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/utils"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FortuneRewardTypesOrder — стабильный порядок секторов на UI (индекс = sector_index в API).
var FortuneRewardTypesOrder = []string{
	"micro", "xp", "discount_3", "days_3", "discount_5", "days_5",
	"days_7", "days_15", "days_30", "days_180",
}

func fortuneSectorIndex(rewardType string) int {
	for i, t := range FortuneRewardTypesOrder {
		if t == rewardType {
			return i
		}
	}
	return 0
}

type fortuneErr struct {
	Code string
	Msg  string
}

func (e *fortuneErr) Error() string { return e.Msg }

// FortuneStatusResponse — GET /cabinet/api/fortune/status.
type FortuneStatusResponse struct {
	Enabled                  bool               `json:"enabled"`
	PanelReady               bool               `json:"panel_ready"`
	CanSpin                  bool               `json:"can_spin"`
	ReasonCode               string             `json:"reason_code,omitempty"`
	SpinsUsedToday           int                `json:"spins_used_today"`
	MaxSpinsPerDay           int                `json:"max_spins_per_day"` // лимит платных спинов за UTC-сутки (для UI)
	DailyFreeEnabled         bool               `json:"daily_free_enabled"`
	DailyFreeAvailable       bool               `json:"daily_free_available"` // FORTUNE_DAILY_FREE_SPIN, сегодня ещё не крутили бесплатно
	MinSubscriptionDays      int                `json:"min_subscription_days"`
	SpinCostDays             int                `json:"spin_cost_days"`
	SubscriptionRemainHrs    float64            `json:"subscription_remain_hours,omitempty"`
	SubscriptionRemainDays   int                `json:"subscription_remain_days"`
	ExpireAt                 *time.Time         `json:"expire_at,omitempty"`
	Sectors                  []FortuneSectorDTO `json:"sectors"`
	WinnerFeed               *FortuneWinnerFeedMeta `json:"winner_feed,omitempty"`
}

// FortuneSectorDTO — описание сектора для отрисовки колеса (веса RNG не сериализуем).
type FortuneSectorDTO struct {
	Index        int    `json:"index"`
	RewardType   string `json:"reward_type"`
	Weight       int    `json:"-"`
	DisplayDays  int    `json:"display_days,omitempty"`
	DisplayPct   int    `json:"display_percent,omitempty"`
}

// FortuneSpinResponse — POST /cabinet/api/fortune/spin.
type FortuneSpinResponse struct {
	SectorIndex   int        `json:"sector_index"`
	RewardType    string     `json:"reward_type"`
	RewardValue   int        `json:"reward_value"`
	CostDays      int        `json:"cost_days"`
	IsFreeSpin    bool       `json:"is_free_spin"`
	IsDailyFree   bool       `json:"is_daily_free,omitempty"`
	NewExpireAt   *time.Time `json:"new_expire_at,omitempty"`
	LoyaltyXPNew  *int64     `json:"loyalty_xp_new,omitempty"`
}

// FortuneService — бизнес-логика колеса фортуны.
type FortuneService struct {
	pool     *pgxpool.Pool
	links    *repository.AccountCustomerLinkRepo
	boot     *bootstrap.CustomerBootstrap
	customer *database.CustomerRepository
	purchase *database.PurchaseRepository
	promo    *database.PromoRepository
	fortRepo *repository.FortuneRepo
	rw       *remnawave.Client

	mu              sync.Mutex
	fortunePromoID  int64
	promoResolveErr error
}

// NewFortuneService — конструктор.
func NewFortuneService(
	pool *pgxpool.Pool,
	links *repository.AccountCustomerLinkRepo,
	boot *bootstrap.CustomerBootstrap,
	customer *database.CustomerRepository,
	purchase *database.PurchaseRepository,
	promo *database.PromoRepository,
	fortRepo *repository.FortuneRepo,
	rw *remnawave.Client,
) *FortuneService {
	return &FortuneService{
		pool:     pool,
		links:    links,
		boot:     boot,
		customer: customer,
		purchase: purchase,
		promo:    promo,
		fortRepo: fortRepo,
		rw:       rw,
	}
}

func (s *FortuneService) resolveFortunePromoID(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fortunePromoID > 0 {
		return s.fortunePromoID, nil
	}
	if s.fortunePromoID < 0 {
		return 0, s.promoResolveErr
	}
	p, err := s.promo.FindByCode(ctx, repository.FortunePromoCode())
	if err != nil {
		s.fortunePromoID = -1
		s.promoResolveErr = fmt.Errorf("fortune: load anchor promo: %w", err)
		return 0, s.promoResolveErr
	}
	if p == nil {
		s.fortunePromoID = -1
		s.promoResolveErr = errors.New("fortune: anchor promo __CABINET_FORTUNE__ missing (run migration 000033)")
		return 0, s.promoResolveErr
	}
	s.fortunePromoID = p.ID
	return s.fortunePromoID, nil
}

func (s *FortuneService) cfg() cabcfg.FortuneWheelConfig {
	return cabcfg.GetFortuneWheel()
}

func weightForType(c cabcfg.FortuneWheelConfig, rt string) int {
	switch rt {
	case "micro":
		return c.WeightMicro
	case "xp":
		return c.WeightXP
	case "discount_3":
		return c.WeightDiscount3
	case "days_3":
		return c.WeightDays3
	case "discount_5":
		return c.WeightDiscount5
	case "days_5":
		return c.WeightDays5
	case "days_7":
		return c.WeightDays7
	case "days_15":
		return c.WeightDays15
	case "days_30":
		return c.WeightDays30
	case "days_180":
		return c.WeightDays180
	default:
		return 0
	}
}

func (s *FortuneService) buildSectors(c cabcfg.FortuneWheelConfig) []FortuneSectorDTO {
	out := make([]FortuneSectorDTO, 0, len(FortuneRewardTypesOrder))
	for i, rt := range FortuneRewardTypesOrder {
		w := weightForType(c, rt)
		dto := FortuneSectorDTO{Index: i, RewardType: rt, Weight: w}
		switch rt {
		case "days_3":
			dto.DisplayDays = c.RewardDays3
		case "days_5":
			dto.DisplayDays = c.RewardDays5
		case "days_7":
			dto.DisplayDays = c.RewardDays7
		case "days_15":
			dto.DisplayDays = c.RewardDays15
		case "days_30":
			dto.DisplayDays = c.RewardDays30
		case "days_180":
			dto.DisplayDays = c.RewardDays180
		case "discount_3":
			dto.DisplayPct = c.RewardDiscount3Percent
		case "discount_5":
			dto.DisplayPct = c.RewardDiscount5Percent
		}
		out = append(out, dto)
	}
	return out
}

func (s *FortuneService) subscriptionRemain(cust *database.Customer) (hours float64, ok bool) {
	if cust == nil || cust.ExpireAt == nil {
		return 0, false
	}
	if !cust.ExpireAt.After(time.Now().UTC()) {
		return 0, false
	}
	return cust.ExpireAt.Sub(time.Now().UTC()).Hours(), true
}

func (s *FortuneService) loadCustomer(ctx context.Context, accountID int64) (*database.Customer, error) {
	link, err := s.links.FindByAccountID(ctx, accountID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if errors.Is(err, repository.ErrNotFound) {
		if s.boot == nil {
			return nil, nil
		}
		link, err = s.boot.EnsureForAccount(ctx, accountID, "")
		if err != nil {
			return nil, err
		}
	}
	if link == nil {
		return nil, nil
	}
	return s.customer.FindById(ctx, link.CustomerID)
}

// Status — GET /cabinet/api/fortune/status.
func (s *FortuneService) Status(ctx context.Context, accountID int64) (*FortuneStatusResponse, error) {
	cfg := s.cfg()
	out := &FortuneStatusResponse{
		Enabled:              cfg.Enabled,
		PanelReady:           s.rw != nil,
		MaxSpinsPerDay:       cfg.MaxSpinsPerDay,
		DailyFreeEnabled:     cfg.DailyFreeSpin,
		MinSubscriptionDays:  cfg.MinSubscriptionDays,
		SpinCostDays:         cfg.SpinCostDays,
		Sectors:              s.buildSectors(cfg),
	}
	if !cfg.Enabled {
		out.ReasonCode = "module_disabled"
		return out, nil
	}
	if s.rw == nil {
		out.ReasonCode = "no_panel"
		return out, nil
	}
	s.appendWinnerFeedMeta(out, cfg)

	cust, err := s.loadCustomer(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if cust == nil {
		out.ReasonCode = "no_customer"
		return out, nil
	}

	hasPaidSub, err := s.purchase.HasPaidSubscription(ctx, cust.ID)
	if err != nil {
		return nil, err
	}
	if !hasPaidSub {
		out.ReasonCode = "no_paid"
		return out, nil
	}

	h, ok := s.subscriptionRemain(cust)
	out.SubscriptionRemainHrs = h
	out.ExpireAt = cust.ExpireAt
	if ok && h > 0 {
		out.SubscriptionRemainDays = int(math.Ceil(h / 24.0))
	}
	minH := float64(cfg.MinSubscriptionDays) * 24
	if !ok || h < minH {
		out.ReasonCode = "insufficient_days"
		return out, nil
	}

	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	n, err := s.fortRepo.CountPaidSpinsBetween(ctx, cust.ID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}
	out.SpinsUsedToday = n

	var dailyAvail bool
	if cfg.DailyFreeSpin {
		dailyUsed, derr := s.fortRepo.HasDailyFreeSpinToday(ctx, cust.ID, dayStart, dayEnd)
		if derr != nil {
			return nil, derr
		}
		dailyAvail = !dailyUsed
	}
	out.DailyFreeAvailable = dailyAvail

	if n >= cfg.MaxSpinsPerDay && !dailyAvail {
		out.ReasonCode = "daily_limit"
		return out, nil
	}

	if _, err := s.resolveFortunePromoID(ctx); err != nil {
		out.ReasonCode = "not_configured"
		return out, nil
	}

	out.CanSpin = true
	out.ReasonCode = ""
	return out, nil
}

func randIntn(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("randIntn: n<=0")
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	u := binary.BigEndian.Uint64(buf[:])
	return int(u % uint64(n)), nil
}

func (s *FortuneService) pickRewardType(cfg cabcfg.FortuneWheelConfig) (string, error) {
	type pair struct {
		t string
		w int
	}
	var pairs []pair
	sum := 0
	for _, rt := range FortuneRewardTypesOrder {
		w := weightForType(cfg, rt)
		if w <= 0 {
			continue
		}
		pairs = append(pairs, pair{t: rt, w: w})
		sum += w
	}
	if sum <= 0 || len(pairs) == 0 {
		return "micro", errors.New("fortune: empty weight table")
	}
	r, err := randIntn(sum)
	if err != nil {
		return "", err
	}
	acc := 0
	for _, p := range pairs {
		acc += p.w
		if r < acc {
			return p.t, nil
		}
	}
	return pairs[len(pairs)-1].t, nil
}

func rewardValueFor(cfg cabcfg.FortuneWheelConfig, rewardType string) (int, error) {
	switch rewardType {
	case "micro":
		lo, hi := cfg.RewardMicroXPMin, cfg.RewardMicroXPMax
		if hi < lo {
			hi = lo
		}
		span := hi - lo + 1
		n, err := randIntn(span)
		if err != nil {
			return lo, nil
		}
		return lo + n, nil
	case "xp":
		return cfg.RewardXPAmount, nil
	case "discount_3":
		return cfg.RewardDiscount3Percent, nil
	case "discount_5":
		return cfg.RewardDiscount5Percent, nil
	case "days_3":
		return cfg.RewardDays3, nil
	case "days_5":
		return cfg.RewardDays5, nil
	case "days_7":
		return cfg.RewardDays7, nil
	case "days_15":
		return cfg.RewardDays15, nil
	case "days_30":
		return cfg.RewardDays30, nil
	case "days_180":
		return cfg.RewardDays180, nil
	default:
		return 0, fmt.Errorf("unknown reward %q", rewardType)
	}
}

// Spin — POST /cabinet/api/fortune/spin.
func (s *FortuneService) Spin(ctx context.Context, accountID int64) (*FortuneSpinResponse, error) {
	cfg := s.cfg()
	if !cfg.Enabled {
		return nil, &fortuneErr{Code: "module_disabled", Msg: "fortune wheel disabled"}
	}
	if s.rw == nil {
		return nil, &fortuneErr{Code: "no_panel", Msg: "remnawave panel not configured"}
	}
	promoID, err := s.resolveFortunePromoID(ctx)
	if err != nil {
		return nil, &fortuneErr{Code: "not_configured", Msg: err.Error()}
	}

	cust, err := s.loadCustomer(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if cust == nil {
		return nil, &fortuneErr{Code: "no_customer", Msg: "no linked customer"}
	}

	hasPaidSub, err := s.purchase.HasPaidSubscription(ctx, cust.ID)
	if err != nil {
		return nil, err
	}
	if !hasPaidSub {
		return nil, &fortuneErr{Code: "no_paid", Msg: "paid subscription required"}
	}

	h, ok := s.subscriptionRemain(cust)
	minH := float64(cfg.MinSubscriptionDays) * 24
	if !ok || h < minH {
		return nil, &fortuneErr{Code: "insufficient_days", Msg: "not enough subscription time left"}
	}

	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	paidToday, err := s.fortRepo.CountPaidSpinsBetween(ctx, cust.ID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}

	var isFree, isDailyFree bool
	if cfg.DailyFreeSpin {
		dailyUsed, derr := s.fortRepo.HasDailyFreeSpinToday(ctx, cust.ID, dayStart, dayEnd)
		if derr != nil {
			return nil, derr
		}
		if !dailyUsed {
			isFree, isDailyFree = true, true
		}
	}
	if !isFree && paidToday >= cfg.MaxSpinsPerDay {
		return nil, &fortuneErr{Code: "daily_limit", Msg: "daily spin limit reached"}
	}

	if !isFree {
		if h < minH+float64(cfg.SpinCostDays)*24 {
			return nil, &fortuneErr{Code: "insufficient_days", Msg: "spin would leave less than minimum subscription buffer"}
		}
	}

	rewardType, err := s.pickRewardType(cfg)
	if err != nil {
		return nil, err
	}
	rewardVal, err := rewardValueFor(cfg, rewardType)
	if err != nil {
		return nil, err
	}

	costDays := 0
	if !isFree {
		costDays = cfg.SpinCostDays
	}

	var lastUser *remnawave.User
	if costDays > 0 {
		u, err := s.rw.ShrinkSubscriptionByDaysPreserveSquads(ctx, cust.ID, cust.TelegramID, costDays)
		if err != nil {
			return nil, fmt.Errorf("fortune: shrink subscription: %w", err)
		}
		lastUser = u
	}

	switch rewardType {
	case "days_3", "days_5", "days_7", "days_15", "days_30", "days_180":
		days := rewardVal
		if days <= 0 {
			break
		}
		u, err := s.rw.ExtendSubscriptionByDaysPreserveSquads(ctx, cust.ID, cust.TelegramID, days)
		if err != nil {
			slog.Error("fortune: extend after shrink failed", "customer_id", utils.MaskHalfInt64(cust.ID), "error", err)
			return nil, fmt.Errorf("fortune: extend subscription: %w", err)
		}
		lastUser = u
	case "xp":
		if err := s.customer.IncrementLoyaltyXP(ctx, cust.ID, int64(rewardVal)); err != nil {
			slog.Error("fortune: loyalty xp", "customer_id", utils.MaskHalfInt64(cust.ID), "error", err)
			return nil, fmt.Errorf("fortune: loyalty xp: %w", err)
		}
	case "micro":
		if err := s.customer.IncrementLoyaltyXP(ctx, cust.ID, int64(rewardVal)); err != nil {
			return nil, fmt.Errorf("fortune: micro xp: %w", err)
		}
	case "discount_3", "discount_5":
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		pd, err := s.promo.GetPendingDiscountByCustomerIDForUpdate(ctx, tx, cust.ID)
		if err != nil {
			return nil, err
		}
		apply := true
		if pd != nil && pd.Percent >= rewardVal {
			apply = false
		}
		if apply {
			if err := s.promo.UpsertPendingDiscount(ctx, tx, cust.ID, promoID, rewardVal, nil, true, database.PendingDiscountUnlimitedPayments); err != nil {
				return nil, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
	}

	var newExp *time.Time
	if lastUser != nil {
		t := lastUser.ExpireAt.UTC()
		newExp = &t
		upd := map[string]interface{}{"expire_at": newExp}
		if lastUser.SubscriptionUrl != "" {
			sl := lastUser.SubscriptionUrl
			upd["subscription_link"] = &sl
		}
		if err := s.customer.UpdateFields(ctx, cust.ID, upd); err != nil {
			slog.Error("fortune: customer expire sync failed", "customer_id", utils.MaskHalfInt64(cust.ID), "error", err)
		}
	} else if costDays == 0 && (rewardType == "xp" || rewardType == "micro") {
		// только XP — подтянуть expire из RW для консистентности (опционально)
		if s.rw != nil && !cust.IsWebOnly {
			if u, err := s.rw.GetUserTrafficInfo(ctx, cust.TelegramID); err == nil && u != nil {
				t := u.ExpireAt.UTC()
				newExp = &t
				_ = s.customer.UpdateFields(ctx, cust.ID, map[string]interface{}{
					"expire_at":          newExp,
					"subscription_link": u.SubscriptionUrl,
				})
			}
		}
	}

	spinLogVal := rewardVal
	if err := s.fortRepo.InsertSpin(ctx, cust.ID, rewardType, spinLogVal, costDays, isFree, isDailyFree); err != nil {
		slog.Error("fortune: insert spin log failed", "customer_id", utils.MaskHalfInt64(cust.ID), "error", err)
	}

	resp := &FortuneSpinResponse{
		SectorIndex:  fortuneSectorIndex(rewardType),
		RewardType:   rewardType,
		RewardValue:  rewardVal,
		CostDays:     costDays,
		IsFreeSpin:   isFree,
		IsDailyFree:  isDailyFree,
		NewExpireAt:  newExp,
	}
	if rewardType == "xp" || rewardType == "micro" {
		if fresh, err := s.customer.FindById(ctx, cust.ID); err == nil && fresh != nil {
			xp := fresh.LoyaltyXP
			resp.LoyaltyXPNew = &xp
		}
	}

	slog.Info("fortune spin",
		"customer_id", utils.MaskHalfInt64(cust.ID),
		"reward_type", rewardType,
		"reward_value", rewardVal,
		"cost_days", costDays,
		"free", isFree,
		"daily_free", isDailyFree,
	)

	return resp, nil
}

// FortuneClientErrorCode — ошибки бизнес-правил колеса (4xx), ok=false если не fortuneErr.
func FortuneClientErrorCode(err error) (code, msg string, ok bool) {
	var fe *fortuneErr
	if errors.As(err, &fe) {
		return fe.Code, fe.Msg, true
	}
	return "", "", false
}
