package statusprobe

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore хранит журнал замеров в таблицах status_probe_run и status_probe_sample.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Load(ctx context.Context) (map[string][]RunRecord, error) {
	out := map[string][]RunRecord{}
	if s == nil || s.pool == nil {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT probe_key, at, world_ok, world_total, russia_ok, russia_total
		FROM status_probe_run
		ORDER BY probe_key, at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var rec RunRecord
		if err := rows.Scan(&key, &rec.At, &rec.WorldOK, &rec.WorldTotal, &rec.RussiaOK, &rec.RussiaTotal); err != nil {
			return nil, err
		}
		out[key] = append(out[key], rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for key, runs := range out {
		if len(runs) > historyRuns {
			out[key] = runs[len(runs)-historyRuns:]
		}
	}
	return out, nil
}

// Save записывает снимок: новые замеры добавляются, лишние (старше последних historyRuns)
// и строки, которых в снимке нет (сервер убрали из списка), удаляются.
func (s *PGStore) Save(ctx context.Context, runs map[string][]RunRecord) error {
	if s == nil || s.pool == nil {
		return nil
	}
	var keys []string
	var times []time.Time
	var wOK, wN, rOK, rN []int32
	rowKeys := make([]string, 0, len(runs))
	for key, perKey := range runs {
		rowKeys = append(rowKeys, key)
		for _, rec := range perKey {
			keys = append(keys, key)
			times = append(times, rec.At)
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
			INSERT INTO status_probe_run (probe_key, at, world_ok, world_total, russia_ok, russia_total)
			SELECT * FROM unnest($1::text[], $2::timestamptz[], $3::int[], $4::int[], $5::int[], $6::int[])
			ON CONFLICT (probe_key, at) DO NOTHING`,
			keys, times, wOK, wN, rOK, rN); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM status_probe_run WHERE probe_key <> ALL($1::text[])`, rowKeys); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM status_probe_run r USING (
			SELECT probe_key, at,
			       row_number() OVER (PARTITION BY probe_key ORDER BY at DESC) AS n
			FROM status_probe_run
		) old
		WHERE r.probe_key = old.probe_key AND r.at = old.at AND old.n > $1::int`, historyRuns); err != nil {
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

// ImportLegacy переносит последние замеры из прежнего JSON в bot_runtime_settings, если таблицы пусты.
// Журнал по дням не переносим: полоска теперь считает замеры, а не сутки. Старые ключи не трогаем.
func (s *PGStore) ImportLegacy(ctx context.Context, legacy *KVHistory) error {
	if s == nil || s.pool == nil || legacy == nil {
		return nil
	}
	var rows int
	if err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM status_probe_run) + (SELECT count(*) FROM status_probe_sample)`).Scan(&rows); err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	latest, err := legacy.LoadLatest(ctx)
	if err != nil {
		return err
	}
	if len(latest) == 0 {
		return nil
	}
	if err := s.SaveLatest(ctx, latest); err != nil {
		return err
	}
	slog.Info("status probe samples imported", "samples", len(latest))
	return nil
}
