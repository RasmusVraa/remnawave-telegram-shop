package database

import (
	"errors"
	"fmt"
	"strings"
)

// broadcastAudienceOrder — канонический порядок сегментов. В нём же они
// возвращаются из NormalizeBroadcastAudiences, чтобы один и тот же выбор в
// разном порядке давал одинаковый запрос и одинаковую строку в логах.
var broadcastAudienceOrder = []string{
	BroadcastAudienceAll,
	BroadcastAudienceTestBroadcast,
	BroadcastAudienceActiveAll,
	BroadcastAudienceActivePaid,
	BroadcastAudienceActiveTrial,
	BroadcastAudienceInactiveAll,
	BroadcastAudienceInactivePaid,
	BroadcastAudienceInactiveTrial,
}

// broadcastAudienceParent — какой сегмент целиком включает данный. «Все
// активные» уже содержат и оплативших, и триал, поэтому выбранные вместе они
// ничего не добавляют: дочерний сегмент выкидываем, а не складываем.
var broadcastAudienceParent = map[string]string{
	BroadcastAudienceActiveAll:     BroadcastAudienceAll,
	BroadcastAudienceInactiveAll:   BroadcastAudienceAll,
	BroadcastAudienceActivePaid:    BroadcastAudienceActiveAll,
	BroadcastAudienceActiveTrial:   BroadcastAudienceActiveAll,
	BroadcastAudienceInactivePaid:  BroadcastAudienceInactiveAll,
	BroadcastAudienceInactiveTrial: BroadcastAudienceInactiveAll,
}

// ErrBroadcastTestCombined — тестовую рассылку смешали с настоящими сегментами.
// Тест уходит только админу, и в паре с «Все активные» он означал бы, что
// «проверочное» сообщение на самом деле получат все активные.
var ErrBroadcastTestCombined = errors.New("test broadcast cannot be combined with other audiences")

// NormalizeBroadcastAudiences приводит выбор сегментов рассылки к виду, по
// которому строится запрос: без пробелов и повторов, без сегментов, уже
// входящих в выбранный родительский, в каноническом порядке. Устаревшие
// active / inactive читаются как active_all / inactive_all.
//
// Пустой результат не ошибка: решать, что делать с пустым выбором, вызывающему.
func NormalizeBroadcastAudiences(audiences []string) ([]string, error) {
	picked := make(map[string]bool, len(audiences))
	for _, raw := range audiences {
		aud := strings.TrimSpace(raw)
		switch aud {
		case "":
			continue
		case BroadcastAudienceActive:
			aud = BroadcastAudienceActiveAll
		case BroadcastAudienceInactive:
			aud = BroadcastAudienceInactiveAll
		}
		if !isKnownBroadcastAudience(aud) {
			return nil, fmt.Errorf("unknown broadcast audience: %s", aud)
		}
		picked[aud] = true
	}

	if picked[BroadcastAudienceTestBroadcast] && len(picked) > 1 {
		return nil, ErrBroadcastTestCombined
	}

	out := make([]string, 0, len(picked))
	for _, aud := range broadcastAudienceOrder {
		if picked[aud] && !broadcastAudienceCovered(aud, picked) {
			out = append(out, aud)
		}
	}
	return out, nil
}

func isKnownBroadcastAudience(aud string) bool {
	for _, known := range broadcastAudienceOrder {
		if known == aud {
			return true
		}
	}
	return false
}

func broadcastAudienceCovered(aud string, picked map[string]bool) bool {
	for p := broadcastAudienceParent[aud]; p != ""; p = broadcastAudienceParent[p] {
		if picked[p] {
			return true
		}
	}
	return false
}
