package config

import (
	"reflect"
	"testing"
)

func TestParseHomeQuickActions(t *testing.T) {
	ok := map[string][]string{
		"":                 {"promo", "invite"},
		"  ":               {"promo", "invite"},
		"invite,promo":     {"invite", "promo"},
		" Fortune , news ": {"fortune", "news"},
		"support,partner":  {"support", "partner"},
	}
	for in, want := range ok {
		got, err := ParseHomeQuickActions(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}

	for _, in := range []string{"promo", "promo,promo", "promo,invite,news", "promo,renew", "promo,", "extra,promo"} {
		if _, err := ParseHomeQuickActions(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestApplyRuntimePatch_homeQuickActions(t *testing.T) {
	t.Cleanup(func() {
		confMu.Lock()
		delete(runtimeOverrides, "CABINET_HOME_QUICK_ACTIONS")
		delete(runtimeOverrideSet, "CABINET_HOME_QUICK_ACTIONS")
		confMu.Unlock()
	})

	if _, err := ApplyRuntimePatch(map[string]string{"CABINET_HOME_QUICK_ACTIONS": "promo,promo"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"CABINET_HOME_QUICK_ACTIONS": "fortune,promo"}); err != nil {
		t.Fatal(err)
	}
	if got := HomeQuickActions(); !reflect.DeepEqual(got, []string{"fortune", "promo"}) {
		t.Fatalf("got %v", got)
	}
}
