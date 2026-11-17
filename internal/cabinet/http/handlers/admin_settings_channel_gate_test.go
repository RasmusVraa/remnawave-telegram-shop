package handlers

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/channelgate"
	"remnawave-tg-shop-bot/internal/config"
)

type cgAPI struct{ botStatus models.ChatMemberType }

func (cgAPI) GetMe(context.Context) (*models.User, error) { return &models.User{ID: 1}, nil }
func (cgAPI) GetChat(context.Context, *bot.GetChatParams) (*models.ChatFullInfo, error) {
	return &models.ChatFullInfo{Type: models.ChatTypeChannel, Username: "meows_news", Title: "t"}, nil
}
func (c cgAPI) GetChatMember(context.Context, *bot.GetChatMemberParams) (*models.ChatMember, error) {
	return &models.ChatMember{Type: c.botStatus}, nil
}
func (cgAPI) GetChatMemberCount(context.Context, *bot.GetChatMemberCountParams) (int, error) {
	return 10, nil
}

func resetGateConfig(t *testing.T) {
	t.Helper()
	_, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	t.Cleanup(func() {
		_, _ = config.ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_ENABLED": "false", "CHANNEL_GATE_CHANNEL": ""})
	})
}

func TestChannelGatePreflight(t *testing.T) {
	ctx := context.Background()
	admin := &AdminSettingsHandler{channelGate: channelgate.New(cgAPI{botStatus: models.ChatMemberTypeAdministrator}, nil)}
	notAdmin := &AdminSettingsHandler{channelGate: channelgate.New(cgAPI{botStatus: models.ChatMemberTypeMember}, nil)}

	cases := []struct {
		name  string
		h     *AdminSettingsHandler
		setup map[string]string // что уже сохранено до PATCH
		patch map[string]string
		want  string
	}{
		{"не про подписку — не проверяем", notAdmin, nil, map[string]string{"TRIAL_DAYS": "3"}, ""},
		{"выключение не проверяется", notAdmin, nil, map[string]string{"CHANNEL_GATE_ENABLED": "false"}, ""},
		{"включить без канала", admin, nil, map[string]string{"CHANNEL_GATE_ENABLED": "true"}, "channel_gate:no_channel"},
		{"включить с рабочим каналом", admin, nil, map[string]string{"CHANNEL_GATE_ENABLED": "true", "CHANNEL_GATE_CHANNEL": "@meows_news"}, ""},
		{"бот не админ — отказ", notAdmin, nil, map[string]string{"CHANNEL_GATE_ENABLED": "true", "CHANNEL_GATE_CHANNEL": "@meows_news"}, "channel_gate:bot_not_admin"},
		{"канал сохранить, пока выключено — без проверки", notAdmin, nil, map[string]string{"CHANNEL_GATE_CHANNEL": "@meows_news"}, ""},
		{"смена канала при включённом — проверяется", notAdmin,
			map[string]string{"CHANNEL_GATE_CHANNEL": "@meows_news", "CHANNEL_GATE_ENABLED": "true"},
			map[string]string{"CHANNEL_GATE_CHANNEL": "@another_news"}, "channel_gate:bot_not_admin"},
		{"неверный формат оставляем ApplyRuntimePatch", admin, nil, map[string]string{"CHANNEL_GATE_ENABLED": "true", "CHANNEL_GATE_CHANNEL": "https://t.me/+secret"}, ""},
	}
	for _, c := range cases {
		resetGateConfig(t)
		if c.setup != nil {
			if _, err := config.ApplyRuntimePatch(c.setup); err != nil {
				t.Fatalf("%s: setup: %v", c.name, err)
			}
		}
		if got := c.h.channelGatePreflight(ctx, c.patch); got != c.want {
			t.Errorf("%s: получили %q, ожидали %q", c.name, got, c.want)
		}
	}
}
