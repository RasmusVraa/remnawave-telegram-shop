package config

import "strings"

// Push в браузер по видам уведомлений кабинета: что из ленты под колокольчиком
// дублировать системным уведомлением. Настраивает админ в разделе
// «Уведомления»; в ленту попадает всё независимо от этих настроек.
//
// По умолчанию push — только то, ради чего стоит отвлечь: важное, ответ
// поддержки, блокировка сервера и напоминание о конце подписки. Сайт, который
// шлёт push на каждую мелочь, люди отключают, а Chrome сам отзывает разрешение
// у сайтов, чьи уведомления массово игнорируют, — и тогда не дойдёт и важное.

// Виды для CabinetPushEnabled: виды уведомлений кабинета (database.CabinetNotification*)
// и два особых — важное любого вида и тестовая рассылка админу.
const (
	CabinetPushImportant     = "important"
	CabinetPushTestBroadcast = "test_broadcast"
)

type cabinetPushSetting struct {
	kind string
	key  string
	def  bool
}

// cabinetPushSettings — порядок здесь — порядок переключателей в админке.
var cabinetPushSettings = []cabinetPushSetting{
	{CabinetPushImportant, "CABINET_PUSH_IMPORTANT", true},
	{"support", "CABINET_PUSH_SUPPORT", true},
	{"torrent", "CABINET_PUSH_TORRENT", true},
	{"expiring", "CABINET_PUSH_EXPIRING", true},
	{CabinetPushTestBroadcast, "CABINET_PUSH_TEST_BROADCAST", true},
	{"broadcast", "CABINET_PUSH_BROADCAST", false},
	{"payment", "CABINET_PUSH_PAYMENT", false},
	{"devices", "CABINET_PUSH_DEVICES", false},
	{"referral", "CABINET_PUSH_REFERRAL", false},
	{"loyalty", "CABINET_PUSH_LOYALTY", false},
	{"partner", "CABINET_PUSH_PARTNER", false},
}

// CabinetPushEnabled — слать ли push для этого вида. Неизвестный вид — нет:
// новое уведомление само в push не попадёт, пока ему не завели переключатель.
func CabinetPushEnabled(kind string) bool {
	for _, s := range cabinetPushSettings {
		if s.kind == kind {
			return cabinetBoolCurrentValue(s.key, s.def)
		}
	}
	return false
}

func cabinetBoolCurrentValue(key string, def bool) bool {
	v := strings.TrimSpace(EffectiveEnv(key))
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "true")
}

// cabinetPushRegistry — переключатели группы «Уведомления» в админке.
func cabinetPushRegistry() []SettingField {
	out := make([]SettingField, 0, len(cabinetPushSettings))
	for _, s := range cabinetPushSettings {
		out = append(out, SettingField{
			Key: s.key, Group: "notifications", Type: SettingBool, Instant: true,
			Apply:   applyFortuneBool(s.key),
			Current: cabinetBoolCurrent(s.key, s.def),
		})
	}
	return out
}
