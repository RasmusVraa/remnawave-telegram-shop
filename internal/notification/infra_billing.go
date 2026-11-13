package notification

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/adminalert"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
)

// InfraBillingNotifyService — ежедневные напоминания админу о предстоящей оплате нод (инфра-биллинг панели).
type InfraBillingNotifyService struct {
	remnawave *remnawave.Client
	repo      *database.InfraBillingRepository
	bot       *bot.Bot
	tm        *translation.Manager
}

func NewInfraBillingNotifyService(
	rw *remnawave.Client,
	repo *database.InfraBillingRepository,
	b *bot.Bot,
	tm *translation.Manager,
) *InfraBillingNotifyService {
	return &InfraBillingNotifyService{remnawave: rw, repo: repo, bot: b, tm: tm}
}

func calendarDaysUntilBilling(now, until time.Time) int {
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	untilDate := time.Date(until.Year(), until.Month(), until.Day(), 0, 0, 0, 0, until.Location())
	return int(untilDate.Sub(nowDate).Hours() / 24)
}

// ProcessInfraBillingReminders вызывается из cron (например, 16:00 UTC, как подписки).
func (s *InfraBillingNotifyService) ProcessInfraBillingReminders(ctx context.Context) error {
	if s == nil || s.repo == nil || s.remnawave == nil || s.bot == nil || s.tm == nil {
		return nil
	}
	st, err := s.repo.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("infra billing settings: %w", err)
	}
	if !st.NotifyBefore1 && !st.NotifyBefore3 && !st.NotifyBefore7 && !st.NotifyBefore14 {
		return nil
	}
	// Вид «Оплата серверов» выключен в «Оповещениях админу» — не шлём и не
	// отмечаем отправленным: после включения напоминание ещё придёт.
	if _, ok := adminalert.Resolve(config.AdminAlertInfra); !ok {
		return nil
	}

	nodesBody, err := s.remnawave.GetInfraBillingNodes(ctx)
	if err != nil {
		return fmt.Errorf("infra billing nodes: %w", err)
	}

	lang := config.DefaultLanguage()
	now := time.Now()

	thresholds := make([]int, 0, 4)
	if st.NotifyBefore1 {
		thresholds = append(thresholds, 1)
	}
	if st.NotifyBefore3 {
		thresholds = append(thresholds, 3)
	}
	if st.NotifyBefore7 {
		thresholds = append(thresholds, 7)
	}
	if st.NotifyBefore14 {
		thresholds = append(thresholds, 14)
	}

	for i := range nodesBody.BillingNodes {
		bn := &nodesBody.BillingNodes[i]
		next := bn.NextBillingAt
		daysLeft := calendarDaysUntilBilling(now, next)

		for _, th := range thresholds {
			if daysLeft != th {
				continue
			}
			sent, err := s.repo.WasNotifySent(ctx, bn.UUID, next, th)
			if err != nil {
				slog.Error("infra billing was sent check", "error", err)
				continue
			}
			if sent {
				continue
			}

			nodeName := html.EscapeString(bn.Node.Name)
			provName := html.EscapeString(bn.Provider.Name)
			dateStr := next.In(now.Location()).Format("02.01.2006")
			text := fmt.Sprintf(
				s.tm.GetText(lang, "admin_infra_billing_cron_body"),
				th,
				nodeName,
				provName,
				dateStr,
			)

			if err := adminalert.Send(ctx, config.AdminAlertInfra, text, nil); err != nil {
				slog.Error("infra billing notify send", "billing_uuid", bn.UUID, "threshold", th, "error", err)
				continue
			}
			if err := s.repo.MarkNotifySent(ctx, bn.UUID, next, th); err != nil {
				slog.Error("infra billing mark sent", "error", err)
			}
		}
	}
	return nil
}
