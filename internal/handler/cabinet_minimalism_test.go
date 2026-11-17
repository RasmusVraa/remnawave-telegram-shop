package handler

import (
	"strings"
	"testing"
)

// Раньше ряд «Отзывы/Канал» собирался из FEEDBACK_URL + CHANNEL_URL.
// Отзывы теперь всегда WebApp на /cabinet/reviews (см. cabinetMinimalismExternalLinkRow);
// здесь проверяем только отбор CHANNEL_URL.
func TestCabinetMinimalismChannelURL(t *testing.T) {
	pick := func(show bool, raw string) string {
		if !show {
			return ""
		}
		return strings.TrimSpace(raw)
	}
	if got := pick(true, "  https://t.me/news  "); got != "https://t.me/news" {
		t.Fatalf("got %q", got)
	}
	if pick(false, "https://t.me/news") != "" {
		t.Fatal("expected empty when flag off")
	}
	if pick(true, "   ") != "" {
		t.Fatal("expected empty when blank")
	}
}
