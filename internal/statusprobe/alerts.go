package statusprobe

import (
	"strings"
	"time"
)

// Оповещения админу о сбоях серверов. Причина тревоги открывается, когда
// проблема держится alertStreak замеров подряд, и закрывается первым хорошим
// замером: так один сбойный зонд не будит админа, а «восстановилось» приходит сразу.
// Напоминаний, пока сбой висит, нет.

// Причины тревоги. У строки они независимы: каждая открывается и закрывается сама.
const (
	// AlertRussia — из России не ответили STATUS_RU_FAIL_THRESHOLD зондов и больше.
	AlertRussia = "ru"
	// AlertWorld — не ответил ни один зонд из мира (только обычные серверы).
	AlertWorld = "world"
	// AlertOperators — у обхода не ответил хотя бы один оператор с белыми списками.
	AlertOperators = "operators"
)

// alertStreak — столько замеров подряд должна держаться проблема.
const alertStreak = 2

// maxAlertNames — сколько не ответивших зондов перечислять в сообщении.
const maxAlertNames = 5

// AlertEvent — открылась или закрылась тревога по одной причине одной строки.
type AlertEvent struct {
	Name    string
	Country string
	Reason  string
	// Open — тревога открылась; false — сервер восстановился.
	Open bool
	// Since — когда открылась (для «восстановился»: сколько длился сбой).
	Since time.Time
	At    time.Time

	RussiaOK    int
	RussiaTotal int
	WorldOK     int
	WorldTotal  int
	// Failed — кто не ответил: «сеть (город)» у зондов, название у операторов.
	Failed []string
	// FailedMore — сколько ещё не ответило сверх списка.
	FailedMore int
}

// OnAlerts задаёт, кому отдавать события тревог. Вызывается до Watch.
// fn зовётся после прохода, вне блокировки; долгую работу он уносит в фон сам.
func (r *Runner) OnAlerts(fn func([]AlertEvent)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.onAlerts = fn
	r.mu.Unlock()
}

func (r *Runner) emitAlerts(events []AlertEvent) {
	if len(events) == 0 {
		return
	}
	r.mu.Lock()
	fn := r.onAlerts
	r.mu.Unlock()
	if fn != nil {
		fn(events)
	}
}

// operatorsJudge — обход судится по операторам: есть ключ Latency Lab и данные прохода.
func operatorsJudge(target Target, s Sample) bool {
	return target.Whitelist && s.WhitelistTotal > 0 && LatencyLabConfigured()
}

// pingAlertsLocked — причины, которые решает обычный пинг Globalping. Под r.mu.
func (r *Runner) pingAlertsLocked(target Target, now time.Time) []AlertEvent {
	if !ProbesEnabled() {
		return nil
	}
	s, ok := r.samples[target.Key]
	if !ok {
		return nil
	}
	var events []AlertEvent
	byOperators := operatorsJudge(target, s)
	if byOperators {
		// Обход с операторами: Россию из датацентров не слушаем.
		s.Alerts = forgetAlert(s.Alerts, AlertRussia)
	} else if s.RussiaTotal > 0 {
		fails := s.RussiaTotal - s.RussiaOK
		events = appendAlert(events, &s, target, AlertRussia, s.RussiaFailStreak >= alertStreak, fails < RussiaFailThreshold(), now)
	}
	switch {
	case target.Whitelist || !AlertWorldEnabled():
		s.Alerts = forgetAlert(s.Alerts, AlertWorld)
	case s.WorldTotal > 0:
		events = appendAlert(events, &s, target, AlertWorld, s.WorldFailStreak >= alertStreak, s.WorldOK > 0, now)
	}
	r.samples[target.Key] = s
	return events
}

// operatorAlertsLocked — после полного прохода по операторам. Под r.mu.
func (r *Runner) operatorAlertsLocked(target Target, now time.Time) []AlertEvent {
	if !ProbesEnabled() {
		return nil
	}
	s, ok := r.samples[target.Key]
	if !ok || !s.WhitelistDone || !operatorsJudge(target, s) {
		return nil
	}
	events := appendAlert(nil, &s, target, AlertOperators, s.WhitelistFailStreak >= alertStreak, s.WhitelistFailStreak == 0, now)
	r.samples[target.Key] = s
	return events
}

// appendAlert открывает или закрывает причину и дописывает событие, если что-то сменилось.
func appendAlert(events []AlertEvent, s *Sample, target Target, reason string, bad, good bool, now time.Time) []AlertEvent {
	since, open := s.Alerts[reason]
	switch {
	case !open && bad:
		s.Alerts = setAlert(s.Alerts, reason, now)
		return append(events, newAlertEvent(*s, target, reason, true, now, now))
	case open && good:
		s.Alerts = forgetAlert(s.Alerts, reason)
		return append(events, newAlertEvent(*s, target, reason, false, since, now))
	}
	return events
}

func newAlertEvent(s Sample, target Target, reason string, open bool, since, now time.Time) AlertEvent {
	ev := AlertEvent{
		Name:        target.Name,
		Country:     target.Country,
		Reason:      reason,
		Open:        open,
		Since:       since,
		At:          now,
		RussiaOK:    s.RussiaOK,
		RussiaTotal: s.RussiaTotal,
		WorldOK:     s.WorldOK,
		WorldTotal:  s.WorldTotal,
	}
	if !open {
		return ev
	}
	for _, hit := range s.Probes {
		if hit.OK || !failedFor(reason, hit) {
			continue
		}
		if len(ev.Failed) == maxAlertNames {
			ev.FailedMore++
			continue
		}
		ev.Failed = append(ev.Failed, hitLabel(reason, hit))
	}
	return ev
}

func failedFor(reason string, hit ProbeHit) bool {
	ru := strings.EqualFold(hit.Country, "RU")
	switch reason {
	case AlertOperators:
		return hit.Kind == "whitelist"
	case AlertRussia:
		return hit.Kind != "whitelist" && ru
	case AlertWorld:
		return hit.Kind != "whitelist" && !ru
	}
	return false
}

func hitLabel(reason string, hit ProbeHit) string {
	network := strings.TrimSpace(hit.Network)
	if reason == AlertOperators || strings.TrimSpace(hit.City) == "" {
		if network == "" {
			return "?"
		}
		return network
	}
	if network == "" {
		return hit.City
	}
	return network + " (" + hit.City + ")"
}

// setAlert и forgetAlert меняют копию: снимок замеров уходит в базу вне блокировки,
// и общая карта читалась бы одновременно с записью.
func setAlert(alerts map[string]time.Time, reason string, at time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(alerts)+1)
	for k, v := range alerts {
		out[k] = v
	}
	out[reason] = at
	return out
}

func forgetAlert(alerts map[string]time.Time, reason string) map[string]time.Time {
	if _, ok := alerts[reason]; !ok {
		return alerts
	}
	if len(alerts) == 1 {
		return nil
	}
	out := make(map[string]time.Time, len(alerts)-1)
	for k, v := range alerts {
		if k != reason {
			out[k] = v
		}
	}
	return out
}
