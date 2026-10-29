package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

const (
	batchSize             = 29
	delayBetweenBatches   = time.Second
	adminResultMessageFmt = "✅ Рассылка завершена!\n\n📊 Статистика:\n• Всего пользователей: %d\n• Успешно отправлено: %d\n• Ошибок: %d\n• Из них заблокировали бота: %d"

	// Разметка одна на всех получателей, поэтому её поломка — не сбой доставки,
	// а ошибка в тексте: шлём админу причину, а не счётчик ошибок.
	adminAbortedMessageFmt = "❌ Рассылка остановлена: Telegram не разобрал разметку сообщения.\n\n%v\n\nПроверьте теги: незакрытый тег или символ < в обычном тексте ломают всё сообщение целиком."
)

// Sender отправляет массовые сообщения через Telegram-бота.
type Sender struct {
	customers *database.CustomerRepository
	// history — журнал рассылок; nil отключает запись (тесты, старые вызовы).
	history *database.BroadcastRepository
	tm      *translation.Manager
}

func NewSender(customers *database.CustomerRepository, history *database.BroadcastRepository, tm *translation.Manager) *Sender {
	return &Sender{customers: customers, history: history, tm: tm}
}

// SendOptions — как рассылку записать в историю.
type SendOptions struct {
	// Source — откуда запущена: database.BroadcastSourceCabinet или ...Telegram.
	Source string
	// BroadcastID — запись истории, заведённая заранее: кабинету её id нужен в
	// ответе, чтобы показывать ход отправки. 0 — Send заведёт запись сам.
	BroadcastID int64
	// MediaName — имя прикреплённого файла, только для истории.
	MediaName string
}

// HistoryRecord — запись истории для сообщения, которое собираются разослать.
func HistoryRecord(source string, audiences []string, tariffID *int64, msg Message, mediaName string) database.BroadcastRecord {
	rec := database.BroadcastRecord{
		Source:    source,
		Audiences: audiences,
		TariffID:  tariffID,
		Text:      msg.Text,
		ParseMode: string(msg.ParseMode),
		MediaName: mediaName,
	}
	if msg.Media != nil {
		rec.MediaKind = string(msg.Media.Kind)
		rec.MediaFileID = msg.Media.FileID
	}
	if buttons, err := json.Marshal(msg.Buttons); err == nil {
		rec.Buttons = buttons
	}
	return rec
}

// historyLog — запись хода одной рассылки. Сбой записи только логируется:
// из-за истории рассылка останавливаться не должна.
type historyLog struct {
	repo *database.BroadcastRepository
	id   int64
}

func (h historyLog) progress(ctx context.Context, c database.BroadcastCounters) {
	if h.repo == nil || h.id == 0 {
		return
	}
	if err := h.repo.UpdateProgress(ctx, h.id, c); err != nil {
		slog.Warn("broadcast: history progress", "id", h.id, "error", err)
	}
}

func (h historyLog) finish(ctx context.Context, status string, c database.BroadcastCounters, errText string) {
	if h.repo == nil || h.id == 0 {
		return
	}
	if err := h.repo.Finish(ctx, h.id, status, c, errText); err != nil {
		slog.Warn("broadcast: history finish", "id", h.id, "error", err)
	}
}

