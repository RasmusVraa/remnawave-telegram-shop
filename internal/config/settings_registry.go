package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// cabinetDecorThemeIDs — порядок опций в админке (синхрон с internal/cabinet/config.ValidDecorThemeIDs).
// Порядок = админ-селект по цветовым группам (синхрон с cabinet/config.ValidDecorThemeIDs).
var cabinetDecorThemeIDs = []string{
	"off",
	"green",
	"spring",
	"cyber",
	"neon",
	"ocean",
	"new_year",
	"slate",
	"carbon",
	"aurora",
	"nebula",
	"violet",
	"lavender",
	"pink",
	"valentine",
	"wine",
	"sunset",
	"orange",
	"halloween",
	"yellow",
	"summer",
	"black_friday",
}

func cabinetDecorThemeSet() map[string]struct{} {
	m := make(map[string]struct{}, len(cabinetDecorThemeIDs))
	for _, id := range cabinetDecorThemeIDs {
		m[id] = struct{}{}
	}
	return m
}

type SettingType string

const (
	SettingBool   SettingType = "bool"
	SettingInt    SettingType = "int"
	SettingFloat  SettingType = "float"
	SettingText   SettingType = "text"
	SettingURL    SettingType = "url"
	SettingEnum   SettingType = "enum"
	SettingCSVInt SettingType = "csv_int"
	SettingCSV    SettingType = "csv"
)

// SettingField — метаданные одного editable env-ключа (Phase 1 whitelist).
type SettingField struct {
	Key        string
	Group      string
	Type       SettingType
	EnumValues []string
	MinInt     *int
	MaxInt     *int
	Instant    bool // bool toggles — autosave в UI
	Apply      func(value string) error
	Current    func() string
	Source     func() string // "env" | "db" | "default"
}

var remnaTagPattern = regexp.MustCompile(`^[A-Z0-9_]+$`)

