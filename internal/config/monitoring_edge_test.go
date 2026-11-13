package config

import (
	"reflect"
	"testing"
)

func TestParseAdminTelegramIDsEdgeCases(t *testing.T) {
	cases := []struct {
		raw     string
		want    []int64
		wantErr bool
	}{
		{"+123", []int64{123}, false},
		{"0,123", []int64{123}, false}, // ноль в начале не делает главным «никого»
		{"123,0,456", []int64{123, 456}, false},
		{"12 34", nil, true},                // пробел внутри числа — не два id
		{"99999999999999999999", nil, true}, // переполнение int64
		{"123,", []int64{123}, false},
		{",", []int64{}, false},
		{"1e9", nil, true},
	}
	for _, c := range cases {
		_, got, err := parseAdminTelegramIDs(c.raw)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", c.raw, err, c.wantErr)
			continue
		}
		if !c.wantErr && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q = %v, want %v", c.raw, got, c.want)
		}
	}
}

// Вызывающий не должен суметь поменять список админов через возвращённый срез.
func TestGetAdminTelegramIdsReturnsCopy(t *testing.T) {
	prev := conf.adminTelegramIds
	t.Cleanup(func() { conf.adminTelegramIds = prev })
	conf.adminTelegramIds = []int64{111, 222}
	ids := GetAdminTelegramIds()
	ids[0] = 999
	if !IsAdminTelegramID(111) || IsAdminTelegramID(999) {
		t.Fatal("GetAdminTelegramIds must return a copy")
	}
}

func TestMonitoringDaysParsing(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 365},
		{"0", 0}, // 0 — не чистить
		{"30", 30},
		{" 30 ", 30},
		{"-1", 365},
		{"3651", 365},
		{"3650", 3650},
		{"abc", 365},
	}
	for _, c := range cases {
		t.Setenv(monitoringRetentionKey, c.raw)
		if got := MonitoringRetentionDays(); got != c.want {
			t.Errorf("MONITORING_RETENTION_DAYS=%q -> %d, want %d", c.raw, got, c.want)
		}
	}
	t.Setenv(monitoringNoisyKey, "")
	t.Setenv(monitoringBotActionsDays, "")
	if MonitoringNoisyRetentionDays() != 90 || MonitoringBotActionsRetentionDays() != 14 {
		t.Fatal("defaults for noisy/bot retention changed")
	}
}

func TestMonitoringSupportWaitMinutes(t *testing.T) {
	for raw, want := range map[string]int{"": 15, "0": 0, "-1": 15, "1": 1, "1440": 1440, "1441": 15, "x": 15} {
		t.Setenv(monitoringSupportWaitKey, raw)
		if got := MonitoringSupportWaitMinutes(); got != want {
			t.Errorf("MONITORING_SUPPORT_WAIT_MIN=%q -> %d, want %d", raw, got, want)
		}
	}
}

func TestMonitoringBotActionsDefaultOff(t *testing.T) {
	t.Setenv(monitoringBotActionsKey, "")
	if MonitoringBotActions() {
		t.Fatal("bot actions must be off by default")
	}
	t.Setenv(monitoringBotActionsKey, "TRUE")
	if !MonitoringBotActions() {
		t.Fatal("TRUE must enable bot actions")
	}
}

// Все ключи «Мониторинга» есть в реестре «Настроек бота» и в группе monitoring.
func TestMonitoringRegistryKeys(t *testing.T) {
	want := map[string]bool{
		monitoringBotActionsKey: true, monitoringRetentionKey: true, monitoringNoisyKey: true,
		monitoringBotActionsDays: true, monitoringSupportWaitKey: true,
	}
	for _, f := range RuntimeSettingsRegistry() {
		if want[f.Key] {
			if f.Group != "monitoring" {
				t.Errorf("%s in group %q", f.Key, f.Group)
			}
			delete(want, f.Key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing in registry: %v", want)
	}
}
