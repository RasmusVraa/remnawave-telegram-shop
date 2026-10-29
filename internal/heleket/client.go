package heleket

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // G501: подпись Heleket определена их API как md5
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.heleket.com"
	DefaultTimeout = 30 * time.Second

	// DefaultCurrency — номинал счёта. Рубль, чтобы purchase.amount оставался
	// рублёвым как у остальных касс и статистика не требовала конвертации.
	DefaultCurrency = "RUB"

	// DefaultLifetime — сколько живёт счёт, секунды. Heleket принимает 300…43200.
	DefaultLifetime = 3600

	minLifetime = 300
	maxLifetime = 43200

	// maxAdditionalData — ограничение Heleket на additional_data.
	maxAdditionalData = 255
)

type Client struct {
	baseURL     string
	merchantID  string
	apiKey      string
	callbackURL string
	currency    string
	orderPrefix string
	lifetime    int
	httpClient  *http.Client
}

// NewClient собирает клиента Heleket.
//
// callbackURL — полный https-адрес вебхука; пустой означает «Heleket не зовёт
// нас обратно», и подтверждение оплаты остаётся целиком на поллинге.
func NewClient(merchantID, apiKey, callbackURL, currency, orderPrefix string, lifetime int) *Client {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = DefaultCurrency
	}
	if lifetime <= 0 {
		lifetime = DefaultLifetime
	}
	if lifetime < minLifetime {
		lifetime = minLifetime
	}
	if lifetime > maxLifetime {
		lifetime = maxLifetime
	}
	return &Client{
		baseURL:     DefaultBaseURL,
		merchantID:  strings.TrimSpace(merchantID),
		apiKey:      strings.TrimSpace(apiKey),
		callbackURL: strings.TrimSpace(callbackURL),
		currency:    currency,
		orderPrefix: normalizePrefix(orderPrefix),
		lifetime:    lifetime,
		httpClient:  &http.Client{Timeout: DefaultTimeout},
	}
}

func (c *Client) IsConfigured() bool {
	return c != nil && c.merchantID != "" && c.apiKey != ""
}

// OrderPrefix — префикс order_id, отделяющий счета этого стенда от чужих
// на том же мерчанте.
func (c *Client) OrderPrefix() string {
	if c == nil {
		return DefaultOrderPrefix
	}
	return c.orderPrefix
}

// Lifetime — сколько живёт счёт, секунды.
func (c *Client) Lifetime() int {
	if c == nil || c.lifetime <= 0 {
		return DefaultLifetime
	}
	return c.lifetime
}

// Currency — валюта счёта, в которой считается amount.
func (c *Client) Currency() string {
	if c == nil || c.currency == "" {
		return DefaultCurrency
	}
	return c.currency
}

