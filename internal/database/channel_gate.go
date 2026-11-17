package database

import (
	"context"
	"fmt"
	"time"
)

// Обязательная подписка на канал: отметки в customer (миграция 000071).
// Нужны только для статистики «увидели экран → подписались → ушли» —
// на допуск клиента они не влияют.

// MarkChannelGateShown запоминает, когда клиент впервые увидел экран подписки.
// Повторные вызовы ничего не меняют.
func (cr *CustomerRepository) MarkChannelGateShown(ctx context.Context, customerID int64) error {
	_, err := cr.pool.Exec(ctx,
		`UPDATE customer SET channel_gate_shown_at = now() WHERE id = $1 AND channel_gate_shown_at IS NULL`,
		customerID)
	if err != nil {
		return fmt.Errorf("mark channel gate shown: %w", err)
	}
	return nil
}

// MarkChannelGateAttempt считает нажатие «Да, я подписался».
func (cr *CustomerRepository) MarkChannelGateAttempt(ctx context.Context, customerID int64) error {
	_, err := cr.pool.Exec(ctx,
		`UPDATE customer SET channel_gate_attempts = channel_gate_attempts + 1 WHERE id = $1`,
		customerID)
	if err != nil {
		return fmt.Errorf("mark channel gate attempt: %w", err)
	}
	return nil
}

// MarkChannelGatePassed запоминает, когда клиент прошёл проверку после того, как
// увидел экран. Кто прошёл молча (уже был подписан), в статистику не попадает.
func (cr *CustomerRepository) MarkChannelGatePassed(ctx context.Context, customerID int64) error {
	_, err := cr.pool.Exec(ctx,
		`UPDATE customer SET channel_gate_passed_at = now()
		 WHERE id = $1 AND channel_gate_shown_at IS NOT NULL AND channel_gate_passed_at IS NULL`,
		customerID)
	if err != nil {
		return fmt.Errorf("mark channel gate passed: %w", err)
	}
	return nil
}

// MarkChannelGatePassedByTelegramID — то же по telegram_id: Telegram подтвердил подписку
// человеку, который раньше видел экран, но вернулся не кнопкой, а повторным /start или
// обновлением страницы. Без этого он через сутки попал бы в «ушли».
func (cr *CustomerRepository) MarkChannelGatePassedByTelegramID(ctx context.Context, telegramID int64) error {
	_, err := cr.pool.Exec(ctx,
		`UPDATE customer SET channel_gate_passed_at = now()
		 WHERE telegram_id = $1 AND channel_gate_shown_at IS NOT NULL AND channel_gate_passed_at IS NULL`,
		telegramID)
	if err != nil {
		return fmt.Errorf("mark channel gate passed by telegram id: %w", err)
	}
	return nil
}

// channelGateGrace — сколько клиент считается «ещё думающим» после показа
// экрана; позже без подписки он «ушёл».
const channelGateGrace = 24 * time.Hour

// channelGateNewWindow — клиент «новый», если экран подписки он увидел не позже
// чем через час после регистрации: пришёл и сразу упёрся в него. Остальные —
// старые клиенты, у которых функция включилась на ходу; VPN у них работает, и
// «ушедшими» их считать нельзя.
const channelGateNewWindow = time.Hour

// ChannelGateStats — цифры блока «Подписка на канал» за окно [from, to) по дате показа экрана.
type ChannelGateStats struct {
	// Новые клиенты.
	Shown       int64 // увидели экран
	Passed      int64 // подписались и прошли проверку
	NotPassed   int64 // не подписались за сутки — ушли
	InProgress  int64 // экран увидели меньше суток назад, ещё не подписались
	TriedFailed int64 // из ушедших: нажимали «Да, я подписался», подписки не было
	// Старые клиенты, увидевшие экран и не подписавшиеся (VPN у них работает).
	OldNotPassed int64
}

// ChannelGateStats считает статистику подписки на канал.
func (s *StatsRepository) ChannelGateStats(ctx context.Context, from, to time.Time) (*ChannelGateStats, error) {
	const q = `
WITH s AS (
  SELECT
    channel_gate_shown_at  AS shown_at,
    channel_gate_passed_at AS passed_at,
    channel_gate_attempts  AS attempts,
    COALESCE(channel_gate_shown_at - created_at <= $4::interval, true) AS is_new
  FROM customer
  WHERE channel_gate_shown_at >= $1 AND channel_gate_shown_at < $2
)
SELECT
  COUNT(*) FILTER (WHERE is_new),
  COUNT(*) FILTER (WHERE is_new AND passed_at IS NOT NULL),
  COUNT(*) FILTER (WHERE is_new AND passed_at IS NULL AND shown_at <= $3),
  COUNT(*) FILTER (WHERE is_new AND passed_at IS NULL AND shown_at >  $3),
  COUNT(*) FILTER (WHERE is_new AND passed_at IS NULL AND shown_at <= $3 AND attempts > 0),
  COUNT(*) FILTER (WHERE NOT is_new AND passed_at IS NULL)
FROM s`
	out := &ChannelGateStats{}
	cutoff := time.Now().UTC().Add(-channelGateGrace)
	err := s.pool.QueryRow(ctx, q, from.UTC(), to.UTC(), cutoff, fmt.Sprintf("%d seconds", int(channelGateNewWindow.Seconds()))).
		Scan(&out.Shown, &out.Passed, &out.NotPassed, &out.InProgress, &out.TriedFailed, &out.OldNotPassed)
	if err != nil {
		return nil, fmt.Errorf("stats channel gate: %w", err)
	}
	return out, nil
}
