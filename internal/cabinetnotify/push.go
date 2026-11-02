package cabinetnotify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/database"
)

// pushPayload — то, что service worker кабинета показывает системным уведомлением.
type pushPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Important bool   `json:"important,omitempty"`
	// Tag — повторный push с тем же тегом заменяет прежний, а не копится.
	Tag string `json:"tag,omitempty"`
	// URL — куда вести по нажатию: главная с открытой лентой.
	URL string `json:"url"`
	// Icon — значок уведомления: логотип бренда, как в манифесте PWA.
	Icon string `json:"icon,omitempty"`
}

const (
	pushWorkers = 8
	// pushTTL — сколько push-сервис держит сообщение для выключенного
	// устройства. Сутки: вчерашнее «перебои» утром уже не новость.
	pushTTL = 24 * 60 * 60
)

type pusher struct {
	repo   *database.CabinetNotificationRepository
	client *http.Client

	mu      sync.Mutex
	public  string
	private string
}

func newPusher(repo *database.CabinetNotificationRepository) *pusher {
	return &pusher{repo: repo, client: &http.Client{Timeout: 15 * time.Second}}
}

// keys — VAPID-ключи инсталляции: из базы, при первом обращении генерируются.
func (p *pusher) keys(ctx context.Context) (public, private string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.public != "" {
		return p.public, p.private, nil
	}
	public, private, err = p.repo.VapidKeys(ctx, func() (string, string, error) {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		return pub, priv, err
	})
	if err != nil {
		return "", "", err
	}
	p.public, p.private = public, private
	return public, private, nil
}

func (p *pusher) publicKey(ctx context.Context) (string, error) {
	pub, _, err := p.keys(ctx)
	return pub, err
}

// subscriber — контакт владельца в VAPID-подписи: push-сервисы пишут туда,
// если с рассылкой что-то не так. Адрес кабинета подходит, если он https.
func subscriber() string {
	u := strings.TrimSpace(cabcfg.PublicURL())
	if strings.HasPrefix(u, "https://") {
		return u
	}
	return "mailto:noreply@example.invalid"
}

func brandTitle() string {
	if b := strings.TrimSpace(cabcfg.BrandName()); b != "" {
		return b
	}
	return "VPN"
}

// sendNotification шлёт push всем подписанным браузерам получателей уведомления.
func (p *pusher) sendNotification(ctx context.Context, notificationID int64, payload pushPayload) {
	subs, err := p.repo.PushSubscriptionsForNotification(ctx, notificationID)
	if err != nil {
		slog.Error("cabinet push: subscriptions", "notification_id", notificationID, "error", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	public, private, err := p.keys(ctx)
	if err != nil {
		slog.Error("cabinet push: vapid keys", "error", err)
		return
	}
	if payload.URL == "" {
		payload.URL = "/cabinet/dashboard#notifications"
	}
	if payload.Icon == "" {
		payload.Icon = "/cabinet/favicon.svg"
		if u := strings.TrimSpace(cabcfg.BrandLogoURLForClient()); u != "" {
			payload.Icon = u
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	opts := &webpush.Options{
		HTTPClient:      p.client,
		Subscriber:      subscriber(),
		VAPIDPublicKey:  public,
		VAPIDPrivateKey: private,
		TTL:             pushTTL,
		Urgency:         webpush.UrgencyNormal,
	}
	if payload.Important {
		opts.Urgency = webpush.UrgencyHigh
	}

	jobs := make(chan database.CabinetPushSubscription)
	var wg sync.WaitGroup
	var sent, gone, failed int
	var countMu sync.Mutex
	for range min(pushWorkers, len(subs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sub := range jobs {
				res := p.sendOne(ctx, sub, body, opts)
				countMu.Lock()
				switch res {
				case pushSent:
					sent++
				case pushGone:
					gone++
				default:
					failed++
				}
				countMu.Unlock()
			}
		}()
	}
	for _, sub := range subs {
		jobs <- sub
	}
	close(jobs)
	wg.Wait()
	slog.Info("cabinet push sent", "notification_id", notificationID, "sent", sent, "gone", gone, "failed", failed)
}

type pushResult int

const (
	pushSent pushResult = iota
	pushGone
	pushFailed
)

func (p *pusher) sendOne(ctx context.Context, sub database.CabinetPushSubscription, body []byte, opts *webpush.Options) pushResult {
	resp, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, opts)
	if err != nil {
		slog.Warn("cabinet push: send", "subscription_id", sub.ID, "error", err)
		return pushFailed
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// Браузер отписался или переустановлен: подписка мертва навсегда.
		if err := p.repo.DeletePushSubscription(ctx, 0, sub.Endpoint); err != nil {
			slog.Warn("cabinet push: delete gone subscription", "error", err)
		}
		return pushGone
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		p.repo.TouchPushSubscription(ctx, sub.ID)
		return pushSent
	default:
		slog.Warn("cabinet push: rejected", "subscription_id", sub.ID, "status", resp.StatusCode)
		return pushFailed
	}
}