// RuntimeSettingsRegistry — Phase 1 whitelist (~50 ключей).
func RuntimeSettingsRegistry() []SettingField {
	return []SettingField{
		// --- loyalty ---
		{
			Key: "LOYALTY_ENABLED", Group: "loyalty", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.loyaltyEnabled = v }),
			Current: func() string { return boolStr(conf.loyaltyEnabled) },
		},
		{
			Key: "LOYALTY_MAX_TOTAL_DISCOUNT_PERCENT", Group: "loyalty", Type: SettingInt,
			MinInt: intPtr(1), MaxInt: intPtr(100),
			Apply: applyIntField(func(v int) error {
				if v < 1 || v > 100 {
					return fmt.Errorf("must be 1–100")
				}
				conf.loyaltyMaxTotalDiscountPercent = v
				return nil
			}),
			Current: func() string { return strconv.Itoa(conf.loyaltyMaxTotalDiscountPercent) },
		},
		{
			Key: "RUB_PER_STAR", Group: "stars", Type: SettingFloat,
			Apply: applyFloatField(func(v float64) error {
				if v < 0 {
					return fmt.Errorf("must be >= 0")
				}
				conf.rubPerStar = v
				return nil
			}),
			Current: func() string {
				if conf.rubPerStar == 0 {
					return "0"
				}
				return strconv.FormatFloat(conf.rubPerStar, 'f', -1, 64)
			},
		},

		// --- payments ---
		//
		// Включение метода без реквизитов было бы молчаливым бездействием:
		// клиент не смог бы создать счёт. Поэтому каждый переключатель
		// проверяет наличие ключей в .env и отказывает с объяснением.
		{
			Key: "YOOKASA_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(YookasaHasCredentials,
				"set YOOKASA_URL, YOOKASA_SHOP_ID, YOOKASA_SECRET_KEY and YOOKASA_EMAIL in .env first",
				func(v bool) { conf.isYookasaEnabled = v }),
			Current: func() string { return boolStr(conf.isYookasaEnabled) },
		},
		{
			Key: "CRYPTO_PAY_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(CryptoPayHasCredentials,
				"set CRYPTO_PAY_URL and CRYPTO_PAY_TOKEN in .env first",
				func(v bool) { conf.isCryptoEnabled = v }),
			Current: func() string { return boolStr(conf.isCryptoEnabled) },
		},
		{
			// Stars оплачиваются через токен самого бота — отдельных реквизитов нет.
			Key: "TELEGRAM_STARS_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.isTelegramStarsEnabled = v }),
			Current: func() string { return boolStr(conf.isTelegramStarsEnabled) },
		},
		{
			Key: "PLATEGA_SBP_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(PlategaHasCredentials, plategaCredentialsHint,
				func(v bool) { conf.isPlategaSBPEnabled = v }),
			Current: func() string { return boolStr(conf.isPlategaSBPEnabled) },
		},
		{
			Key: "PLATEGA_CARDS_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(PlategaHasCredentials, plategaCredentialsHint,
				func(v bool) { conf.isPlategaCardsEnabled = v }),
			Current: func() string { return boolStr(conf.isPlategaCardsEnabled) },
		},
		{
			Key: "PLATEGA_ACQUIRING_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(PlategaHasCredentials, plategaCredentialsHint,
				func(v bool) { conf.isPlategaAcquiringEnabled = v }),
			Current: func() string { return boolStr(conf.isPlategaAcquiringEnabled) },
		},
		{
			Key: "PLATEGA_WORLDWIDE_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(PlategaHasCredentials, plategaCredentialsHint,
				func(v bool) { conf.isPlategaWorldwideEnabled = v }),
			Current: func() string { return boolStr(conf.isPlategaWorldwideEnabled) },
		},
		{
			Key: "PLATEGA_CRYPTO_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(PlategaHasCredentials, plategaCredentialsHint,
				func(v bool) { conf.isPlategaCryptoEnabled = v }),
			Current: func() string { return boolStr(conf.isPlategaCryptoEnabled) },
		},
		{
			Key: "HELEKET_ENABLED", Group: "payments", Type: SettingBool, Instant: true,
			Apply: applyPaymentToggle(HeleketHasCredentials, heleketCredentialsHint,
				func(v bool) { conf.isHeleketEnabled = v }),
			Current: func() string { return boolStr(conf.isHeleketEnabled) },
		},

		// --- moynalog ---
		//
		// Выведены только те настройки, что действительно применяются без
		// рестарта. MOYNALOG_RETRY_CRON сюда не входит: расписание читается
		// один раз при старте (receiptRetryChecker), и переключатель в админке
		// молча ничего бы не менял. Логин и пароль — тоже: это секреты, им
		// место в .env.
		{
			Key: "MOYNALOG_ENABLED", Group: "moynalog", Type: SettingBool, Instant: true,
			Apply: func(value string) error {
				v := strings.TrimSpace(strings.ToLower(value))
				if v != "true" && v != "false" {
					return fmt.Errorf("must be true or false")
				}
				// Без учётных данных клиент не собран, и включение было бы
				// молчаливым бездействием — честнее отказать с объяснением.
				if v == "true" && !MoynalogHasCredentials() {
					return fmt.Errorf("set MOYNALOG_USERNAME and MOYNALOG_PASSWORD in .env first")
				}
				conf.isMoynalogEnabled = v == "true"
				return nil
			},
			Current: func() string { return boolStr(conf.isMoynalogEnabled) },
		},
		{
			Key: "MOYNALOG_RECEIPT_FOR", Group: "moynalog", Type: SettingCSV,
			Apply: func(value string) error {
				parseMoynalogReceiptFor(value, true)
				return nil
			},
			Current: moynalogReceiptForCurrent,
		},
		{
			Key: "MOYNALOG_RETRY_MAX_AGE_HOURS", Group: "moynalog", Type: SettingInt,
			MinInt: intPtr(1),
			Apply: applyIntField(func(v int) error {
				if v < 1 {
					return fmt.Errorf("must be >= 1")
				}
				conf.moynalogRetryMaxAgeHours = v
				return nil
			}),
			Current: func() string { return strconv.Itoa(conf.moynalogRetryMaxAgeHours) },
		},

		// --- payments_notify ---
		{
			Key: "PAYMENTS_NOTIFY_ENABLED", Group: "payments_notify", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.paymentsNotifyEnabled = v }),
			Current: func() string { return boolStr(conf.paymentsNotifyEnabled) },
		},
		{
			// Знак не проверяем: у супергруппы chat id отрицательный
			// (-1001234567890), и запрет на минус делал настройку из админки
			// пригодной только для личных чатов — при том что из .env то же
			// значение принималось.
			Key: "PAYMENTS_NOTIFY_CHAT_ID", Group: "payments_notify", Type: SettingText,
			Apply: applyInt64Field(func(v int64) error {
				conf.paymentsNotifyChatID = v
				return nil
			}),
			Current: func() string { return strconv.FormatInt(conf.paymentsNotifyChatID, 10) },
		},
		{
			Key: "PAYMENTS_NOTIFY_MESSAGE_THREAD_ID", Group: "payments_notify", Type: SettingInt,
			Apply: applyIntField(func(v int) error {
				if v < 0 {
					return fmt.Errorf("must be >= 0")
				}
				conf.paymentsNotifyMessageThreadID = v
				return nil
			}),
			Current: func() string { return strconv.Itoa(conf.paymentsNotifyMessageThreadID) },
		},
		{
			Key: "PAYMENTS_NOTIFY_EVENTS", Group: "payments_notify", Type: SettingCSV,
			Apply: func(value string) error {
				applyPaymentsNotifyEvents(value)
				return nil
			},
			Current: paymentsNotifyEventsCurrent,
		},

		// --- trial ---
		{
			Key: "TRIAL_DAYS", Group: "trial", Type: SettingInt,
			MinInt:  intPtr(1),
			Apply:   applyIntField(func(v int) error { conf.trialDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.trialDays) },
		},
		{
			Key: "TRIAL_TRAFFIC_LIMIT", Group: "trial", Type: SettingInt,
			MinInt:  intPtr(1),
			Apply:   applyIntField(func(v int) error { conf.trialTrafficLimit = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.trialTrafficLimit) },
		},
		{
			Key: "TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY", Group: "trial", Type: SettingEnum,
			EnumValues: []string{"day", "week", "month", "month_rolling", "never"},
			Apply:      applyTrafficResetStrategy(func(v string) { conf.trialTrafficLimitResetStrategy = v }),
			Current:    func() string { return conf.trialTrafficLimitResetStrategy },
		},
		{
			Key: "TRIAL_ADD_TO_PAID", Group: "trial", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.trialAddsToPaid = v }),
			Current: func() string { return boolStr(conf.trialAddsToPaid) },
		},
		{
			Key: "TRAFFIC_LIMIT_RESET_STRATEGY", Group: "trial", Type: SettingEnum,
			EnumValues: []string{"day", "week", "month", "month_rolling", "never"},
			Apply:      applyTrafficResetStrategy(func(v string) { conf.trafficLimitResetStrategy = v }),
			Current:    func() string { return conf.trafficLimitResetStrategy },
		},

		// --- tags ---
		{
			Key: "REMNAWAVE_TAG", Group: "tags", Type: SettingText,
			Apply:   applyRemnaTag(func(v string) { conf.remnawaveTag = v }),
			Current: func() string { return conf.remnawaveTag },
		},
		{
			Key: "TRIAL_REMNAWAVE_TAG", Group: "tags", Type: SettingText,
			Apply:   applyRemnaTagOptional(func(v string) { conf.trialRemnawaveTag = v }),
			Current: func() string { return conf.trialRemnawaveTag },
		},

		// --- hwid ---
		{
			Key: "HWID_EXTRA_DEVICES_ENABLED", Group: "hwid", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.hwidExtraDevicesEnabled = v }),
			Current: func() string { return boolStr(conf.hwidExtraDevicesEnabled) },
		},
		{
			Key: "HWID_ADD_PRICE", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.hwidAddPrice = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.hwidAddPrice) },
		},
		{
			Key: "HWID_ADD_STARS_PRICE", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.hwidAddStarsPrice = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.hwidAddStarsPrice) },
		},
		{
			Key: "HWID_MAX_DEVICE", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(1),
			Apply:   applyIntField(func(v int) error { conf.hwidMaxDevices = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.hwidMaxDevices) },
		},
		{
			Key: "TRIAL_HWID_LIMIT", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.trialHwidLimit = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.trialHwidLimit) },
		},
		{
			Key: "PAID_HWID_LIMIT", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.paidHwidLimit = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.paidHwidLimit) },
		},
		{
			Key: "HWID_FALLBACK_DEVICE_LIMIT", Group: "hwid", Type: SettingInt,
			MinInt:  intPtr(1),
			Apply:   applyIntField(func(v int) error { conf.hwidFallbackDeviceLimit = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.hwidFallbackDeviceLimit) },
		},

		// --- referral (progressive only; REFERRAL_MODE / REFERRAL_DAYS — только .env) ---
		{
			Key: "REFERRAL_FIRST_REFERRER_DAYS", Group: "referral", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.referralFirstReferrerDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.referralFirstReferrerDays) },
		},
		{
			Key: "REFERRAL_FIRST_REFEREE_DAYS", Group: "referral", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.referralFirstRefereeDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.referralFirstRefereeDays) },
		},
		{
			Key: "REFERRAL_REPEAT_REFERRER_DAYS", Group: "referral", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.referralRepeatReferrerDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.referralRepeatReferrerDays) },
		},
		{
			Key: "REFERRAL_SCALE_BY_MONTHS", Group: "referral", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.referralScaleByMonths = v }),
			Current: func() string { return boolStr(conf.referralScaleByMonths) },
		},

		// --- partner ---
		//
		// Проценты — значения по умолчанию: индивидуальные условия партнёра
		// хранятся в partner.first_percent / renewal_percent и перекрывают их.
		// Правка процентов не пересчитывает прошлые начисления: в
		// partner_earning записан процент, действовавший в момент платежа.
		{
			Key: "PARTNER_PROGRAM_ENABLED", Group: "partner", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.partnerProgramEnabled = v }),
			Current: func() string { return boolStr(conf.partnerProgramEnabled) },
		},
		{
			Key: "PARTNER_APPLICATIONS_ENABLED", Group: "partner", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.partnerApplicationsEnabled = v }),
			Current: func() string { return boolStr(conf.partnerApplicationsEnabled) },
		},
		{
			Key: "PARTNER_AUTO_APPROVE", Group: "partner", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.partnerAutoApprove = v }),
			Current: func() string { return boolStr(conf.partnerAutoApprove) },
		},
		{
			Key: "PARTNER_FIRST_PERCENT", Group: "partner", Type: SettingFloat,
			Apply: applyFloatField(func(v float64) error {
				if v < 0 || v > 100 {
					return fmt.Errorf("must be between 0 and 100")
				}
				conf.partnerFirstPercent = v
				return nil
			}),
			Current: func() string { return floatStr(conf.partnerFirstPercent) },
		},
		{
			Key: "PARTNER_RENEWAL_PERCENT", Group: "partner", Type: SettingFloat,
			Apply: applyFloatField(func(v float64) error {
				if v < 0 || v > 100 {
					return fmt.Errorf("must be between 0 and 100")
				}
				conf.partnerRenewalPercent = v
				return nil
			}),
			Current: func() string { return floatStr(conf.partnerRenewalPercent) },
		},
		{
			Key: "PARTNER_HOLD_DAYS", Group: "partner", Type: SettingInt,
			MinInt: intPtr(0), MaxInt: intPtr(365),
			Apply:   applyIntField(func(v int) error { conf.partnerHoldDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.partnerHoldDays) },
		},
		{
			Key: "PARTNER_MIN_PAYOUT", Group: "partner", Type: SettingFloat,
			Apply: applyFloatField(func(v float64) error {
				if v < 0 {
					return fmt.Errorf("must be >= 0")
				}
				conf.partnerMinPayout = v
				return nil
			}),
			Current: func() string { return floatStr(conf.partnerMinPayout) },
		},
		{
			Key: "PARTNER_PAYOUT_COOLDOWN_DAYS", Group: "partner", Type: SettingInt,
			MinInt: intPtr(0), MaxInt: intPtr(365),
			Apply:   applyIntField(func(v int) error { conf.partnerPayoutCooldownDays = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.partnerPayoutCooldownDays) },
		},
		{
			Key: "PARTNER_MAX_LINKS", Group: "partner", Type: SettingInt,
			MinInt: intPtr(1), MaxInt: intPtr(100),
			Apply:   applyIntField(func(v int) error { conf.partnerMaxLinks = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.partnerMaxLinks) },
		},
		{
			Key: "PARTNER_COUNT_EXTRA_HWID", Group: "partner", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.partnerCountExtraHwid = v }),
			Current: func() string { return boolStr(conf.partnerCountExtraHwid) },
		},
		{
			Key: "PARTNER_NOTIFY_ENABLED", Group: "partner", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.partnerNotifyEnabled = v }),
			Current: func() string { return boolStr(conf.partnerNotifyEnabled) },
		},
		{
			// Отрицательные значения допустимы и обязательны: у супергруппы
			// chat id вида -1001234567890. Ноль — «в личку админу».
			Key: "PARTNER_NOTIFY_CHAT_ID", Group: "partner", Type: SettingText,
			Apply:   applyInt64Field(func(v int64) error { conf.partnerNotifyChatID = v; return nil }),
			Current: func() string { return strconv.FormatInt(conf.partnerNotifyChatID, 10) },
		},
		{
			Key: "PARTNER_NOTIFY_MESSAGE_THREAD_ID", Group: "partner", Type: SettingInt,
			MinInt: intPtr(0),
			Apply: applyIntField(func(v int) error {
				conf.partnerNotifyThreadID = v
				return nil
			}),
			Current: func() string { return strconv.Itoa(conf.partnerNotifyThreadID) },
		},

		// --- access ---
		{
			Key: "FORWARD_USER_MESSAGES_TO_ADMIN", Group: "access", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.forwardUserMessagesToAdmin = v }),
			Current: func() string { return boolStr(conf.forwardUserMessagesToAdmin) },
		},
		{
			Key: "SUSPICIOUS_USER_FILTER_ENABLED", Group: "access", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.suspiciousUserFilterEnabled = v }),
			Current: func() string { return boolStr(conf.suspiciousUserFilterEnabled) },
		},
		{
			Key: "BLOCKED_TELEGRAM_IDS", Group: "access", Type: SettingCSVInt,
			Apply: func(value string) error {
				m, err := parseTelegramIDList(value)
				if err != nil {
					return err
				}
				conf.blockedTelegramIds = m
				return nil
			},
			Current: func() string { return formatTelegramIDList(conf.blockedTelegramIds) },
		},
		{
			Key: "WHITELISTED_TELEGRAM_IDS", Group: "access", Type: SettingCSVInt,
			Apply: func(value string) error {
				m, err := parseTelegramIDList(value)
				if err != nil {
					return err
				}
				conf.whitelistedTelegramIds = m
				return nil
			},
			Current: func() string { return formatTelegramIDList(conf.whitelistedTelegramIds) },
		},

		// --- cabinet (оформление SPA) ---
		{
			Key: "CABINET_LIGHT_THEME_ENABLED", Group: "cabinet", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_LIGHT_THEME_ENABLED"),
			Current: cabinetLightThemeCurrent(),
		},
		{
			Key: "CABINET_DECOR_THEME", Group: "cabinet", Type: SettingEnum, Instant: true,
			EnumValues: cabinetDecorThemeIDs,
			Apply:      applyCabinetDecorTheme(),
			Current:    cabinetDecorThemeCurrent(),
		},
		{
			Key: "CABINET_DECOR_AUTO_ENABLED", Group: "cabinet", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_DECOR_AUTO_ENABLED"),
			Current: cabinetBoolCurrent("CABINET_DECOR_AUTO_ENABLED", false),
		},
		{
			Key: "CABINET_DECOR_SCHEDULE", Group: "cabinet", Type: SettingText,
			Apply:   applyCabinetDecorSchedule(),
			Current: cabinetDecorScheduleCurrent(),
		},
		{
			Key: "CABINET_SUBSCRIPTION_SHOW_LOYALTY", Group: "cabinet", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_SUBSCRIPTION_SHOW_LOYALTY"),
			Current: cabinetBoolCurrent("CABINET_SUBSCRIPTION_SHOW_LOYALTY", false),
		},

		// --- landing (тексты и секции публичной витрины; пустой текст = перевод) ---
		{
			Key: "LANDING_HERO_TITLE", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_HERO_TITLE", 160),
			Current: fortuneCurrent("LANDING_HERO_TITLE"),
		},
		{
			Key: "LANDING_HERO_SUBTITLE", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_HERO_SUBTITLE", 280),
			Current: fortuneCurrent("LANDING_HERO_SUBTITLE"),
		},
		{
			Key: "LANDING_NOTE", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_NOTE", 200),
			Current: fortuneCurrent("LANDING_NOTE"),
		},
		{
			Key: "LANDING_STAT_TRAFFIC_VALUE", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_STAT_TRAFFIC_VALUE", 40),
			Current: fortuneCurrent("LANDING_STAT_TRAFFIC_VALUE"),
		},
		{
			Key: "LANDING_STAT_TRAFFIC_LABEL", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_STAT_TRAFFIC_LABEL", 60),
			Current: fortuneCurrent("LANDING_STAT_TRAFFIC_LABEL"),
		},
		{
			Key: "LANDING_STAT_DEVICES_VALUE", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_STAT_DEVICES_VALUE", 40),
			Current: fortuneCurrent("LANDING_STAT_DEVICES_VALUE"),
		},
		{
			Key: "LANDING_STAT_DEVICES_LABEL", Group: "landing", Type: SettingText, Instant: true,
			Apply:   applyLandingText("LANDING_STAT_DEVICES_LABEL", 60),
			Current: fortuneCurrent("LANDING_STAT_DEVICES_LABEL"),
		},
		{
			Key: "LANDING_SHOW_TARIFFS", Group: "landing", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("LANDING_SHOW_TARIFFS"),
			Current: cabinetBoolCurrent("LANDING_SHOW_TARIFFS", true),
		},
		{
			Key: "LANDING_SHOW_STEPS", Group: "landing", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("LANDING_SHOW_STEPS"),
			Current: cabinetBoolCurrent("LANDING_SHOW_STEPS", true),
		},
		{
			Key: "LANDING_SHOW_FEATURES", Group: "landing", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("LANDING_SHOW_FEATURES"),
			Current: cabinetBoolCurrent("LANDING_SHOW_FEATURES", true),
		},
		{
			Key: "LANDING_SHOW_FAQ", Group: "landing", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("LANDING_SHOW_FAQ"),
			Current: cabinetBoolCurrent("LANDING_SHOW_FAQ", true),
		},

		// --- status (публичная страница /status) ---
		{
			Key: "STATUS_PROBES_ENABLED", Group: "status", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("STATUS_PROBES_ENABLED"),
			Current: cabinetBoolCurrent("STATUS_PROBES_ENABLED", true),
		},
		{
			Key: "STATUS_WORLD_PROBES", Group: "status", Type: SettingInt, Instant: true,
			MinInt: intPtr(1), MaxInt: intPtr(10),
			Apply:   applyBoundedInt("STATUS_WORLD_PROBES", 1, 10),
			Current: cabinetIntCurrent("STATUS_WORLD_PROBES", 3),
		},
		{
			Key: "STATUS_RUSSIA_PROBES", Group: "status", Type: SettingInt, Instant: true,
			MinInt: intPtr(1), MaxInt: intPtr(30),
			Apply:   applyBoundedInt("STATUS_RUSSIA_PROBES", 1, 30),
			Current: cabinetIntCurrent("STATUS_RUSSIA_PROBES", 20),
		},
		{
			Key: "STATUS_PROBE_INTERVAL_MIN", Group: "status", Type: SettingInt, Instant: true,
			MinInt: intPtr(5), MaxInt: intPtr(180),
			Apply:   applyBoundedInt("STATUS_PROBE_INTERVAL_MIN", 5, 180),
			Current: cabinetIntCurrent("STATUS_PROBE_INTERVAL_MIN", 20),
		},
		{
			Key: "STATUS_SHOW_MAP", Group: "status", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("STATUS_SHOW_MAP"),
			Current: cabinetBoolCurrent("STATUS_SHOW_MAP", true),
		},
		{
			Key: "STATUS_PAGE_TITLE", Group: "status", Type: SettingText, Instant: true,
			Apply:   applyLandingText("STATUS_PAGE_TITLE", 120),
			Current: fortuneCurrent("STATUS_PAGE_TITLE"),
		},
		{
			Key: "STATUS_PAGE_LEAD", Group: "status", Type: SettingText, Instant: true,
			Apply:   applyLandingText("STATUS_PAGE_LEAD", 400),
			Current: fortuneCurrent("STATUS_PAGE_LEAD"),
		},

		// --- cabinet_connect (подключение и кнопки в Telegram) ---
		{
			Key: "CABINET_DEEPLINK_HAPP_ENCRYPT", Group: "cabinet_connect", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_DEEPLINK_HAPP_ENCRYPT"),
			Current: cabinetBoolCurrent("CABINET_DEEPLINK_HAPP_ENCRYPT", false),
		},
		{
			Key: "CABINET_DEEPLINK_HAPP_CRYPT_VERSION", Group: "cabinet_connect", Type: SettingEnum, Instant: true,
			EnumValues: []string{"crypt5", "crypt4"},
			Apply:      applyCabinetHappCryptVersion(),
			Current:    cabinetHappCryptVersionCurrent(),
		},
		{
			Key: "CABINET_DEEPLINK_INCY_ENCRYPT", Group: "cabinet_connect", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_DEEPLINK_INCY_ENCRYPT"),
			Current: cabinetBoolCurrent("CABINET_DEEPLINK_INCY_ENCRYPT", false),
		},
		{
			Key: "CABINET_TELEGRAM_SHOW_CHANNEL_BUTTON", Group: "cabinet_connect", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_TELEGRAM_SHOW_CHANNEL_BUTTON"),
			Current: cabinetBoolCurrent("CABINET_TELEGRAM_SHOW_CHANNEL_BUTTON", false),
		},
		{
			Key: "CABINET_TELEGRAM_SHOW_FEEDBACK_BUTTON", Group: "cabinet_connect", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool("CABINET_TELEGRAM_SHOW_FEEDBACK_BUTTON"),
			Current: cabinetBoolCurrent("CABINET_TELEGRAM_SHOW_FEEDBACK_BUTTON", false),
		},

		// --- tariffs (витрина кабинета, режим sales tariffs) ---
		{
			Key: "CABINET_TARIFF_PRICE_DISPLAY", Group: "tariffs", Type: SettingEnum, Instant: true,
			EnumValues: []string{"monthly", "marketing"},
			Apply:      applyCabinetTariffPriceDisplay(),
			Current:    cabinetTariffPriceDisplayCurrent(),
		},
		{
			Key: "CABINET_TARIFF_SAVINGS_BADGE", Group: "tariffs", Type: SettingEnum, Instant: true,
			EnumValues: []string{"none", "corner", "old_price"},
			Apply:      applyCabinetTariffSavingsBadge(),
			Current:    cabinetTariffSavingsBadgeCurrent(),
		},

		// --- lifecycle (без cron / master toggle) ---
		{
			Key: "LIFECYCLE_NO_CONNECT_PAID_ENABLED", Group: "lifecycle", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.lifecycleNoConnectPaidEnabled = v }),
			Current: func() string { return boolStr(conf.lifecycleNoConnectPaidEnabled) },
		},
		{
			Key: "LIFECYCLE_NO_CONNECT_TRIAL_ENABLED", Group: "lifecycle", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.lifecycleNoConnectTrialEnabled = v }),
			Current: func() string { return boolStr(conf.lifecycleNoConnectTrialEnabled) },
		},
		{
			Key: "LIFECYCLE_NO_CONNECT_DELAY_HOURS", Group: "lifecycle", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.lifecycleNoConnectDelayHours = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.lifecycleNoConnectDelayHours) },
		},
		{
			Key: "LIFECYCLE_NO_CONNECT_MAX_AGE_HOURS", Group: "lifecycle", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.lifecycleNoConnectMaxAgeHours = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.lifecycleNoConnectMaxAgeHours) },
		},
		{
			Key: "LIFECYCLE_WINBACK_ENABLED", Group: "lifecycle", Type: SettingBool, Instant: true,
			Apply:   applyBoolField(func(v bool) { conf.lifecycleWinbackEnabled = v }),
			Current: func() string { return boolStr(conf.lifecycleWinbackEnabled) },
		},
		{
			Key: "LIFECYCLE_WINBACK_DAYS_AFTER_EXPIRY", Group: "lifecycle", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.lifecycleWinbackDaysAfterExpiry = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.lifecycleWinbackDaysAfterExpiry) },
		},
		{
			Key: "LIFECYCLE_WINBACK_DISCOUNT_PERCENT", Group: "lifecycle", Type: SettingInt,
			MinInt: intPtr(0), MaxInt: intPtr(100),
			Apply: applyIntField(func(v int) error {
				if v < 0 || v > 100 {
					return fmt.Errorf("must be 0–100")
				}
				conf.lifecycleWinbackDiscountPercent = v
				return nil
			}),
			Current: func() string { return strconv.Itoa(conf.lifecycleWinbackDiscountPercent) },
		},
		{
			Key: "LIFECYCLE_WINBACK_DISCOUNT_TTL_HOURS", Group: "lifecycle", Type: SettingInt,
			MinInt:  intPtr(0),
			Apply:   applyIntField(func(v int) error { conf.lifecycleWinbackDiscountTTLHours = v; return nil }),
			Current: func() string { return strconv.Itoa(conf.lifecycleWinbackDiscountTTLHours) },
		},
		{
			Key: "LIFECYCLE_VIDEO_GUIDE_URL", Group: "lifecycle", Type: SettingURL,
			Apply:   applyStringField(func(v string) { conf.lifecycleVideoGuideURL = v }),
			Current: func() string { return conf.lifecycleVideoGuideURL },
		},
		{
			Key: "LIFECYCLE_SUPPORT_CONTACT", Group: "lifecycle", Type: SettingText,
			Apply:   applyStringField(func(v string) { conf.lifecycleSupportContact = v }),
			Current: func() string { return conf.lifecycleSupportContact },
		},

		// --- links ---
		{Key: "CHANNEL_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.channelURL = v }), Current: func() string { return conf.channelURL }},
		{Key: "FEEDBACK_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.feedbackURL = v }), Current: func() string { return conf.feedbackURL }},
		{Key: "TOS_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.tosURL = v }), Current: func() string { return conf.tosURL }},
		{Key: "SERVER_SELECTION_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.serverSelectionURL = v }), Current: func() string { return conf.serverSelectionURL }},
		{Key: "PUBLIC_OFFER_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.publicOfferURL = v }), Current: func() string { return conf.publicOfferURL }},
		{Key: "PRIVACY_POLICY_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.privacyPolicyURL = v }), Current: func() string { return conf.privacyPolicyURL }},
		{Key: "TERMS_OF_SERVICE_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.termsOfServiceURL = v }), Current: func() string { return conf.termsOfServiceURL }},
		{Key: "VIDEO_GUIDE_URL", Group: "links", Type: SettingURL, Apply: applyStringField(func(v string) { conf.videoGuideURL = v }), Current: func() string { return conf.videoGuideURL }},

		// --- fortune ---
		{Key: "FORTUNE_ENABLED", Group: "fortune", Type: SettingBool, Instant: true, Apply: applyFortuneBool("FORTUNE_ENABLED"), Current: fortuneCurrent("FORTUNE_ENABLED")},
		{Key: "FORTUNE_DAILY_FREE_SPIN", Group: "fortune", Type: SettingBool, Instant: true, Apply: applyFortuneBool("FORTUNE_DAILY_FREE_SPIN"), Current: fortuneCurrent("FORTUNE_DAILY_FREE_SPIN")},
		{Key: "FORTUNE_WINNER_TICKER_ENABLED", Group: "fortune", Type: SettingBool, Instant: true, Apply: applyFortuneBool("FORTUNE_WINNER_TICKER_ENABLED"), Current: fortuneCurrent("FORTUNE_WINNER_TICKER_ENABLED")},
		{Key: "FORTUNE_WINNER_TICKER_FAKE_FILL", Group: "fortune", Type: SettingBool, Instant: true, Apply: applyFortuneBool("FORTUNE_WINNER_TICKER_FAKE_FILL"), Current: fortuneCurrent("FORTUNE_WINNER_TICKER_FAKE_FILL")},
		{Key: "FORTUNE_MAX_SPINS_PER_DAY", Group: "fortune", Type: SettingInt, MinInt: intPtr(1), Apply: applyFortuneInt("FORTUNE_MAX_SPINS_PER_DAY", 1), Current: fortuneCurrent("FORTUNE_MAX_SPINS_PER_DAY")},
		{Key: "FORTUNE_MIN_SUBSCRIPTION_DAYS", Group: "fortune", Type: SettingInt, MinInt: intPtr(1), Apply: applyFortuneInt("FORTUNE_MIN_SUBSCRIPTION_DAYS", 1), Current: fortuneCurrent("FORTUNE_MIN_SUBSCRIPTION_DAYS")},
		{Key: "FORTUNE_SPIN_COST_DAYS", Group: "fortune", Type: SettingInt, MinInt: intPtr(1), Apply: applyFortuneInt("FORTUNE_SPIN_COST_DAYS", 1), Current: fortuneCurrent("FORTUNE_SPIN_COST_DAYS")},
		{Key: "FORTUNE_WEIGHT_MICRO", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_MICRO", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_MICRO")},
		{Key: "FORTUNE_WEIGHT_XP", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_XP", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_XP")},
		{Key: "FORTUNE_WEIGHT_DISCOUNT_3", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DISCOUNT_3", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DISCOUNT_3")},
		{Key: "FORTUNE_WEIGHT_DAYS_3", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_3", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_3")},
		{Key: "FORTUNE_WEIGHT_DISCOUNT_5", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DISCOUNT_5", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DISCOUNT_5")},
		{Key: "FORTUNE_WEIGHT_DAYS_5", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_5", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_5")},
		{Key: "FORTUNE_WEIGHT_DAYS_7", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_7", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_7")},
		{Key: "FORTUNE_WEIGHT_DAYS_15", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_15", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_15")},
		{Key: "FORTUNE_WEIGHT_DAYS_30", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_30", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_30")},
		{Key: "FORTUNE_WEIGHT_DAYS_180", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_WEIGHT_DAYS_180", 0), Current: fortuneCurrent("FORTUNE_WEIGHT_DAYS_180")},
		{Key: "FORTUNE_REWARD_XP_AMOUNT", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_XP_AMOUNT", 0), Current: fortuneCurrent("FORTUNE_REWARD_XP_AMOUNT")},
		{Key: "FORTUNE_REWARD_MICRO_XP_MIN", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_MICRO_XP_MIN", 0), Current: fortuneCurrent("FORTUNE_REWARD_MICRO_XP_MIN")},
		{Key: "FORTUNE_REWARD_MICRO_XP_MAX", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_MICRO_XP_MAX", 0), Current: fortuneCurrent("FORTUNE_REWARD_MICRO_XP_MAX")},
		{Key: "FORTUNE_REWARD_DISCOUNT_3_PERCENT", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DISCOUNT_3_PERCENT", 0), Current: fortuneCurrent("FORTUNE_REWARD_DISCOUNT_3_PERCENT")},
		{Key: "FORTUNE_REWARD_DISCOUNT_5_PERCENT", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DISCOUNT_5_PERCENT", 0), Current: fortuneCurrent("FORTUNE_REWARD_DISCOUNT_5_PERCENT")},
		{Key: "FORTUNE_REWARD_DAYS_3", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_3", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_3")},
		{Key: "FORTUNE_REWARD_DAYS_5", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_5", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_5")},
		{Key: "FORTUNE_REWARD_DAYS_7", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_7", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_7")},
		{Key: "FORTUNE_REWARD_DAYS_15", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_15", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_15")},
		{Key: "FORTUNE_REWARD_DAYS_30", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_30", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_30")},
		{Key: "FORTUNE_REWARD_DAYS_180", Group: "fortune", Type: SettingInt, MinInt: intPtr(0), Apply: applyFortuneInt("FORTUNE_REWARD_DAYS_180", 0), Current: fortuneCurrent("FORTUNE_REWARD_DAYS_180")},
	}
}

