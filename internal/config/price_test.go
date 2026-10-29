package config

import "testing"

// Период вне продажи стоит 0, а не цену одного месяца: иначе подделанная кнопка
// бота продавала month=120 по цене месяца.
func TestPriceUnknownPeriodIsZero(t *testing.T) {
	orig := conf
	t.Cleanup(func() { conf = orig })
	conf.price1, conf.price3, conf.price6, conf.price12 = 100, 270, 500, 900
	conf.starsPrice1, conf.starsPrice3, conf.starsPrice6, conf.starsPrice12 = 50, 140, 260, 480

	for m, want := range map[int]int{1: 100, 3: 270, 6: 500, 12: 900} {
		if got := Price(m); got != want {
			t.Fatalf("Price(%d) = %d, want %d", m, got, want)
		}
	}
	for _, m := range []int{0, 2, 24, 120} {
		if got := Price(m); got != 0 {
			t.Fatalf("Price(%d) = %d, want 0", m, got)
		}
		if got := StarsPrice(m); got != 0 {
			t.Fatalf("StarsPrice(%d) = %d, want 0", m, got)
		}
	}
}
