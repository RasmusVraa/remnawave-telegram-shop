package service

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinet/supportbot"
)

var (
	testPNG  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("p"), 64)...)
	testJPEG = append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte("j"), 64)...)
)

func TestDetectSupportImageMime(t *testing.T) {
	bmp := append([]byte("BM"), make([]byte, 12)...)
	bmp = append(bmp, 40, 0, 0, 0)
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", testJPEG, "image/jpeg"},
		{"png", testPNG, "image/png"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp"},
		{"gif", []byte("GIF89a......"), "image/gif"},
		{"bmp", bmp, "image/bmp"},
		{"tiff", []byte("II*\x00...."), "image/tiff"},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), "image/heic"},
		{"heif", []byte("\x00\x00\x00\x18ftypmif1\x00\x00\x00\x00mif1miaf"), "image/heif"},
		{"avif", []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avifmif1miaf"), "image/avif"},
		{"mp4", []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isommp41"), ""},
		{"pdf", []byte("%PDF-1.7"), ""},
		{"not bmp", []byte("BM-not-a-bitmap!!!"), ""},
		{"html", []byte("<html><script>"), ""},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := detectSupportImageMime(c.data); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateSupportUploads(t *testing.T) {
	ok := SupportUpload{Name: "a.png", Data: testPNG}
	if err := validateSupportUploads(nil); err != nil {
		t.Fatalf("no files: %v", err)
	}
	if err := validateSupportUploads([]SupportUpload{ok, {Name: "b.jpg", Data: testJPEG}}); err != nil {
		t.Fatalf("valid files: %v", err)
	}
	six := []SupportUpload{ok, ok, ok, ok, ok, ok}
	if err := validateSupportUploads(six); !errors.Is(err, ErrSupportTooManyFiles) {
		t.Fatalf("6 files: %v", err)
	}
	big := SupportUpload{Data: append(append([]byte{}, testPNG...), make([]byte, SupportMaxFileBytes)...)}
	if err := validateSupportUploads([]SupportUpload{big}); !errors.Is(err, ErrSupportFileTooLarge) {
		t.Fatalf("big file: %v", err)
	}
	nine := SupportUpload{Data: append(append([]byte{}, testPNG...), make([]byte, 9<<20)...)}
	if err := validateSupportUploads([]SupportUpload{nine, nine, nine}); !errors.Is(err, ErrSupportFileTooLarge) {
		t.Fatalf("27 MB total: %v", err)
	}
	if err := validateSupportUploads([]SupportUpload{{Name: "x.png", Data: []byte("%PDF-1.7")}}); !errors.Is(err, ErrSupportUnsupportedMedia) {
		t.Fatalf("pdf renamed to png: %v", err)
	}
	if err := validateSupportUploads([]SupportUpload{{Name: "x.png"}}); !errors.Is(err, ErrSupportUnsupportedMedia) {
		t.Fatalf("empty file: %v", err)
	}
}

func TestNormalizeSupportTextAllowsEmptyOnlyWithImages(t *testing.T) {
	if _, err := normalizeSupportText("  ", false); !errors.Is(err, ErrSupportInvalidMsg) {
		t.Fatalf("empty text without images: %v", err)
	}
	if got, err := normalizeSupportText("  ", true); err != nil || got != "" {
		t.Fatalf("empty text with images: %q %v", got, err)
	}
	if got, _ := normalizeSupportText(" привет ", false); got != "привет" {
		t.Fatalf("trim: %q", got)
	}
}

func TestAttachmentsFromBotKeepsOnlyImages(t *testing.T) {
	w, h, size := 1280, 720, 1234
	name := "IMG_1.heic"
	got := attachmentsFromBot([]supportbot.AttachmentInfo{
		{ID: 10, Mime: "image/jpeg", Width: &w, Height: &h, Size: &size},
		{ID: 11, Mime: "IMAGE/HEIC", Name: &name},
		{ID: 12, Mime: "text/html"},
		{ID: 0, Mime: "image/png"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d attachments: %+v", len(got), got)
	}
	if got[0].SupportBotAttachmentID != 10 || *got[0].Width != 1280 || got[0].Name != nil {
		t.Fatalf("photo: %+v", got[0])
	}
	if got[1].Mime != "image/heic" || *got[1].Name != "IMG_1.heic" {
		t.Fatalf("heic: %+v", got[1])
	}
}

func TestBridgeSendErrMapsFileRejections(t *testing.T) {
	cases := []struct {
		err  error
		want error
	}{
		{&supportbot.StatusError{Status: 413, Body: `{"error": "file_too_large"}`}, ErrSupportFileTooLarge},
		{&supportbot.StatusError{Status: 415, Body: `{"error": "unsupported_media"}`}, ErrSupportUnsupportedMedia},
		{&supportbot.StatusError{Status: 400, Body: `{"error": "too_many_files"}`}, ErrSupportTooManyFiles},
		{&supportbot.StatusError{Status: 502, Body: `{"error": "failed"}`}, ErrSupportBridge},
		{fmt.Errorf("dial tcp: refused"), ErrSupportBridge},
	}
	for _, c := range cases {
		if got := bridgeSendErr(c.err); !errors.Is(got, c.want) {
			t.Errorf("%v: got %v, want %v", c.err, got, c.want)
		}
	}
}

func TestMessageDTOCarriesAttachments(t *testing.T) {
	w := 800
	m := repository.SupportMessage{ID: 1, Direction: repository.SupportMsgOut, CreatedAt: time.Unix(0, 0)}
	dto := messageDTO(m, []repository.SupportAttachment{{ID: 5, Mime: "image/jpeg", Width: &w}})
	if len(dto.Attachments) != 1 || dto.Attachments[0].ID != 5 || *dto.Attachments[0].Width != 800 {
		t.Fatalf("attachments: %+v", dto.Attachments)
	}
	if dto.Text != "" || dto.AuthorLabel != "Поддержка" {
		t.Fatalf("dto: %+v", dto)
	}
	if plain := messageDTO(m, nil); plain.Attachments != nil {
		t.Fatalf("message without images must omit attachments: %+v", plain.Attachments)
	}
}
