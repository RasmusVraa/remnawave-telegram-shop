package config

import (
	"fmt"
	"strings"
)

// Способы оплаты внутри Platega (PLATEGA_PAYMENT_METHODS).
//
// Каждый способ из списка — отдельная кнопка в боте и кабинете, порядок —
// как в списке. Для всех, кроме "all", при создании платежа передаётся
// paymentMethod и покупатель сразу попадает на нужный способ.
//
// "all" — общая форма Platega без paymentMethod: способ плательщик выбирает на
// её странице из тех, что включены в кабинете Platega. Пусто — "all".
//
// Список id дублируется во фронте (features/checkout/paymentMethods.tsx).

// PlategaPaymentMethodAll — общая форма Platega.
const PlategaPaymentMethodAll = "all"

// PlategaPaymentMethodIDs — допустимые значения в порядке опций админки.
var PlategaPaymentMethodIDs = []string{
	PlategaPaymentMethodAll,
	"sbp",
	"acquiring",
	"worldwide",
	"crypto",
}

// ParsePlategaPaymentMethods разбирает «sbp,crypto». Пустая строка — только "all".
func ParsePlategaPaymentMethods(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{PlategaPaymentMethodAll}, nil
	}
	out := make([]string, 0, len(PlategaPaymentMethodIDs))
	for _, p := range strings.Split(value, ",") {
		id := strings.ToLower(strings.TrimSpace(p))
		if id == "" {
			continue
		}
		if !isPlategaPaymentMethodID(id) {
			return nil, fmt.Errorf("unknown Platega payment method %q", p)
		}
		for _, prev := range out {
			if prev == id {
				return nil, fmt.Errorf("Platega payment method %q is used twice", id)
			}
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return []string{PlategaPaymentMethodAll}, nil
	}
	return out, nil
}

func isPlategaPaymentMethodID(id string) bool {
	for _, known := range PlategaPaymentMethodIDs {
		if known == id {
			return true
		}
	}
	return false
}

// PlategaPaymentMethods — действующий список кнопок Platega. Битое значение
// не должно ломать оплату: тогда работает общая форма.
func PlategaPaymentMethods() []string {
	ids, err := ParsePlategaPaymentMethods(EffectiveEnv("PLATEGA_PAYMENT_METHODS"))
	if err != nil {
		return []string{PlategaPaymentMethodAll}
	}
	return ids
}

// PlategaMethodEnabled — есть ли способ в списке кнопок и включена ли сама Platega.
func PlategaMethodEnabled(id string) bool {
	if !IsPlategaEnabled() {
		return false
	}
	for _, m := range PlategaPaymentMethods() {
		if m == id {
			return true
		}
	}
	return false
}

func applyPlategaPaymentMethods() func(string) error {
	return func(value string) error {
		ids, err := ParsePlategaPaymentMethods(value)
		if err != nil {
			return err
		}
		norm := strings.Join(ids, ",")
		if norm == PlategaPaymentMethodAll {
			norm = ""
		}
		setRuntimeOverride("PLATEGA_PAYMENT_METHODS", norm)
		return nil
	}
}

func plategaPaymentMethodsCurrent() func() string {
	return func() string {
		ids, err := ParsePlategaPaymentMethods(effectiveEnvUnderRLock("PLATEGA_PAYMENT_METHODS"))
		if err != nil || (len(ids) == 1 && ids[0] == PlategaPaymentMethodAll) {
			return ""
		}
		return strings.Join(ids, ",")
	}
}
