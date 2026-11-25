package config

import (
	"strings"

	botcfg "remnawave-tg-shop-bot/internal/config"
)

// LightThemeEnabled — CABINET_LIGHT_THEME_ENABLED (runtime/env). По умолчанию true: светлая тема доступна.
func LightThemeEnabled() bool {
	v := strings.TrimSpace(botcfg.EffectiveEnv("CABINET_LIGHT_THEME_ENABLED"))
	if v == "" {
		return true
	}
	return strings.EqualFold(v, "true")
}

// UserThemePickerEnabled — CABINET_USER_THEME_PICKER_ENABLED (runtime/env). По умолчанию false:
// выбор оформления в профиле админ включает сам. Сам выбор хранится в браузере пользователя.
func UserThemePickerEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(botcfg.EffectiveEnv("CABINET_USER_THEME_PICKER_ENABLED")), "true")
}
