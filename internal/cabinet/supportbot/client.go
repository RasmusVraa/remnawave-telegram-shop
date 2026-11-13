package supportbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout = 15 * time.Second
	// Отправка файлов: support-bot сам заливает их в Telegram, это дольше текста.
	uploadTimeout = 60 * time.Second
	// Bot API отдаёт файлы до 20 МБ; запас — на заголовки и округление.
	maxDownloadBytes = 21 << 20
)

// ErrNotFound — support-bot не нашёл вложение (или оно не этого аккаунта).
var ErrNotFound = errors.New("supportbot: not found")

// StatusError — support-bot ответил не 2xx. Body — его JSON вида {"error": "<код>"}.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("supportbot: status %d: %s", e.Status, e.Body)
}

type Client struct {
	baseURL      string
	secret       string
	httpClient   *http.Client
	uploadClient *http.Client
}

func NewClient(baseURL, secret string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:  strings.TrimSpace(secret),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		uploadClient: &http.Client{
			Timeout: uploadTimeout,
		},
	}
}

type PostMessageRequest struct {
	AccountID           int64  `json:"account_id"`
	ShopTicketID        int64  `json:"shop_ticket_id"`
	IsNewTicket         bool   `json:"is_new_ticket"`
	ClientMessageID     string `json:"client_message_id"` // UUID idempotency key
	DisplayName         string `json:"display_name"`
	TelegramID          *int64 `json:"telegram_id,omitempty"`
	TelegramLabel       string `json:"telegram_label"`
	Email               string `json:"email"`
	SubscriptionSummary string `json:"subscription_summary"`
	Text                string `json:"text"`

	// CustomerID и RemnawaveUserID — для /profile в саппорт-боте. У web-only
	// клиента в панели нет telegramId, и найти его профиль бот может только по id.
	CustomerID      *int64 `json:"customer_id,omitempty"`
	RemnawaveUserID *int64 `json:"remnawave_user_id,omitempty"`
}

// AttachmentInfo — изображение в support-bot: ответ на сообщение с файлами и поле
// attachments в webhook. ID — id вложения в support-bot (для FetchAttachment).
type AttachmentInfo struct {
	ID     int64   `json:"id"`
	Mime   string  `json:"mime"`
	Width  *int    `json:"width,omitempty"`
	Height *int    `json:"height,omitempty"`
	Size   *int    `json:"size,omitempty"`
	Name   *string `json:"name,omitempty"`
}

type PostMessageResponse struct {
	SupportBotTicketID int64            `json:"support_bot_ticket_id"`
	ThreadID           int64            `json:"thread_id"`
	Attachments        []AttachmentInfo `json:"attachments"`
}

// UploadFile — файл клиента для PostCabinetMessageWithFiles.
type UploadFile struct {
	Name string
	Data []byte
}

func (c *Client) PostCabinetMessage(ctx context.Context, req PostMessageRequest) (*PostMessageResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return c.postMessage(ctx, c.httpClient, "application/json", body)
}

// PostCabinetMessageWithFiles — то же сообщение multipart'ом: часть payload (JSON)
// и части files в порядке показа.
func (c *Client) PostCabinetMessageWithFiles(ctx context.Context, req PostMessageRequest, files []UploadFile) (*PostMessageResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pw, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="payload"`},
		"Content-Type":        {"application/json"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := pw.Write(payload); err != nil {
		return nil, err
	}
	for _, f := range files {
		fw, err := mw.CreateFormFile("files", f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(f.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return c.postMessage(ctx, c.uploadClient, mw.FormDataContentType(), buf.Bytes())
}

func (c *Client) postMessage(ctx context.Context, hc *http.Client, contentType string, body []byte) (*PostMessageResponse, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("supportbot: client not configured")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/cabinet/message", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+c.secret)

	resp, err := hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("supportbot: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{Status: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}
	var out PostMessageResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("supportbot: decode response: %w", err)
	}
	return &out, nil
}

// FetchAttachment скачивает изображение вложения. accountID — владелец тикета:
// support-bot проверяет его сам и на чужое отвечает 404 (ErrNotFound).
func (c *Client) FetchAttachment(ctx context.Context, attachmentID, accountID int64) ([]byte, string, error) {
	if c == nil || c.baseURL == "" {
		return nil, "", fmt.Errorf("supportbot: client not configured")
	}
	u := c.baseURL + "/internal/cabinet/file/" + strconv.FormatInt(attachmentID, 10) +
		"?account_id=" + strconv.FormatInt(accountID, 10)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.secret)

	resp, err := c.uploadClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("supportbot: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return nil, "", &StatusError{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("supportbot: read file: %w", err)
	}
	if len(data) > maxDownloadBytes {
		return nil, "", fmt.Errorf("supportbot: file too large")
	}
	return data, resp.Header.Get("Content-Type"), nil
}
