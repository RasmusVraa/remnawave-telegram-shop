package config

import (
	"reflect"
	"testing"
)

func resetPaymentMethodOverrides(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		confMu.Lock()
		for _, k := range []string{"YOOKASA_PAYMENT_METHODS", "CABINET_CHECKOUT_DEFAULT_METHOD"} {
			delete(runtimeOverrides, k)
			delete(runtimeOverrideSet, k)
		}
		confMu.Unlock()
	})
}

func TestParseYookasaPaymentMethods(t *testing.T) {
	ok := map[string][]string{
		"":                                {"all"},
		" , ":                             {"all"},
		"sbp":                             {"sbp"},
		" SBP , bank_card ":               {"sbp", "bank_card"},
		"bank_card,all":                   {"bank_card", "all"},
		"sberbank,tinkoff_bank,yoo_money": {"sberbank", "tinkoff_bank", "yoo_money"},
	}
	for in, want := range ok {
		got, err := ParseYookasaPaymentMethods(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}

	for _, in := range []string{"sbp,sbp", "card", "sbp,qiwi"} {
		if _, err := ParseYookasaPaymentMethods(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestResolveYookasaPaymentMethod(t *testing.T) {
	resetPaymentMethodOverrides(t)

	cases := []struct {
		setting, requested, want string
	}{
		// Настройка не задана — как до неё: payment_method_data не передаётся.
		{"", "", ""},
		{"", "sbp", ""},
		{"sbp", "", "sbp"},
		{"sbp", "bank_card", "sbp"},
		{"sbp,bank_card", "bank_card", "bank_card"},
		{"sbp,bank_card", "", "sbp"},
		{"sbp,all", "", ""},
		{"sbp,all", "all", ""},
		{"sbp,all", "yoo_money", ""},
		{"sbp,all", "SBP", "sbp"},
	}
	for _, c := range cases {
		if _, err := ApplyRuntimePatch(map[string]string{"YOOKASA_PAYMENT_METHODS": c.setting}); err != nil {
			t.Fatalf("%q: %v", c.setting, err)
		}
		if got := ResolveYookasaPaymentMethod(c.requested); got != c.want {
			t.Errorf("setting %q, requested %q: got %q, want %q", c.setting, c.requested, got, c.want)
		}
	}
}

func TestApplyRuntimePatch_yookasaPaymentMethods(t *testing.T) {
	resetPaymentMethodOverrides(t)

	if _, err := ApplyRuntimePatch(map[string]string{"YOOKASA_PAYMENT_METHODS": "sbp,sbp"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"YOOKASA_PAYMENT_METHODS": " SBP, bank_card"}); err != nil {
		t.Fatal(err)
	}
	if got := yookasaPaymentMethodsCurrent()(); got != "sbp,bank_card" {
		t.Fatalf("current = %q", got)
	}
	// Одно "all" хранится как пусто: это и есть значение по умолчанию.
	if _, err := ApplyRuntimePatch(map[string]string{"YOOKASA_PAYMENT_METHODS": "all"}); err != nil {
		t.Fatal(err)
	}
	if got := yookasaPaymentMethodsCurrent()(); got != "" {
		t.Fatalf("current = %q", got)
	}
}

func TestParseCheckoutDefaultMethod(t *testing.T) {
	ok := map[string]string{
		"":              "",
		"yookassa:sbp":  "yookassa:sbp",
		"YooKassa:All":  "yookassa:all",
		" platega ":     "platega",
		"platega_sbp":   "platega_sbp",
		"platega_cards": "platega_acquiring",
		"telegram":      "telegram",
		"heleket":       "heleket",
	}
	for in, want := range ok {
		got, err := ParseCheckoutDefaultMethod(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"yookassa", "yookassa:card", "tribute", "platega_unknown"} {
		if _, err := ParseCheckoutDefaultMethod(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestParsePlategaPaymentMethods(t *testing.T) {
	ok := map[string][]string{
		"":                   {"all"},
		" , ":                {"all"},
		"sbp":                {"sbp"},
		" ALL , sbp ,crypto": {"all", "sbp", "crypto"},
	}
	for in, want := range ok {
		got, err := ParsePlategaPaymentMethods(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"sbp,sbp", "cards", "card"} {
		if _, err := ParsePlategaPaymentMethods(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}