func runtimeSettingsByKey() map[string]SettingField {
	out := make(map[string]SettingField, len(RuntimeSettingsRegistry()))
	for _, f := range RuntimeSettingsRegistry() {
		out[f.Key] = f
	}
	return out
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intPtr(v int) *int { return &v }

// floatStr печатает число без хвостовых нулей: в поле админки должно стоять
// «40», а не «40.00000», иначе каждое сохранение выглядит как правка.
func floatStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

const plategaCredentialsHint = "set PLATEGA_MERCHANT_ID and PLATEGA_SECRET in .env first"

const heleketCredentialsHint = "set HELEKET_MERCHANT_ID and HELEKET_API_KEY in .env first"

// applyPaymentToggle — переключатель способа оплаты, который нельзя включить
// без реквизитов. Выключить можно всегда: если ключи из .env убрали, метод
// должен оставаться отключаемым.
func applyPaymentToggle(hasCredentials func() bool, hint string, set func(bool)) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "true" && v != "false" {
			return fmt.Errorf("must be true or false")
		}
		if v == "true" && !hasCredentials() {
			return fmt.Errorf("%s", hint)
		}
		set(v == "true")
		return nil
	}
}

func applyBoolField(set func(bool)) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "true" && v != "false" {
			return fmt.Errorf("must be true or false")
		}
		set(v == "true")
		return nil
	}
}

