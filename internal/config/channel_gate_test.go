package config

import "testing"

func TestNormalizeChannelUsername(t *testing.T) {
	ok := map[string]string{
		"":                                 "",
		"   ":                              "",
		"@meows_news":                      "@meows_news",
		"meows_news":                       "@meows_news",
		"t.me/meows_news":                  "@meows_news",
		"https://t.me/meows_news":          "@meows_news",
		"HTTP://T.ME/meows_news":           "@meows_news",
		"https://t.me/meows_news/123":      "@meows_news",
		"https://t.me/meows_news?start=ab": "@meows_news",
		"telegram.me/meows_news":           "@meows_news",
		"  @Meows_News  ":                  "@Meows_News",
	}
	for in, want := range ok {
		got, err := NormalizeChannelUsername(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q err=%v, want %q", in, got, err, want)
		}
	}
	bad := []string{
		"https://t.me/+AbCdEfGhIj",
		"t.me/joinchat/AAAA",
		"https://t.me/c/1234567890/5",
		"-1001234567890",
		"@ab",
		"@1news_channel",
		"not a channel",
		"@name-with-dash",
	}
	for _, in := range bad {
		if got, err := NormalizeChannelUsername(in); err == nil {
			t.Errorf("%q должен отклоняться, получили %q", in, got)
		}
	}
}

func TestChannelGateSettingsRoundTrip(t *testing.T) {
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_CHANNEL": "https://t.me/meows_news"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	})
	if got := ChannelGateChannel(); got != "@meows_news" {
		t.Fatalf("канал должен храниться как @name, получили %q", got)
	}
	if ChannelGateActive() {
		t.Fatal("канал без включённого переключателя ничего не требует")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "true"}); err != nil {
		t.Fatal(err)
	}
	if !ChannelGateActive() || ChannelGateURL() != "https://t.me/meows_news" {
		t.Fatalf("active=%v url=%q", ChannelGateActive(), ChannelGateURL())
	}
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_CHANNEL": "https://t.me/+secret"}); err == nil {
		t.Fatal("приватная ссылка должна отклоняться при сохранении")
	}
	if ChannelGateChannel() != "@meows_news" {
		t.Fatal("после отклонённого сохранения прежний канал должен остаться")
	}
}

func TestChannelGateDefaultsOff(t *testing.T) {
	if got, _ := NormalizeChannelUsername(""); got != "" {
		t.Fatal("пустой канал")
	}
	// Значение по умолчанию: выключено (envBoolDefault без переменной).
	t.Setenv("CHANNEL_GATE_ENABLED", "")
	if envBoolDefault("CHANNEL_GATE_ENABLED", false) {
		t.Fatal("по умолчанию требование подписки выключено")
	}
}
