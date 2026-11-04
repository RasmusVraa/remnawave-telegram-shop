package statusprobe

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dayKeep — сколько дней держим в таблице. Окно страницы короче (historyDays),
// запас нужен, чтобы граница дня и часовой пояс не отрезали вчерашнюю строку.
const dayKeep = 45

// PGStore хранит журнал замеров в таблицах status_probe_day и status_probe_sample.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Load(ctx context.Context) (map[string]map[string]DayRecord, error) {
	out := map[string]map[string]DayRecord{}
	if s == nil || s.pool == nil {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT probe_key, to_char(day, 'YYYY-MM-DD'), world_ok, world_total, russia_ok, russia_total
		FROM status_probe_day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, day string
		var rec DayRecord
		if err := rows.Scan(&key, &day, &rec.WorldOK, &rec.WorldTotal, &rec.RussiaOK, &rec.RussiaTotal); err != nil {
			return nil, err
		}
		if out[key] == nil {
			out[key] = map[string]DayRecord{}
		}
		out[key][day] = rec
	}
	return out, rows.Err()
}

// Save записывает снимок целиком: строки, которых в нём нет (сервер убрали из списка), удаляются.
func (s *PGStore) Save(ctx context.Context, days map[string]map[string]DayRecord) error {
	if s == nil || s.pool == nil {
		return nil
	}
	var keys []string
	var dates []time.Time
	var wOK, wN, rOK, rN []int32
	rowKeys := make([]string, 0, len(days))
	for key, perDay := range days {
		rowKeys = append(rowKeys, key)
		for day, rec := range perDay {
			at, err := time.Parse("2006-01-02", day)
			if err != nil {
				continue
			}
			keys = append(keys, key)
			dates = append(dates, at)
			wOK = append(wOK, int32(rec.WorldOK))
			wN = append(wN, int32(rec.WorldTotal))
			rOK = append(rOK, int32(rec.RussiaOK))
			rN = append(rN, int32(rec.RussiaTotal))
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if len(keys) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO status_probe_day (probe_key, day, world_ok, world_total, russia_ok, russia_total)
			SELECT * FROM unnest($1::text[], $2::date[], $3::int[], $4::int[], $5::int[], $6::int[])
			ON CONFLICT (probe_key, day) DO UPDATE SET
				world_ok = EXCLUDED.world_ok, world_total = EXCLUDED.world_total,
				russia_ok = EXCLUDED.russia_ok, russia_total = EXCLUDED.russia_total`,
			keys, dates, wOK, wN, rOK, rN); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM status_probe_day WHERE probe_key <> ALL($1::text[])`, rowKeys); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM status_probe_day WHERE day < (now() AT TIME ZONE 'UTC')::date - $1::int`, dayKeep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PGStore) LoadLatest(ctx context.Context) (map[string]Sample, error) {
	out := map[string]Sample{}
	if s == nil || s.pool == nil {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT probe_key, payload FROM status_probe_sample`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var item persistedSample
		if err := json.Unmarshal(raw, &item); err != nil {
			slog.Warn("status probe sample skipped", "key", key, "error", err)
			continue
		}
		out[key] = Sample(item)
	}
	return out, rows.Err()
}

// SaveLatest записывает последние замеры целиком, лишние строки удаляет.
func (s *PGStore) SaveLatest(ctx context.Context, samples map[string]Sample) error {
	if s == nil || s.pool == nil {
		return nil
	}
	keys := make([]string, 0, len(samples))
	payloads := make([]string, 0, len(samples))
	for key, sample := range samples {
		raw, err := json.Marshal(persistedSample(sample))
		if err != nil {
			return err
		}
		keys = append(keys, key)
		payloads = append(payloads, string(raw))
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if len(keys) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO status_probe_sample (probe_key, payload, updated_at)
			SELECT k, p::jsonb, now() FROM unnest($1::text[], $2::text[]) AS t(k, p)
			ON CONFLICT (probe_key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = now()`,
			keys, payloads); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM status_probe_sample WHERE probe_key <> ALL($1::text[])`, keys); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ImportLegacy переносит журнал из прежнего JSON в bot_runtime_settings, если таблицы пусты.
// Нужен один раз после обновления, чтобы полоска за 30 дней не обнулилась. Старые ключи не трогаем.
func (s *PGStore) ImportLegacy(ctx context.Context, legacy *KVHistory) error {
	if s == nil || s.pool == nil || legacy == nil {
		return nil
	}
	var rows int
	if err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM status_probe_day) + (SELECT count(*) FROM status_probe_sample)`).Scan(&rows); err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	days, err := legacy.Load(ctx)
	if err != nil {
		return err
	}
	latest, err := legacy.LoadLatest(ctx)
	if err != nil {
		return err
	}
	if len(days) == 0 && len(latest) == 0 {
		return nil
	}
	if err := s.Save(ctx, days); err != nil {
		return err
	}
	if err := s.SaveLatest(ctx, latest); err != nil {
		return err
	}
	slog.Info("status probe history imported", "targets", len(days), "samples", len(latest))
	return nil
}
