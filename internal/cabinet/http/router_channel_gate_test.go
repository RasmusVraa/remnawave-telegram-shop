package cabinethttp

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Явный список маршрутов, закрытых обязательной подпиской на канал (SPEC.md п.4).
// Тест следит за обоими списками сразу: новый маршрут в /me, /payments, /fortune,
// /promocodes не может появиться без решения «закрывать или нет» — иначе он
// остался бы открытым молча, а оплата или поддержка при ошибке — закрытыми.
var channelGateClosed = map[string]bool{
	"/cabinet/api/me/subscription":            true,
	"/cabinet/api/me/subscription/reset-link": true,
	"/cabinet/api/me/connect-invite":          true,
	"/cabinet/api/me/deeplink":                true,
	"/cabinet/api/me/trial":                   true,
	"/cabinet/api/me/trial/activate":          true,
	"/cabinet/api/me/devices":                 true,
	"/cabinet/api/me/devices/delete":          true,
	"/cabinet/api/me/devices/rename":          true,
	"/cabinet/api/me/traffic":                 true,
	"/cabinet/api/me/traffic-usage":           true,
	"/cabinet/api/me/hwid-extra/apply":        true,
	"/cabinet/api/me/loyalty":                 true,
	"/cabinet/api/me/loyalty/history":         true,
	"/cabinet/api/fortune/spin":               true,
	"/cabinet/api/me/referrals":               true,
	"/cabinet/api/me/purchases":               true,
	"/cabinet/api/promocodes/state":           true,
	"/cabinet/api/promocodes/apply":           true,
	"/cabinet/api/payments/checkout":          true,
	"/cabinet/api/payments/preview":           true,
	"/cabinet/api/payments/last-method":       true,
	"/cabinet/api/payments/traffic/preview":   true,
	"/cabinet/api/payments/hwid/preview":      true,
	"/cabinet/api/payments/traffic/checkout":  true,
	"/cabinet/api/payments/hwid/checkout":     true,
}

// Остаются открытыми намеренно: аккаунт, привязки, выход, статус уже созданного
// платежа (оплатил до включения — должен увидеть «оплачено»), безвредное чтение колеса.
var channelGateOpen = map[string]bool{
	"/cabinet/api/me":                        true,
	"/cabinet/api/me/channel-gate/check":     true,
	"/cabinet/api/me/language":               true,
	"/cabinet/api/me/password":               true,
	"/cabinet/api/me/email/verify/resend":    true,
	"/cabinet/api/me/email/link":             true,
	"/cabinet/api/me/email/link/verify-code": true,
	"/cabinet/api/me/identities/unlink":      true,
	"/cabinet/api/me/telegram/link/start":    true,
	"/cabinet/api/me/google/link/start":      true,
	"/cabinet/api/me/yandex/link/start":      true,
	"/cabinet/api/me/vk/link/start":          true,
	"/cabinet/api/me/account/delete":         true,
	"/cabinet/api/payments/":                 true, // GET /payments/{id}/status
	// Уведомления (колокольчик): ответы поддержки и важные рассылки должны доходить и до неподписанных.
	"/cabinet/api/me/notifications":        true,
	"/cabinet/api/me/notifications/delete": true,
	"/cabinet/api/me/notifications/read":   true,
	"/cabinet/api/me/notifications/unread": true,
	"/cabinet/api/me/push/key":             true,
	"/cabinet/api/me/push/subscribe":       true,
	"/cabinet/api/me/push/unsubscribe":     true,
	"/cabinet/api/fortune/status":          true,
	"/cabinet/api/fortune/recent-wins":     true,
}

var (
	routeStartRe = regexp.MustCompile(`^\s*(?:api\.Handle\()?"(/cabinet/api/[^"]+)",?\s*$|^\s*api\.Handle\("(/cabinet/api/[^"]+)",`)
	gatedPrefix  = []string{"/cabinet/api/me", "/cabinet/api/payments", "/cabinet/api/fortune", "/cabinet/api/promocodes"}
)

func TestChannelGateRoutesAreClassified(t *testing.T) {
	raw, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	// Текст каждого блока маршрута: от строки с путём до следующего пути.
	blocks := map[string]string{}
	var current string
	for _, line := range lines {
		if m := routeStartRe.FindStringSubmatch(line); m != nil {
			current = m[1] + m[2]
		}
		if current != "" {
			blocks[current] += line + "\n"
		}
	}

	var unknown []string
	for path, body := range blocks {
		underGate := false
		for _, p := range gatedPrefix {
			if path == p || strings.HasPrefix(path, p+"/") || strings.HasPrefix(path, p+"-") {
				underGate = true
			}
		}
		if !underGate || strings.HasPrefix(path, "/cabinet/api/me/partner") {
			continue
		}
		gated := strings.Contains(body, "channelGate.Require()")
		switch {
		case channelGateClosed[path] && !gated:
			t.Errorf("%s должен быть закрыт подпиской (channelGate.Require() после RequireAuth), но не закрыт", path)
		case channelGateOpen[path] && gated:
			t.Errorf("%s должен оставаться открытым (оплата/поддержка/аккаунт), но закрыт подпиской", path)
		case !channelGateClosed[path] && !channelGateOpen[path]:
			unknown = append(unknown, path)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("новые маршруты без решения по обязательной подписке: %v\nДобавьте каждый в channelGateClosed или channelGateOpen (и в router.go поставьте channelGate.Require(), если закрываете)", unknown)
	}

	// Каждый закрытый и каждый открытый маршрут существует: список не отстаёт от кода.
	for path := range channelGateClosed {
		if _, ok := blocks[path]; !ok {
			t.Errorf("маршрут %s из списка закрытых не найден в router.go", path)
		}
	}
	for path := range channelGateOpen {
		if _, ok := blocks[path]; !ok {
			t.Errorf("маршрут %s из списка открытых не найден в router.go", path)
		}
	}

	// Кабинет партнёра собран через partnerRead/partnerWrite: гейт стоит в обоих.
	full := string(raw)
	for _, name := range []string{"partnerRead := func", "partnerWrite := func"} {
		i := strings.Index(full, name)
		if i < 0 {
			t.Fatalf("%s не найден в router.go", name)
		}
		end := i + 400
		if end > len(full) {
			end = len(full)
		}
		if !strings.Contains(full[i:end], "channelGate.Require()") {
			t.Errorf("%s: нет channelGate.Require() — партнёрские маршруты остались бы открытыми", name)
		}
	}

	// Вебхуки оплат, поддержка, статус платежа и админка подпиской не закрыты.
	for path, body := range blocks {
		if strings.HasPrefix(path, "/cabinet/api/support") || strings.HasPrefix(path, "/cabinet/api/admin/") ||
			strings.HasPrefix(path, "/cabinet/api/auth/") || strings.HasPrefix(path, "/cabinet/api/public/") ||
			strings.HasPrefix(path, "/cabinet/api/internal/") {
			if strings.Contains(body, "channelGate.Require()") {
				t.Errorf("%s не должен закрываться обязательной подпиской", path)
			}
		}
	}
}