// marshalBody сериализует тело ровно так, как оно уйдёт в сеть.
//
// Подпись считается от этих же байтов, поэтому маршалим один раз и больше не
// пересобираем: json.Encoder по умолчанию экранирует < > & в \uXXXX, и повторный
// маршалинг легко даёт другую строку — а значит другую подпись.
func marshalBody(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Sign — подпись Heleket: md5( base64(тело) + API_KEY ).
func Sign(body []byte, apiKey string) string {
	encoded := base64.StdEncoding.EncodeToString(body)
	sum := md5.Sum([]byte(encoded + apiKey)) //nolint:gosec // G401: см. импорт
	return hex.EncodeToString(sum[:])
}

func (c *Client) post(ctx context.Context, path string, payload any, out any) error {
	if !c.IsConfigured() {
		return fmt.Errorf("heleket client not configured")
	}
	body, err := marshalBody(payload)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("merchant", c.merchantID)
	req.Header.Set("sign", Sign(body, c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
		// Причина отказа живёт в теле: «Payment not found» приезжает с 422,
		// а не с 404, и по одному статусу её не опознать.
		var envelope apiResponse
		if json.Unmarshal(respBody, &envelope) == nil {
			apiErr.Message = envelope.Message
			apiErr.State = envelope.State
		}
		return apiErr
	}

	var envelope apiResponse
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}

// CreatePayment создаёт счёт и возвращает uuid + ссылку на страницу оплаты.
//
// orderID — идентификатор на нашей стороне (id покупки). Он же делает вызов
// идемпотентным: повторный запрос с тем же order_id возвращает тот же счёт.
func (c *Client) CreatePayment(ctx context.Context, orderID, amount, description, returnURL string) (*Payment, error) {
	req := &CreatePaymentRequest{
		Amount:      amount,
		Currency:    c.Currency(),
		OrderID:     orderID,
		URLCallback: c.callbackURL,
		Lifetime:    c.lifetime,
	}
	if description != "" {
		req.AdditionalData = truncate(description, maxAdditionalData)
	}
	if returnURL != "" {
		req.URLReturn = returnURL
		req.URLSuccess = returnURL
	}

	var payment Payment
	if err := c.post(ctx, "/v1/payment", req, &payment); err != nil {
		return nil, fmt.Errorf("create heleket payment failed: %w", err)
	}
	if strings.TrimSpace(payment.UUID) == "" || strings.TrimSpace(payment.URL) == "" {
		return nil, fmt.Errorf("heleket did not return a payment link")
	}
	return &payment, nil
}

// GetPaymentInfo — статус счёта по uuid или order_id. Это единственный источник
// правды о статусе: и вебхук, и поллинг сверяются именно с ним.
//
// Возвращает (nil, nil), если счёта у мерчанта нет.
func (c *Client) GetPaymentInfo(ctx context.Context, uuid, orderID string) (*Payment, error) {
	uuid = strings.TrimSpace(uuid)
	orderID = strings.TrimSpace(orderID)
	if uuid == "" && orderID == "" {
		return nil, fmt.Errorf("heleket: uuid or order_id required")
	}
	req := infoRequest{}
	if uuid != "" {
		req.UUID = uuid
	} else {
		req.OrderID = orderID
	}

	var payment Payment
	if err := c.post(ctx, "/v1/payment/info", req, &payment); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.IsPaymentNotFound() {
			return nil, nil
		}
		return nil, fmt.Errorf("get heleket payment info failed: %w", err)
	}
	if strings.TrimSpace(payment.UUID) == "" && strings.TrimSpace(payment.OrderID) == "" {
		return nil, nil
	}
	return &payment, nil
}

// VerifyCallback проверяет подпись вебхука.
//
// Подпись считается от тела запроса без поля sign, поэтому вырезаем это поле
// прямо из сырых байтов: пересобрать JSON через map нельзя — Go сортирует ключи
// при маршалинге, и порядок полей (а с ним и подпись) поедет. Плюс PHP на
// стороне Heleket экранирует слэши как \/, и сырые байты это сохраняют.
//
// Проверка не единственная защита: статус всё равно перезапрашивается через
// GetPaymentInfo, поэтому подделанный колбэк ничего не зачисляет.
func VerifyCallback(rawBody []byte, sign, apiKey string) bool {
	sign = strings.TrimSpace(sign)
	if sign == "" || apiKey == "" {
		return false
	}
	stripped, ok := stripSignField(rawBody)
	if !ok {
		return false
	}
	calc := Sign(stripped, apiKey)
	return subtle.ConstantTimeCompare([]byte(calc), []byte(sign)) == 1
}

// stripSignField убирает пару "sign":"…" вместе с одной прилегающей запятой,
// оставляя остальные байты нетронутыми.
//
// Ключ ищется строго на верхнем уровне объекта: наивный поиск первого вхождения
// подстроки "sign" вырезал бы вложенный ключ (например {"a":{"sign":…},"sign":…})
// и ломался бы о строковое значение "sign" в соседнем поле.
func stripSignField(body []byte) ([]byte, bool) {
	keyStart, valueEnd, ok := findTopLevelSign(body)
	if !ok {
		return nil, false
	}
	start, end := keyStart, valueEnd

	// Снимаем ровно одну запятую — идущую следом, иначе предшествующую.
	j := skipJSONSpace(body, end)
	if j < len(body) && body[j] == ',' {
		end = j + 1
	} else {
		k := start
		for k > 0 && isJSONSpace(body[k-1]) {
			k--
		}
		if k > 0 && body[k-1] == ',' {
			start = k - 1
		}
	}

	out := make([]byte, 0, len(body)-(end-start))
	out = append(out, body[:start]...)
	out = append(out, body[end:]...)
	return out, true
}

// findTopLevelSign возвращает границы пары "sign": <значение> на верхнем уровне.
// Попутно проверяет, что тело — синтаксически целый JSON-объект.
func findTopLevelSign(body []byte) (keyStart, valueEnd int, ok bool) {
	i := skipJSONSpace(body, 0)
	if i >= len(body) || body[i] != '{' {
		return 0, 0, false
	}
	i++

	found := false
	i = skipJSONSpace(body, i)
	if i < len(body) && body[i] == '}' {
		return 0, 0, false
	}

	for {
		i = skipJSONSpace(body, i)
		if i >= len(body) || body[i] != '"' {
			return 0, 0, false
		}
		ks := i
		ke, okStr := scanJSONString(body, i)
		if !okStr {
			return 0, 0, false
		}
		isSign := string(body[ks:ke]) == `"sign"`

		i = skipJSONSpace(body, ke)
		if i >= len(body) || body[i] != ':' {
			return 0, 0, false
		}
		i = skipJSONSpace(body, i+1)

		ve, okVal := skipJSONValue(body, i)
		if !okVal {
			return 0, 0, false
		}
		if isSign && !found {
			keyStart, valueEnd, found = ks, ve, true
		}

		i = skipJSONSpace(body, ve)
		if i >= len(body) {
			return 0, 0, false
		}
		switch body[i] {
		case ',':
			i++
		case '}':
			return keyStart, valueEnd, found
		default:
			return 0, 0, false
		}
	}
}

// scanJSONString принимает индекс открывающей кавычки и возвращает индекс сразу
// за закрывающей.
func scanJSONString(b []byte, i int) (int, bool) {
	if i >= len(b) || b[i] != '"' {
		return 0, false
	}
	i++
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1, true
		default:
			i++
		}
	}
	return 0, false
}

// skipJSONValue пропускает одно значение любого типа и возвращает индекс за ним.
func skipJSONValue(b []byte, i int) (int, bool) {
	if i >= len(b) {
		return 0, false
	}
	switch b[i] {
	case '"':
		return scanJSONString(b, i)
	case '{', '[':
		open, closeCh := b[i], byte('}')
		if open == '[' {
			closeCh = ']'
		}
		depth := 0
		for i < len(b) {
			switch b[i] {
			case '"':
				ne, ok := scanJSONString(b, i)
				if !ok {
					return 0, false
				}
				i = ne
				continue
			case open:
				depth++
			case closeCh:
				depth--
				if depth == 0 {
					return i + 1, true
				}
			}
			i++
		}
		return 0, false
	default:
		// Число, true/false/null — читаем до разделителя.
		start := i
		for i < len(b) && b[i] != ',' && b[i] != '}' && b[i] != ']' && !isJSONSpace(b[i]) {
			i++
		}
		if i == start {
			return 0, false
		}
		return i, true
	}
}

func skipJSONSpace(b []byte, i int) int {
	for i < len(b) && isJSONSpace(b[i]) {
		i++
	}
	return i
}

func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// FormatAmount — сумма для Heleket: две цифры после точки, точка разделителем.
func FormatAmount(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}
