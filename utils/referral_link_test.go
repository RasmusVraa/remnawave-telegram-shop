package utils

import "testing"

func TestReferralStartURL(t *testing.T) {
	cases := []struct {
		bot  string
		id   int64
		want string
	}{
		{"https://t.me/MeowsBot", 42, "https://t.me/MeowsBot?start=ref_42"},
		{"https://t.me/MeowsBot/", 42, "https://t.me/MeowsBot?start=ref_42"},
		{"@MeowsBot", 42, "https://t.me/MeowsBot?start=ref_42"},
		{"", 42, ""},
		{"https://t.me/MeowsBot", 0, ""},
		{"MeowsBot", 42, ""},
	}
	for _, tc := range cases {
		if got := ReferralStartURL(tc.bot, tc.id); got != tc.want {
			t.Errorf("ReferralStartURL(%q, %d) = %q, want %q", tc.bot, tc.id, got, tc.want)
		}
	}
}
