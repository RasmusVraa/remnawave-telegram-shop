package handler

import (
	"net/url"
	"strings"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/config"
)

// BuildCabinetWebAppURL exported версия для использования вне пакета handler
func BuildCabinetWebAppURL(path string) string {
	return cabinetWebAppURL(path)
}

func cabinetWebAppURL(path string) string {
	entry := strings.TrimSpace(cabcfg.MiniAppEntryURL())
	if entry == "" || !strings.HasPrefix(path, "/") {
		return ""
	}
	base, err := url.Parse(entry)
	if err != nil {
		return ""
	}
	target, err := url.Parse(path)
	if err != nil {
		return ""
	}
	return base.ResolveReference(target).String()
}

// reviewsWebAppURL — адрес страницы отзывов в кабинете, если раздел включён и
// кабинет доступен как Mini App. Пусто — кнопка «Отзывы» остаётся внешней ссылкой.
func reviewsWebAppURL() string {
	if !config.ReviewsEnabled() {
		return ""
	}
	return cabinetWebAppURL("/cabinet/reviews")
}
