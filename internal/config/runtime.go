package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	confMu             sync.RWMutex
	runtimeOverrides   map[string]string
	runtimeOverrideSet map[string]struct{}
)

func init() {
	runtimeOverrides = make(map[string]string)
	runtimeOverrideSet = make(map[string]struct{})
}

// EffectiveEnv — значение env с учётом runtime overrides из админки.
func EffectiveEnv(key string) string {
	confMu.RLock()
	defer confMu.RUnlock()
	return effectiveEnvUnderRLock(key)
}

func effectiveEnvUnderRLock(key string) string {
	if _, ok := runtimeOverrideSet[key]; ok {
		return runtimeOverrides[key]
	}
	return os.Getenv(key)
}

func setRuntimeOverride(key, value string) {
	runtimeOverrides[key] = value
	runtimeOverrideSet[key] = struct{}{}
}

// LoadRuntimeOverrides применяет сохранённые в БД overrides поверх InitConfig().
func LoadRuntimeOverrides(overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}
	byKey := runtimeSettingsByKey()
	confMu.Lock()
	defer confMu.Unlock()

	for key, value := range overrides {
		field, ok := byKey[key]
		if !ok {
			slog.Warn("runtime settings: unknown key in DB, skipped", "key", key)
			continue
		}
		if err := field.Apply(value); err != nil {
			// Сохранённое значение перестало быть допустимым — например, способ
			// оплаты был включён из админки, а потом из .env убрали реквизиты.
			// Это данные из прошлого, а не действие администратора прямо сейчас,
			// и ронять из-за них старт нельзя: бот уходил бы в цикл перезапуска,
			// причём чинить пришлось бы правкой БД. Пропускаем — значение
			// останется тем, что дал .env, а админ увидит расхождение в админке.
			//
			// Живой PATCH из админки (ApplyRuntimePatch) ошибку по-прежнему
			// возвращает: там она нужна, чтобы админ увидел причину отказа.
			slog.Error("runtime settings: stored value rejected, keeping value from .env",
				"key", key, "error", err)
			continue
		}
		runtimeOverrides[key] = value
		runtimeOverrideSet[key] = struct{}{}
	}
	return nil
}

// ApplyRuntimePatch валидирует и применяет patch (hot-reload). При ошибке откатывает уже применённые ключи.
func ApplyRuntimePatch(patch map[string]string) (changed []string, err error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("empty patch")
	}
	if len(patch) > 50 {
		return nil, fmt.Errorf("too many keys in patch")
	}
	byKey := runtimeSettingsByKey()

	confMu.Lock()
	defer confMu.Unlock()

	oldValues := make(map[string]string, len(patch))
	hadOverride := make(map[string]bool, len(patch))
	oldOverrideVal := make(map[string]string, len(patch))
	for key := range patch {
		field, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("unknown or read-only key: %s", key)
		}
		oldValues[key] = field.Current()
		if _, ok := runtimeOverrideSet[key]; ok {
			hadOverride[key] = true
			oldOverrideVal[key] = runtimeOverrides[key]
		}
	}

	changed = make([]string, 0, len(patch))
	for key, value := range patch {
		value = strings.TrimSpace(value)
		if len(value) > 4096 {
			rollbackRuntimePatch(byKey, changed, oldValues, hadOverride, oldOverrideVal)
			return nil, fmt.Errorf("%s: value too long", key)
		}
		field := byKey[key]
		if err := validateSettingBounds(field, value); err != nil {
			rollbackRuntimePatch(byKey, changed, oldValues, hadOverride, oldOverrideVal)
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if err := field.Apply(value); err != nil {
			rollbackRuntimePatch(byKey, changed, oldValues, hadOverride, oldOverrideVal)
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		runtimeOverrides[key] = value
		runtimeOverrideSet[key] = struct{}{}
		changed = append(changed, key)
	}
	return changed, nil
}

func rollbackRuntimePatch(
	byKey map[string]SettingField,
	changed []string,
	oldValues map[string]string,
	hadOverride map[string]bool,
	oldOverrideVal map[string]string,
) {
	for _, key := range changed {
		field := byKey[key]
		_ = field.Apply(oldValues[key])
		if hadOverride[key] {
			runtimeOverrides[key] = oldOverrideVal[key]
			runtimeOverrideSet[key] = struct{}{}
		} else {
			delete(runtimeOverrides, key)
			delete(runtimeOverrideSet, key)
		}
	}
}

func validateSettingBounds(field SettingField, value string) error {
	switch field.Type {
	case SettingInt:
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid integer")
		}
		if field.MinInt != nil && v < *field.MinInt {
			return fmt.Errorf("must be >= %d", *field.MinInt)
		}
		if field.MaxInt != nil && v > *field.MaxInt {
			return fmt.Errorf("must be <= %d", *field.MaxInt)
		}
	case SettingFloat:
		v, err := parseFiniteFloat(value)
		if err != nil {
			return fmt.Errorf("invalid number")
		}
		if v < 0 {
			return fmt.Errorf("must be >= 0")
		}
	case SettingURL:
		s := strings.TrimSpace(value)
		if s == "" {
			return nil
		}
		if !strings.HasPrefix(strings.ToLower(s), "http://") && !strings.HasPrefix(strings.ToLower(s), "https://") && !strings.HasPrefix(s, "tg://") && !strings.HasPrefix(s, "t.me/") && !strings.HasPrefix(s, "https://t.me/") {
			return fmt.Errorf("URL must start with http:// or https://")
		}
	}
	return nil
}

// SettingSource возвращает откуда взято текущее значение: db | env | default.
func SettingSource(key string) string {
	confMu.RLock()
	defer confMu.RUnlock()
	return settingSourceUnderRLock(key)
}

func settingSourceUnderRLock(key string) string {
	if _, ok := runtimeOverrideSet[key]; ok {
		return "db"
	}
	if os.Getenv(key) != "" {
		return "env"
	}
	return "default"
}

// FortuneEnvKeys — FORTUNE_* ключи из registry.
func FortuneEnvKeys() []string {
	var keys []string
	for _, f := range RuntimeSettingsRegistry() {
		if f.Group == "fortune" {
			keys = append(keys, f.Key)
		}
	}
	return keys
}

// ReadConfRL выполняет fn под read-lock (для Current() callbacks в API).
func ReadConfRL(fn func()) {
	confMu.RLock()
	defer confMu.RUnlock()
	fn()
}
