package config

import (
	"log/slog"
	"strings"
	"time"

	botcfg "remnawave-tg-shop-bot/internal/config"
)

// ValidDecorThemeIDs — whitelist CABINET_DECOR_THEME (расширять при новых пресетах).
// Порядок = админ-селект по цветовым группам (green → blue → purple → pink → warm → gold).
var ValidDecorThemeIDs = []string{
	"off",
	"green",
	"spring",
	"cyber",
	"neon",
	"ocean",
	"new_year",
	"new_year_2",
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
	"halloween_2",
	"yellow",
	"summer",
	"black_friday",
}

var validDecorThemes map[string]struct{}

func init() {
	validDecorThemes = make(map[string]struct{}, len(ValidDecorThemeIDs))
	for _, id := range ValidDecorThemeIDs {
		validDecorThemes[id] = struct{}{}
	}
}

// DecorTheme — CABINET_DECOR_THEME (runtime/env). По умолчанию off.
func DecorTheme() string {
	v := strings.TrimSpace(strings.ToLower(botcfg.EffectiveEnv("CABINET_DECOR_THEME")))
	if v == "" {
		return "off"
	}
	if _, ok := validDecorThemes[v]; ok {
		return v
	}
	return "off"
}

// ValidDecorThemes — копия whitelist для внешних пакетов.
func ValidDecorThemes() []string {
	out := make([]string, len(ValidDecorThemeIDs))
	copy(out, ValidDecorThemeIDs)
	return out
}

// IsValidDecorTheme — проверка значения enum.
func IsValidDecorTheme(value string) bool {
	_, ok := validDecorThemes[strings.TrimSpace(strings.ToLower(value))]
	return ok
}

// DecorAutoEnabled — CABINET_DECOR_AUTO_ENABLED: включать ли тему по календарю.
func DecorAutoEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(botcfg.EffectiveEnv("CABINET_DECOR_AUTO_ENABLED")), "true")
}

// DecorScheduleRules — окна из CABINET_DECOR_SCHEDULE; пусто — встроенный пресет.
//
// Битый JSON здесь не ошибка, а данные из прошлого (env правили руками): пишем
// в лог и работаем без авто-темы, вместо того чтобы ронять запрос кабинета.
func DecorScheduleRules() []botcfg.DecorScheduleRule {
	raw := strings.TrimSpace(botcfg.EffectiveEnv("CABINET_DECOR_SCHEDULE"))
	if raw == "" {
		return botcfg.DefaultDecorSchedule()
	}
	rules, err := botcfg.ParseDecorSchedule(raw)
	if err != nil {
		slog.Error("cabinet decor: invalid CABINET_DECOR_SCHEDULE", "error", err)
		return nil
	}
	return rules
}

// ScheduledDecorTheme — тема окна, в которое попадает дата; "" если совпадений нет.
func ScheduledDecorTheme(at time.Time) string {
	if !DecorAutoEnabled() {
		return ""
	}
	theme := botcfg.ResolveDecorScheduleTheme(DecorScheduleRules(), at)
	if theme == "" || !IsValidDecorTheme(theme) {
		return ""
	}
	return theme
}

// EffectiveDecorTheme — то, что видит кабинет: тема праздничного окна, а вне
// окон — выбранная админом вручную (CABINET_DECOR_THEME).
//
// Окно перебивает ручную тему намеренно: смысл авто-расписания в том, чтобы на
// Новый год оформление включилось само, а после праздника вернулась базовая тема.
func EffectiveDecorTheme() string {
	if theme := ScheduledDecorTheme(time.Now()); theme != "" {
		return theme
	}
	return DecorTheme()
}
