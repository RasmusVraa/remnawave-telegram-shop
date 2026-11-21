package handlers

import (
	"context"
	"log/slog"
	"strings"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/database"
)

// ReviewIdentity — имя и аватар автора для отзыва. Telegram-аватар не превращаем
// в ссылку: достаём его позже по id отзыва, поэтому храним только telegram id.
func (h *MeHandler) ReviewIdentity(ctx context.Context, accountID int64) reviewAuthor {
	var ids []repository.Identity
	if h.ids != nil {
		if list, err := h.ids.ListByAccount(ctx, accountID); err == nil {
			ids = list
		}
	}
	var telegramID *int64
	if tid := h.telegramIDForAccount(ctx, accountID); tid != nil {
		telegramID = tid
	}
	var cust = h.customerForIdentity(ctx, accountID)
	p := h.resolveIdentityProfile(ctx, accountID, ids, cust, telegramID)

	out := reviewAuthor{Name: strings.TrimSpace(p.DisplayName)}
	if out.Name == "" {
		out.Name = strings.TrimSpace(p.Username)
	}
	switch av := strings.TrimSpace(p.AvatarURL); {
	case av == "":
	case strings.HasPrefix(av, "/cabinet/api/avatar"):
		// Своя отдача Telegram-аватарки: сохраняем только владельца.
		if telegramID != nil {
			out.AvatarKind, out.TelegramID = repository.ReviewAvatarTelegram, telegramID
			out.PreviewURL = av
		}
	default:
		out.AvatarKind, out.AvatarURL, out.PreviewURL = repository.ReviewAvatarURL, av, av
	}
	return out
}

func (h *MeHandler) customerForIdentity(ctx context.Context, accountID int64) *database.Customer {
	if h.links == nil || h.customers == nil {
		return nil
	}
	link, err := h.links.FindByAccountID(ctx, accountID)
	if err != nil || link == nil {
		return nil
	}
	cust, err := h.customers.FindById(ctx, link.CustomerID)
	if err != nil {
		return nil
	}
	return cust
}

// ReviewAvatar скачивает аватар Telegram для публичной отдачи.
func (h *MeHandler) ReviewAvatar(ctx context.Context, telegramID int64) ([]byte, string, string, bool) {
	if h.tgProfiles == nil {
		return nil, "", "", false
	}
	a, ok, err := h.tgProfiles.Avatar(ctx, telegramID)
	if err != nil {
		slog.Warn("review avatar: fetch failed", "error", err.Error())
		return nil, "", "", false
	}
	if !ok {
		return nil, "", "", false
	}
	return a.Body, a.ContentType, a.ETag, true
}
