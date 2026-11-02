package config

import (
	"fmt"
	"strings"
)

// Способы оплаты внутри ЮKassa (YOOKASA_PAYMENT_METHODS).
//
// Каждый способ из списка — отдельная кнопка в боте и кабинете. Для всех,
// кроме "all", при создании платежа передаётся payment_method_data.type, и
// покупатель сразу попадает на нужный способ: порядок на форме ЮKassa
// поменять нельзя, а СБП там внизу и её не замечают. Обратная сторона — на
// форме остаётся только переданный способ, переключиться на другой нельзя.
//
// "all" — без payment_method_data, форма ЮKassa со всеми способами. Пусто —
// "all", как было до появления настройки.
//
// Список id дублируется во фронте (features/checkout/paymentMethods.ts).

// YookasaPaymentMethodAll — форма ЮKassa со всеми способами.
const YookasaPaymentMethodAll = "all"

// YookasaPaymentMethodIDs — допустимые значения в порядке опций админки.
// Кроме "all" — коды payment_method_data.type из API ЮKassa.
var YookasaPaymentMethodIDs = []string{
	YookasaPaymentMethodAll,
	"sbp",
	"bank_card",
	"sberbank",
	"tinkoff_bank",
	"yoo_money",
}

// ParseYookasaPaymentMethods разбирает «sbp,bank_card». Пустая строка — только
// "all"; иначе непустой список известных id без повторов.
func ParseYookasaPaymentMethods(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{YookasaPaymentMethodAll}, nil
	}
	out := make([]string, 0, len(YookasaPaymentMethodIDs))
	for _, p := range strings.Split(value, ",") {
		id := strings.ToLower(strings.TrimSpace(p))
		if id == "" {
			continue
		}
		if !isYookasaPaymentMethodID(id) {
			return nil, fmt.Errorf("unknown YooKassa payment method %q", p)
		}
		for _, prev := range out {
			if prev == id {
				return nil, fmt.Errorf("YooKassa payment method %q is used twice", id)
			}
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return []string{YookasaPaymentMethodAll}, nil
	}
	return out, nil
}

func isYookasaPaymentMethodID(id string) bool {
	for _, known := range YookasaPaymentMethodIDs {
		if known == id {
			return true
		}
	}
	return false
}

// YookasaPaymentMethods — действующий список кнопок ЮKassa. Битое значение в
// .env не должно ломать оплату: тогда работает форма со всеми способами.
func YookasaPaymentMethods() []string {
	ids, err := ParseYookasaPaymentMethods(EffectiveEnv("YOOKASA_PAYMENT_METHODS"))
	if err != nil {
		return []string{YookasaPaymentMethodAll}
	}
	return ids
}

// ResolveYookasaPaymentMethod — какой payment_method_data.type передать в
// ЮKassa по выбору покупателя. Пустая строка — не передавать ничего (форма со
// всеми способами).
//
// Выбор вне текущего списка не ошибка: кнопка в старом сообщении бота или
// открытая до смены настройки страница кабинета. Тогда берётся форма со всеми
// способами, если она в списке, иначе первый способ списка.
func ResolveYookasaPaymentMethod(requested string) string {
	methods := YookasaPaymentMethods()
	requested = strings.ToLower(strings.TrimSpace(requested))
	pick := ""
	for _, m := range methods {
		if m == requested {
			pick = m
			break
		}
	}
	if pick == "" {
		pick = methods[0]
		for _, m := range methods {
			if m == YookasaPaymentMethodAll {
				pick = m
				break
			}
		}
	}
	if pick == YookasaPaymentMethodAll {
		return ""
	}
	return pick
}

func applyYookasaPaymentMethods() func(string) error {
	return func(value string) error {
		ids, err := ParseYookasaPaymentMethods(value)
		if err != nil {
			return err
		}
		// Храним нормализованное значение: «SBP, bank_card» → «sbp,bank_card».
		// Одно "all" — то же, что пусто.
		norm := strings.Join(ids, ",")
		if norm == YookasaPaymentMethodAll {
			norm = ""
		}
		setRuntimeOverride("YOOKASA_PAYMENT_METHODS", norm)
		return nil
	}
}

func yookasaPaymentMethodsCurrent() func() string {
	return func() string {
		ids, err := ParseYookasaPaymentMethods(effectiveEnvUnderRLock("YOOKASA_PAYMENT_METHODS"))
		if err != nil || (len(ids) == 1 && ids[0] == YookasaPaymentMethodAll) {
			return ""
		}
		return strings.Join(ids, ",")
	}
}

// Способ, выбранный на странице оплаты кабинета заранее
// (CABINET_CHECKOUT_DEFAULT_METHOD).
//
// Покупателю, который уже платил, кабинет подставляет его прошлый способ; эта
// настройка — для тех, кто платит впервые. Значение — id кнопки кабинета:
// "yookassa:<способ ЮKassa>" или провайдер (platega_sbp, telegram, heleket…).
// Пусто — ничего не выбрано заранее, как было до появления настройки. Если
// выбранный способ потом выключили, кабинет его просто не подставит.

// CheckoutMethodProviders — провайдеры кабинета, кроме ЮKassa (у неё id с
// двоеточием и способом). Синхрон с repository.CheckoutProvider*.
var CheckoutMethodProviders = []string{
	"cryptopay",
	"telegram",
	"platega_sbp",
	"platega_cards",
	"platega_acquiring",
	"platega_worldwide",
	"platega_crypto",
	"heleket",
}

// CheckoutMethodYookassaPrefix — начало id кнопки ЮKassa: "yookassa:sbp".
const CheckoutMethodYookassaPrefix = "yookassa:"

// ParseCheckoutDefaultMethod проверяет id кнопки. Пусто — допустимо.
func ParseCheckoutDefaultMethod(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if m, ok := strings.CutPrefix(value, CheckoutMethodYookassaPrefix); ok {
		if !isYookasaPaymentMethodID(m) {
			return "", fmt.Errorf("unknown YooKassa payment method %q", m)
		}
		return value, nil
	}
	for _, p := range CheckoutMethodProviders {
		if p == value {
			return value, nil
		}
	}
	return "", fmt.Errorf("unknown payment method %q", value)
}

// CheckoutDefaultMethod — действующее значение; битое — пусто.
func CheckoutDefaultMethod() string {
	v, err := ParseCheckoutDefaultMethod(EffectiveEnv("CABINET_CHECKOUT_DEFAULT_METHOD"))
	if err != nil {
		return ""
	}
	return v
}

func applyCheckoutDefaultMethod() func(string) error {
	return func(value string) error {
		v, err := ParseCheckoutDefaultMethod(value)
		if err != nil {
			return err
		}
		setRuntimeOverride("CABINET_CHECKOUT_DEFAULT_METHOD", v)
		return nil
	}
}

func checkoutDefaultMethodCurrent() func() string {
	return func() string {
		v, err := ParseCheckoutDefaultMethod(effectiveEnvUnderRLock("CABINET_CHECKOUT_DEFAULT_METHOD"))
		if err != nil {
			return ""
		}
		return v
	}
}
