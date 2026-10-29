package moynalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

var (
	ErrAuth      = errors.New("authentication error")
	ErrRetryable = errors.New("retryable error")
	ErrClient    = errors.New("client error")
)

// Client представляет клиент для работы с API МойНалог
type Client struct {
	httpClient *http.Client
	baseURL    string

	username string
	password string

	token atomic.Value

	authMu       sync.Mutex
	authInFlight bool
	authCond     *sync.Cond
}

// NewClient создает новый клиент МойНалог
func NewClient(baseURL, username, password, proxyURL string) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: newHTTPTransport(proxyURL),
		},
		baseURL:  baseURL,
		username: username,
		password: password,
	}
	c.authCond = sync.NewCond(&c.authMu)

	// Автоматическая аутентификация при создании клиента
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.Authenticate(ctx); err != nil {
		slog.Error("Failed to authenticate moynalog client on startup", "error", err)
		// Не паникуем, чтобы не сломать запуск приложения, если МойНалог недоступен
	}

	return c
}

func newHTTPTransport(proxyURL string) *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyURL == "" {
		return transport
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		slog.Error("Invalid moynalog proxy URL, proxy disabled", "error", err)
		return transport
	}

	switch strings.ToLower(parsedURL.Scheme) {
	case "socks5", "socks5h":
		if parsedURL.Host == "" {
			slog.Error("Moynalog proxy host is empty, proxy disabled", "proxyURL", proxyURL)
			return transport
		}

		var auth *proxy.Auth
		if parsedURL.User != nil {
			password, _ := parsedURL.User.Password()
			auth = &proxy.Auth{
				User:     parsedURL.User.Username(),
				Password: password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			slog.Error("Failed to initialize SOCKS5 proxy, proxy disabled", "error", err)
			return transport
		}

		if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
			transport.DialContext = ctxDialer.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	default:
		transport.Proxy = http.ProxyURL(parsedURL)
	}

	return transport
}

// Authenticate выполняет аутентификацию и сохраняет токен
// forceReauth - если true, принудительно выполняет переаутентификацию даже если токен есть
func (c *Client) Authenticate(ctx context.Context) error {
	return c.authenticate(ctx, false)
}

// authenticate выполняет аутентификацию с возможностью принудительной переаутентификации
func (c *Client) authenticate(ctx context.Context, forceReauth bool) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	// Если уже идет авторизация, ждем её завершения
	for c.authInFlight {
		c.authCond.Wait()
	}

	// Проверяем, не авторизовались ли мы пока ждали (только если не принудительная переаутентификация)
	if !forceReauth {
		if token := c.token.Load(); token != nil && token.(string) != "" {
			return nil
		}
	}

	c.authInFlight = true
	defer func() {
		c.authInFlight = false
		c.authCond.Broadcast()
	}()

	authURL := fmt.Sprintf("%s/auth/lkfl", c.baseURL)

	deviceInfo := DeviceInfo{
		SourceDeviceId: "*",
		SourceType:     "WEB",
		AppVersion:     "1.0.0",
		MetaDetails: MetaDetails{
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 YaBrowser/24.12.0 Safari/537.36",
		},
	}

	authRequest := AuthRequest{
		Username:   c.username,
		Password:   c.password,
		DeviceInfo: deviceInfo,
	}

	reqBody, err := json.Marshal(authRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	c.token.Store(authResp.Token)

	slog.Info("Moynalog client authenticated successfully")
	return nil
}

// ensureAuthenticated проверяет и обновляет токен при необходимости
func (c *Client) ensureAuthenticated(ctx context.Context) error {
	if token := c.token.Load(); token != nil && token.(string) != "" {
		return nil
	}

	return c.Authenticate(ctx)
}

// isRetryableError проверяет, можно ли повторить запрос при этой ошибке
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		// Temporary устарел, но отделяет обрывы соединения от постоянных
		// ошибок — без него чеки при сбое сети не переотправлялись бы.
		return netErr.Timeout() || netErr.Temporary() //nolint:staticcheck // SA1019
	}

	return errors.Is(err, ErrRetryable) || errors.Is(err, ErrAuth)
}