// Send рассылает сообщение выбранным сегментам аудитории; человек из нескольких
// сегментов получит его один раз. При adminID != 0 шлёт итог админу в Telegram.
// Ход и итог пишутся в историю рассылок.
func (s *Sender) Send(
	ctx context.Context,
	b *bot.Bot,
	adminID int64,
	audiences []string,
	tariffID *int64,
	msg Message,
	opts SendOptions,
) SendResult {
	result := SendResult{}
	if s == nil || s.customers == nil || b == nil {
		return result
	}

	history := historyLog{repo: s.history, id: opts.BroadcastID}
	if history.repo != nil && history.id == 0 {
		id, err := history.repo.Create(ctx, HistoryRecord(opts.Source, audiences, tariffID, msg, opts.MediaName))
		if err != nil {
			slog.Warn("broadcast: history create", "error", err)
		}
		history.id = id
	}

	recipients, err := s.customers.GetBroadcastRecipientsForAudiences(ctx, audiences, tariffID)
	if err != nil {
		slog.Error("broadcast: get recipients", "error", err, "audiences", audiences)
		history.finish(ctx, database.BroadcastStatusFailed, database.BroadcastCounters{}, err.Error())
		if adminID != 0 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: adminID,
				Text:   fmt.Sprintf("❌ Ошибка при получении списка пользователей: %v", err),
			})
		}
		return result
	}

	if len(recipients) == 0 {
		history.finish(ctx, database.BroadcastStatusFailed, database.BroadcastCounters{}, "no recipients")
		if adminID != 0 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: adminID,
				Text:   "❌ Нет пользователей для рассылки",
			})
		}
		return result
	}

	totalUsers := len(recipients)
	eligibleUsers := 0
	for _, rec := range recipients {
		if !utils.IsSyntheticTelegramID(rec.TelegramID) {
			eligibleUsers++
		}
	}
	sentCount := 0
	failedCount := 0
	// blockedCount — часть failedCount: получатель закрыл бота или удалил аккаунт.
	blockedCount := 0
	var aborted error
	counters := func() database.BroadcastCounters {
		return database.BroadcastCounters{
			Total:   eligibleUsers,
			Sent:    sentCount,
			Blocked: blockedCount,
			Failed:  failedCount - blockedCount,
		}
	}
	history.progress(ctx, counters())

	for i := 0; i < totalUsers; i += batchSize {
		end := i + batchSize
		if end > totalUsers {
			end = totalUsers
		}

		batch := recipients[i:end]
		for _, rec := range batch {
			if utils.IsSyntheticTelegramID(rec.TelegramID) {
				continue
			}
			markup := BuildReplyMarkup(s.tm, rec.Language, msg.Buttons, rec.TelegramID)
			_, sendErr := Deliver(ctx, b, rec.TelegramID, msg, markup)
			if sendErr != nil {
				slog.Warn("broadcast: send message", "userId", rec.TelegramID, "error", sendErr)
				failedCount++
				// 403 — «bot was blocked by the user» или удалённый аккаунт: до
				// этого человека не достучаться, и это не сбой рассылки.
				if errors.Is(sendErr, bot.ErrorForbidden) {
					blockedCount++
				}
				/*
				 * Битую разметку ловим на первом же получателе.
				 *
				 * Telegram отвергает всё сообщение целиком, если HTML не
				 * разбирается: незакрытый тег, случайный «<» в тексте. Раньше
				 * такая рассылка молча падала на всех пяти тысячах подряд и
				 * админ узнавал об этом из итоговой строки «ошибок: 5000».
				 * Дальше слать бессмысленно — текст один на всех.
				 */
				if sentCount == 0 && isParseError(sendErr) {
					aborted = sendErr
					break
				}
			} else {
				sentCount++
			}
		}

		if aborted != nil {
			break
		}
		history.progress(ctx, counters())
		if end < totalUsers {
			time.Sleep(delayBetweenBatches)
		}
	}

	result = SendResult{
		TotalUsers:   eligibleUsers,
		SentCount:    sentCount,
		FailedCount:  failedCount,
		BlockedCount: blockedCount,
	}

	if aborted != nil {
		history.finish(ctx, database.BroadcastStatusAborted, counters(), aborted.Error())
	} else {
		history.finish(ctx, database.BroadcastStatusDone, counters(), "")
	}

	if adminID != 0 {
		text := fmt.Sprintf(adminResultMessageFmt, eligibleUsers, sentCount, failedCount, blockedCount)
		if aborted != nil {
			text = fmt.Sprintf(adminAbortedMessageFmt, aborted)
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminID, Text: text})
	}

	slog.Info("broadcast completed",
		"totalUsers", totalUsers,
		"eligibleUsers", eligibleUsers,
		"sent", sentCount,
		"failed", failedCount,
		"blocked", blockedCount,
		"audiences", audiences,
		"historyId", history.id,
	)
	return result
}

// Deliver отправляет одно сообщение — текстом или подписью к вложению.
//
// Экспортирована, потому что тем же кодом рисуется предпросмотр черновика в
// Telegram-админке: предпросмотр обязан выглядеть ровно как то, что уйдёт
// людям, а две копии ветвления по видам вложения этого не гарантируют.
func Deliver(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	msg Message,
	markup models.ReplyMarkup,
) (*models.Message, error) {
	if msg.Media == nil {
		params := bot.SendMessageParams{
			ChatID:    chatID,
			Text:      msg.Text,
			ParseMode: msg.ParseMode,
		}
		if len(msg.Entities) > 0 {
			params.Entities = msg.Entities
		}
		if markup != nil {
			params.ReplyMarkup = markup
		}
		return b.SendMessage(ctx, &params)
	}

	file := &models.InputFileString{Data: msg.Media.FileID}
	switch msg.Media.Kind {
	case MediaVideo:
		p := &bot.SendVideoParams{
			ChatID:          chatID,
			Video:           file,
			Caption:         msg.Text,
			CaptionEntities: msg.Entities,
			ParseMode:       msg.ParseMode,
			// Иначе получателю придётся скачать файл целиком, прежде чем
			// начнётся воспроизведение.
			SupportsStreaming: true,
		}
		if markup != nil {
			p.ReplyMarkup = markup
		}
		return b.SendVideo(ctx, p)
	case MediaPhoto:
		p := &bot.SendPhotoParams{
			ChatID:          chatID,
			Photo:           file,
			Caption:         msg.Text,
			CaptionEntities: msg.Entities,
			ParseMode:       msg.ParseMode,
		}
		if markup != nil {
			p.ReplyMarkup = markup
		}
		return b.SendPhoto(ctx, p)
	default:
		p := &bot.SendDocumentParams{
			ChatID:          chatID,
			Document:        file,
			Caption:         msg.Text,
			CaptionEntities: msg.Entities,
			ParseMode:       msg.ParseMode,
		}
		if markup != nil {
			p.ReplyMarkup = markup
		}
		return b.SendDocument(ctx, p)
	}
}

// isParseError — Telegram не смог разобрать разметку сообщения.
//
// Текст ошибки, а не код: Bot API отдаёт на это обычный 400 Bad Request, тот же
// самый, что и на десяток других причин, и отличить их можно только по описанию.
func isParseError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "can't parse entities") ||
		strings.Contains(low, "can't parse message text") ||
		strings.Contains(low, "unsupported start tag") ||
		strings.Contains(low, "unmatched end tag")
}
