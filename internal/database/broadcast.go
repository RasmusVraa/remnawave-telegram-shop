package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Откуда запущена рассылка.
const (
	BroadcastSourceCabinet  = "cabinet"
	BroadcastSourceTelegram = "telegram"
)

// Чем кончилась рассылка.
const (
	BroadcastStatusRunning = "running"
	BroadcastStatusDone    = "done"
	// Aborted — Telegram не разобрал разметку на первом же получателе, и
	// отправка остановлена: текст один на всех, дальше слать бессмысленно.
	BroadcastStatusAborted = "aborted"
	// Failed — до отправки не дошло: не удалось получить список получателей
	// или он оказался пуст.
	BroadcastStatusFailed = "failed"
	// Interrupted — процесс бота перезапустился посреди отправки.
	BroadcastStatusInterrupted = "interrupted"
)

// BroadcastRecord — одна рассылка в истории.
type BroadcastRecord struct {
	ID          int64
	Source      string
	Audiences   []string
	TariffID    *int64
	Text        string
	ParseMode   string
	MediaKind   string
	MediaFileID string
	MediaName   string
	// Buttons — кнопки под сообщением в том виде, в каком их отдаёт API
	// кабинета. JSON, а не структура: тип кнопок живёт в пакете broadcast,
	// который сам зависит от database.
	Buttons    json.RawMessage
	Status     string
	Total      int
	Sent       int
	Blocked    int
	Failed     int
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// BroadcastCounters — ход отправки.
type BroadcastCounters struct {
	Total   int
	Sent    int
	Blocked int
	Failed  int
}

// BroadcastRepository хранит историю рассылок.
type BroadcastRepository struct {
	pool *pgxpool.Pool
}

func NewBroadcastRepository(pool *pgxpool.Pool) *BroadcastRepository {
	return &BroadcastRepository{pool: pool}
}

const broadcastColumns = `id, source, audiences, tariff_id, text, parse_mode, media_kind, media_file_id,
	media_name, buttons, status, total, sent, blocked, failed, error, started_at, finished_at`

// Create заводит запись со статусом running и возвращает её id.
func (r *BroadcastRepository) Create(ctx context.Context, rec BroadcastRecord) (int64, error) {
	buttons := rec.Buttons
	if len(buttons) == 0 {
		buttons = json.RawMessage(`{}`)
	}
	audiences := rec.Audiences
	if audiences == nil {
		audiences = []string{}
	}
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO broadcast (source, audiences, tariff_id, text, parse_mode, media_kind, media_file_id,
			media_name, buttons, status, total)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		rec.Source, audiences, rec.TariffID, rec.Text, rec.ParseMode, rec.MediaKind, rec.MediaFileID,
		rec.MediaName, []byte(buttons), BroadcastStatusRunning, rec.Total,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create broadcast: %w", err)
	}
	return id, nil
}

// UpdateProgress переписывает счётчики идущей рассылки. Завершённую не
// трогает: запоздавшая запись из пачки не должна откатить итог.
func (r *BroadcastRepository) UpdateProgress(ctx context.Context, id int64, c BroadcastCounters) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE broadcast SET total = $2, sent = $3, blocked = $4, failed = $5
		WHERE id = $1 AND status = $6`,
		id, c.Total, c.Sent, c.Blocked, c.Failed, BroadcastStatusRunning)
	if err != nil {
		return fmt.Errorf("update broadcast progress: %w", err)
	}
	return nil
}

// Finish записывает итог.
func (r *BroadcastRepository) Finish(ctx context.Context, id int64, status string, c BroadcastCounters, errText string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE broadcast
		SET status = $2, total = $3, sent = $4, blocked = $5, failed = $6, error = $7, finished_at = now()
		WHERE id = $1`,
		id, status, c.Total, c.Sent, c.Blocked, c.Failed, errText)
	if err != nil {
		return fmt.Errorf("finish broadcast: %w", err)
	}
	return nil
}

// MarkInterrupted закрывает рассылки, которые числятся идущими. Зовётся на
// старте процесса: раз процесс только поднялся, отправлять их уже некому.
func (r *BroadcastRepository) MarkInterrupted(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE broadcast SET status = $1, finished_at = now()
		WHERE status = $2`,
		BroadcastStatusInterrupted, BroadcastStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("mark interrupted broadcasts: %w", err)
	}
	return tag.RowsAffected(), nil
}

// List — страница истории, свежие сверху, и сколько записей всего.
func (r *BroadcastRepository) List(ctx context.Context, limit, offset int) ([]BroadcastRecord, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM broadcast`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count broadcasts: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+broadcastColumns+` FROM broadcast
		ORDER BY started_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list broadcasts: %w", err)
	}
	defer rows.Close()

	out := make([]BroadcastRecord, 0, limit)
	for rows.Next() {
		rec, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate broadcasts: %w", err)
	}
	return out, total, nil
}

// Get — одна запись; nil, если такой нет.
func (r *BroadcastRepository) Get(ctx context.Context, id int64) (*BroadcastRecord, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+broadcastColumns+` FROM broadcast WHERE id = $1`, id)
	rec, err := scanBroadcast(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func scanBroadcast(row pgx.Row) (BroadcastRecord, error) {
	var rec BroadcastRecord
	var buttons []byte
	err := row.Scan(&rec.ID, &rec.Source, &rec.Audiences, &rec.TariffID, &rec.Text, &rec.ParseMode,
		&rec.MediaKind, &rec.MediaFileID, &rec.MediaName, &buttons, &rec.Status, &rec.Total, &rec.Sent,
		&rec.Blocked, &rec.Failed, &rec.Error, &rec.StartedAt, &rec.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rec, err
		}
		return rec, fmt.Errorf("scan broadcast: %w", err)
	}
	rec.Buttons = json.RawMessage(buttons)
	return rec, nil
}