// CreateIncome создает чек о доходе и возвращает присвоенный ФНС идентификатор.
//
// operationTime — время самой оплаты (purchase.paid_at), а НЕ время вызова.
// Это принципиально для очереди повторов: чек, отправленный после многодневного
// простоя ФНС, должен зарегистрироваться днём оплаты, иначе доход уедет в другой
// день, а на стыке месяцев — в другой налоговый период.
func (c *Client) CreateIncome(ctx context.Context, amount float64, description string, operationTime time.Time) (string, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	incomeURL := fmt.Sprintf("%s/income", c.baseURL)

	service := Service{
		Name:     description,
		Amount:   amount,
		Quantity: 1,
	}

	client := IncomeClient{
		ContactPhone: nil,
		DisplayName:  nil,
		INN:          nil,
		IncomeType:   "FROM_INDIVIDUAL",
	}

	incomeRequest := CreateIncomeRequest{
		OperationTime:                   operationTime,
		RequestTime:                     time.Now(),
		Services:                        []Service{service},
		TotalAmount:                     fmt.Sprintf("%.2f", amount),
		Client:                          client,
		PaymentType:                     "CASH",
		IgnoreMaxTotalIncomeRestriction: false,
	}

	reqBody, err := json.Marshal(incomeRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal income request: %w", err)
	}

	const (
		maxRetries     = 3
		baseDelay      = 500 * time.Millisecond
		maxAuthRetries = 2
	)

	var (
		lastErr     error
		authRetries int
	)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Проверяем отмену контекста
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		receiptID, err := c.sendIncomeRequest(ctx, incomeURL, reqBody, amount)
		if err == nil {
			return receiptID, nil
		}

		lastErr = err
		slog.Warn("Failed to create income", "attempt", attempt, "maxRetries", maxRetries, "error", err)

		// Если это ошибка авторизации, очищаем токен и пытаемся переавторизоваться
		if errors.Is(err, ErrAuth) {
			if authRetries >= maxAuthRetries {
				return "", fmt.Errorf("max authentication retries exceeded: %w", err)
			}

			slog.Info("Authentication error detected, clearing token and reauthenticating", "attempt", attempt, "authRetries", authRetries+1)
			// Очищаем истекший токен перед переаутентификацией
			c.token.Store("")
			// Принудительно выполняем переаутентификацию
			if authErr := c.authenticate(ctx, true); authErr != nil {
				return "", fmt.Errorf("reauthentication failed: %w", authErr)
			}

			authRetries++
			// Не считаем переаутентификацию как отдельную попытку
			attempt--
			continue
		}

		// Если это не retryable ошибка, возвращаем ошибку сразу
		if !isRetryableError(err) {
			return "", fmt.Errorf("non-retryable error: %w", err)
		}

		// Задержка перед следующей попыткой (если не последняя попытка)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(baseDelay * time.Duration(1<<(attempt-1))):
			}
		}
	}

	return "", fmt.Errorf("failed to create income after %d attempts: %w", maxRetries, lastErr)
}

// sendIncomeRequest отправляет запрос на создание чека и возвращает его ID.
func (c *Client) sendIncomeRequest(ctx context.Context, incomeURL string, reqBody []byte, amount float64) (string, error) {
	token := c.token.Load()
	if token == nil {
		return "", fmt.Errorf("%w: token is empty", ErrAuth)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", incomeURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.(string)))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 YaBrowser/24.12.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Проверяем сетевые ошибки
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() || netErr.Temporary() { //nolint:staticcheck // SA1019, см. isRetryableError
				slog.Warn("Network error (retryable)", "error", err, "timeout", netErr.Timeout(), "temporary", netErr.Temporary()) //nolint:staticcheck // SA1019
				return "", fmt.Errorf("%w: %w", ErrRetryable, err)
			}
		}
		slog.Error("Network error (non-retryable)", "error", err)
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		b, _ := io.ReadAll(resp.Body)
		slog.Warn("Authentication error from moynalog", "status", resp.StatusCode, "body", string(b))
		return "", fmt.Errorf("%w: status %d: %s", ErrAuth, resp.StatusCode, b)

	case resp.StatusCode >= 500:
		b, _ := io.ReadAll(resp.Body)
		slog.Warn("Server error from moynalog", "status", resp.StatusCode, "body", string(b))
		return "", fmt.Errorf("%w: status %d: %s", ErrRetryable, resp.StatusCode, b)

	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated:
		b, _ := io.ReadAll(resp.Body)
		slog.Error("Client error from moynalog", "status", resp.StatusCode, "body", string(b))
		return "", fmt.Errorf("%w: status %d: %s", ErrClient, resp.StatusCode, b)
	}

	var incomeResp CreateIncomeResponse
	if err := json.NewDecoder(resp.Body).Decode(&incomeResp); err != nil {
		return "", err
	}

	slog.Info("Income receipt created successfully", "id", incomeResp.ID, "amount", amount)
	return incomeResp.ID, nil
}
