package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// IsCabinetTelegramMinimalismActive exported версия для использования вне пакета handler
func IsCabinetTelegramMinimalismActive() bool {
	return cabinetTelegramMinimalismActive()
}

// cabinetTelegramMinimalismActive — кабинет включён, в env выбран minimalism и собирается WebApp URL.
func cabinetTelegramMinimalismActive() bool {
	if !cabcfg.IsEnabled() {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(cabcfg.TelegramUIMode()), "minimalism") {
		return false
	}
	if cabinetWebAppURL("/cabinet/tariffs") == "" {
		return false
	}
	return true
}

// buildCabinetMinimalismCoreRows — по одной кнопке в ряд.
// Без SubscriptionLink: сверху «Попробовать бесплатно» (как в classic), затем тарифы, поддержка, инфо — без кнопки «Подписка».
// С SubscriptionLink: купить (тарифы), подписка, поддержка, инфо.
func (h Handler) buildCabinetMinimalismCoreRows(langCode string, customer *database.Customer) [][]models.InlineKeyboardButton {
	var kb [][]models.InlineKeyboardButton
	add := func(btn models.InlineKeyboardButton) {
		kb = append(kb, []models.InlineKeyboardButton{btn})
	}
	noSub := customer == nil || customer.SubscriptionLink == nil
	if noSub && config.TrialDays() > 0 {
		add(h.resolveTrialButton(langCode, "cabinet_minimal_btn_trial"))
	}
	if buy := cabinetWebAppURL("/cabinet/tariffs"); buy != "" {
		add(h.translation.WithButton(langCode, "cabinet_minimal_btn_buy", models.InlineKeyboardButton{
			WebApp: &models.WebAppInfo{URL: buy},
		}))
	}
	if !noSub {
		if sub := cabinetWebAppURL("/cabinet/dashboard"); sub != "" {
			add(h.translation.WithButton(langCode, "cabinet_minimal_btn_subscription", models.InlineKeyboardButton{
				WebApp: &models.WebAppInfo{URL: sub},
			}))
		}
	}
	if su := strings.TrimSpace(config.SupportURL()); su != "" {
		add(h.translation.WithButton(langCode, "cabinet_minimal_btn_support", models.InlineKeyboardButton{URL: su}))
	} else if w := cabinetWebAppURL("/cabinet/support"); w != "" {
		add(h.translation.WithButton(langCode, "cabinet_minimal_btn_support", models.InlineKeyboardButton{
			WebApp: &models.WebAppInfo{URL: w},
		}))
	}
	if w := cabinetWebAppURL("/cabinet/support#cabinet-info"); w != "" {
		add(h.translation.WithButton(langCode, "cabinet_minimal_btn_info", models.InlineKeyboardButton{
			WebApp: &models.WebAppInfo{URL: w},
		}))
	}
	if row := h.cabinetMinimalismExternalLinkRow(langCode); len(row) > 0 {
		kb = append(kb, row)
	}
	return kb
}

// cabinetMinimalismExternalLinkRow — опциональные URL-кнопки «Отзывы» / «Канал» внизу меню.
// 1 кнопка в ряд, если включена одна; 2 в ряд, если включены обе (и заданы URL).
func (h Handler) cabinetMinimalismExternalLinkRow(langCode string) []models.InlineKeyboardButton {
	urls := selectCabinetMinimalismExternalURLs(
		cabcfg.TelegramShowFeedbackButton(),
		cabcfg.TelegramShowChannelButton(),
		config.FeedbackURL(),
		config.ChannelURL(),
	)
	if len(urls) == 0 {
		return nil
	}
	row := make([]models.InlineKeyboardButton, 0, len(urls))
	for _, item := range urls {
		row = append(row, h.translation.WithButton(langCode, item.key, models.InlineKeyboardButton{URL: item.url}))
	}
	return row
}

type cabinetMinimalismExternalURL struct {
	key string
	url string
}

// selectCabinetMinimalismExternalURLs — чистая сборка ссылок для ряда (порядок: отзывы → канал).
func selectCabinetMinimalismExternalURLs(showFeedback, showChannel bool, feedbackURL, channelURL string) []cabinetMinimalismExternalURL {
	var out []cabinetMinimalismExternalURL
	if showFeedback {
		if u := strings.TrimSpace(feedbackURL); u != "" {
			out = append(out, cabinetMinimalismExternalURL{key: "cabinet_minimal_btn_feedback", url: u})
		}
	}
	if showChannel {
		if u := strings.TrimSpace(channelURL); u != "" {
			out = append(out, cabinetMinimalismExternalURL{key: "cabinet_minimal_btn_channel", url: u})
		}
	}
	return out
}

func (h Handler) buildCabinetMinimalismGreetingHTML(_ context.Context, customer *database.Customer, langCode, displayName string) string {
	tm := h.translation
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = tm.GetText(langCode, "vpn_username_unknown")
	}
	now := time.Now()
	var b strings.Builder
	b.WriteString(fmt.Sprintf(tm.GetText(langCode, "cabinet_minimal_user_line"), escapeHTML(name)))
	b.WriteString("\n\n")

	active := customer != nil && customer.ExpireAt != nil && customer.ExpireAt.After(now)
	if customer == nil {
		b.WriteString(tm.GetText(langCode, "cabinet_minimal_status_none"))
	} else if active {
		b.WriteString(tm.GetText(langCode, "cabinet_minimal_status_active"))
	} else if customer.ExpireAt != nil {
		b.WriteString(tm.GetText(langCode, "cabinet_minimal_status_expired"))
	} else {
		b.WriteString(tm.GetText(langCode, "cabinet_minimal_status_none"))
	}
	b.WriteString("\n")

	dateStr := tm.GetText(langCode, "cabinet_minimal_date_placeholder")
	if customer != nil && customer.ExpireAt != nil {
		dateStr = customer.ExpireAt.Format("02.01.2006")
	}
	b.WriteString(fmt.Sprintf(tm.GetText(langCode, "cabinet_minimal_date_line"), dateStr))

	if active && customer != nil && customer.ExpireAt != nil {
		dLeft := daysLeft(*customer.ExpireAt, now)
		const expiringWithin = 14
		if dLeft > 0 && dLeft <= expiringWithin {
			b.WriteString("\n")
			b.WriteString(fmt.Sprintf(tm.GetText(langCode, "cabinet_minimal_expiring"), dLeft))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(tm.GetText(langCode, "cabinet_minimal_footer"))
	return b.String()
}
