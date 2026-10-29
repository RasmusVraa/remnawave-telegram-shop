package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/sync"
)

func (h Handler) SyncUsersCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	lang := update.Message.From.LanguageCode
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   h.syncReport(ctx, lang),
	})
	if err != nil {
		slog.Error("Error sending sync message", "error", err)
	}
}

// syncReport запускает синхронизацию и возвращает текст для админа: итог или
// причину, по которой она не выполнена.
func (h Handler) syncReport(ctx context.Context, lang string) string {
	res, err := h.syncService.Sync(ctx)
	counts := fmt.Sprintf(h.translation.GetText(lang, "admin_sync_counts"), res.Created, res.Updated, res.Deleted, res.Kept)
	switch {
	case err == nil:
		return h.translation.GetText(lang, "admin_sync_done") + "\n" + counts
	case errors.Is(err, sync.ErrSyncRunning):
		return h.translation.GetText(lang, "admin_sync_running")
	case errors.Is(err, database.ErrSyncTooManyDeletions):
		slog.Warn("sync: deletion skipped by safety guard", "error", err)
		return h.translation.GetText(lang, "admin_sync_delete_blocked") + "\n" + counts
	default:
		slog.Error("sync failed", "error", err)
		return h.translation.GetText(lang, "admin_sync_failed")
	}
}
