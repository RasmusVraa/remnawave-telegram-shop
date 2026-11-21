package handlers

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/cabinet/repository"
)

func sampleReview() *repository.Review {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return &repository.Review{
		ID: 7, Rating: 4, Body: "Всё работает", Status: repository.ReviewStatusPublished,
		Visibility: repository.ReviewVisibilityNameAvatar, DisplayName: "Мария",
		AvatarKind: repository.ReviewAvatarTelegram, ClientSince: &since,
		CreatedAt: now, UpdatedAt: now, PublishedAt: &now,
	}
}

// Аноним не светит ни имя, ни аватар, даже если они сохранены в строке.
func TestToPublicReviewAnonymousHidesAuthor(t *testing.T) {
	rev := sampleReview()
	rev.Visibility = repository.ReviewVisibilityAnonymous
	dto := toPublicReview(rev)
	if dto.AuthorName != nil || dto.AuthorAvatarURL != nil {
		t.Fatalf("аноним раскрыт: %+v", dto)
	}
}

// Аватар Telegram отдаётся ссылкой по id отзыва, без токена с сроком жизни.
func TestToPublicReviewAvatarIsStableURL(t *testing.T) {
	dto := toPublicReview(sampleReview())
	if dto.AuthorAvatarURL == nil || !strings.HasPrefix(*dto.AuthorAvatarURL, "/cabinet/api/public/reviews/7/avatar") {
		t.Fatalf("ссылка на аватар: %v", dto.AuthorAvatarURL)
	}
	if strings.Contains(*dto.AuthorAvatarURL, "t=") {
		t.Fatalf("в ссылке не должно быть токена: %s", *dto.AuthorAvatarURL)
	}
}

func TestToPublicReviewTrialAndReply(t *testing.T) {
	rev := sampleReview()
	rev.TariffLabel = ReviewTrialLabel
	at := rev.CreatedAt.Add(time.Hour)
	rev.ReplyBody, rev.ReplyAt = "Спасибо!", &at
	dto := toPublicReview(rev)
	if !dto.IsTrial || dto.TariffLabel != "" {
		t.Fatalf("пробный: %+v", dto)
	}
	if dto.Reply == nil || dto.Reply.Body != "Спасибо!" {
		t.Fatalf("ответ команды потерян: %+v", dto.Reply)
	}
	if dto.ClientSince == nil || *dto.ClientSince != "2026-08-01" {
		t.Fatalf("клиент с: %v", dto.ClientSince)
	}
}

func TestReviewIDFromPath(t *testing.T) {
	cases := []struct {
		path, suffix string
		id           int64
		ok           bool
	}{
		{"/cabinet/api/public/reviews/12/avatar", "avatar", 12, true},
		{"/cabinet/api/public/reviews/12/other", "avatar", 0, false},
		{"/cabinet/api/public/reviews/abc/avatar", "avatar", 0, false},
		{"/cabinet/api/public/reviews/", "avatar", 0, false},
	}
	for _, c := range cases {
		id, ok := reviewIDFromPath(c.path, c.suffix)
		if id != c.id || ok != c.ok {
			t.Errorf("%s: got %d,%v want %d,%v", c.path, id, ok, c.id, c.ok)
		}
	}
}
