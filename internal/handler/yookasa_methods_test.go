package handler

import (
	"fmt"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// Каждому способу из настройки нужен код для callback, и код должен
// читаться обратно в тот же способ.
func TestYookasaMethodCallbackCodes(t *testing.T) {
	seen := map[string]string{}
	for _, m := range config.YookasaPaymentMethodIDs {
		code, ok := yookasaMethodCallbackCodes[m]
		if !ok {
			t.Fatalf("no callback code for %q", m)
		}
		if prev, dup := seen[code]; dup {
			t.Fatalf("code %q used by %q and %q", code, prev, m)
		}
		seen[code] = m
		if got := yookasaMethodFromCallback(code); got != m {
			t.Errorf("%q → %q → %q", m, code, got)
		}
	}
	if got := yookasaMethodFromCallback(""); got != "" {
		t.Errorf("empty code → %q", got)
	}
	if got := yookasaMethodFromCallback("zz"); got != "" {
		t.Errorf("unknown code → %q", got)
	}
}

// callback_data у Telegram — не длиннее 64 байт, иначе кнопка не отправится.
func TestYookasaCallbackFitsTelegramLimit(t *testing.T) {
	const limit = 64
	longest := ""
	for _, code := range yookasaMethodCallbackCodes {
		if len(code) > len(longest) {
			longest = code
		}
	}
	suffix := "&ym=" + longest
	cases := []string{
		paymentCallbackQuery("999999", "12", string(database.InvoiceTypeYookasa), "99999", 99) + suffix,
		paymentCallbackQuery("", "12", string(database.InvoiceTypeYookasa), "99999", 99) + suffix,
		fmt.Sprintf("%s?target=%d&invoiceType=%s", CallbackAddDevicePayment, 100, database.InvoiceTypeYookasa) + suffix,
	}
	for _, c := range cases {
		if len(c) > limit {
			t.Errorf("%d bytes > %d: %s", len(c), limit, c)
		}
	}
}
