package config

import "testing"

func TestMustParseTrustedProxies(t *testing.T) {
	got := mustParseTrustedProxies(" 203.0.113.0/24, 198.51.100.7 ,2001:db8::/32,, 10.1.2.3/8")
	want := []string{"203.0.113.0/24", "198.51.100.7/32", "2001:db8::/32", "10.0.0.0/8"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("item %d = %s, want %s", i, got[i], want[i])
		}
	}
	if len(mustParseTrustedProxies("")) != 0 {
		t.Fatal("empty value must give no prefixes")
	}
}

func TestMustParseTrustedProxies_PanicsOnTypo(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid entry")
		}
	}()
	mustParseTrustedProxies("203.0.113.0/24, 203.0.113.300")
}
