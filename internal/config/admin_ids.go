package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ADMIN_TELEGRAM_ID принимает несколько id через запятую: «111, 222».
//
// Пока это касается только веб-админки: войти в /cabinet/admin может любой из
// перечисленных. Админка в боте, уведомления и всё, что «пишет админу», по-прежнему
// работают с первым id — там он же служит чатом для ответа и ключом состояния,
// и второй админ получал бы чужие ответы (см. GetAdminTelegramId).

// parseAdminTelegramIDs разбирает список. Пробелы и пустые элементы
// пропускаются, повторы убираются, порядок сохраняется.
//
// Ошибка — как и раньше с одним числом: пустая строка или нечисло. Молча
// выкинутый id означал бы, что человек, которого указали админом, им не стал.
//
// primary — первый ненулевой элемент как есть: раньше в переменной могло стоять
// отрицательное число (чат-группа), и уведомления «админу» уходили туда. Так и
// остаётся. В список тех, кого пускает веб-админка, идут только положительные:
// войти в кабинет может человек, а не группа.
func parseAdminTelegramIDs(raw string) (primary int64, admins []int64, err error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil, fmt.Errorf("empty")
	}
	admins = []int64{}
	seen := map[int64]bool{}
	first := true
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, perr := strconv.ParseInt(part, 10, 64)
		if perr != nil {
			return 0, nil, fmt.Errorf("invalid telegram id %q", part)
		}
		if first && id != 0 {
			primary, first = id, false
		}
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		admins = append(admins, id)
	}
	return primary, admins, nil
}

// GetAdminTelegramIds — все админы из ADMIN_TELEGRAM_ID, первый — главный.
func GetAdminTelegramIds() []int64 {
	return append([]int64(nil), conf.adminTelegramIds...)
}

// IsAdminTelegramID — входит ли id в список админов. Используется веб-админкой.
func IsAdminTelegramID(id int64) bool {
	if id <= 0 {
		return false
	}
	for _, a := range conf.adminTelegramIds {
		if a == id {
			return true
		}
	}
	return false
}
