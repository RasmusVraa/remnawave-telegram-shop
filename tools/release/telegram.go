package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/proxy"
)

const telegramMaxMessageRunes = 4096

type telegramConfig struct {
	Token           string
	ChatID          string
	MessageThreadID int
	Footer          TelegramFooter
	ProxyURL        string
}

type sendMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	MessageThreadID       int    `json:"message_thread_id,omitempty"`
}

type telegramAPIResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// sendTelegramHTML отправляет пост и возвращает его message_id: на него
// отвечает альбом скриншотов.
func sendTelegramHTML(cfg telegramConfig, html string) (int, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return 0, fmt.Errorf("RELEASE_TG_BOT_TOKEN is empty")
	}
	if strings.TrimSpace(cfg.ChatID) == "" {
		return 0, fmt.Errorf("RELEASE_TG_CHAT_ID is empty")
	}
	if strings.TrimSpace(html) == "" {
		return 0, fmt.Errorf("telegram message is empty")
	}
	if n := utf8.RuneCountInString(html); n > telegramMaxMessageRunes {
		return 0, fmt.Errorf("telegram message is %d runes (limit %d); shorten Telegram section in RELEASE_NOTES.md", n, telegramMaxMessageRunes)
	}

	reqBody := sendMessageRequest{
		ChatID:                cfg.ChatID,
		Text:                  html,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}
	if cfg.MessageThreadID > 0 {
		reqBody.MessageThreadID = cfg.MessageThreadID
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal telegram request: %w", err)
	}

	result, err := callTelegram(cfg, "sendMessage", "application/json", payload)
	if err != nil {
		return 0, err
	}
	var msg struct {
		MessageID int `json:"message_id"`
	}
	// Пост уже ушёл: без message_id альбом просто не станет ответом на него.
	_ = json.Unmarshal(result, &msg)
	return msg.MessageID, nil
}

// callTelegram вызывает метод Bot API и возвращает поле result ответа.
func callTelegram(cfg telegramConfig, method, contentType string, payload []byte) (json.RawMessage, error) {
	resp, err := postTelegram(cfg, method, contentType, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read telegram response: %w", err)
	}

	var apiResp telegramAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode telegram response: %w (status=%d body=%s)", err, resp.StatusCode, truncate(string(body), 200))
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("telegram API error (%s): %s", method, apiResp.Description)
	}
	return apiResp.Result, nil
}

// postTelegram отправляет запрос через прокси, а если тот недоступен — напрямую.
//
// Прокси на машине релиза может быть не поднят (или мешать включённый VPN), и тогда
// без фоллбэка релиз просто не публикуется. При этом откат делается ТОЛЬКО когда
// запрос заведомо не дошёл до Telegram: ошибка подключения к самому прокси или
// невозможность его разобрать. На любую другую ошибку — включая таймаут и любой
// ответ API — повтора нет: запрос мог долететь, и вторая попытка отправила бы
// анонс в канал дважды.
func postTelegram(cfg telegramConfig, method, contentType string, payload []byte) (*http.Response, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", cfg.Token, method)

	attempt := func(proxyURL string) (*http.Response, error) {
		client, err := newTelegramHTTPClient(proxyURL)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build telegram request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
		return client.Do(req)
	}

	resp, err := attempt(cfg.ProxyURL)
	if err == nil {
		return resp, nil
	}

	proxyConfigured := strings.TrimSpace(cfg.ProxyURL) != ""
	if !proxyConfigured || !isProxyUnreachable(err) {
		// Не оборачиваем ошибку net/http: в ней полный URL с токеном бота.
		return nil, fmt.Errorf("telegram %s failed (proxy=%q): %s",
			method, emptyAs(cfg.ProxyURL, "none"), sanitizeDialError(err))
	}

	fmt.Printf("telegram: прокси %s недоступен, пробую напрямую\n", cfg.ProxyURL)
	resp, err = attempt("")
	if err != nil {
		return nil, fmt.Errorf("telegram %s failed (прокси %q недоступен, прямое соединение тоже): %s",
			method, cfg.ProxyURL, sanitizeDialError(err))
	}
	return resp, nil
}

// isProxyUnreachable отличает «не смогли подключиться к прокси» от прочих сбоев.
//
// Признак должен быть узким: если ошибка возникла уже после того, как запрос ушёл
// в сеть, повторять отправку нельзя.
func isProxyUnreachable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "proxyconnect"): // http/https proxy: соединение с прокси не установилось
		return true
	case strings.Contains(msg, "socks connect"): // socks5: то же самое
		return true
	case strings.Contains(msg, "unsupported protocol scheme"): // прокси задан с опечаткой в схеме
		return true
	default:
		return false
	}
}

func newTelegramHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("RELEASE_TG_PROXY_URL: %w", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			transport.Proxy = http.ProxyURL(parsed)
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if parsed.User != nil {
				pass, _ := parsed.User.Password()
				auth = &proxy.Auth{
					User:     parsed.User.Username(),
					Password: pass,
				}
			}
			dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("RELEASE_TG_PROXY_URL socks5: %w", err)
			}
			transport.Proxy = nil
			if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
				transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return ctxDialer.DialContext(ctx, network, addr)
				}
			} else {
				transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				}
			}
		default:
			return nil, fmt.Errorf("RELEASE_TG_PROXY_URL: unsupported scheme %q (use http/https/socks5)", parsed.Scheme)
		}
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

func sanitizeDialError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Strip accidental token leakage if present.
	if i := strings.Index(msg, "/bot"); i >= 0 {
		rest := msg[i+4:]
		if j := strings.Index(rest, "/"); j >= 0 {
			msg = msg[:i+4] + "<redacted>" + rest[j:]
		}
	}
	return msg
}

func parseThreadID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("RELEASE_TG_MESSAGE_THREAD_ID: %w", err)
	}
	if id < 0 {
		return 0, fmt.Errorf("RELEASE_TG_MESSAGE_THREAD_ID must be >= 0")
	}
	return id, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
