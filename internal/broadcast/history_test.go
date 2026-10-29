package broadcast

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
)

// Кнопки в истории лежат под ключами API кабинета: «взять за основу»
// подставляет их в форму как есть, и другое имя поля потеряло бы кнопку.
func TestHistoryRecordButtonsUseCabinetKeys(t *testing.T) {
	msg := Message{
		Text:      "<b>Привет</b>",
		ParseMode: models.ParseModeHTML,
		Media:     &Media{FileID: "AgAC", Kind: MediaPhoto},
		Buttons:   RecipientButtons{Connect: true, ShareReferral: true, Links: []string{"tariffs"}},
	}
	rec := HistoryRecord(database.BroadcastSourceCabinet, []string{"active_all"}, nil, msg, "nl.png")

	var got map[string]any
	if err := json.Unmarshal(rec.Buttons, &got); err != nil {
		t.Fatalf("buttons json: %v", err)
	}
	want := map[string]any{
		"buy": false, "connect": true, "promo": false, "main_menu": false,
		"share_referral": true, "links": []any{"tariffs"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
	if rec.ParseMode != "HTML" || rec.MediaKind != "photo" || rec.MediaFileID != "AgAC" || rec.MediaName != "nl.png" {
		t.Fatalf("record = %+v", rec)
	}
}
