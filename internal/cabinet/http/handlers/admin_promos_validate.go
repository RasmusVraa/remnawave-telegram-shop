package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"remnawave-tg-shop-bot/internal/database"
)

// promoNoteMaxRunes — длина заметки к промокоду (VARCHAR(200), миграция 000069).
const promoNoteMaxRunes = 200

// normalizePromoNote обрезает пробелы по краям; пустая заметка — NULL.
// ok=false, если заметка длиннее promoNoteMaxRunes.
func normalizePromoNote(s string) (note *string, ok bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > promoNoteMaxRunes {
		return nil, false
	}
	if s == "" {
		return nil, true
	}
	return &s, true
}

// promoTariffAllowed — «только для тарифа» действует лишь у «Дней подписки»
// (promo/service.go проверяет tariff_id только там), у остальных типов тариф
// ни на что не влиял бы. NULL допустим всегда.
func promoTariffAllowed(promoType string, hasTariff bool) string {
	if hasTariff && promoType != database.PromoTypeSubscriptionDays {
		return "tariff only for subscription days"
	}
	return ""
}

func parsePromoPatchFields(raw map[string]json.RawMessage, allowed map[string]bool) (map[string]interface{}, error) {
	fields := make(map[string]interface{})
	for k, v := range raw {
		if !allowed[k] {
			continue
		}
		switch k {
		case "valid_until":
			if string(v) == "null" {
				fields[k] = nil
				continue
			}
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return nil, fmt.Errorf("invalid valid_until")
			}
			if s == "" {
				fields[k] = nil
				continue
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return nil, fmt.Errorf("invalid valid_until")
			}
			fields[k] = t
		case "note":
			if string(v) == "null" {
				fields[k] = nil
				continue
			}
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return nil, fmt.Errorf("invalid note")
			}
			note, ok := normalizePromoNote(s)
			if !ok {
				return nil, fmt.Errorf("note too long")
			}
			if note == nil {
				fields[k] = nil
			} else {
				fields[k] = *note
			}
		case "tariff_id":
			if string(v) == "null" {
				fields[k] = nil
				continue
			}
			var id int64
			if err := json.Unmarshal(v, &id); err != nil {
				return nil, fmt.Errorf("invalid tariff_id")
			}
			fields[k] = id
		case "max_uses":
			var val interface{}
			if err := json.Unmarshal(v, &val); err != nil {
				return nil, fmt.Errorf("invalid field %s", k)
			}
			// Ноль — безлимит (как в боте): храним NULL, чтобы 0 больше не попадал в базу.
			if n, ok := val.(float64); ok && n == 0 {
				val = nil
			}
			fields[k] = val
		default:
			var val interface{}
			if err := json.Unmarshal(v, &val); err != nil {
				return nil, fmt.Errorf("invalid field %s", k)
			}
			fields[k] = val
		}
	}
	return fields, nil
}

func validatePromoUpdateFields(promoType string, fields map[string]interface{}) error {
	if v, ok := fields["tariff_id"]; ok {
		if msg := promoTariffAllowed(promoType, v != nil); msg != "" {
			return fmt.Errorf("%s", msg)
		}
	}
	if v, ok := fields["max_uses"]; ok && v != nil {
		n, err := promoFieldInt(v)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid max_uses")
		}
	}
	if v, ok := fields["discount_max_subscription_payments_per_customer"]; ok {
		n, err := promoFieldInt(v)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid discount_max_subscription_payments_per_customer")
		}
	}
	if v, ok := fields["subscription_days"]; ok {
		if promoType == "subscription_days" {
			n, err := promoFieldInt(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid subscription_days")
			}
		}
	}
	if v, ok := fields["trial_days"]; ok {
		if promoType == "trial" {
			n, err := promoFieldInt(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid trial_days")
			}
		}
	}
	if v, ok := fields["extra_hwid_delta"]; ok && v != nil {
		if promoType == "extra_hwid" {
			if _, err := promoFieldInt(v); err != nil {
				return fmt.Errorf("invalid extra_hwid_delta")
			}
		}
	}
	if v, ok := fields["discount_percent"]; ok {
		if promoType == "discount" {
			n, err := promoFieldInt(v)
			if err != nil || n < 1 || n > 100 {
				return fmt.Errorf("invalid discount_percent")
			}
		}
	}
	if v, ok := fields["discount_ttl_hours"]; ok && v != nil {
		if promoType == "discount" {
			n, err := promoFieldInt(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid discount_ttl_hours")
			}
		}
	}
	return nil
}

func promoFieldInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("not a number")
	}
}
