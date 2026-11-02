// Package translationfs отдаёт тексты бота и кабинета, вшитые в бинарник.
// Нужны как база: том ./translations на сервере часто отстаёт от образа,
// и без вшитых файлов новые ключи показывались бы сырыми id.
package translationfs

import "embed"

// FS — ru/en (+ admin_*) и cabinet FAQ/app-config из релиза.
//
//go:embed *.json
//go:embed cabinet/FAQ.json
//go:embed cabinet/app-config.json
var FS embed.FS