func applyIntField(set func(int) error) func(string) error {
	return func(value string) error {
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid integer")
		}
		return set(v)
	}
}

func applyInt64Field(set func(int64) error) func(string) error {
	return func(value string) error {
		s := strings.TrimSpace(value)
		if s == "" {
			return set(0)
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer")
		}
		return set(v)
	}
}

func applyFloatField(set func(float64) error) func(string) error {
	return func(value string) error {
		v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return fmt.Errorf("invalid number")
		}
		return set(v)
	}
}

func applyStringField(set func(string)) func(string) error {
	return func(value string) error {
		set(strings.TrimSpace(value))
		return nil
	}
}

func applyEnumField(allowed []string, set func(string)) func(string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[strings.ToLower(a)] = struct{}{}
	}
	return func(value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		if _, ok := allowedSet[v]; !ok {
			return fmt.Errorf("invalid value")
		}
		set(v)
		return nil
	}
}

func applyTrafficResetStrategy(set func(string)) func(string) error {
	return applyEnumField([]string{"day", "week", "month", "month_rolling", "never"}, set)
}

func applyRemnaTag(set func(string)) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(value)
		if v == "" {
			return fmt.Errorf("required")
		}
		if !remnaTagPattern.MatchString(v) {
			return fmt.Errorf("format: ^[A-Z0-9_]+$")
		}
		set(v)
		return nil
	}
}

