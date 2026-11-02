package config

import (
	"fmt"
	"strings"
)

// Быстрые действия на главной кабинета (CABINET_HOME_QUICK_ACTIONS).
//
// Кнопок четыре. Первые две («Добавить доп. устройство» и «Продлить
// подписку») зависят от состояния подписки и не настраиваются. Настройка
// задаёт кнопки на местах 3 и 4: два разных id через запятую, порядок
// значений — порядок на экране. Пусто — «promo,invite», как было до
// появления настройки.
//
// Список id дублируется во фронте (features/home/homeQuickActions.ts).

// HomeQuickActionIDs — id в порядке опций админского селекта.
var HomeQuickActionIDs = []string{
	"promo",
	"invite",
	"partner",
	"fortune",
	"support",
	"info",
	"news",
	"feedback",
	"sublink",
}

// HomeQuickActionSlots — сколько мест настраивается.
const HomeQuickActionSlots = 2

var defaultHomeQuickActions = []string{"promo", "invite"}

// DefaultHomeQuickActions — значение для пустой настройки.
func DefaultHomeQuickActions() []string {
	return append([]string(nil), defaultHomeQuickActions...)
}

// ParseHomeQuickActions разбирает «promo,invite». Пустая строка — значение
// по умолчанию; иначе ровно два разных известных id.
func ParseHomeQuickActions(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultHomeQuickActions(), nil
	}
	known := make(map[string]struct{}, len(HomeQuickActionIDs))
	for _, id := range HomeQuickActionIDs {
		known[id] = struct{}{}
	}
	parts := strings.Split(value, ",")
	if len(parts) != HomeQuickActionSlots {
		return nil, fmt.Errorf("expected %d actions, got %d", HomeQuickActionSlots, len(parts))
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		id := strings.ToLower(strings.TrimSpace(p))
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("unknown action %q", p)
		}
		for _, prev := range out {
			if prev == id {
				return nil, fmt.Errorf("action %q is used twice", id)
			}
		}
		out = append(out, id)
	}
	return out, nil
}

// HomeQuickActions — действующие кнопки мест 3 и 4. Битое значение в .env не
// должно ломать главную: тогда отдаём значение по умолчанию.
func HomeQuickActions() []string {
	ids, err := ParseHomeQuickActions(EffectiveEnv("CABINET_HOME_QUICK_ACTIONS"))
	if err != nil {
		return DefaultHomeQuickActions()
	}
	return ids
}

func applyCabinetHomeQuickActions() func(string) error {
	return func(value string) error {
		if _, err := ParseHomeQuickActions(value); err != nil {
			return err
		}
		setRuntimeOverride("CABINET_HOME_QUICK_ACTIONS", strings.TrimSpace(value))
		return nil
	}
}

func cabinetHomeQuickActionsCurrent() func() string {
	return func() string {
		ids, err := ParseHomeQuickActions(effectiveEnvUnderRLock("CABINET_HOME_QUICK_ACTIONS"))
		if err != nil {
			ids = DefaultHomeQuickActions()
		}
		return strings.Join(ids, ",")
	}
}
