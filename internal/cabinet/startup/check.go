// Package startup — проверки целостности БД, которые должны проходить до того,
// как web-кабинет начнёт принимать запросы.
//
// Сейчас здесь только one-shot проверка на коллизию synthetic-диапазона
// telegram_id (см. docs/cabinet/mvp-tz.md раздел 7.3 и раздел 22).
package startup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSyntheticRangeCollision означает, что в таблице customer есть реальные
// Telegram-клиенты (is_web_only = FALSE) с telegram_id >= base, то есть на них
// легко выпишется synthetic id нового web-only клиента — и upsert столкнётся.
//
// Если это произошло на проде (Telegram раздал клиенту id в synthetic-диапазоне),
// лечение — поднять CABINET_WEB_TELEGRAM_ID_BASE и пересоздать web-only клиентов
// на новой базе. Bot должен падать с явной инструкцией.
var ErrSyntheticRangeCollision = errors.New("real telegram_id found in synthetic range")

// VerifySyntheticIDRange выполняется при старте, если CABINET_ENABLED=true.
//
// Проверяет, что ни один не-web-only клиент не попадает в диапазон synthetic id.
// Если попадает — возвращает ошибку с деталями: сколько строк и минимальный
// нарушивший id, чтобы сразу было видно размер проблемы.
//
// Подразумевается, что колонка customer.is_web_only уже существует (создаётся
// миграцией 000017). Если по какой-то причине колонка отсутствует (старая БД,
// миграции не прошли) — возвращаем исходную ошибку запроса, чтобы падать громко.
func VerifySyntheticIDRange(ctx context.Context, pool *pgxpool.Pool, base int64) error {
	if pool == nil {
		return errors.New("startup: db pool is nil")
	}
	if base <= 0 {
		return fmt.Errorf("startup: invalid synthetic id base %d", base)
	}

	// Self-heal: реальные Telegram user id не достигают synthetic base (7e18+).
	// Если в БД telegram_id >= base при is_web_only=FALSE — это рассинхрон (dev-unlink,
	// старые данные, ручной SQL). Такие строки должны быть web-only, иначе проверка
	// ниже ложно «паникует» и бот не стартует.
	const repair = `
		UPDATE customer
		SET is_web_only = TRUE
		WHERE telegram_id >= $1 AND is_web_only = FALSE
	`
	tag, err := pool.Exec(ctx, repair, base)
	if err != nil {
		return fmt.Errorf("startup: repair synthetic is_web_only: %w", err)
	}
	if n := tag.RowsAffected(); n > 0 {
		slog.Warn("startup: repaired customer.is_web_only for synthetic-range telegram_id",
			"rows", n, "web_tg_id_base", base)
	}

	const query = `
		SELECT COUNT(*)::BIGINT, COALESCE(MIN(telegram_id), 0)::BIGINT
		FROM customer
		WHERE telegram_id >= $1
		  AND is_web_only = FALSE
	`
	var count, minID int64
	if err := pool.QueryRow(ctx, query, base).Scan(&count, &minID); err != nil {
		return fmt.Errorf("startup: verify synthetic range: %w", err)
	}

	if count == 0 {
		return nil
	}
	return fmt.Errorf(
		"%w: %d real customer(s) already have telegram_id >= %d (min observed: %d); "+
			"raise CABINET_WEB_TELEGRAM_ID_BASE above %d and re-migrate web-only customers",
		ErrSyntheticRangeCollision, count, base, minID, minID,
	)
}
