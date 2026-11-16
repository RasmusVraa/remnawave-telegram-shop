package statusprobe

import (
	"os"
	"strings"
	"time"
)

// Состояния узла на странице статуса.
const (
	StateUp      = "up"
	StatePartial = "partial"
	StateDown    = "down"
	// StateUnknown — про сервер ничего не известно: ни узла в панели, ни замера.
	StateUnknown = "unknown"
)

// failStreakDown — столько замеров подряд без единого ответа, чтобы объявить сервер недоступным.
const failStreakDown = 2

func stateRank(state string) int {
	switch state {
	case StateUp:
		return 0
	case StatePartial:
		return 1
	case StateDown:
		return 2
	default:
		return -1
	}
}

// WorseState возвращает более тяжёлое из двух состояний. Пустое («нет данных») не учитывается.
func WorseState(a, b string) string {
	if stateRank(b) > stateRank(a) {
		return b
	}
	return a
}

// RussiaLow — из России ответила половина зондов или меньше. Та же граница,
// что у красного столбика истории.
func (s Sample) RussiaLow() bool {
	return s.RussiaTotal > 0 && s.RussiaOK*2 <= s.RussiaTotal
}

// WhitelistNone — в проходе по операторам не ответил ни один.
func (s Sample) WhitelistNone() bool {
	return s.WhitelistTotal > 0 && s.WhitelistOK == 0
}

// ProbeState — что говорят одни зонды, без панели и выхода каскада.
// threshold — сколько зондов из России должны не ответить, чтобы сервер стал «частично».
//
// Обычный сервер: «недоступен», если два замера подряд не ответил никто или из России
// ответила половина и меньше; «частично», если из России не ответили threshold и больше,
// не ответил мир или один замер подряд не ответил никто. Меньше threshold сбоев — шум зондов.
//
// Обход с проверкой операторов: решают только операторы. Мир и датацентры России
// клиенту на мобильном интернете с белыми списками ничего не говорят.
// Без данных по операторам обход судится по зондам России, без мира.
func ProbeState(s Sample, bypass bool, threshold int) string {
	if threshold < 1 {
		threshold = 1
	}
	if s.FailStreak >= failStreakDown {
		return StateDown
	}
	if bypass && s.WhitelistTotal > 0 {
		switch {
		case s.WhitelistLowStreak >= failStreakDown:
			return StateDown
		case s.WhitelistOK < s.WhitelistTotal:
			return StatePartial
		}
		return StateUp
	}
	switch {
	case s.RussiaLowStreak >= failStreakDown:
		return StateDown
	case s.FailStreak == 1:
		return StatePartial
	case s.RussiaTotal > 0 && s.RussiaTotal-s.RussiaOK >= threshold:
		return StatePartial
	case !bypass && s.WorldTotal > 0 && s.WorldOK == 0:
		return StatePartial
	}
	return StateUp
}

// EffectiveState собирает итоговый статус строки.
//   - entry — состояние узла входа в панели; пусто, если узел не найден;
//   - sample — последний замер зондов, nil если замера ещё нет;
//   - bypass — строка-обход, судится по операторам;
//   - exit — состояние зарубежного выхода каскада; пусто, если не задан.
//
// Зонды считает ProbeState. Если узла панели нет — решают зонды. Панель говорит
// «не работает», а зонды достучались — «частично»: нода могла потерять связь
// с панелью, но отвечать из интернета. Ничего не известно вовсе — StateUnknown, а не «частично».
func EffectiveState(entry string, sample *Sample, bypass bool, exit string) string {
	state := entry
	if stateRank(state) < 0 {
		state = ""
	}
	if sample != nil {
		probe := ProbeState(*sample, bypass, RussiaFailThreshold())
		switch {
		case state == "":
			state = probe
		case state == StateDown && probe != StateDown && !sample.AllFailed():
			state = StatePartial
		default:
			state = WorseState(state, probe)
		}
	}
	if state == "" {
		if exit == StateDown || exit == StatePartial {
			return exit
		}
		return StateUnknown
	}
	return WorseState(state, exit)
}

// LatencyLabConfigured — задан ключ Latency Lab, операторы с белыми списками проверяются.
func LatencyLabConfigured() bool {
	return strings.TrimSpace(os.Getenv("LATENCYLAB_TOKEN")) != ""
}

