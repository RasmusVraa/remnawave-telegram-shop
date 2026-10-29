package config

import (
	"testing"
)

func TestApplyRuntimePatch_loyaltyToggle(t *testing.T) {
	orig := conf.loyaltyEnabled
	t.Cleanup(func() {
		confMu.Lock()
		conf.loyaltyEnabled = orig
		delete(runtimeOverrideSet, "LOYALTY_ENABLED")
		delete(runtimeOverrides, "LOYALTY_ENABLED")
		confMu.Unlock()
	})

	changed, err := ApplyRuntimePatch(map[string]string{"LOYALTY_ENABLED": "false"})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "LOYALTY_ENABLED" {
		t.Fatalf("changed=%v", changed)
	}
	if LoyaltyEnabled() {
		t.Fatal("expected loyalty disabled")
	}
	if SettingSource("LOYALTY_ENABLED") != "db" {
		t.Fatalf("source=%s", SettingSource("LOYALTY_ENABLED"))
	}
}

func TestApplyRuntimePatch_unknownKeyRejected(t *testing.T) {
	_, err := ApplyRuntimePatch(map[string]string{"TELEGRAM_TOKEN": "x"})
	if err == nil {
		t.Fatal("expected error for secret key")
	}
}

func TestApplyRuntimePatch_remnaTagFormat(t *testing.T) {
	_, err := ApplyRuntimePatch(map[string]string{"REMNAWAVE_TAG": "bad-tag"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseTelegramIDList(t *testing.T) {
	m, err := parseTelegramIDList("123, 456")
	if err != nil {
		t.Fatal(err)
	}
	if !m[123] || !m[456] {
		t.Fatalf("map=%v", m)
	}
	_, err = parseTelegramIDList("abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEffectiveEnv_override(t *testing.T) {
	confMu.Lock()
	runtimeOverrides["FORTUNE_ENABLED"] = "true"
	runtimeOverrideSet["FORTUNE_ENABLED"] = struct{}{}
	confMu.Unlock()
	t.Cleanup(func() {
		confMu.Lock()
		delete(runtimeOverrides, "FORTUNE_ENABLED")
		delete(runtimeOverrideSet, "FORTUNE_ENABLED")
		confMu.Unlock()
	})
	if EffectiveEnv("FORTUNE_ENABLED") != "true" {
		t.Fatalf("got %q", EffectiveEnv("FORTUNE_ENABLED"))
	}
}

func TestApplyRuntimePatch_cabinetDecorTheme(t *testing.T) {
	changed, err := ApplyRuntimePatch(map[string]string{"CABINET_DECOR_THEME": "neon"})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "CABINET_DECOR_THEME" {
		t.Fatalf("changed=%v", changed)
	}
	if EffectiveEnv("CABINET_DECOR_THEME") != "neon" {
		t.Fatalf("got %q", EffectiveEnv("CABINET_DECOR_THEME"))
	}
	t.Cleanup(func() {
		confMu.Lock()
		delete(runtimeOverrides, "CABINET_DECOR_THEME")
		delete(runtimeOverrideSet, "CABINET_DECOR_THEME")
		confMu.Unlock()
	})
}

func TestApplyRuntimePatch_cabinetDecorThemeInvalid(t *testing.T) {
	_, err := ApplyRuntimePatch(map[string]string{"CABINET_DECOR_THEME": "easter"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// NaN и ±Inf strconv принимает, а проверка «не меньше нуля» их пропускала.
func TestApplyRuntimePatch_rejectsNonFiniteFloat(t *testing.T) {
	var key string
	for _, f := range RuntimeSettingsRegistry() {
		if f.Type == SettingFloat {
			key = f.Key
			break
		}
	}
	if key == "" {
		t.Skip("no float runtime setting")
	}
	before := EffectiveEnv(key)
	for _, v := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "infinity"} {
		if _, err := ApplyRuntimePatch(map[string]string{key: v}); err == nil {
			t.Errorf("%s=%s accepted", key, v)
		}
	}
	if after := EffectiveEnv(key); after != before {
		t.Fatalf("%s changed from %q to %q", key, before, after)
	}
}

func TestParseFiniteFloat(t *testing.T) {
	if v, err := parseFiniteFloat(" 12.5 "); err != nil || v != 12.5 {
		t.Fatalf("12.5: v=%v err=%v", v, err)
	}
	for _, s := range []string{"NaN", "Inf", "-inf", "abc"} {
		if _, err := parseFiniteFloat(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}