func applyRemnaTagOptional(set func(string)) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(value)
		if v != "" && !remnaTagPattern.MatchString(v) {
			return fmt.Errorf("format: ^[A-Z0-9_]+$")
		}
		set(v)
		return nil
	}
}

func applyPaymentsNotifyEvents(value string) {
	conf.paymentsNotifySendPaid = false
	conf.paymentsNotifySendCancel = false
	s := strings.TrimSpace(value)
	if s == "" {
		conf.paymentsNotifySendPaid = true
		conf.paymentsNotifySendCancel = true
		return
	}
	for _, p := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "paid":
			conf.paymentsNotifySendPaid = true
		case "cancel":
			conf.paymentsNotifySendCancel = true
		}
	}
}

func paymentsNotifyEventsCurrent() string {
	var parts []string
	if conf.paymentsNotifySendPaid {
		parts = append(parts, "paid")
	}
	if conf.paymentsNotifySendCancel {
		parts = append(parts, "cancel")
	}
	return strings.Join(parts, ",")
}

// moynalogReceiptForCurrent — текущий MOYNALOG_RECEIPT_FOR в каноничном виде.
// Токены совпадают с теми, что понимает parseMoynalogReceiptFor.
func moynalogReceiptForCurrent() string {
	var parts []string
	if conf.moynalogReceiptYookasa {
		parts = append(parts, "yookassa")
	}
	if conf.moynalogReceiptPlatega {
		parts = append(parts, "platega")
	}
	if conf.moynalogReceiptCrypto {
		parts = append(parts, "crypto")
	}
	return strings.Join(parts, ",")
}

