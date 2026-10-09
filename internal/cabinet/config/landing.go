package config

import (
	"strings"

	botcfg "remnawave-tg-shop-bot/internal/config"
)

// LandingPublic — тексты и видимость секций лендинга для публичного bootstrap.
// Пустая строка значит «взять перевод из кабинета». Ключи живут в bot_runtime_settings,
// схема таблиц не меняется.
func LandingPublic() map[string]any {
	return map[string]any{
		"hero_title":          landingText("LANDING_HERO_TITLE"),
		"hero_subtitle":       landingText("LANDING_HERO_SUBTITLE"),
		"note":                landingText("LANDING_NOTE"),
		"stat_traffic_value":  landingText("LANDING_STAT_TRAFFIC_VALUE"),
		"stat_traffic_label":  landingText("LANDING_STAT_TRAFFIC_LABEL"),
		"stat_devices_value":  landingText("LANDING_STAT_DEVICES_VALUE"),
		"stat_devices_label":  landingText("LANDING_STAT_DEVICES_LABEL"),
		"show_tariffs":        landingBool("LANDING_SHOW_TARIFFS", true),
		"show_steps":          landingBool("LANDING_SHOW_STEPS", true),
		"show_features":       landingBool("LANDING_SHOW_FEATURES", true),
		"show_faq":            landingBool("LANDING_SHOW_FAQ", true),
	}
}

func landingText(key string) string {
	return strings.TrimSpace(botcfg.EffectiveEnv(key))
}

func landingBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(botcfg.EffectiveEnv(key)))
	switch v {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
