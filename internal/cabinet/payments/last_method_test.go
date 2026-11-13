package payments

import (
	"testing"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
)

// Каждый провайдер кабинета должен узнаваться обратно по типу счёта: иначе
// после оплаты им страница оплаты не подставит прошлый способ.
func TestCheckoutMethodForInvoiceRoundTrip(t *testing.T) {
	providers := []string{
		repository.CheckoutProviderCryptoPay,
		repository.CheckoutProviderTelegram,
		repository.CheckoutProviderPlatega,
		repository.CheckoutProviderPlategaSBP,
		repository.CheckoutProviderPlategaAcquiring,
		repository.CheckoutProviderPlategaWorldwide,
		repository.CheckoutProviderPlategaCrypto,
		repository.CheckoutProviderHeleket,
	}
	for _, p := range providers {
		it, err := mapProviderToInvoiceType(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if got := checkoutMethodForInvoice(it, nil); got != p {
			t.Errorf("%s → %s → %q", p, it, got)
		}
	}
}

func TestCheckoutMethodForInvoiceYookassa(t *testing.T) {
	sbp := "sbp"
	empty := ""
	cases := []struct {
		method *string
		want   string
	}{
		{nil, "yookassa:all"},
		{&empty, "yookassa:all"},
		{&sbp, "yookassa:sbp"},
	}
	for _, c := range cases {
		if got := checkoutMethodForInvoice(database.InvoiceTypeYookasa, c.method); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
	if got := checkoutMethodForInvoice(database.InvoiceTypeTribute, nil); got != "" {
		t.Errorf("tribute → %q", got)
	}
}
