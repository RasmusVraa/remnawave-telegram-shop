package broadcast

import (
	"net/url"
	"testing"

	"github.com/go-telegram/bot/models"

	botcfg "remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
)

// Кнопка «Пригласить друга» несёт ссылку именно того, кому ушло сообщение:
// иначе приглашённые засчитались бы не тому человеку.
func TestBuildReplyMarkupShareReferral(t *testing.T) {
	tm := translation.GetInstance()
	if err := tm.InitTranslations("../../translations", "ru"); err != nil {
		t.Fatalf("init translations: %v", err)
	}
	prev := botcfg.BotURL()
	botcfg.SetBotURL("https://t.me/MeowsTestBot")
	defer botcfg.SetBotURL(prev)

	markup, ok := BuildReplyMarkup(tm, "ru", RecipientButtons{ShareReferral: true}, 777).(models.InlineKeyboardMarkup)
	if !ok || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("ожидалась одна кнопка, получено %#v", markup)
	}
	btn := markup.InlineKeyboard[0][0]
	u, err := url.Parse(btn.URL)
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	if u.Host != "t.me" || u.Path != "/share/url" {
		t.Fatalf("кнопка ведёт не в окно «Переслать»: %s", btn.URL)
	}
	if got := u.Query().Get("url"); got != "https://t.me/MeowsTestBot?start=ref_777" {
		t.Fatalf("реферальная ссылка = %q", got)
	}
	if u.Query().Get("text") == "" {
		t.Fatal("пустой текст приглашения")
	}

	// Бот ещё не узнал своё имя — кнопку не рисуем, а не шлём битую ссылку.
	botcfg.SetBotURL("")
	if m := BuildReplyMarkup(tm, "ru", RecipientButtons{ShareReferral: true}, 777); m != nil {
		t.Fatalf("без BotURL кнопки быть не должно, получено %#v", m)
	}
}
