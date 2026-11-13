package config

import (
	"reflect"
	"testing"
)

func TestParseAdminTelegramIDs(t *testing.T) {
	cases := []struct {
		raw     string
		want    []int64
		wantErr bool
	}{
		{"123", []int64{123}, false},
		{" 123 ", []int64{123}, false},
		{"123,456", []int64{123, 456}, false},
		{"123, 456", []int64{123, 456}, false},
		{"123,,456,", []int64{123, 456}, false},
		{"456,123,456", []int64{456, 123}, false},
		{"0", []int64{}, false},  // раньше 0 значил «админа нет» — так и осталось
		{"-5", []int64{}, false}, // группа: в веб-админку не пускает, уведомления шлёт туда
		{"", nil, true},
		{"   ", nil, true},
		{"abc", nil, true},
		{"123,abc", nil, true},
		{"123;456", nil, true},
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

// Главный id — для бота и уведомлений. Отрицательный (чат-группа) раньше работал
// и должен работать дальше, хотя в веб-админку группу не пускаем.
func TestParseAdminTelegramIDsPrimary(t *testing.T) {
	cases := map[string]int64{
		"123":         123,
		"123,456":     123,
		"-100123":     -100123,
		"-100123,456": -100123,
		"0":           0,
		"0,123":       123,
		" 456 , 123 ": 456,
	}
	for raw, want := range cases {
		got, _, err := parseAdminTelegramIDs(raw)
		if err != nil || got != want {
			t.Errorf("%q: primary = %d, err = %v; want %d", raw, got, err, want)
		}
	}
}

func TestIsAdminTelegramID(t *testing.T) {
	prev := conf.adminTelegramIds
	t.Cleanup(func() { conf.adminTelegramIds = prev })
	conf.adminTelegramIds = []int64{111, 222}
	if !IsAdminTelegramID(111) || !IsAdminTelegramID(222) {
		t.Fatal("listed admins must pass")
	}
	if IsAdminTelegramID(333) || IsAdminTelegramID(0) || IsAdminTelegramID(-111) {
		t.Fatal("others must not pass")
	}
}
