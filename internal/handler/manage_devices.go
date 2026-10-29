package handler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) ManageDevicesCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return
	}

	callbackMessage := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode

	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "telegramId", utils.MaskHalfInt64(update.CallbackQuery.From.ID))
		return
	}
	if err := h.cleanupExpiredExtraHwid(ctx, customer); err != nil {
		slog.Error("Error cleaning expired extra hwid", "error", err)
		return
	}

	now := time.Now()
	activeSub := customer.ExpireAt != nil && customer.ExpireAt.After(now)
	showAddDevice := h.hasPaidSubscription(ctx, customer.ID) && config.HwidExtraDevicesEnabled() && activeSub
	if !showAddDevice {
		h.DevicesCallbackHandler(ctx, b, update)
		return
	}

	userInfo, err := h.syncService.GetRemnawaveClient().GetUserTrafficInfo(ctx, customer.TelegramID)
	if err != nil {
		slog.Error("Error getting user info", "error", err)
		return
	}
	deviceLimitValue := 0
	if userInfo != nil && userInfo.HwidDeviceLimit != nil && *userInfo.HwidDeviceLimit > 0 {
		deviceLimitValue = *userInfo.HwidDeviceLimit
	} else {
		deviceLimitValue = config.GetHwidFallbackDeviceLimit()
	}
	if deviceLimitValue < 0 {
		deviceLimitValue = 0
	}
	deviceCount := 0
	if userInfo != nil && userInfo.ID > 0 {
		devices, err := h.syncService.GetRemnawaveClient().GetUserDevices(ctx, userInfo.ID)
		if err == nil {
			deviceCount = len(devices)
		}
	}
	title := fmt.Sprintf("%s\n\n📱 Устройства: %d / %s", h.translation.GetText(langCode, "manage_devices_title"), deviceCount, formatDeviceLimit(deviceLimitValue, langCode))

	// Закреплённый админом лимит докупкой не меняется — вместо кнопки пояснение.
	pinned := customer.HwidLimitOverride != nil
	if pinned {
		title += "\n\n" + h.translation.GetText(langCode, "hwid_pinned_by_admin")
	}

	var keyboard [][]models.InlineKeyboardButton
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "devices_button", models.InlineKeyboardButton{CallbackData: CallbackDevices}),
	})
	if !pinned {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			h.translation.WithButton(langCode, "add_device_button", models.InlineKeyboardButton{CallbackData: CallbackAddDevice}),
		})
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
	})

	_, err = editCallbackOriginToHTMLText(ctx, b, callbackMessage, title, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("Error sending manage devices message", err)
}

func formatDeviceLimit(value int, langCode string) string {
	if value <= 0 {
		return translation.GetInstance().GetText(langCode, "vpn_unlimited")
	}
	return strconv.Itoa(value)
}
