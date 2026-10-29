package utils

import (
	"strconv"
	"strings"
)

// ReferralStartURL — реферальная ссылка на бота: t.me/<bot>?start=ref_<telegram_id>.
//
// botURL ожидается в виде https://t.me/MyBot (так его кладёт main из getMe) или
// @MyBot. Пустая строка — ссылку собрать нельзя, кнопку с ней рисовать не надо.
func ReferralStartURL(botURL string, referrerTelegramID int64) string {
	botURL = strings.TrimSpace(botURL)
	if botURL == "" || referrerTelegramID == 0 {
		return ""
	}
	u := strings.TrimRight(botURL, "/")
	ref := "?start=ref_" + strconv.FormatInt(referrerTelegramID, 10)
	switch {
	case strings.HasPrefix(u, "https://t.me/"), strings.HasPrefix(u, "http://t.me/"):
		return u + ref
	case strings.HasPrefix(u, "@"):
		return "https://t.me/" + strings.TrimPrefix(u, "@") + ref
	}
	return ""
}
