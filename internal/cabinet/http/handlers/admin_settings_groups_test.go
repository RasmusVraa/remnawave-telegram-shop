package handlers

import (
	"testing"

	"remnawave-tg-shop-bot/internal/config"
)

// Порядок групп в buildAdminSettingsResponse работает белым списком: группы,
// которой в нём нет, в ответе не будет, и в админке она просто не появится —
// без ошибки, просто пустое место. Именно так «Мой налог» уехал в релиз
// невидимым. Тест держит реестр и список синхронными.
func TestEveryRegistryGroupIsDisplayed(t *testing.T) {
	displayed := make(map[string]bool)
	for _, g := range buildAdminSettingsResponse().Groups {
		displayed[g.ID] = true
	}

	seen := make(map[string]bool)
	for _, f := range config.RuntimeSettingsRegistry() {
		if seen[f.Group] {
			continue
		}
		seen[f.Group] = true
		if !displayed[f.Group] {
			t.Errorf("группа %q есть в реестре настроек, но не отдаётся админке — "+
				"добавьте её в order в buildAdminSettingsResponse", f.Group)
		}
	}

	if len(seen) == 0 {
		t.Fatal("реестр настроек пуст — тест ничего не проверил")
	}
}

// Группа «Мой налог» должна доезжать до админки со всеми тремя настройками:
// включением, способами оплаты и сроком повторов.
func TestMoynalogGroupIsExposed(t *testing.T) {
	var fields []string
	for _, g := range buildAdminSettingsResponse().Groups {
		if g.ID != "moynalog" {
			continue
		}
		for _, f := range g.Fields {
			fields = append(fields, f.Key)
		}
	}

	if len(fields) == 0 {
		t.Fatal("группа moynalog не отдаётся админке")
	}

	want := []string{"MOYNALOG_ENABLED", "MOYNALOG_RECEIPT_FOR", "MOYNALOG_RETRY_MAX_AGE_HOURS"}
	for _, key := range want {
		found := false
		for _, got := range fields {
			if got == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("настройка %q не отдаётся в группе moynalog (есть: %v)", key, fields)
		}
	}
}

// Группа «Уведомления» доезжает до админки: переключатель на каждый вид push,
// с умолчаниями — важное, поддержка, блокировка, конец подписки и тестовая
// рассылка включены, остальное выключено.
func TestNotificationsGroupIsExposed(t *testing.T) {
	want := map[string]string{
		"CABINET_PUSH_IMPORTANT":      "true",
		"CABINET_PUSH_SUPPORT":        "true",
		"CABINET_PUSH_TORRENT":        "true",
		"CABINET_PUSH_EXPIRING":       "true",
		"CABINET_PUSH_TEST_BROADCAST": "true",
		"CABINET_PUSH_BROADCAST":      "false",
		"CABINET_PUSH_PAYMENT":        "false",
		"CABINET_PUSH_DEVICES":        "false",
		"CABINET_PUSH_REFERRAL":       "false",
		"CABINET_PUSH_LOYALTY":        "false",
		"CABINET_PUSH_PARTNER":        "false",
	}
	got := map[string]string{}
	for _, g := range buildAdminSettingsResponse().Groups {
		if g.ID != "notifications" {
			continue
		}
		for _, f := range g.Fields {
			if f.Type != "bool" || !f.Instant {
				t.Errorf("%s: type=%s instant=%v, want instant bool", f.Key, f.Type, f.Instant)
			}
			got[f.Key] = f.Value
		}
	}
	for key, def := range want {
		if v, ok := got[key]; !ok {
			t.Errorf("%s не отдаётся в группе notifications", key)
		} else if v != def {
			t.Errorf("%s = %q, want default %q", key, v, def)
		}
	}
	if len(got) != len(want) {
		t.Errorf("в группе %d полей, want %d: %v", len(got), len(want), got)
	}
}

// В общем редакторе настроек Instant значит «сохранить сразу при переключении»:
// у переключателя и списка. Текст или число с Instant не уходят по «Сохранить
// секцию», а черновик откатывается к значению с сервера — поле выглядит
// редактируемым, но ничего не сохраняет. Так сломались TRAFFIC_BYPASS_LABEL и
// TRAFFIC_ALERT_PERCENT. Группа status живёт на своей странице и сохраняет поля
// явным списком, поэтому её не проверяем.
func TestInstantOnlyForTogglesAndEnums(t *testing.T) {
	for _, g := range buildAdminSettingsResponse().Groups {
		if g.ID == "status" {
			continue
		}
		for _, f := range g.Fields {
			if f.Instant && f.Type != "bool" && f.Type != "enum" {
				t.Errorf("%s (группа %s): type=%s с instant — в админке не сохранится", f.Key, g.ID, f.Type)
			}
		}
	}
}
