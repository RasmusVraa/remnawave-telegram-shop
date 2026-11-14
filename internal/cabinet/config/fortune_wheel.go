package config

import (
	"log/slog"
	"strconv"
	"strings"

	botcfg "remnawave-tg-shop-bot/internal/config"
)

// FortuneWheelConfig — настройки колеса фортуны (FORTUNE_* env). Безопасно до InitConfig кабинета.
type FortuneWheelConfig struct {
	Enabled bool

	// MaxSpinsPerDay — максимум платных спинов за UTC-сутки (бесплатный daily не входит в счётчик).
	MaxSpinsPerDay      int
	MinSubscriptionDays int
	DailyFreeSpin       bool // FORTUNE_DAILY_FREE_SPIN — один бесплатный спин в сутки (UTC)
	SpinCostDays        int

	WeightMicro     int
	WeightXP        int
	WeightDiscount3 int
	WeightDays3     int
	WeightDiscount5 int
	WeightDays5     int
	WeightDays7     int
	WeightDays15    int
	WeightDays30    int
	WeightDays180   int
	// WeightFreeSpin — шанс приза «бесплатный спин» (FORTUNE_WEIGHT_FREE_SPIN); 0 — сектор скрыт и не выпадает.
	WeightFreeSpin int

	RewardXPAmount         int
	RewardMicroXPMin       int
	RewardMicroXPMax       int
	RewardDiscount3Percent int
	RewardDiscount5Percent int
	RewardDays3            int
	RewardDays5            int
	RewardDays7            int
	RewardDays15           int
	RewardDays30           int
	RewardDays180          int
	// RewardFreeSpins — сколько бесплатных спинов даёт приз (FORTUNE_REWARD_FREE_SPINS).
	RewardFreeSpins int

	// WinnerTickerEnabled — бегущая строка / блок «кто выиграл» на странице колеса (FORTUNE_WINNER_TICKER_ENABLED).
	WinnerTickerEnabled bool
	// WinnerTickerFakeFill — только синтетическая лента (без реальных спинов из БД); иначе только реальные данные (FORTUNE_WINNER_TICKER_FAKE_FILL).
	WinnerTickerFakeFill bool
}

var fortuneWheel FortuneWheelConfig

func fortuneEffectiveEnv(key string) string {
	return botcfg.EffectiveEnv(key)
}

func fortuneInt(key string, def int) int {
	v := strings.TrimSpace(fortuneEffectiveEnv(key))
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("cabinet fortune: invalid int, using default", "key", key, "value", v, "default", def)
		return def
	}
	if i < 0 {
		slog.Warn("cabinet fortune: negative value clamped to 0", "key", key, "value", i)
		return 0
	}
	return i
}

func fortuneBool(key string, def bool) bool {
	v := strings.TrimSpace(fortuneEffectiveEnv(key))
	if v == "" {
		return def
	}
	return v == "true"
}

// ReloadFortuneWheel перечитывает FORTUNE_* (в т.ч. runtime overrides) без restart.
func ReloadFortuneWheel() {
	initFortuneWheel()
}