func parseTelegramIDList(value string) (map[int64]bool, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return map[int64]bool{}, nil
	}
	out := make(map[int64]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid telegram id: %s", part)
		}
		out[id] = true
	}
	return out, nil
}

func formatTelegramIDList(m map[int64]bool) string {
	if len(m) == 0 {
		return ""
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	return strings.Join(ids, ",")
}

func applyFortuneBool(key string) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "true" && v != "false" {
			return fmt.Errorf("must be true or false")
		}
		setRuntimeOverride(key, v)
		return nil
	}
}

func applyBoundedInt(key string, min, max int) func(string) error {
	return func(value string) error {
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid integer")
		}
		if v < min || v > max {
			return fmt.Errorf("must be between %d and %d", min, max)
		}
		setRuntimeOverride(key, strconv.Itoa(v))
		return nil
	}
}

func cabinetIntCurrent(key string, def int) func() string {
	return func() string {
		v := strings.TrimSpace(effectiveEnvUnderRLock(key))
		if v == "" {
			return strconv.Itoa(def)
		}
		return v
	}
}

func applyFortuneInt(key string, min int) func(string) error {
	return func(value string) error {
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid integer")
		}
		if v < min {
			return fmt.Errorf("must be >= %d", min)
		}
		setRuntimeOverride(key, strconv.Itoa(v))
		return nil
	}
}

