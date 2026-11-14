package notification

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/statusprobe"
)

func TestStatusAlertText(t *testing.T) {
	at := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	got := StatusAlertText([]statusprobe.AlertEvent{
		{Name: "Германия <2>", Country: "DE", Reason: statusprobe.AlertRussia, Open: true, RussiaOK: 3, RussiaTotal: 10,
			Failed: []string{"Selectel (Saint Petersburg)", "Timeweb (Moscow)"}, FailedMore: 2},
		{Name: "Швеция", Country: "SE", Reason: statusprobe.AlertWorld, Open: true},
		{Name: "Обход", Country: "RU", Reason: statusprobe.AlertOperators, Open: true, Failed: []string{"МегаФон", "Т2"}},
		{Name: "Эстония", Country: "EE", Reason: statusprobe.AlertRussia, Since: at.Add(-100 * time.Minute), At: at},
	}, "ru", "https://cabinet.example.com/")

	for _, want := range []string{
		"🔴 <b>Сбой серверов</b>",
		"🇩🇪 <b>Германия &lt;2&gt;</b> — из России 3 из 10\nне ответили: Selectel (Saint Petersburg), Timeweb (Moscow) и ещё 2",
		"🇸🇪 <b>Швеция</b> — из мира не отвечает",
		"<b>Обход</b> — операторы: не отвечают МегаФон, Т2",
		"✅ <b>Восстановились</b>\n\n🇪🇪 <b>Эстония</b> — из России, сбой длился 1 ч 40 мин",
		`<a href="https://cabinet.example.com/cabinet/admin/status">Открыть статус</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "Сбой") > strings.Index(got, "Восстановились") {
		t.Error("outages go first")
	}
	if StatusAlertText(nil, "ru", "") != "" {
		t.Error("no events — no message")
	}
	en := StatusAlertText([]statusprobe.AlertEvent{{Name: "X", Reason: statusprobe.AlertWorld, Since: at, At: at.Add(30 * time.Second)}}, "en", "")
	if !strings.Contains(en, "Back up") || !strings.Contains(en, "under a minute") || strings.Contains(en, "<a ") {
		t.Errorf("en: %s", en)
	}
}

func TestStatusAlertTextRegistry(t *testing.T) {
	at := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	got := StatusAlertText([]statusprobe.AlertEvent{
		{Name: "Германия", Country: "DE", Reason: statusprobe.AlertRegistry, Open: true, Note: statusprobe.RegistryKindRegistry},
		{Name: "Швеция", Country: "SE", Reason: statusprobe.AlertRegistry, Open: true, Note: statusprobe.RegistryKindCDN},
		{Name: "Эстония", Country: "EE", Reason: statusprobe.AlertRegistry, Since: at.Add(-3 * time.Hour), At: at},
	}, "ru", "")
	for _, want := range []string{
		"🇩🇪 <b>Германия</b> — адрес найден в реестре РКН, из России может не открываться",
		"🇸🇪 <b>Швеция</b> — адрес в диапазонах заблокированного CDN, из России может не открываться",
		"🇪🇪 <b>Эстония</b> — больше не в списках блокировок, был в списках 3 ч",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "сбой длился") {
		t.Errorf("registry recovery must not say «сбой длился»:\n%s", got)
	}
}