// initFortuneWheel вызывается из InitConfig() после conf.enabled=true.
func initFortuneWheel() {
	fortuneWheel = FortuneWheelConfig{}
	fortuneWheel.Enabled = fortuneBool("FORTUNE_ENABLED", false)
	// Стоимость спина в днях читаем всегда — для подписи в UI даже при FORTUNE_ENABLED=false.
	fortuneWheel.SpinCostDays = fortuneInt("FORTUNE_SPIN_COST_DAYS", 1)
	if fortuneWheel.SpinCostDays < 1 {
		fortuneWheel.SpinCostDays = 1
	}
	fortuneWheel.WinnerTickerEnabled = fortuneBool("FORTUNE_WINNER_TICKER_ENABLED", true)
	fortuneWheel.WinnerTickerFakeFill = fortuneBool("FORTUNE_WINNER_TICKER_FAKE_FILL", false)
	if !fortuneWheel.Enabled {
		return
	}
	fortuneWheel.MaxSpinsPerDay = fortuneInt("FORTUNE_MAX_SPINS_PER_DAY", 4)
	if fortuneWheel.MaxSpinsPerDay < 1 {
		fortuneWheel.MaxSpinsPerDay = 4
	}
	fortuneWheel.DailyFreeSpin = fortuneBool("FORTUNE_DAILY_FREE_SPIN", true)
	fortuneWheel.MinSubscriptionDays = fortuneInt("FORTUNE_MIN_SUBSCRIPTION_DAYS", 2)
	if fortuneWheel.MinSubscriptionDays < 1 {
		fortuneWheel.MinSubscriptionDays = 2
	}

	fortuneWheel.WeightMicro = fortuneInt("FORTUNE_WEIGHT_MICRO", 478)
	fortuneWheel.WeightXP = fortuneInt("FORTUNE_WEIGHT_XP", 355)
	fortuneWheel.WeightDiscount3 = fortuneInt("FORTUNE_WEIGHT_DISCOUNT_3", 60)
	fortuneWheel.WeightDays3 = fortuneInt("FORTUNE_WEIGHT_DAYS_3", 25)
	fortuneWheel.WeightDiscount5 = fortuneInt("FORTUNE_WEIGHT_DISCOUNT_5", 40)
	fortuneWheel.WeightDays5 = fortuneInt("FORTUNE_WEIGHT_DAYS_5", 15)
	fortuneWheel.WeightDays7 = fortuneInt("FORTUNE_WEIGHT_DAYS_7", 10)
	fortuneWheel.WeightDays15 = fortuneInt("FORTUNE_WEIGHT_DAYS_15", 5)
	fortuneWheel.WeightDays30 = fortuneInt("FORTUNE_WEIGHT_DAYS_30", 2)
	fortuneWheel.WeightDays180 = fortuneInt("FORTUNE_WEIGHT_DAYS_180", 0)
	fortuneWheel.WeightFreeSpin = fortuneInt("FORTUNE_WEIGHT_FREE_SPIN", 10)

	fortuneWheel.RewardXPAmount = fortuneInt("FORTUNE_REWARD_XP_AMOUNT", 35)
	fortuneWheel.RewardMicroXPMin = fortuneInt("FORTUNE_REWARD_MICRO_XP_MIN", 5)
	fortuneWheel.RewardMicroXPMax = fortuneInt("FORTUNE_REWARD_MICRO_XP_MAX", 15)
	if fortuneWheel.RewardMicroXPMax < fortuneWheel.RewardMicroXPMin {
		fortuneWheel.RewardMicroXPMax = fortuneWheel.RewardMicroXPMin
	}
	fortuneWheel.RewardDiscount3Percent = fortuneInt("FORTUNE_REWARD_DISCOUNT_3_PERCENT", 3)
	fortuneWheel.RewardDiscount5Percent = fortuneInt("FORTUNE_REWARD_DISCOUNT_5_PERCENT", 5)
	fortuneWheel.RewardDays3 = fortuneInt("FORTUNE_REWARD_DAYS_3", 3)
	fortuneWheel.RewardDays5 = fortuneInt("FORTUNE_REWARD_DAYS_5", 5)
	fortuneWheel.RewardDays7 = fortuneInt("FORTUNE_REWARD_DAYS_7", 7)
	fortuneWheel.RewardDays15 = fortuneInt("FORTUNE_REWARD_DAYS_15", 15)
	fortuneWheel.RewardDays30 = fortuneInt("FORTUNE_REWARD_DAYS_30", 30)
	fortuneWheel.RewardDays180 = fortuneInt("FORTUNE_REWARD_DAYS_180", 180)
	fortuneWheel.RewardFreeSpins = fortuneInt("FORTUNE_REWARD_FREE_SPINS", 1)
	if fortuneWheel.RewardFreeSpins < 1 {
		fortuneWheel.RewardFreeSpins = 1
	}

	sumW := fortuneWheel.WeightMicro + fortuneWheel.WeightXP + fortuneWheel.WeightDiscount3 +
		fortuneWheel.WeightDays3 + fortuneWheel.WeightDiscount5 + fortuneWheel.WeightDays5 +
		fortuneWheel.WeightDays7 + fortuneWheel.WeightDays15 + fortuneWheel.WeightDays30 + fortuneWheel.WeightDays180 + fortuneWheel.WeightFreeSpin
	if sumW <= 0 {
		slog.Warn("cabinet fortune: sum of weights is 0, restoring defaults")
		fortuneWheel.WeightMicro = 478
		fortuneWheel.WeightXP = 355
		fortuneWheel.WeightDiscount3 = 60
		fortuneWheel.WeightDays3 = 25
		fortuneWheel.WeightDiscount5 = 40
		fortuneWheel.WeightDays5 = 15
		fortuneWheel.WeightDays7 = 10
		fortuneWheel.WeightDays15 = 5
		fortuneWheel.WeightDays30 = 2
		fortuneWheel.WeightDays180 = 0
		fortuneWheel.WeightFreeSpin = 10
	}

	slog.Info("cabinet fortune wheel config",
		"enabled", true,
		"max_spins_per_day", fortuneWheel.MaxSpinsPerDay,
		"daily_free_spin", fortuneWheel.DailyFreeSpin,
		"min_subscription_days", fortuneWheel.MinSubscriptionDays,
		"spin_cost_days", fortuneWheel.SpinCostDays,
	)
}

// GetFortuneWheel возвращает копию конфигурации колеса (FORTUNE_ENABLED=false → нулевая структура кроме Enabled).
func GetFortuneWheel() FortuneWheelConfig {
	return fortuneWheel
}