func fortuneCurrent(key string) func() string {
	return func() string { return effectiveEnvUnderRLock(key) }
}

func cabinetLightThemeCurrent() func() string {
	return func() string {
		v := strings.TrimSpace(effectiveEnvUnderRLock("CABINET_LIGHT_THEME_ENABLED"))
		if v == "" {
			return "true"
		}
		return v
	}
}

// cabinetBoolCurrent — Current для bool-настройки с явным дефолтом при пустом env.
func applyLandingText(key string, maxRunes int) func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(value)
		if utf8.RuneCountInString(v) > maxRunes {
			return fmt.Errorf("must be at most %d characters", maxRunes)
		}
		setRuntimeOverride(key, v)
		return nil
	}
}

func cabinetBoolCurrent(key string, def bool) func() string {
	return func() string {
		v := strings.TrimSpace(effectiveEnvUnderRLock(key))
		if v == "" {
			return boolStr(def)
		}
		return strings.ToLower(v)
	}
}

func applyCabinetDecorTheme() func(string) error {
	allowed := cabinetDecorThemeSet()
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if _, ok := allowed[v]; !ok {
			return fmt.Errorf("invalid decor theme %q", value)
		}
		setRuntimeOverride("CABINET_DECOR_THEME", v)
		return nil
	}
}

