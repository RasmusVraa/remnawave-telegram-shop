package handler

import "testing"

func TestValidSubscriptionMonths(t *testing.T) {
	for _, m := range []int{1, 3, 6, 12} {
		if !validSubscriptionMonths(m) {
			t.Fatalf("month %d must be sold", m)
		}
	}
	// Подделанные периоды: раньше любой из них стоил как один месяц.
	for _, m := range []int{0, -1, 2, 24, 120, 1200} {
		if validSubscriptionMonths(m) {
			t.Fatalf("month %d must be rejected", m)
		}
	}
}

func TestExtraHwidAllowed(t *testing.T) {
	if !extraHwidAllowed(0) || !extraHwidAllowed(2) {
		t.Fatal("non-negative extra must be allowed when HWID_MAX_DEVICE is not set")
	}
	if extraHwidAllowed(-1) {
		t.Fatal("negative extra must be rejected")
	}
}
