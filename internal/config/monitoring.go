package config

import (
	"strconv"
	"strings"
)

// Настройки страницы «Мониторинг»: что писать в журнал событий admin_event и
// сколько хранить. Меняются в «Настройках бота» без перезапуска.

const (
	monitoringBotActionsKey    = "MONITORING_BOT_ACTIONS"
	monitoringRetentionKey     = "MONITORING_RETENTION_DAYS"
	monitoringNoisyKey         = "MONITORING_RETENTION_NOISY_DAYS"
	monitoringBotActionsDays   = "MONITORING_BOT_ACTIONS_DAYS"
	monitoringSupportWaitKey   = "MONITORING_SUPPORT_WAIT_MIN"
	monitoringRetentionMaxDays = 3650
)

// MonitoringBotActions — писать ли каждое нажатие кнопки и команду в боте.
// По умолчанию нет: это десятки тысяч строк в сутки, нужны они редко.
func MonitoringBotActions() bool {
	return cabinetBoolCurrentValue(monitoringBotActionsKey, false)
}

// MonitoringRetentionDays — сколько дней хранить важные события. 0 — не чистить.
func MonitoringRetentionDays() int {
	return monitoringDays(monitoringRetentionKey, 365)
}

// MonitoringNoisyRetentionDays — сколько хранить частые события: входы, переписку,
// сообщения в бот, торрент-репорты. 0 — не чистить.
func MonitoringNoisyRetentionDays() int {
	return monitoringDays(monitoringNoisyKey, 90)
}

// MonitoringBotActionsRetentionDays — сколько хранить нажатия в боте. 0 — не чистить.
func MonitoringBotActionsRetentionDays() int {
	return monitoringDays(monitoringBotActionsDays, 14)
}

// MonitoringSupportWaitMinutes — через сколько минут обращение без ответа
// попадает в «Требует внимания». 0 — не показывать это вовсе.
func MonitoringSupportWaitMinutes() int {
	v, err := strconv.Atoi(strings.TrimSpace(EffectiveEnv(monitoringSupportWaitKey)))
	if err != nil || v < 0 || v > 1440 {
		return 15
	}
	return v
}

func monitoringDays(key string, def int) int {
	raw := strings.TrimSpace(EffectiveEnv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 || v > monitoringRetentionMaxDays {
		return def
	}
	return v
}

// monitoringRegistry — группа «Мониторинг» в «Настройках бота».
func monitoringRegistry() []SettingField {
	days := func(key string, def int) SettingField {
		return SettingField{
			Key: key, Group: "monitoring", Type: SettingInt,
			MinInt: intPtr(0), MaxInt: intPtr(monitoringRetentionMaxDays),
			Apply:   applyBoundedInt(key, 0, monitoringRetentionMaxDays),
			Current: cabinetIntCurrent(key, def),
		}
	}
	return []SettingField{
		{
			Key: monitoringBotActionsKey, Group: "monitoring", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool(monitoringBotActionsKey),
			Current: cabinetBoolCurrent(monitoringBotActionsKey, false),
		},
		{
			Key: monitoringSupportWaitKey, Group: "monitoring", Type: SettingInt,
			MinInt: intPtr(0), MaxInt: intPtr(1440),
			Apply:   applyBoundedInt(monitoringSupportWaitKey, 0, 1440),
			Current: cabinetIntCurrent(monitoringSupportWaitKey, 15),
		},
		days(monitoringRetentionKey, 365),
		days(monitoringNoisyKey, 90),
		days(monitoringBotActionsDays, 14),
	}
}
