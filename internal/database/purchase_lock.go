package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Блокировка обработки покупки.
//
// Оплату одной покупки подтверждают сразу несколько источников: вебхук кассы,
// его повторы, поллер, Heleket-сверка. Каждый читает покупку, видит pending,
// продлевает подписку в Remnawave и только потом пишет paid. Без блокировки два
// источника, пришедшие одновременно, продлили бы подписку дважды — а вебхук
// YooKassa публичный, и прислать его параллельно 20 раз может кто угодно.
//
// Блокировка — сессионная advisory-блокировка PostgreSQL, а не мьютекс в
// процессе: она общая для всех реплик и не оставляет «зависших» покупок — если
// процесс упадёт посреди обработки, соединение закроется и блокировка снимется.

// ErrPurchaseLocked — покупку прямо сейчас обрабатывает кто-то другой. Это не
// сбой: держатель блокировки доведёт её до paid сам, а если не сможет, следующий
// вебхук или проход поллера попробует снова.
var ErrPurchaseLocked = errors.New("purchase is already being processed")

// purchaseLockClass — первая половина ключа pg_try_advisory_lock(int4, int4).
// Двухчастные ключи не пересекаются с однокомпонентными bigint (их берёт, например,
// чат поддержки по account_id), поэтому номер покупки не столкнётся с номером аккаунта.
const purchaseLockClass int32 = 0x70757263 // "purc"

// purchaseLockKey берёт младшие 32 бита id: столкнуться могут только покупки,
// чьи номера отличаются ровно на 2^32, — в этой базе таких не будет.
func purchaseLockKey(purchaseID int64) int32 {
	return int32(uint32(purchaseID))
}

// LockForProcessing захватывает покупку на время обработки, не дожидаясь чужой
// блокировки: если покупку уже обрабатывают, сразу возвращает ErrPurchaseLocked.
// Ждать нельзя — каждый ожидающий держал бы соединение из пула, и пачка
// параллельных вебхуков выбрала бы его целиком.
//
// Блокировка держится на отдельном соединении до вызова unlock. Статус покупки
// нужно перечитывать уже после захвата: предыдущий держатель мог успеть
// записать paid.
func (pr *PurchaseRepository) LockForProcessing(ctx context.Context, purchaseID int64) (unlock func(), err error) {
	return tryAdvisoryLock(ctx, pr.pool, purchaseLockClass, purchaseLockKey(purchaseID), ErrPurchaseLocked, "purchase")
}

// tryAdvisoryLock берёт сессионную advisory-блокировку (class, key) без
// ожидания: занята — сразу errLocked. Блокировка держится на отдельном
// соединении пула до вызова unlock.
func tryAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, class, key int32, errLocked error, what string) (unlock func(), err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s lock: acquire connection: %w", what, err)
	}
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1::int4, $2::int4)`, class, key).Scan(&locked); err != nil {
		conn.Release()
		return nil, fmt.Errorf("%s lock: %w", what, err)
	}
	if !locked {
		conn.Release()
		return nil, errLocked
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			// Контекст обработки к этому моменту может быть уже отменён,
			// а снять блокировку нужно в любом случае.
			uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var released bool
			err := conn.QueryRow(uctx, `SELECT pg_advisory_unlock($1::int4, $2::int4)`, class, key).Scan(&released)
			if err != nil || !released {
				// Соединение с неизвестным состоянием блокировки в пул не
				// возвращаем: блокировка жила бы, пока его не закроют. Закрытое
				// соединение пул выбросит.
				slog.Error(what+" lock: unlock failed, dropping connection",
					"key", key, "released", released, "error", err)
				_ = conn.Conn().Close(uctx)
			}
			conn.Release()
		})
	}, nil
}

// ErrTrialLocked — пробный период этому клиенту прямо сейчас выдаёт другой запрос.
var ErrTrialLocked = errors.New("trial activation is already in progress")

// trialLockClass — класс ключа блокировки выдачи пробного периода.
const trialLockClass int32 = 0x7472696c // "tril"

// LockTrial захватывает выдачу пробного периода клиенту. Проверку «пробный ещё
// не выдан» нужно делать уже после захвата: без блокировки несколько быстрых
// нажатий (каждый апдейт Telegram — своя горутина) проходили проверку разом и
// складывали пробные дни.
func (cr *CustomerRepository) LockTrial(ctx context.Context, customerID int64) (unlock func(), err error) {
	return tryAdvisoryLock(ctx, cr.pool, trialLockClass, int32(uint32(customerID)), ErrTrialLocked, "trial")
}

// CancelIfOpen переводит покупку в cancel, только если она ещё new или pending,
// и сообщает, была ли отмена. Безусловная запись статуса могла затереть paid:
// отмена от кассы приходит асинхронно и может обогнать её же подтверждение оплаты.
func (pr *PurchaseRepository) CancelIfOpen(ctx context.Context, purchaseID int64) (bool, error) {
	tag, err := pr.pool.Exec(ctx,
		`UPDATE purchase SET status = $2 WHERE id = $1 AND status IN ($3, $4)`,
		purchaseID, PurchaseStatusCancel, PurchaseStatusNew, PurchaseStatusPending)
	if err != nil {
		return false, fmt.Errorf("cancel purchase %d: %w", purchaseID, err)
	}
	return tag.RowsAffected() == 1, nil
}
