package cabinetnotify

import (
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

// Push по умолчанию — только то, ради чего стоит отвлечь; остальное ждёт в ленте.
func TestPushWorthy_Defaults(t *testing.T) {
	cases := []struct {
		kind      string
		important bool
		want      bool
	}{
		{database.CabinetNotificationBroadcast, true, true},
		{database.CabinetNotificationBroadcast, false, false},
		{database.CabinetNotificationExpiring, true, true},
		{database.CabinetNotificationExpiring, false, true},
		{database.CabinetNotificationSupport, false, true},
		{database.CabinetNotificationTorrent, false, true},
		{database.CabinetNotificationPayment, false, false},
		{database.CabinetNotificationReferral, false, false},
		{database.CabinetNotificationLoyalty, false, false},
		{database.CabinetNotificationDevices, false, false},
		{database.CabinetNotificationPartner, false, false},
		{"unknown_kind", false, false},
	}
	for _, c := range cases {
		if got := pushWorthy(c.kind, c.important); got != c.want {
			t.Errorf("pushWorthy(%q, %v) = %v, want %v", c.kind, c.important, got, c.want)
		}
	}
	if !isTestBroadcast([]string{database.BroadcastAudienceTestBroadcast}) || isTestBroadcast([]string{database.BroadcastAudienceActiveAll}) {
		t.Error("isTestBroadcast misdetects the audience")
	}
}

// Переключатели админки меняют правило: оплату можно включить, важное —
// выключить, и тогда важная рассылка идёт по правилу обычной.
func TestPushWorthy_Settings(t *testing.T) {
	t.Setenv("CABINET_PUSH_PAYMENT", "true")
	t.Setenv("CABINET_PUSH_IMPORTANT", "false")
	if !pushWorthy(database.CabinetNotificationPayment, false) {
		t.Error("payment push enabled in settings, but not sent")
	}
	if pushWorthy(database.CabinetNotificationBroadcast, true) {
		t.Error("important push disabled, but an important broadcast still goes as push")
	}
	if !pushWorthy(database.CabinetNotificationSupport, true) {
		t.Error("important support reply must still follow the support toggle")
	}
}
