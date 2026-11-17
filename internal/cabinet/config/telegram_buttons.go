package config

import (
	"strings"

	botcfg "remnawave-tg-shop-bot/internal/config"
)

// TelegramShowChannelButton — CABINET_TELEGRAM_SHOW_CHANNEL_BUTTON (runtime/env).
// По умолчанию false. При true в режиме CABINET_TELEGRAM_UI_MODE=minimalism
// внизу меню бота показывается кнопка «Канал» (URL из CHANNEL_URL).
func TelegramShowChannelButton() bool {
	return strings.EqualFold(strings.TrimSpace(botcfg.EffectiveEnv("CABINET_TELEGRAM_SHOW_CHANNEL_BUTTON")), "true")
}

// TelegramShowFeedbackButton — CABINET_TELEGRAM_SHOW_FEEDBACK_BUTTON (runtime/env).
// По умолчанию false. При true в режиме CABINET_TELEGRAM_UI_MODE=minimalism
// внизу меню бота показывается кнопка «Отзывы» → WebApp /cabinet/reviews.
func TelegramShowFeedbackButton() bool {
	return strings.EqualFold(strings.TrimSpace(botcfg.EffectiveEnv("CABINET_TELEGRAM_SHOW_FEEDBACK_BUTTON")), "true")
}
