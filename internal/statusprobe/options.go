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
	return clampEnvInt("STATUS_WORLD_PROBES", 1, 1, 10), clampEnvInt("STATUS_RUSSIA_PROBES", 10, 1, 30)
}

// RussiaFailThreshold — сколько зондов из России должны не ответить, чтобы сервер
// стал «частично». Один сбой из десяти — обычный шум зондов, поэтому по умолчанию 2.
func RussiaFailThreshold() int {
	return clampEnvInt("STATUS_RU_FAIL_THRESHOLD", 2, 1, 30)
}

// AlertWorldEnabled — писать админу, когда обычный сервер не отвечает из мира.
// Пустое значение значит «да».
func AlertWorldEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.EffectiveEnv("STATUS_ALERT_WORLD"))) {
	case "false", "0":
		return false
	default:
		return true
	}
}

// ProbeIntervalMinutes — как часто пинговать, если у страны не задан свой интервал.
func ProbeIntervalMinutes() int {
	return clampEnvInt("STATUS_PROBE_INTERVAL_MIN", 60, 5, 180)
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
