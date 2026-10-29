package handlers

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"remnawave-tg-shop-bot/internal/broadcast"
)

func TestBroadcastSendReqResolveAudiences(t *testing.T) {
	cases := []struct {
		name     string
		req      broadcastSendReq
		want     []string
		wantCode int
	}{
		{
			name: "несколько сегментов",
			req:  broadcastSendReq{Audiences: []string{"inactive_paid", "active_all"}},
			want: []string{"active_all", "inactive_paid"},
		},
		{
			// Вкладка кабинета, открытая до обновления, шлёт одно поле audience.
			name: "старое поле audience",
			req:  broadcastSendReq{Audience: "active_paid"},
			want: []string{"active_paid"},
		},
		{
			name: "audiences важнее audience",
			req:  broadcastSendReq{Audience: "all", Audiences: []string{"active_trial"}},
			want: []string{"active_trial"},
		},
		{name: "пусто", req: broadcastSendReq{}, wantCode: http.StatusBadRequest},
		{name: "неизвестный сегмент", req: broadcastSendReq{Audiences: []string{"vip"}}, wantCode: http.StatusBadRequest},
		{
			name:     "тест вместе с настоящими",
			req:      broadcastSendReq{Audiences: []string{"test_broadcast", "active_all"}},
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			got, ok := tc.req.resolveAudiences(rec)
			if tc.wantCode != 0 {
				if ok || rec.Code != tc.wantCode {
					t.Fatalf("ok=%v code=%d, want code %d", ok, rec.Code, tc.wantCode)
				}
				return
			}
			if !ok {
				t.Fatalf("unexpected error %d: %s", rec.Code, rec.Body.String())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBroadcastSendReqHasContent(t *testing.T) {
	req := broadcastSendReq{Text: "  "}
	req.normalize()
	if req.hasContent() {
		t.Fatal("empty text")
	}
	req.Text = "hello"
	if !req.hasContent() {
		t.Fatal("text")
	}
	req.Text = ""
	req.Media = &broadcastMediaReq{FileID: "abc"}
	if !req.hasContent() {
		t.Fatal("media")
	}
}

func TestBroadcastMediaContentType(t *testing.T) {
	cases := map[string]broadcast.MediaKind{
		"image/png":       broadcast.MediaPhoto,
		"image/jpeg":      broadcast.MediaPhoto,
		"video/mp4":       broadcast.MediaVideo,
		"video/quicktime": broadcast.MediaVideo,
	}
	for ct, want := range cases {
		kind, ok := broadcastMediaContentType(ct)
		if !ok || kind != want {
			t.Errorf("%s: kind=%q ok=%v, want %q", ct, kind, ok, want)
		}
	}
	if _, ok := broadcastMediaContentType("image/gif"); ok {
		t.Error("gif should be rejected")
	}
	if _, ok := broadcastMediaContentType("application/pdf"); ok {
		t.Error("pdf should be rejected")
	}
}

// Неизвестный вид не должен уезжать в SendPhoto: тот на видео просто откажет.
func TestRecipientMediaFallsBackToDocument(t *testing.T) {
	req := broadcastSendReq{Media: &broadcastMediaReq{FileID: "abc", Kind: "sticker"}}
	if got := req.recipientMedia().Kind; got != broadcast.MediaDocument {
		t.Errorf("unknown kind => %q, want %q", got, broadcast.MediaDocument)
	}
	req.Media.Kind = string(broadcast.MediaVideo)
	if got := req.recipientMedia().Kind; got != broadcast.MediaVideo {
		t.Errorf("video kind => %q", got)
	}
}
