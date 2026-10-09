package statusprobe

import (
	"strconv"
	"strings"

	"remnawave-tg-shop-bot/internal/config"
)

// ProbesEnabled — зонды Globalping. Пустое значение значит «включено».
// Доступность узлов панели от этого флага не зависит.
func ProbesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.EffectiveEnv("STATUS_PROBES_ENABLED"))) {
	case "false", "0":
		return false
	default:
		return true
	}
}

// ShowMap — рисовать карту на публичной странице. Пустое значение значит «да».
func ShowMap() bool {
	switch strings.ToLower(strings.TrimSpace(config.EffectiveEnv("STATUS_SHOW_MAP"))) {
	case "false", "0":
		return false
	default:
		return true
	}
}

// ProbeLimits — сколько зондов брать из мира и из России.
func ProbeLimits() (world, russia int) {
	return clampEnvInt("STATUS_WORLD_PROBES", 3, 1, 10), clampEnvInt("STATUS_RUSSIA_PROBES", 20, 1, 30)
}

// ProbeIntervalMinutes — как часто пинговать, если у страны не задан свой интервал.
func ProbeIntervalMinutes() int {
	return clampEnvInt("STATUS_PROBE_INTERVAL_MIN", 20, 5, 180)
}

// PageCopy — заголовок и подзаголовок. Пустая строка значит «взять перевод кабинета».
func PageCopy() (title, lead string) {
	return strings.TrimSpace(config.EffectiveEnv("STATUS_PAGE_TITLE")),
		strings.TrimSpace(config.EffectiveEnv("STATUS_PAGE_LEAD"))
}

func clampEnvInt(key string, def, min, max int) int {
	raw := strings.TrimSpace(config.EffectiveEnv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return def
	}
	return n
}