// QuotaEstimate — сколько единиц Globalping уходит в час на текущий список.
type QuotaEstimate struct {
	PerHour int `json:"per_hour"`
	Limit   int `json:"limit"`
	// MaxCost — сколько единиц стоит самая дорогая проверка из списка. Globalping не запускает
	// замер, если остаток меньше его цены, поэтому «осталось 5» при цене 16 — это исчерпанный лимит.
	MaxCost int `json:"max_cost"`
	// TokenSet — задан GLOBALPING_TOKEN (лимит выше).
	TokenSet bool `json:"token_set"`
	// LatencyLabSet — задан LATENCYLAB_TOKEN, проверка обхода возможна.
	LatencyLabSet bool `json:"latencylab_set"`
	// GlobalpingPausedUntil / LatencyLabPausedUntil — сервис ответил 429, проверки стоят до этого времени.
	GlobalpingPausedUntil *time.Time `json:"globalping_paused_until,omitempty"`
	LatencyLabPausedUntil *time.Time `json:"latencylab_paused_until,omitempty"`
	// GlobalpingPauseNote / LatencyLabPauseNote — что сервис написал в ответе 429.
	GlobalpingPauseNote string `json:"globalping_pause_note,omitempty"`
	LatencyLabPauseNote string `json:"latencylab_pause_note,omitempty"`
	// Real — настоящий остаток у Globalping; nil, если спросить не вышло.
	Real *GlobalpingLimits `json:"real,omitempty"`
	// Registry — состояние проверки по спискам блокировок (cheburcheck).
	Registry *RegistryQuota `json:"registry,omitempty"`
	// Cheburcheck — живые сканеры в российских сетях и выбор источника «России».
	Cheburcheck *CheburcheckQuota `json:"cheburcheck,omitempty"`
}

// CheburcheckQuota — что показать в карточке Cheburcheck и в переключателе источника.
type CheburcheckQuota struct {
	// Configured — задан CHEBURCHECK_TOKEN.
	Configured bool `json:"configured"`
	// Mode — выбор админа (auto, globalping, cheburcheck); Active — кто даёт «Россию» сейчас.
	Mode   string `json:"mode"`
	Active string `json:"active"`
	// PausedUntil — сервис ответил 429 или не принял ключ.
	PausedUntil *time.Time `json:"paused_until,omitempty"`
	PauseNote   string     `json:"pause_note,omitempty"`
	// Credits — остаток кредитов; ноль и пусто не показываем: проверки идут и при нуле.
	Credits *int `json:"credits,omitempty"`
	// LastAt — последний ответ по любому серверу, Pass из Total сканеров открыли его.
	LastAt *time.Time `json:"last_at,omitempty"`
	Pass   int        `json:"pass,omitempty"`
	Total  int        `json:"total,omitempty"`
}

// RegistryQuota — что показать в карточке «Реестр РКН»: свежесть списков сервиса,
// пауза после 429 и итог по серверам.
type RegistryQuota struct {
	Info        *RegistryInfo `json:"info,omitempty"`
	PausedUntil *time.Time    `json:"paused_until,omitempty"`
	// Checked — у скольких строк есть ответ, Listed — сколько из них числятся в списках.
	Checked int `json:"checked"`
	Listed  int `json:"listed"`
}

// EstimateQuota считает расход по тем же правилам, что и замер: один зонд — одна единица.
// Latency Lab здесь не учитывается, у него своя квота.
func EstimateQuota(list []ConfiguredTarget) QuotaEstimate {
	defWorld, defRussia := ProbeLimits()
	total := 0.0
	maxCost := 0
	for _, item := range list {
		if !item.Enabled || item.NoExternal || strings.TrimSpace(item.Address) == "" {
			continue
		}
		world, russia := item.WorldProbes, item.RussiaProbes
		if world <= 0 {
			world = defWorld
		}
		if russia <= 0 {
			russia = defRussia
		}
		cost := world + russia
		if !item.AllowsRepeatCities() {
			// Города берутся с запасом: на пять больше, чем нужно.
			extra := russia + 5
			if extra > 30 {
				extra = 30
			}
			cost = world + extra
		}
		if cost > maxCost {
			maxCost = cost
		}
		minutes := item.IntervalMin
		if minutes <= 0 {
			minutes = ProbeIntervalMinutes()
		}
		total += float64(cost) * 60 / float64(minutes)
	}
	token := strings.TrimSpace(os.Getenv("GLOBALPING_TOKEN")) != ""
	limit := 250
	if token {
		limit = 500
	}
	return QuotaEstimate{
		PerHour:       int(total + 0.5),
		Limit:         limit,
		MaxCost:       maxCost,
		TokenSet:      token,
		LatencyLabSet: LatencyLabConfigured(),
	}
}
