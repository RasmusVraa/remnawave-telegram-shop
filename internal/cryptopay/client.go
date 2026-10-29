package cryptopay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CryptoPayApi interface {
	CreateInvoice(invoiceReq *InvoiceRequest) (*InvoiceResponse, error)
	GetInvoices(status, fiat, asset, invoiceIds string, offset, limit int) (*[]InvoiceResponse, error)
}

// ctxKey изолирует ключи контекста от внешних пакетов.
type ctxKey string

const (
	// CtxKeyPaidBtnURL — если задан, используется вместо config.BotURL() как
	// paid_btn_url в CryptoPay-инвойсе. Используется web-кабинетом, чтобы после
	// оплаты пользователь возвращался на /cabinet/payment/status/:id.
	CtxKeyPaidBtnURL ctxKey = "cryptopay.paid_btn_url"
)

// PaidBtnURLFromCtx читает переопределение paid_btn_url, если оно проставлено.
func PaidBtnURLFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(CtxKeyPaidBtnURL).(string); ok {
		return v
	}
	return ""
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// httpTimeout — без него зависший ответ CryptoPay держал бы проход поллера бесконечно.
const httpTimeout = 30 * time.Second

func NewCryptoPayClient(url string, tokn string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: httpTimeout},
		baseURL:    url,
		token:      tokn,
	}
}

func (c *Client) CreateInvoice(invoiceReq *InvoiceRequest) (*InvoiceResponse, error) {
	jsonData, err := json.Marshal(invoiceReq)
	if err != nil {
		return nil, fmt.Errorf("error marshaling invoice: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/createInvoice", c.baseURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error while creating invoice req: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Crypto-Pay-API-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error while making invoice req: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error while reading invoice resp: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API return error. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponseWrapper[InvoiceResponse]
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("error while unmarshiling response: %w", err)
	}

	if !apiResp.Ok {
		return nil, fmt.Errorf("API create failed: %v", apiResp.Ok)
	}

	return &apiResp.Result, nil
}

func (c *Client) GetInvoices(status, fiat, asset, invoiceIds string, offset, limit int) (*[]InvoiceResponse, error) {
	endpoint := fmt.Sprintf("%s/api/getInvoices", c.baseURL)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error while creating request: %w", err)
	}

	q := req.URL.Query()

	if status != "" {
		q.Add("status", status)
	}

	if offset > 0 {
		q.Add("offset", fmt.Sprintf("%d", offset))
	}

	if limit > 0 {
		q.Add("limit", fmt.Sprintf("%d", limit))
	}

	if invoiceIds != "" {
		q.Add("invoice_ids", invoiceIds)
	}

	if fiat != "" {
		q.Add("fiat", fiat)
	}

	if asset != "" {
		q.Add("asset", asset)
	}

	req.URL.RawQuery = q.Encode()
	req.Header.Set("Crypto-Pay-API-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error while making query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error while reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned error. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var apiResp ResponseListWrapper[InvoiceResponse]
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("error while unmarshaling json: %w", err)
	}

	if !apiResp.Ok {
		return nil, fmt.Errorf("API get invoices failed: %v", apiResp.Ok)
	}

	return &apiResp.Result.Items, nil
}
