package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"remnawave-tg-shop-bot/internal/config"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/remnawave"
)

// DevicesCallbackHandler обрабатывает нажатие на кнопку "Мои устройства"
func (h Handler) DevicesCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}

	if customer == nil {
		slog.Error("customer not exist", "telegramId", callback.From.ID)
		return
	}

	// Получаем информацию о пользователе и лимите устройств
	userID, deviceLimit, err := h.syncService.GetRemnawaveClient().GetUserInfo(ctx, callback.From.ID)
	if err != nil {
		slog.Error("Error getting user info", "error", err)

		// Проверяем, является ли ошибка "user not found"
		msg := callback.Message.Message
		if strings.Contains(err.Error(), "user not found") {
			_, err = editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(langCode, "no_devices"), models.ParseModeHTML, models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
					},
				},
			}, nil)
		} else {
			_, err = editCallbackOriginToHTMLText(ctx, b, msg, h.translation.GetText(langCode, "devices_error"), models.ParseModeHTML, models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
					},
				},
			}, nil)
		}
		logEditError("Error editing message", err)
		return
	}

	// Если лимит устройств равен 0, используем fallback значение из конфигурации
	if deviceLimit == 0 {
		deviceLimit = config.GetHwidFallbackDeviceLimit()
	}

	// Получаем список устройств пользователя по id
	devices, err := h.syncService.GetRemnawaveClient().GetUserDevices(ctx, userID)
	if err != nil {
		slog.Error("Error getting user devices", "error", err)
		_, err = editCallbackOriginToHTMLText(ctx, b, callback.Message.Message, h.translation.GetText(langCode, "devices_error"), models.ParseModeHTML, models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
				},
			},
		}, nil)
		logEditError("Error editing message", err)
		return
	}

	// Формируем сообщение с устройствами
	messageText := h.translation.GetText(langCode, "devices_title")

	// Добавляем информацию о лимите устройств
	messageText += fmt.Sprintf(h.translation.GetText(langCode, "device_limit"), len(devices), deviceLimit)

	var keyboard [][]models.InlineKeyboardButton

	if len(devices) == 0 {
		messageText += h.translation.GetText(langCode, "no_devices")
	} else {
		var deviceRow []models.InlineKeyboardButton
		for i, device := range devices {
			// Создаем более информативное название устройства
			deviceName := h.getDeviceDisplayName(device, i+1)

			// Определяем эмодзи для устройства
			deviceEmoji := h.getDeviceEmoji(deviceName)

			// Форматируем дату добавления
			addedAt := device.CreatedAt.Format("02.01.2006 15:04")

			// Новый формат без ID с динамическим эмодзи
			deviceInfoTemplate := h.translation.GetText(langCode, "device_info_new")
			formattedDeviceInfo := fmt.Sprintf(deviceInfoTemplate,
				fmt.Sprintf("№%d", i+1),
				deviceName,
				addedAt)

			// Добавляем эмодзи в начало, если он определен
			if deviceEmoji != "" {
				messageText += deviceEmoji + " " + formattedDeviceInfo
			} else {
				messageText += formattedDeviceInfo
			}

			// Добавляем кнопку удаления для каждого устройства (по 2 в ряд)
			deviceRow = append(deviceRow, models.InlineKeyboardButton{
				Text:         fmt.Sprintf("🗑️ %s", deviceName),
				CallbackData: fmt.Sprintf("%s%s", CallbackDeleteDevice, device.Hwid),
			})
			if len(deviceRow) == 2 {
				keyboard = append(keyboard, deviceRow)
				deviceRow = nil
			}
		}
		if len(deviceRow) > 0 {
			keyboard = append(keyboard, deviceRow)
		}

		// Добавляем информативный текст под устройствами
		messageText += "\n" + h.translation.GetText(langCode, "device_delete_info")
	}

	// Добавляем кнопку "Назад"
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		h.translation.WithButton(langCode, "back_button", models.InlineKeyboardButton{CallbackData: CallbackConnect}),
	})

	_, err = editCallbackOriginToHTMLText(ctx, b, callback.Message.Message, messageText, models.ParseModeHTML, models.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}, nil)
	logEditError("Error editing message", err)
}

// DeleteDeviceCallbackHandler обрабатывает удаление устройства
func (h Handler) DeleteDeviceCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	// Извлекаем HWID из callback data
	hwid := strings.TrimPrefix(callback.Data, CallbackDeleteDevice)
	if hwid == "" {
		slog.Error("Empty HWID in delete device callback")
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.From.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}

	if customer == nil {
		slog.Error("customer not exist", "telegramId", callback.From.ID)
		return
	}

	// Получаем UUID пользователя
	userID, _, err := h.syncService.GetRemnawaveClient().GetUserInfo(ctx, callback.From.ID)
	if err != nil {
		slog.Error("Error getting user info", "error", err)
		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            h.translation.GetText(langCode, "device_delete_error"),
			ShowAlert:       true,
		})
		if err != nil {
			slog.Error("Error answering callback query", "error", err)
		}
		return
	}

	// Удаляем устройство
	err = h.syncService.GetRemnawaveClient().DeleteUserDevice(ctx, userID, hwid)
	if err != nil {
		slog.Error("Error deleting device", "error", err)
		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            h.translation.GetText(langCode, "device_delete_error"),
			ShowAlert:       true,
		})
		if err != nil {
			slog.Error("Error answering callback query", "error", err)
		}
		return
	}

	// Успешное удаление - убираем алерт, просто обновляем список
	_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            h.translation.GetText(langCode, "device_deleted"),
		ShowAlert:       false,
	})
	if err != nil {
		slog.Error("Error answering callback query", "error", err)
	}

	// Обновляем список устройств
	h.DevicesCallbackHandler(ctx, b, update)
}

// getDeviceDisplayName создает информативное название устройства
func (h Handler) getDeviceDisplayName(device remnawave.Device, deviceNumber int) string {
	// Собираем доступную информацию об устройстве
	var deviceInfo []string

	// Добавляем модель устройства, если доступна
	if device.DeviceModel != nil && *device.DeviceModel != "" {
		deviceInfo = append(deviceInfo, *device.DeviceModel)
	}

	// Добавляем платформу, если доступна
	if device.Platform != nil && *device.Platform != "" {
		deviceInfo = append(deviceInfo, *device.Platform)
	}

	// Добавляем версию ОС, если доступна
	if device.OsVersion != nil && *device.OsVersion != "" {
		deviceInfo = append(deviceInfo, *device.OsVersion)
	}

	// Если есть дополнительная информация, возвращаем её
	if len(deviceInfo) > 0 {
		return strings.Join(deviceInfo, ", ")
	}

	// Если информации нет, возвращаем базовое название
	return fmt.Sprintf("Device %d", deviceNumber)
}

// getDeviceEmoji определяет эмодзи для устройства на основе его названия
func (h Handler) getDeviceEmoji(deviceName string) string {
	deviceNameLower := strings.ToLower(deviceName)

	// Проверяем на десктопные ОС
	desktopKeywords := []string{"windows", "linux", "macos"}
	for _, keyword := range desktopKeywords {
		if strings.Contains(deviceNameLower, keyword) {
			return "🖥️"
		}
	}

	// Проверяем на мобильные ОС
	mobileKeywords := []string{"android", "ios", "apple", "iphone", "samsung", "google", "pixel", "xiaomi", "honor", "huawei"}
	for _, keyword := range mobileKeywords {
		if strings.Contains(deviceNameLower, keyword) {
			return "📱"
		}
	}

	// Если не найдено совпадений, возвращаем пустую строку
	return ""
}
