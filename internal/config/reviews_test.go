package config

import "testing"

func TestReviewsDefaults(t *testing.T) {
	for _, k := range []string{ReviewsEnabledKey, ReviewsWritePolicyKey, ReviewsPublishModeKey, ReviewsLandingLimitKey, ReviewsNotifyEventsKey} {
		t.Setenv(k, "")
	}
	if ReviewsEnabled() {
		t.Error("отзывы должны быть выключены по умолчанию")
	}
	if got := ReviewsWritePolicy(); got != ReviewsWriteTrialPaid {
		t.Errorf("политика по умолчанию: %q", got)
	}
	if !ReviewsModerated() {
		t.Error("по умолчанию новые отзывы идут на проверку")
	}
	if got := ReviewsLandingLimit(); got != 6 {
		t.Errorf("лимит лендинга: %d", got)
	}
	if !ReviewsNotifyEvent(ReviewsEventNew) || !ReviewsNotifyEvent(ReviewsEventEdited) {
		t.Error("пустое значение — оба события")
	}
}

func TestReviewsValuesAreClamped(t *testing.T) {
	t.Setenv(ReviewsWritePolicyKey, "мусор")
	if got := ReviewsWritePolicy(); got != ReviewsWriteTrialPaid {
		t.Errorf("неизвестная политика должна падать на умолчание: %q", got)
	}
	t.Setenv(ReviewsLandingLimitKey, "99")
	if got := ReviewsLandingLimit(); got != 12 {
		t.Errorf("лимит выше 12: %d", got)
	}
	t.Setenv(ReviewsLandingLimitKey, "-3")
	if got := ReviewsLandingLimit(); got != 6 {
		t.Errorf("отрицательный лимит: %d", got)
	}
	t.Setenv(ReviewsPublishModeKey, "instant")
	if ReviewsModerated() {
		t.Error("режим instant — без проверки")
	}
}

func TestReviewsNotifyEvents(t *testing.T) {
	t.Setenv(ReviewsNotifyEventsKey, "none")
	if ReviewsNotifyEvent(ReviewsEventNew) {
		t.Error("none — ни одного события")
	}
	t.Setenv(ReviewsNotifyEventsKey, "edited")
	if ReviewsNotifyEvent(ReviewsEventNew) || !ReviewsNotifyEvent(ReviewsEventEdited) {
		t.Error("только edited")
	}
	if err := applyReviewsNotifyEvents("new,bogus"); err == nil {
		t.Error("неизвестное событие должно отклоняться")
	}
}