func cabinetDecorThemeCurrent() func() string {
	allowed := cabinetDecorThemeSet()
	return func() string {
		v := strings.TrimSpace(strings.ToLower(effectiveEnvUnderRLock("CABINET_DECOR_THEME")))
		if v == "" {
			return "off"
		}
		if _, ok := allowed[v]; ok {
			return v
		}
		return "off"
	}
}

func applyCabinetTariffPriceDisplay() func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "monthly" && v != "marketing" {
			return fmt.Errorf("invalid tariff price display %q", value)
		}
		setRuntimeOverride("CABINET_TARIFF_PRICE_DISPLAY", v)
		return nil
	}
}

func cabinetTariffPriceDisplayCurrent() func() string {
	return func() string {
		v := strings.TrimSpace(strings.ToLower(effectiveEnvUnderRLock("CABINET_TARIFF_PRICE_DISPLAY")))
		if v == "marketing" {
			return "marketing"
		}
		return "monthly"
	}
}

// applyCabinetTariffSavingsBadge — вид плашки «−N %» на карточках сроков
// (шаг 2 витрины): none — без плашки, corner — в углу карточки, old_price —
// зачёркнутая база «цена 1 мес × N» рядом с ценой периода.
func applyCabinetTariffSavingsBadge() func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "none" && v != "corner" && v != "old_price" {
			return fmt.Errorf("invalid tariff savings badge %q", value)
		}
		setRuntimeOverride("CABINET_TARIFF_SAVINGS_BADGE", v)
		return nil
	}
}

func cabinetTariffSavingsBadgeCurrent() func() string {
	return func() string {
		v := strings.TrimSpace(strings.ToLower(effectiveEnvUnderRLock("CABINET_TARIFF_SAVINGS_BADGE")))
		if v == "corner" || v == "old_price" {
			return v
		}
		return "none"
	}
}

// applyCabinetHappCryptVersion — формат зашифрованного deep link Happ.
// crypt4 нужен там, где у пользователей остались сборки Happ 4.x: нового
// crypt5 они не понимают и показывают «URL подписки не валидна».
func applyCabinetHappCryptVersion() func(string) error {
	return func(value string) error {
		v := strings.TrimSpace(strings.ToLower(value))
		if v != "crypt5" && v != "crypt4" {
			return fmt.Errorf("invalid happ crypt version %q", value)
		}
		setRuntimeOverride("CABINET_DEEPLINK_HAPP_CRYPT_VERSION", v)
		return nil
	}
}

func cabinetHappCryptVersionCurrent() func() string {
	return func() string {
		v := strings.TrimSpace(strings.ToLower(effectiveEnvUnderRLock("CABINET_DEEPLINK_HAPP_CRYPT_VERSION")))
		if v == "crypt4" {
			return "crypt4"
		}
		return "crypt5"
	}
}
