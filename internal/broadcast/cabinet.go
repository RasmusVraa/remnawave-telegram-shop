package broadcast

import (
	"context"
	"html"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"
)

// CabinetRequest — рассылка, которую нужно положить в ленту кабинета.
type CabinetRequest struct {
	BroadcastID int64
	Audiences   []string
	TariffID    *int64
	Message     Message
	Important   bool
	// AdminTelegramID — получатель тестовой рассылки.
	AdminTelegramID int64
}

// CabinetPublisher кладёт рассылку в ленту кабинета. Реализация живёт в
// cabinetnotify; интерфейс здесь, чтобы broadcast не зависел от неё.
type CabinetPublisher interface {
	PublishBroadcast(ctx context.Context, req CabinetRequest) (recipients int, err error)
}

// CabinetBody — текст рассылки в Telegram HTML для ленты кабинета.
//
// Из web-админки текст уже приходит разметкой. Из Telegram-админки —
// простым текстом с entities: их переводим в теги, иначе в кабинете пропадали
// бы жирный, ссылки и цитаты.
func CabinetBody(msg Message) string {
	switch {
	case msg.ParseMode == models.ParseModeHTML:
		return msg.Text
	case len(msg.Entities) > 0:
		return EntitiesToHTML(msg.Text, msg.Entities)
	default:
		return html.EscapeString(msg.Text)
	}
}

// CabinetButtons — кнопки рассылки для ленты кабинета. Ключи те же, что в
// клавиатуре бота (buy, connect, promo, share_referral и разделы кабинета):
// кабинет подписывает их так же, как бот, — с тем же эмодзи. «Главное меню»
// бота в кабинете смысла не имеет и пропадает.
func CabinetButtons(b RecipientButtons) []string {
	var keys []string
	add := func(key string) {
		for _, k := range keys {
			if k == key {
				return
			}
		}
		keys = append(keys, key)
	}
	if b.Buy {
		add("buy")
	}
	if b.Connect {
		add("connect")
	}
	if b.Promo {
		add("promo")
	}
	if b.ShareReferral {
		add("share_referral")
	}
	for _, key := range NormalizeCabinetLinkKeys(b.Links) {
		add(key)
	}
	return keys
}

// entityTags — открывающий и закрывающий тег Telegram HTML для entity.
// Пустые строки — entity без разметки (упоминания, хэштеги, эмодзи).
func entityTags(e models.MessageEntity, text string) (open, close string) {
	switch e.Type {
	case models.MessageEntityTypeBold:
		return "<b>", "</b>"
	case models.MessageEntityTypeItalic:
		return "<i>", "</i>"
	case models.MessageEntityTypeUnderline:
		return "<u>", "</u>"
	case models.MessageEntityTypeStrikethrough:
		return "<s>", "</s>"
	case models.MessageEntityTypeSpoiler:
		return "<tg-spoiler>", "</tg-spoiler>"
	case models.MessageEntityTypeCode:
		return "<code>", "</code>"
	case models.MessageEntityTypePre:
		return "<pre>", "</pre>"
	case models.MessageEntityTypeBlockquote:
		return "<blockquote>", "</blockquote>"
	case models.MessageEntityTypeExpandableBlockquote:
		return "<blockquote expandable>", "</blockquote>"
	case models.MessageEntityTypeTextLink:
		if e.URL == "" {
			return "", ""
		}
		return `<a href="` + html.EscapeString(e.URL) + `">`, "</a>"
	case models.MessageEntityTypeURL:
		// Голая ссылка в тексте: в Telegram она кликабельна сама, в кабинете —
		// только тегом.
		href := strings.TrimSpace(text)
		if href == "" {
			return "", ""
		}
		if !strings.Contains(href, "://") {
			href = "https://" + href
		}
		return `<a href="` + html.EscapeString(href) + `">`, "</a>"
	default:
		return "", ""
	}
}

// EntitiesToHTML переводит текст с entities в Telegram HTML.
//
// Смещения entities считаются в UTF-16 единицах, а не в байтах и не в рунах:
// эмодзи до форматированного куска иначе сдвинули бы теги внутрь слов.
// Telegram отдаёт entities вложенными (без частичных пересечений), поэтому
// хватает порядка: внешние открываются раньше и закрываются позже.
func EntitiesToHTML(text string, entities []models.MessageEntity) string {
	units := utf16.Encode([]rune(text))
	n := len(units)

	type tag struct {
		pos    int
		open   bool
		length int
		idx    int
		s      string
	}
	var tags []tag
	for i, e := range entities {
		if e.Length <= 0 || e.Offset < 0 || e.Offset >= n {
			continue
		}
		end := e.Offset + e.Length
		if end > n {
			end = n
		}
		piece := string(utf16.Decode(units[e.Offset:end]))
		open, close := entityTags(e, piece)
		if open == "" {
			continue
		}
		tags = append(tags,
			tag{pos: e.Offset, open: true, length: end - e.Offset, idx: i, s: open},
			tag{pos: end, open: false, length: end - e.Offset, idx: i, s: close},
		)
	}
	// На одной позиции сначала закрываются кончившиеся (внутренние раньше),
	// потом открываются начавшиеся (внешние раньше) — так теги не пересекаются.
	sort.SliceStable(tags, func(a, b int) bool {
		x, y := tags[a], tags[b]
		if x.pos != y.pos {
			return x.pos < y.pos
		}
		if x.open != y.open {
			return !x.open
		}
		if x.length != y.length {
			if x.open {
				return x.length > y.length
			}
			return x.length < y.length
		}
		if x.open {
			return x.idx < y.idx
		}
		return x.idx > y.idx
	})

	var b strings.Builder
	last := 0
	for _, t := range tags {
		if t.pos > last {
			b.WriteString(html.EscapeString(string(utf16.Decode(units[last:t.pos]))))
			last = t.pos
		}
		b.WriteString(t.s)
	}
	if last < n {
		b.WriteString(html.EscapeString(string(utf16.Decode(units[last:]))))
	}
	return b.String()
}
