package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RuntimeSettingsRepository — key-value overrides для hot-reload config из админки.
type RuntimeSettingsRepository struct {
	pool *pgxpool.Pool
}

// NewRuntimeSettingsRepository — конструктор.
func NewRuntimeSettingsRepository(pool *pgxpool.Pool) *RuntimeSettingsRepository {
	return &RuntimeSettingsRepository{pool: pool}
}

// RuntimeSettingRow — одна запись override.
type RuntimeSettingRow struct {
	Key       string
	Value     string
	UpdatedBy *int64
}

// GetAll возвращает все сохранённые overrides.
func (r *RuntimeSettingsRepository) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM bot_runtime_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

// UpsertBatch сохраняет несколько ключей в одной транзакции.
func (r *RuntimeSettingsRepository) UpsertBatch(ctx context.Context, settings map[string]string, updatedBy *int64) error {
	if len(settings) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for key, value := range settings {
		_, err := tx.Exec(ctx, `
INSERT INTO bot_runtime_settings (key, value, updated_at, updated_by)
VALUES ($1, $2, NOW(), $3)
ON CONFLICT (key) DO UPDATE SET
  value = EXCLUDED.value,
  updated_at = NOW(),
  updated_by = EXCLUDED.updated_by`,
			key, value, updatedBy)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
