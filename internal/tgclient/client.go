// Package tgclient — HTTP-клиент для Bot API с общим лимитом отправки и
// повтором после 429.
//
// Telegram ограничивает бота целиком (около 30 сообщений в секунду на всех
// получателей), а не каждую рассылку отдельно. Рассылка держала свои 29/с, но
// параллельно шли сообщения после оплаты, напоминания об окончании подписки и
// lifecycle-уведомления — без всякого учёта. Превышение Telegram отвечает 429 с
// retry_after, а бот засчитывал такое сообщение как ошибку: получатель его не
// получал никогда.
//
// Клиент ставится в bot.WithHTTPClient, поэтому покрывает все отправки
// процесса сразу.
package tgclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// SendsPerSecond — общий потолок отправки сообщений, с запасом от 30/с.
	SendsPerSecond = 25
	// maxRetries — сколько раз повторяем запрос после 429.
	maxRetries = 3
	// maxRetryAfter — дольше не ждём: такой ответ — уже не всплеск, а бан на
	// время, и держать обработчик минутами хуже, чем вернуть ошибку.
	maxRetryAfter = 30 * time.Second
)

// Doer — то, что принимает bot.WithHTTPClient.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client ограничивает частоту отправки сообщений и повторяет запросы после 429.
type Client struct {
	base    Doer
	limiter *rate.Limiter
	sleep   func(context.Context, time.Duration) error
}

// New оборачивает base. perSecond — общий лимит методов отправки.
func New(base Doer, perSecond int) *Client {
	return &Client{
		base:    base,
		limiter: rate.NewLimiter(rate.Limit(perSecond), perSecond),
		sleep:   sleepCtx,
	}
}

// isSend — метод отправляет сообщение в чат и попадает под лимит Telegram.
// getUpdates, ответы на callback и правки сообщений лимитом не тормозим:
// иначе во время рассылки бот медленно отвечал бы на кнопки.
func isSend(method string) bool {
	return strings.HasPrefix(method, "send") ||
		strings.HasPrefix(method, "copyMessage") ||
		strings.HasPrefix(method, "forwardMessage")
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	method := path.Base(req.URL.Path)
	if method == "getUpdates" {
		return c.base.Do(req)
	}

	// Тело у go-telegram/bot — поток из io.Pipe: для повтора его нужно держать.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	ctx := req.Context()
	for attempt := 0; ; attempt++ {
		if isSend(method) {
			if err := c.limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}
		r := req.Clone(ctx)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))

		resp, err := c.base.Do(r)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= maxRetries {
			return resp, err
		}

		// Тело ответа читаем, чтобы узнать retry_after, и подкладываем обратно:
		// если повторять не будем, библиотека разберёт 429 как обычно.
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		if readErr != nil {
			return resp, nil
		}
		wait := retryAfter(respBody)
		if wait <= 0 || wait > maxRetryAfter {
			return resp, nil
		}
		slog.Warn("telegram: 429, retrying", "method", method, "retry_after", wait, "attempt", attempt+1)
		if err := c.sleep(ctx, wait); err != nil {
			return resp, nil
		}
	}
}

func retryAfter(body []byte) time.Duration {
	var r struct {
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(body, &r) != nil {
		return 0
	}
	return time.Duration(r.Parameters.RetryAfter) * time.Second
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
