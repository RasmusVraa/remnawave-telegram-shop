package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpiredRetention — сколько хранить запись после истечения срока. Истёкшая
// сессия или код уже ничего не открывают; неделя оставлена для разбора
// обращений («не приходит код», «выкинуло из кабинета»).
const ExpiredRetention = 7 * 24 * time.Hour

// cleanupBatch — строк за один DELETE. На первом запуске у давно работающего
// кабинета их может быть сотни тысяч, а пул живёт со statement_timeout 30s.
const cleanupBatch = 5000

// expiringTables — таблицы с expires_at, которые раньше копились вечно.
var expiringTables = []string{
	"cabinet_session",
	"cabinet_email_verification",
	"cabinet_password_reset_token",
	"cabinet_email_merge_verification",
}

// PurgeExpired удаляет сессии и одноразовые коды, истёкшие раньше cutoff.
// Возвращает число удалённых строк по таблицам.
//
// Сессии: rotated_to_session_id ссылается с ON DELETE SET NULL, поэтому
// удаление старых звеньев цепочки ротации не трогает живую сессию. Обнаружение
// повторного использования для удалённого токена не нужно: он истёк и
// отклоняется по сроку.
func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time) (map[string]int64, error) {
	deleted := make(map[string]int64, len(expiringTables))
	for _, table := range expiringTables {
		// Имя таблицы — из константного списка выше, не из ввода.
		q := `DELETE FROM ` + table + ` WHERE id IN (
			SELECT id FROM ` + table + ` WHERE expires_at < $1 LIMIT $2)` //nolint:gosec // G202: table из константного списка
		for {
			tag, err := pool.Exec(ctx, q, cutoff, cleanupBatch)
			if err != nil {
				return deleted, fmt.Errorf("purge %s: %w", table, err)
			}
			deleted[table] += tag.RowsAffected()
			if tag.RowsAffected() < cleanupBatch {
				break
			}
		}
	}
	return deleted, nil
}
