package handlers

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/cabinet/auth/jwt"
	"remnawave-tg-shop-bot/internal/cabinet/http/middleware"
	cabsvc "remnawave-tg-shop-bot/internal/cabinet/service"
)

type testPart struct {
	field, filename string
	data            []byte
}

func supportMultipartRequest(t *testing.T, parts ...testPart) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		var w interface{ Write([]byte) (int, error) }
		var err error
		if p.filename != "" {
			w, err = mw.CreateFormFile(p.field, p.filename)
		} else {
			w, err = mw.CreateFormField(p.field)
		}
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(p.data)
	}
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/cabinet/api/support/messages", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestReadSupportMultipart(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nxxxx")
	r := supportMultipartRequest(t,
		testPart{field: "text", data: []byte("вот скрин")},
		testPart{field: "files", filename: "a.png", data: png},
		testPart{field: "files", filename: "b.png", data: png},
		testPart{field: "other", data: []byte("ignored")},
	)
	text, files, err := readSupportMultipart(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatal(err)
	}
	if text != "вот скрин" || len(files) != 2 || files[1].Name != "b.png" || !bytes.Equal(files[0].Data, png) {
		t.Fatalf("text=%q files=%+v", text, files)
	}
}

func TestReadSupportMultipartLimits(t *testing.T) {
	one := testPart{field: "files", filename: "a.png", data: []byte("x")}
	six := []testPart{one, one, one, one, one, one}
	if _, _, err := readSupportMultipart(httptest.NewRecorder(), supportMultipartRequest(t, six...)); !errors.Is(err, cabsvc.ErrSupportTooManyFiles) {
		t.Fatalf("6 files: %v", err)
	}

	big := testPart{field: "files", filename: "big.png", data: make([]byte, cabsvc.SupportMaxFileBytes+1)}
	if _, _, err := readSupportMultipart(httptest.NewRecorder(), supportMultipartRequest(t, big)); !errors.Is(err, cabsvc.ErrSupportFileTooLarge) {
		t.Fatalf("big file: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/cabinet/api/support/messages", bytes.NewReader([]byte("{}")))
	r.Header.Set("Content-Type", "application/json")
	if _, _, err := readSupportMultipart(httptest.NewRecorder(), r); !errors.Is(err, cabsvc.ErrSupportInvalidMsg) {
		t.Fatalf("not multipart: %v", err)
	}
}

func TestSupportSendMultipartRejectsTooManyFiles(t *testing.T) {
	h := NewSupport(cabsvc.NewSupport(nil, nil, nil, nil, nil, nil, nil, nil))
	one := testPart{field: "files", filename: "a.png", data: []byte("\x89PNG\r\n\x1a\n")}
	r := supportMultipartRequest(t, one, one, one, one, one, one)
	r = r.WithContext(middleware.SetClaims(r.Context(), &jwt.Claims{AccountID: 1}))
	w := httptest.NewRecorder()
	h.SendMessage(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "too many files") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestSupportAttachmentRejectsBadID(t *testing.T) {
	h := NewSupport(cabsvc.NewSupport(nil, nil, nil, nil, nil, nil, nil, nil))
	r := httptest.NewRequest(http.MethodGet, "/cabinet/api/support/attachments/abc", nil)
	r.SetPathValue("id", "abc")
	r = r.WithContext(middleware.SetClaims(r.Context(), &jwt.Claims{AccountID: 1}))
	w := httptest.NewRecorder()
	h.Attachment(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Attachment(w, httptest.NewRequest(http.MethodGet, "/cabinet/api/support/attachments/1", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no claims: status %d", w.Code)
	}
}
