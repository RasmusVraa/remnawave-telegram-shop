package handlers

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/adminalert"
	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/cabinet/repository"
	"remnawave-tg-shop-bot/internal/cabinetnotify"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

// reviewNotifier — оповещения о отзывах: админу в Telegram и клиенту в колокольчик.
type reviewNotifier struct{}

func newReviewNotifier() *reviewNotifier { return &reviewNotifier{} }

// notifyAdmin шлёт админу «новый отзыв» или «отзыв изменён» по маршруту вида
// «Отзывы». Выключенный вид или событие — тихо ничего. Отправка в фоне: клиент
// не должен ждать Telegram.
func (n *reviewNotifier) notifyAdmin(ctx context.Context, created bool, rev *repository.Review) {
	if n == nil || rev == nil {
		return
	}
	event, key := config.ReviewsEventEdited, "admin_review_edited"
	if created {
		event, key = config.ReviewsEventNew, "admin_review_new"
	}
	if !config.ReviewsNotifyEvent(event) {
		return
	}
	route, ok := adminalert.Resolve(config.AdminAlertReviews)
	if !ok {
		return
	}
	lang := config.DefaultLanguage()
	tm := translation.GetInstance()
	name := strings.TrimSpace(rev.DisplayName)
	if name == "" {
		name = tm.GetText(lang, "admin_review_anonymous")
	}
	status := ""
	if rev.Status == repository.ReviewStatusPending {
		status = " " + tm.GetText(lang, "admin_review_pending")
	}
	snippet := []rune(strings.TrimSpace(rev.Body))
	if len(snippet) > 300 {
		snippet = append(snippet[:299], '…')
	}
	text := fmt.Sprintf(tm.GetText(lang, key), html.EscapeString(name), rev.Rating, status, html.EscapeString(string(snippet)))

	markup := reviewsAdminMarkup(tm.GetText(lang, "admin_review_open"), route.Direct)
	utils.Go("review notify admin", func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := adminalert.Send(c, config.AdminAlertReviews, text, markup); err != nil {
			slog.Warn("review notify: admin send failed", "review_id", rev.ID, "error", err)
		}
	})
}

// notifyReply кладёт в колокольчик клиента «команда ответила на ваш отзыв».
func (n *reviewNotifier) notifyReply(ctx context.Context, customerID int64, lang string, rev *repository.Review) {
	if n == nil || rev == nil || customerID <= 0 {
		return
	}
	snippet := []rune(strings.TrimSpace(rev.ReplyBody))
	if len(snippet) > 300 {
		snippet = append(snippet[:299], '…')
	}
	cabinetnotify.Notify(ctx, customerID, lang, cabinetnotify.Event{
		Kind:     database.CabinetNotificationSupport,
		TitleKey: "cabinet_notify_review_reply_title",
		BodyHTML: html.EscapeString(string(snippet)),
		Buttons:  []string{"reviews"},
		// Ответ правят: уведомление по каждой правке не плодим, но новый текст — новое.
		DedupeKey: fmt.Sprintf("review-reply:%d:%d", rev.ID, rev.ReplyAt.Unix()),
	})
}

// reviewsAdminMarkup — кнопка «Открыть в админке». В личке это Mini App: страница
// открывается силами Telegram, а не в браузере. В группах Telegram кнопки Mini App
// не принимает (сообщение отвергается целиком), поэтому там — обычная ссылка.
func reviewsAdminMarkup(label string, direct bool) models.ReplyMarkup {
	const path = "/cabinet/admin/reviews"
	var btn models.InlineKeyboardButton
	if u := handler.BuildCabinetWebAppURL(path); direct && u != "" {
		btn = models.InlineKeyboardButton{Text: label, WebApp: &models.WebAppInfo{URL: u}}
	} else if pub := strings.TrimRight(strings.TrimSpace(cabcfg.PublicURL()), "/"); pub != "" {
		btn = models.InlineKeyboardButton{Text: label, URL: pub + path}
	} else {
		return nil
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{btn}}}
}
