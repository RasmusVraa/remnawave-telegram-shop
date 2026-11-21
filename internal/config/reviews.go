package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Отзывы клиентов: включатель, кто может писать, как публиковать, сколько
// показывать на лендинге и какие события слать админу. Всё живёт в runtime-
// настройках (БД поверх env), поэтому меняется без перезапуска.

const (
	ReviewsEnabledKey      = "REVIEWS_ENABLED"
	ReviewsWritePolicyKey  = "REVIEWS_WRITE_POLICY"
	ReviewsPublishModeKey  = "REVIEWS_PUBLISH_MODE"
	ReviewsLandingLimitKey = "REVIEWS_LANDING_LIMIT"
	// ReviewsNotifyEventsKey — какие события слать админу: new, edited.
	ReviewsNotifyEventsKey = "REVIEWS_NOTIFY_EVENTS"
)

// Кто может писать отзыв (REVIEWS_WRITE_POLICY).
const (
	// ReviewsWriteVerified — любой аккаунт с подтверждённой почтой или Telegram.
	ReviewsWriteVerified = "verified"
	// ReviewsWriteTrialPaid — у клиента хотя бы раз была подписка: пробная или оплаченная.
	ReviewsWriteTrialPaid = "trial_paid"
	// ReviewsWritePaid — хотя бы одна оплаченная подписка.
	ReviewsWritePaid = "paid"
)

// Как появляются новые отзывы (REVIEWS_PUBLISH_MODE).
const (
	ReviewsPublishInstant   = "instant"
	ReviewsPublishModerated = "moderated"
)

// Виды событий для админа (REVIEWS_NOTIFY_EVENTS).
const (
	ReviewsEventNew    = "new"
	ReviewsEventEdited = "edited"
)

var (
	reviewsWritePolicies = []string{ReviewsWriteVerified, ReviewsWriteTrialPaid, ReviewsWritePaid}
	reviewsPublishModes  = []string{ReviewsPublishInstant, ReviewsPublishModerated}
)

// ReviewsEnabled — раздел отзывов включён. По умолчанию выключен: после
// обновления ничего не меняется, пока админ сам не включит.
func ReviewsEnabled() bool {
	return cabinetBoolCurrentValue(ReviewsEnabledKey, false)
}

// ReviewsWritePolicy — кто может писать; неизвестное значение трактуется как умолчание.
func ReviewsWritePolicy() string {
	v := strings.ToLower(strings.TrimSpace(EffectiveEnv(ReviewsWritePolicyKey)))
	for _, p := range reviewsWritePolicies {
		if v == p {
			return v
		}
	}
	return ReviewsWriteTrialPaid
}

// ReviewsModerated — новые отзывы сначала попадают «на проверку».
func ReviewsModerated() bool {
	v := strings.ToLower(strings.TrimSpace(EffectiveEnv(ReviewsPublishModeKey)))
	return v != ReviewsPublishInstant
}

// ReviewsLandingLimit — сколько случайных отзывов показывать на лендинге (0 — блок скрыт).
func ReviewsLandingLimit() int {
	v := strings.TrimSpace(EffectiveEnv(ReviewsLandingLimitKey))
	if v == "" {
		return 6
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 6
	}
	if n > 12 {
		return 12
	}
	return n
}

// ReviewsNotifyEvent — слать ли админу это событие. Пусто — оба (так задумано
// по умолчанию), none — ни одного.
func ReviewsNotifyEvent(event string) bool {
	s := strings.ToLower(strings.TrimSpace(EffectiveEnv(ReviewsNotifyEventsKey)))
	if s == "none" {
		return false
	}
	if s == "" {
		return true
	}
	for _, p := range strings.Split(s, ",") {
		if strings.TrimSpace(p) == event {
			return true
		}
	}
	return false
}

func applyRuntimeEnum(key string, allowed []string) func(string) error {
	return func(value string) error {
		v := strings.ToLower(strings.TrimSpace(value))
		for _, a := range allowed {
			if v == a {
				setRuntimeOverride(key, v)
				return nil
			}
		}
		return fmt.Errorf("invalid value")
	}
}

func applyReviewsNotifyEvents(value string) error {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" || s == "none" {
		setRuntimeOverride(ReviewsNotifyEventsKey, s)
		return nil
	}
	var kept []string
	for _, p := range strings.Split(s, ",") {
		switch p = strings.TrimSpace(p); p {
		case ReviewsEventNew, ReviewsEventEdited:
			kept = append(kept, p)
		default:
			return fmt.Errorf("unknown event %q", p)
		}
	}
	setRuntimeOverride(ReviewsNotifyEventsKey, strings.Join(kept, ","))
	return nil
}

func enumCurrent(key, def string) func() string {
	return func() string {
		if v := strings.ToLower(strings.TrimSpace(effectiveEnvUnderRLock(key))); v != "" {
			return v
		}
		return def
	}
}

// reviewsRegistry — группа «Отзывы» и событие «Отзывы» в оповещениях админу.
func reviewsRegistry() []SettingField {
	return []SettingField{
		{
			Key: ReviewsEnabledKey, Group: "reviews", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool(ReviewsEnabledKey),
			Current: cabinetBoolCurrent(ReviewsEnabledKey, false),
		},
		{
			Key: ReviewsWritePolicyKey, Group: "reviews", Type: SettingEnum, EnumValues: reviewsWritePolicies,
			Apply:   applyRuntimeEnum(ReviewsWritePolicyKey, reviewsWritePolicies),
			Current: enumCurrent(ReviewsWritePolicyKey, ReviewsWriteTrialPaid),
		},
		{
			Key: ReviewsPublishModeKey, Group: "reviews", Type: SettingEnum, EnumValues: reviewsPublishModes,
			Apply:   applyRuntimeEnum(ReviewsPublishModeKey, reviewsPublishModes),
			Current: enumCurrent(ReviewsPublishModeKey, ReviewsPublishModerated),
		},
		{
			Key: ReviewsLandingLimitKey, Group: "reviews", Type: SettingInt,
			Apply:   applyBoundedInt(ReviewsLandingLimitKey, 0, 12),
			Current: cabinetIntCurrent(ReviewsLandingLimitKey, 6),
		},
		{
			Key: ReviewsNotifyEventsKey, Group: "admin_alerts", Type: SettingCSV,
			Apply:   applyReviewsNotifyEvents,
			Current: fortuneCurrent(ReviewsNotifyEventsKey),
		},
	}
}
