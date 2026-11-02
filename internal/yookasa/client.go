package yookasa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/remnawave"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrPaymentNotFound = errors.New("yookasa payment not found")

type YookasaAPI interface {
	CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error)
	GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
}

// ctxKey изолирует ключи контекста от внешних пакетов.
type ctxKey string

const (
	// CtxKeyReturnURL — если задан в контексте, используется как
	// confirmation.return_url вместо config.BotURL(). Используется web-кабинетом
	// для возврата пользователя на /cabinet/payment/status/:id.
	CtxKeyReturnURL ctxKey = "yookasa.return_url"
	// CtxKeyReceiptEmail — если задан, переопределяет receipt.customer.email
	// (по умолчанию config.YookasaEmail()). Используется кабинетом для
	// отправки чека на email клиента, когда он подтверждён.
	CtxKeyReceiptEmail ctxKey = "yookasa.receipt_email"
	// CtxKeyIdempotenceKey — если задан, используется как HTTP-заголовок
	// Idempotence-Key. Без override клиент продолжает генерировать uuid,
	// чтобы не ломать существующие вызовы из бота.
	CtxKeyIdempotenceKey ctxKey = "yookasa.idempotence_key"
	// CtxKeyPaymentMethod — способ ЮKassa, выбранный покупателем (id из
	// config.YookasaPaymentMethodIDs). PaymentService сверяет его с настройкой
	// YOOKASA_PAYMENT_METHODS и кладёт обратно уже итоговый код: клиент
	// передаёт его в payment_method_data как есть, пусто — не передаёт.
	CtxKeyPaymentMethod ctxKey = "yookasa.payment_method"
)

// PaymentMethodFromCtx — способ ЮKassa из контекста, либо пустая строка.
func PaymentMethodFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyPaymentMethod).(string); ok {
		return v
	}
	return ""
}

// returnURLFromCtx возвращает переопределение из контекста, либо пустую строку.
func returnURLFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyReturnURL).(string); ok {
		return v
	}
	return ""
}

func receiptEmailFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyReceiptEmail).(string); ok {
		return v
	}
	return ""
}

func idempotenceKeyFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyIdempotenceKey).(string); ok {
		return v
	}
	return ""
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	authHeader string
}

func NewClient(baseURL, shopID, secretKey string) *Client {
	auth := fmt.Sprintf("%s:%s", shopID, secretKey)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	return &Client{
		httpClient: newHTTPClient(),
		baseURL:    baseURL,
		authHeader: fmt.Sprintf("Basic %s", encodedAuth),
	}
}

// newHTTPClient собирает HTTP-клиент для API ЮKassa, при необходимости через прокси.
//
// Прокси задаётся YOOKASA_PROXY_URL и применяется ТОЛЬКО к запросам ЮKassa.
// Глобальный HTTPS_PROXY здесь не подходит: он увёл бы в прокси и вызовы панели
// Remnawave по внутреннему docker-DNS, которые снаружи не резолвятся.
//
// Схемы: http, https, socks5 (в т.ч. с логином/паролем в URL) — их понимает
// http.Transport напрямую. Таймаут ограничивает всю операцию, включая TLS:
// без него зависший proxy держал бы HTTP-хендлер кабинета до его собственного таймаута.
func newHTTPClient() *http.Client {
	const timeout = 30 * time.Second

	raw := config.YookasaProxyURL()
	if raw == "" {
		return &http.Client{Timeout: timeout}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		slog.Error("Invalid YOOKASA_PROXY_URL, falling back to direct connection", "error", err)
		return &http.Client{Timeout: timeout}
	}
	// url.Parse не считает ошибкой строку без схемы ("host:1080"), но http.Transport
	// на таком значении завалит КАЖДЫЙ запрос с "unsupported protocol scheme".
	// Тихая деградация до прямого соединения понятнее, чем сломанные платежи.
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		slog.Error("YOOKASA_PROXY_URL has unsupported scheme, falling back to direct connection",
			"scheme", parsed.Scheme, "hint", "ожидается http://, https:// или socks5://")
		return &http.Client{Timeout: timeout}
	}

	slog.Info("YooKassa requests go through proxy", "scheme", parsed.Scheme, "host", parsed.Host)
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(parsed),
		},
	}
}

func (c *Client) CreateInvoice(ctx context.Context, amount int, description string, customerId int64, purchaseId int64) (*Payment, error) {
	rub := Amount{
		Value:    strconv.Itoa(amount),
		Currency: "RUB",
	}

	email := receiptEmailFromCtx(ctx)
	if email == "" {
		email = config.YookasaEmail()
	}
	receipt := &Receipt{
		Customer: &Customer{Email: email},
		Items: []Item{
			{
				VatCode:        1,
				Quantity:       "1",
				Description:    description,
				Amount:         rub,
				PaymentSubject: "payment",
				PaymentMode:    "full_payment",
			},
		},
	}

	username, _ := ctx.Value(remnawave.CtxKeyUsername).(string)
	metaData := map[string]any{
		"customerId": customerId,
		"purchaseId": purchaseId,
		"username":   username,
	}

	returnURL := returnURLFromCtx(ctx)
	if returnURL == "" {
		returnURL = config.BotURL()
	}

	paymentRequest := NewPaymentRequest(
		rub,
		returnURL,
		description,
		receipt,
		metaData,
	)
	if method := PaymentMethodFromCtx(ctx); method != "" && method != config.YookasaPaymentMethodAll {
		paymentRequest.PaymentMethodData = &PaymentMethodData{Type: method}
	}

	idempotencyKey := idempotenceKeyFromCtx(ctx)
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}

	payment, err := c.CreatePayment(ctx, paymentRequest, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

func (c *Client) CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error) {
	paymentURL := fmt.Sprintf("%s/payments", c.baseURL)

	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", paymentURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error while reading invoice resp: %w", err)
		}
		return nil, fmt.Errorf("API return error. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var payment Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &payment, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	paymentURL := fmt.Sprintf("%s/payments/%s", c.baseURL, paymentID)

	var payment *Payment

	maxRetries := 5
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", paymentURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", c.authHeader)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			payment = new(Payment)
			if err := json.NewDecoder(resp.Body).Decode(payment); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return payment, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryDelay := baseDelay * time.Duration(1<<attempt)
			log.Printf("Received 429 Too Many Requests. Retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w (status %d)", ErrPaymentNotFound, resp.StatusCode)
		}

		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("exceeded maximum retries due to 429 Too Many Requests")
}
