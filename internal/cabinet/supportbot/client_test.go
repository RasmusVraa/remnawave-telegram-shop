package supportbot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostCabinetMessageWithFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/cabinet/message" || r.Header.Get("Authorization") != "Bearer s3cret" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("not multipart: %v", err)
			return
		}
		var payload PostMessageRequest
		var names []string
		var sizes []int
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("part: %v", err)
				return
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "payload":
				if ct := part.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("payload content type %q", ct)
				}
				if err := json.Unmarshal(b, &payload); err != nil {
					t.Errorf("payload: %v", err)
				}
			case "files":
				names = append(names, part.FileName())
				sizes = append(sizes, len(b))
			}
		}
		if payload.AccountID != 42 || payload.Text != "" || payload.ClientMessageID != "cmid" {
			t.Errorf("payload: %+v", payload)
		}
		if len(names) != 2 || names[0] != "a.png" || names[1] != `скрин "1".jpg` || sizes[0] != 3 {
			t.Errorf("files: %v %v", names, sizes)
		}
		_, _ = w.Write([]byte(`{"support_bot_ticket_id": 12, "thread_id": 77,
			"attachments": [{"id": 9, "mime": "image/jpeg", "width": 1280, "height": 720, "size": 3},
			                {"id": 10, "mime": "image/heic", "name": "x.heic"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", "s3cret")
	resp, err := c.PostCabinetMessageWithFiles(context.Background(),
		PostMessageRequest{AccountID: 42, ShopTicketID: 7, ClientMessageID: "cmid"},
		[]UploadFile{{Name: "a.png", Data: []byte("png")}, {Name: `скрин "1".jpg`, Data: []byte("jpeg")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resp.SupportBotTicketID != 12 || len(resp.Attachments) != 2 {
		t.Fatalf("resp: %+v", resp)
	}
	if a := resp.Attachments[0]; a.ID != 9 || *a.Width != 1280 || a.Name != nil {
		t.Fatalf("photo: %+v", a)
	}
	if a := resp.Attachments[1]; a.Mime != "image/heic" || *a.Name != "x.heic" || a.Width != nil {
		t.Fatalf("heic: %+v", a)
	}
}

func TestPostCabinetMessageStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		_, _ = w.Write([]byte(`{"error": "unsupported_media"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "s").PostCabinetMessageWithFiles(context.Background(), PostMessageRequest{},
		[]UploadFile{{Name: "x.png", Data: []byte("x")}})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusUnsupportedMediaType || se.Body != `{"error": "unsupported_media"}` {
		t.Fatalf("err: %v", err)
	}
}

func TestFetchAttachment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s3cret" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/internal/cabinet/file/9":
			if r.URL.Query().Get("account_id") != "42" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg-bytes"))
		case "/internal/cabinet/file/10":
			http.Error(w, `{"error": "failed"}`, http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "s3cret")
	ctx := context.Background()

	data, ct, err := c.FetchAttachment(ctx, 9, 42)
	if err != nil || string(data) != "jpeg-bytes" || ct != "image/jpeg" {
		t.Fatalf("own file: %q %q %v", data, ct, err)
	}
	if _, _, err := c.FetchAttachment(ctx, 9, 43); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other account: %v", err)
	}
	var se *StatusError
	if _, _, err := c.FetchAttachment(ctx, 10, 42); !errors.As(err, &se) || se.Status != http.StatusBadGateway {
		t.Fatalf("telegram failure: %v", err)
	}
}
