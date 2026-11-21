package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Обязательная подписка на канал: CHANNEL_GATE_ENABLED / CHANNEL_GATE_CHANNEL.
// Настройки живут в админке («Настройки бота → Бот и доступ»), по умолчанию выключено.

// channelUsernamePattern — публичное имя в Telegram: 5–32 символа, латиница, цифры, «_», первая — буква.
var channelUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// NormalizeChannelUsername приводит «@name», «name», «t.me/name», «https://t.me/name»
// к «@name». Пустая строка — пустой результат без ошибки (канал не задан).
// Приватные приглашения (t.me/+…, joinchat, c/…) и числовые id отклоняются:
// публичный канал — единственное, что поддерживает проверка.
func NormalizeChannelUsername(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(lower, prefix) {
			s = s[len(prefix):]
			lower = lower[len(prefix):]
		}
	}
	for _, host := range []string{"t.me/", "telegram.me/", "telegram.dog/"} {
		if strings.HasPrefix(lower, host) {
			s = s[len(host):]
			break
		}
	}
	s = strings.TrimPrefix(s, "@")
	// Хвост ссылки: t.me/name?start=… или t.me/name/123.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if strings.HasPrefix(s, "+") || strings.EqualFold(s, "joinchat") || strings.EqualFold(s, "c") {
		return "", fmt.Errorf("private channel links are not supported, use a public @name")
	}
	if !channelUsernamePattern.MatchString(s) {
		return "", fmt.Errorf("use @name or t.me/name of a public channel")
	}
	return "@" + s, nil
}

// ChannelGateEnabled — включено ли требование подписки (CHANNEL_GATE_ENABLED).
func ChannelGateEnabled() bool {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf.channelGateEnabled
}

// ChannelGateChannel — канал в виде «@name» или пустая строка.
func ChannelGateChannel() string {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf.channelGateChannel
}

// ChannelGateActive — требование реально действует: включено и канал задан.
// Включённый переключатель без канала считается выключенным, чтобы никого не
// запирать на экране, который нечем проверить.
func ChannelGateActive() bool {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf.channelGateEnabled && conf.channelGateChannel != ""
}

// Область действия требования подписки (CHANNEL_GATE_SCOPE).
const (
	// ChannelGateScopeAll — подписка нужна для всего: меню бота и закрытые страницы кабинета.
	ChannelGateScopeAll = "all"
	// ChannelGateScopeTrial — подписка нужна только при получении пробного периода.
	ChannelGateScopeTrial = "trial"
)

// ChannelGateTrialOnly — требование подписки действует только на выдачу пробного
// периода; остальное (меню, оплата, продление) открыто.
func ChannelGateTrialOnly() bool {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf.channelGateScope == ChannelGateScopeTrial
}

// ChannelGateURL — ссылка на канал для кнопки «Открыть канал».
func ChannelGateURL() string {
	ch := ChannelGateChannel()
	if ch == "" {
		return ""
	}
	return "https://t.me/" + strings.TrimPrefix(ch, "@")
}

func channelGateRegistry() []SettingField {
	return []SettingField{
		{
			Key: "CHANNEL_GATE_ENABLED", Group: "channel_gate", Type: SettingBool,
			Apply:   applyBoolField(func(v bool) { conf.channelGateEnabled = v }),
			Current: func() string { return boolStr(conf.channelGateEnabled) },
		},
		{
			Key: "CHANNEL_GATE_CHANNEL", Group: "channel_gate", Type: SettingText,
			Apply: func(value string) error {
				ch, err := NormalizeChannelUsername(value)
				if err != nil {
					return err
				}
				conf.channelGateChannel = ch
				return nil
			},
			Current: func() string { return conf.channelGateChannel },
		},
		{
			Key: "CHANNEL_GATE_SCOPE", Group: "channel_gate", Type: SettingEnum,
			EnumValues: []string{ChannelGateScopeAll, ChannelGateScopeTrial},
			Apply:      applyEnumField([]string{ChannelGateScopeAll, ChannelGateScopeTrial}, func(v string) { conf.channelGateScope = v }),
			Current:    func() string { return conf.channelGateScope },
		},
	}
}
