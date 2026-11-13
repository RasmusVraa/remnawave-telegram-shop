package adminevents

import (
	"context"
	"log/slog"
	"time"

	"remnawave-tg-shop-bot/internal/config"
)

// RunPurge — суточная очистка по срокам хранения из настроек.
func (s *Store) RunPurge(ctx context.Context) {
	n, err := s.Purge(ctx, time.Now(),
		config.MonitoringRetentionDays(),
		config.MonitoringNoisyRetentionDays(),
		config.MonitoringBotActionsRetentionDays())
	if err != nil {
		slog.Error("admin events: purge", "error", err, "deleted", n)
		return
	}
	if n > 0 {
		slog.Info("admin events: purged old rows", "rows", n)
	}
}

// expiredScanWindow — окно поиска истёкших подписок. Задача идёт раз в час,
// окно чуть шире, чтобы не терять клиентов на стыке; повторы отсекает запрос.
const expiredScanWindow = 65 * time.Minute

// RunExpiredScan пишет «подписка истекла» по каждому, у кого она закончилась
// за последний час.
func (s *Store) RunExpiredScan(ctx context.Context, now time.Time) {
	list, err := s.ExpiredSince(ctx, now.Add(-expiredScanWindow), now)
	if err != nil {
		slog.Error("admin events: expired scan", "error", err)
		return
	}
	batch := make([]Event, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.Insert(ctx, batch); err != nil {
			slog.Error("admin events: expired insert", "error", err, "size", len(batch))
		}
		batch = batch[:0]
	}
	for _, c := range list {
		payload := map[string]any{"expired_at": c.ExpireAt.UTC().Format(time.RFC3339)}
		if c.TariffID != nil {
			payload["tariff_id"] = *c.TariffID
		}
		batch = append(batch, Event{
			Type: SubscriptionExpired, Level: LevelImportant, CustomerID: c.CustomerID,
			Source: SourceSystem, Payload: payload, At: c.ExpireAt,
		})
		if len(batch) >= batchSize {
			flush()
		}
	}
	flush()
}

// RunExpiredDigest пишет одну сводную строку за сутки: сколько истекло и сколько
// не продлили. Ноль истёкших — строки нет.
func (s *Store) RunExpiredDigest(ctx context.Context, now time.Time) {
	expired, notRenewed, err := s.ExpiredDigest(ctx, now.Add(-24*time.Hour))
	if err != nil {
		slog.Error("admin events: expired digest", "error", err)
		return
	}
	if expired == 0 {
		return
	}
	Emit(Event{
		Type: SubscriptionExpiredDigest, Source: SourceSystem,
		Payload: map[string]any{"expired": expired, "not_renewed": notRenewed},
	})
}
