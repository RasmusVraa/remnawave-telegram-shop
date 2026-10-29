package broadcast

import "github.com/go-telegram/bot/models"

// RecipientButtons — inline-кнопки под сообщением рассылки (как в TG-админке).
// Links — ключи разделов кабинета (см. CabinetLinks); они рендерятся как web_app-кнопки.
//
// JSON-теги совпадают с полями кнопок в API кабинета: в таком виде кнопки
// лежат в истории рассылок, и «взять за основу» подставляет их в форму как есть.
type RecipientButtons struct {
	Buy      bool `json:"buy"`
	MainMenu bool `json:"main_menu"`
	Promo    bool `json:"promo"`
	Connect  bool `json:"connect"`
	// ShareReferral — «Пригласить друга»: окно «Переслать» Telegram с готовым
	// текстом и личной реферальной ссылкой получателя.
	ShareReferral bool     `json:"share_referral"`
	Links         []string `json:"links"`
}

// IsEmpty — под сообщением не будет ни одной кнопки.
func (f RecipientButtons) IsEmpty() bool {
	return !f.Buy && !f.MainMenu && !f.Promo && !f.Connect && !f.ShareReferral && len(f.Links) == 0
}

// MediaKind — каким методом Bot API уходит прикреплённый файл.
//
// Раньше здесь был флаг AsPhoto, и третьего варианта в нём не помещалось:
// видео пришлось бы отправлять либо картинкой, либо документом — то есть
// файлом, который в чате не проигрывается.
type MediaKind string

const (
	MediaPhoto    MediaKind = "photo"
	MediaVideo    MediaKind = "video"
	MediaDocument MediaKind = "document"
)

// Media — прикреплённый файл (file_id из Telegram Bot API).
type Media struct {
	FileID string
	Kind   MediaKind
}

// Message — что именно уходит получателю.
//
// Собрано в структуру, потому что параметров у отправки стало девять, и
// очередной bool в этом ряду читался бы только по счёту запятых.
//
// Entities и ParseMode — два взаимоисключающих способа задать форматирование.
// Рассылка из Telegram-админки копирует entities исходного сообщения, а
// web-админка отдаёт HTML-разметку из своего редактора; смешивать их нельзя,
// Bot API примет только одно.
type Message struct {
	Text      string
	Entities  []models.MessageEntity
	ParseMode models.ParseMode
	Media     *Media
	Buttons   RecipientButtons
}

// SendResult — итог массовой отправки.
type SendResult struct {
	TotalUsers  int
	SentCount   int
	FailedCount int
	// BlockedCount — часть FailedCount: получатель закрыл бота или удалил аккаунт.
	BlockedCount int
}
