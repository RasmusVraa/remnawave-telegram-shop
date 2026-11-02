package broadcast

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestEntitiesToHTML(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		entities []models.MessageEntity
		want     string
	}{
		{"plain escapes", "a < b & c", nil, "a &lt; b &amp; c"},
		{"bold", "Привет мир", []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 0, Length: 6}}, "<b>Привет</b> мир"},
		{
			// Эмодзи — две UTF-16 единицы: смещение после него считается в них.
			"emoji offset", "🔥 Скидка", []models.MessageEntity{{Type: models.MessageEntityTypeItalic, Offset: 3, Length: 6}},
			"🔥 <i>Скидка</i>",
		},
		{
			"nested same start", "важно", []models.MessageEntity{
				{Type: models.MessageEntityTypeItalic, Offset: 0, Length: 3},
				{Type: models.MessageEntityTypeBold, Offset: 0, Length: 5},
			},
			"<b><i>важ</i>но</b>",
		},
		{
			"text link", "сайт", []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, Offset: 0, Length: 4, URL: "https://x.io/?a=1&b=2"}},
			`<a href="https://x.io/?a=1&amp;b=2">сайт</a>`,
		},
		{
			"bare url", "см. x.io", []models.MessageEntity{{Type: models.MessageEntityTypeURL, Offset: 4, Length: 4}},
			`см. <a href="https://x.io">x.io</a>`,
		},
		{"mention ignored", "@bot", []models.MessageEntity{{Type: models.MessageEntityTypeMention, Offset: 0, Length: 4}}, "@bot"},
	}
	for _, c := range cases {
		if got := EntitiesToHTML(c.text, c.entities); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCabinetButtons(t *testing.T) {
	got := CabinetButtons(RecipientButtons{Buy: true, MainMenu: true, Connect: true, Links: []string{"tariffs", "support", "bogus"}})
	want := []string{"buy", "connect", "tariffs", "support"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
