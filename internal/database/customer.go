package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CustomerRepository struct {
	pool *pgxpool.Pool
}

func NewCustomerRepository(poll *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: poll}
}

// customerSelectColumns порядок полей для SELECT (не использовать * — совместимость со схемой).
// Порядок столбцов синхронизирован со всеми Scan-вызовами и с struct Customer.
const customerSelectColumns = "id, telegram_id, expire_at, created_at, subscription_link, language, extra_hwid, extra_hwid_expires_at, current_tariff_id, subscription_period_start, subscription_period_months, loyalty_xp, telegram_username, is_web_only, legal_accepted_at, remnawave_user_id, remnawave_short_uuid, broadcast_opt_out"

type Customer struct {
	ID                       int64      `db:"id"`
	TelegramID               int64      `db:"telegram_id"`
	ExpireAt                 *time.Time `db:"expire_at"`
	CreatedAt                time.Time  `db:"created_at"`
	SubscriptionLink         *string    `db:"subscription_link"`
	Language                 string     `db:"language"`
	ExtraHwid                int        `db:"extra_hwid"`
	ExtraHwidExpiresAt       *time.Time `db:"extra_hwid_expires_at"`
	CurrentTariffID          *int64     `db:"current_tariff_id"`
	SubscriptionPeriodStart  *time.Time `db:"subscription_period_start"`
	SubscriptionPeriodMonths *int       `db:"subscription_period_months"`
	LoyaltyXP                int64      `db:"loyalty_xp"`
	TelegramUsername         *string    `db:"telegram_username"`
	// IsWebOnly помечает клиента, созданного через web-кабинет (нет реального
	// Telegram-контакта). Для таких клиентов не должны выполняться вызовы
	// Telegram Bot API — см. utils.IsSyntheticTelegramID и docs/cabinet/audit-telegram-id.md.
	IsWebOnly bool `db:"is_web_only"`
	// LegalAcceptedAt — момент принятия политики/оферты в Telegram-боте (NULL = gate).
	LegalAcceptedAt *time.Time `db:"legal_accepted_at"`
	// RemnawaveUserID — числовой id профиля в панели Remnawave 3.x.
	// NULL означает «ещё не резолвили» — заполняется лениво при первом обращении,
	// см. remnawave.ResolveUserForCustomer. Не путать с Customer.ID.
	RemnawaveUserID *int64 `db:"remnawave_user_id"`
	// RemnawaveShortUUID — shortUuid профиля панели (используется в публичных ссылках).
	RemnawaveShortUUID *string `db:"remnawave_short_uuid"`
	// BroadcastOptOut — клиент отказался от массовых рассылок. Сервисные
	// напоминания об окончании подписки этим флагом не глушатся.
	BroadcastOptOut bool `db:"broadcast_opt_out"`
}

func scanCustomer(sc interface{ Scan(dest ...any) error }, c *Customer) error {
	return sc.Scan(
		&c.ID,
		&c.TelegramID,
		&c.ExpireAt,
		&c.CreatedAt,
		&c.SubscriptionLink,
		&c.Language,
		&c.ExtraHwid,
		&c.ExtraHwidExpiresAt,
		&c.CurrentTariffID,
		&c.SubscriptionPeriodStart,
		&c.SubscriptionPeriodMonths,
		&c.LoyaltyXP,
		&c.TelegramUsername,
		&c.IsWebOnly,
		&c.LegalAcceptedAt,
		&c.RemnawaveUserID,
		&c.RemnawaveShortUUID,
		&c.BroadcastOptOut,
	)
}

// SetBroadcastOptOut включает или снимает отказ от массовой рассылки.
func (cr *CustomerRepository) SetBroadcastOptOut(ctx context.Context, customerID int64, optOut bool) error {
	tag, err := cr.pool.Exec(ctx, `UPDATE customer SET broadcast_opt_out = $2 WHERE id = $1`, customerID, optOut)
	if err != nil {
		return fmt.Errorf("set broadcast opt out: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// BroadcastRecipient is a Telegram user with language for localized broadcast keyboards.
type BroadcastRecipient struct {
	TelegramID int64
	Language   string
}

// Broadcast audience filters for GetBroadcastRecipients.
const (
	BroadcastAudienceAll           = "all"
	BroadcastAudienceActive        = "active" // deprecated: используйте ActiveAll
	BroadcastAudienceInactive      = "inactive"
	BroadcastAudienceActivePaid    = "active_paid"
	BroadcastAudienceActiveTrial   = "active_trial"
	BroadcastAudienceActiveAll     = "active_all"
	BroadcastAudienceInactivePaid  = "inactive_paid"
	BroadcastAudienceInactiveTrial = "inactive_trial"
	BroadcastAudienceInactiveAll   = "inactive_all"
	BroadcastAudienceTestBroadcast = "test_broadcast"
)

func (cr *CustomerRepository) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(
			sq.And{
				sq.NotEq{"expire_at": nil},
				sq.GtOrEq{"expire_at": startDate},
				sq.LtOrEq{"expire_at": endDate},
				sq.Eq{"is_web_only": false},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers by expiration range: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := scanCustomer(rows, &customer)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return &customers, nil
}

func (cr *CustomerRepository) FindById(ctx context.Context, id int64) (*Customer, error) {
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer

	err = scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return &customer, nil
}

func (cr *CustomerRepository) FindByTelegramId(ctx context.Context, telegramId int64) (*Customer, error) {
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramId}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer

	err = scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return &customer, nil
}

// FindBySubscriptionLink returns a customer with an exact subscription_link match.
func (cr *CustomerRepository) FindBySubscriptionLink(ctx context.Context, link string) (*Customer, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, nil
	}
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.Eq{"subscription_link": link}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer
	err = scanCustomer(cr.pool.QueryRow(ctx, sql, args...), &customer)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return &customer, nil
}

func (cr *CustomerRepository) Create(ctx context.Context, customer *Customer) (*Customer, error) {
	return cr.FindOrCreate(ctx, customer)
}

func (cr *CustomerRepository) FindOrCreate(ctx context.Context, customer *Customer) (*Customer, error) {
	query := `
	INSERT INTO customer (telegram_id, expire_at, language)
	VALUES ($1, $2, $3)
	ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = customer.telegram_id
	RETURNING ` + customerSelectColumns + `
	`

	row := cr.pool.QueryRow(ctx, query, customer.TelegramID, customer.ExpireAt, customer.Language)
	var result Customer
	if err := scanCustomer(row, &result); err != nil {
		return nil, fmt.Errorf("failed to find or create customer: %w", err)
	}

	slog.Info("user found or created in bot database", "telegramId", utils.MaskHalfInt64(result.TelegramID))
	return &result, nil
}

// ErrWebOnlyCollision — попытка bootstrap'нуть web-only customer на telegram_id,
// который уже занят обычным (is_web_only=FALSE) клиентом. Это нарушает инвариант
// synthetic-диапазона (startup-check должен был поймать заранее). Возвращается
// из CreateWebOnly вместо молчаливого «перехвата» чужой строки.
var ErrWebOnlyCollision = errors.New("customer: synthetic telegram_id is occupied by a real customer")

// CreateWebOnly идемпотентно создаёт customer для web-кабинета с is_web_only=TRUE.
//
// Применяется bootstrap-сервисом после регистрации cabinet_account:
// telegram_id передаётся из utils.SyntheticTelegramID(accountID). При повторном
// вызове (link упал между INSERT customer и INSERT link) возвращает ту же строку.
//
// Если на этот telegram_id уже есть не-web-only клиент (что возможно только
// при сломанном startup-check) — возвращается ErrWebOnlyCollision, чтобы bootstrap
// не «украл» реального клиента.
func (cr *CustomerRepository) CreateWebOnly(ctx context.Context, telegramID int64, language string) (*Customer, error) {
	if language == "" {
		language = "ru"
	}
	query := `
	INSERT INTO customer (telegram_id, language, is_web_only)
	VALUES ($1, $2, TRUE)
	ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = customer.telegram_id
	RETURNING ` + customerSelectColumns

	row := cr.pool.QueryRow(ctx, query, telegramID, language)
	var result Customer
	if err := scanCustomer(row, &result); err != nil {
		return nil, fmt.Errorf("failed to create web-only customer: %w", err)
	}

	if !result.IsWebOnly {
		return nil, fmt.Errorf("%w: telegram_id=%d", ErrWebOnlyCollision, telegramID)
	}
	return &result, nil
}

// SetLegalAcceptedAt фиксирует согласие с политикой/офертой в Telegram-боте.
func (cr *CustomerRepository) SetLegalAcceptedAt(ctx context.Context, customerID int64, at time.Time) error {
	return cr.UpdateFields(ctx, customerID, map[string]interface{}{
		"legal_accepted_at": at,
	})
}

// SetRemnawaveIdentity сохраняет идентификатор профиля панели Remnawave у клиента.
//
// Вызывается лениво: как только профиль резолвится любым способом (stream по
// telegram_id, username, subscription_link), его id кладётся сюда, и дальше
// адресация идёт напрямую по нему. Схема самовосстанавливающаяся — если запись
// оказалась неверной, следующий резолв её перезапишет.
//
// Ошибка записи намеренно НЕ должна ронять вызывающую операцию: это кеш,
// а не источник истины. За это отвечает вызывающий код.
func (cr *CustomerRepository) SetRemnawaveIdentity(ctx context.Context, customerID int64, remnawaveUserID int64, shortUUID string) error {
	if customerID <= 0 || remnawaveUserID <= 0 {
		return nil
	}
	// Частичный уникальный индекс не даст привязать один профиль панели к двум
	// клиентам. Если такое случилось (например, после merge аккаунтов), снимаем
	// привязку с прежнего владельца — актуальным считается последний резолв.
	if _, err := cr.pool.Exec(ctx,
		`UPDATE customer SET remnawave_user_id = NULL, remnawave_short_uuid = NULL
		 WHERE remnawave_user_id = $1 AND id <> $2`,
		remnawaveUserID, customerID); err != nil {
		return fmt.Errorf("clear stale remnawave identity: %w", err)
	}

	var short *string
	if s := strings.TrimSpace(shortUUID); s != "" {
		short = &s
	}
	if _, err := cr.pool.Exec(ctx,
		`UPDATE customer SET remnawave_user_id = $2, remnawave_short_uuid = $3 WHERE id = $1`,
		customerID, remnawaveUserID, short); err != nil {
		return fmt.Errorf("set remnawave identity: %w", err)
	}
	return nil
}

// IncrementLoyaltyXP добавляет накопленный XP лояльности после успешной оплаты.
func (cr *CustomerRepository) IncrementLoyaltyXP(ctx context.Context, customerID int64, delta int64) error {
	if delta <= 0 {
		return nil
	}
	res, err := cr.pool.Exec(ctx, `UPDATE customer SET loyalty_xp = loyalty_xp + $2 WHERE id = $1`, customerID, delta)
	if err != nil {
		return fmt.Errorf("increment loyalty_xp: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("customer not found: %d", customerID)
	}
	return nil
}

// DevCabinetResetTelegramToSynthetic вызывается только из dev-эндпоинта кабинета:
// при реальном telegram_id у связанного customer подменяет его на SyntheticTelegramID(accountID),
// обнуляет telegram_username и выставляет is_web_only (как у web-only клиента).
func (cr *CustomerRepository) DevCabinetResetTelegramToSynthetic(ctx context.Context, customerID, accountID int64) (updated bool, err error) {
	cust, err := cr.FindById(ctx, customerID)
	if err != nil {
		return false, fmt.Errorf("dev reset tg: find customer: %w", err)
	}
	if cust == nil {
		return false, nil
	}
	if utils.IsSyntheticTelegramID(cust.TelegramID) {
		return false, nil
	}
	syn := utils.SyntheticTelegramID(accountID)
	tag, err := cr.pool.Exec(ctx, `
		UPDATE customer
		SET telegram_id = $1, telegram_username = NULL, is_web_only = TRUE
		WHERE id = $2`,
		syn, customerID,
	)
	if err != nil {
		return false, fmt.Errorf("dev reset tg: update: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ApplyLoyaltyXPFullRecalc обнуляет loyalty_xp у всех клиентов и задаёт суммы из карты (полный пересчёт XP из истории оплат).
func (cr *CustomerRepository) ApplyLoyaltyXPFullRecalc(ctx context.Context, sums map[int64]int64) error {
	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE customer SET loyalty_xp = 0`); err != nil {
		return fmt.Errorf("reset loyalty_xp: %w", err)
	}
	for cid, xp := range sums {
		if xp <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE customer SET loyalty_xp = $2 WHERE id = $1`, cid, xp); err != nil {
			return fmt.Errorf("set loyalty_xp customer %d: %w", cid, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

// CountCustomersWithLoyaltyXPAtLeast число клиентов с loyalty_xp >= threshold.
func (cr *CustomerRepository) CountCustomersWithLoyaltyXPAtLeast(ctx context.Context, threshold int64) (int64, error) {
	var n int64
	err := cr.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer WHERE loyalty_xp >= $1`, threshold).Scan(&n)
	return n, err
}

// CountCustomersLoyaltyXPHalfOpen число клиентов с loyalty_xp >= minXP и (если задано) loyalty_xp < maxXPExclusive.
// Для верхней границы последнего уровня передайте maxXPExclusive == nil.
func (cr *CustomerRepository) CountCustomersLoyaltyXPHalfOpen(ctx context.Context, minXP int64, maxXPExclusive *int64) (int64, error) {
	var n int64
	var err error
	if maxXPExclusive == nil {
		err = cr.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM customer WHERE loyalty_xp >= $1`, minXP).Scan(&n)
	} else {
		err = cr.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM customer WHERE loyalty_xp >= $1 AND loyalty_xp < $2`, minXP, *maxXPExclusive).Scan(&n)
	}
	return n, err
}

func (cr *CustomerRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"telegram_id":                {},
		"expire_at":                  {},
		"subscription_link":          {},
		"language":                   {},
		"extra_hwid":                 {},
		"extra_hwid_expires_at":      {},
		"current_tariff_id":          {},
		"subscription_period_start":  {},
		"subscription_period_months": {},
		"loyalty_xp":                 {},
		"telegram_username":          {},
		"is_web_only":                {},
		"legal_accepted_at":          {},
	}

	buildUpdate := sq.Update("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("failed to update customer: field %q is not allowed", field)
		}
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	result, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		if err := tx.Rollback(ctx); err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(id))
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramIDs}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := scanCustomer(rows, &customer)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) CreateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	builder := sq.Insert("customer").
		Columns("telegram_id", "expire_at", "language", "subscription_link").
		PlaceholderFormat(sq.Dollar)
	for _, cust := range customers {
		builder = builder.Values(cust.TelegramID, cust.ExpireAt, cust.Language, cust.SubscriptionLink)
	}
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build batch insert query: %w", err)
	}

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sqlStr, args...)
	if err != nil {
		if err := tx.Rollback(ctx); err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) UpdateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	query := "UPDATE customer SET expire_at = c.expire_at, subscription_link = c.subscription_link FROM (VALUES "
	var args []interface{}
	for i, cust := range customers {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d::bigint, $%d::timestamp, $%d::text)", i*3+1, i*3+2, i*3+3)
		args = append(args, cust.TelegramID, cust.ExpireAt, cust.SubscriptionLink)
	}
	query += ") AS c(telegram_id, expire_at, subscription_link) WHERE customer.telegram_id = c.telegram_id"

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	_, err = cr.pool.Exec(ctx, query, args...)
	if err != nil {
		if err := tx.Rollback(ctx); err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return fmt.Errorf("failed to execute batch update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// DeleteByNotInTelegramIds удаляет строки customer, которых нет в списке telegram_id
// из Remnawave-синхронизации. Web-only клиенты кабинета (is_web_only=TRUE) не трогаем —
// у них нет telegram_id в ответе панели, иначе каждый Sync стирал бы кабинетских пользователей.
// См. docs/cabinet/audit-telegram-id.md, раздел 1.6.
func (cr *CustomerRepository) DeleteByNotInTelegramIds(ctx context.Context, telegramIDs []int64) error {
	if len(telegramIDs) == 0 {
		// Не удаляем клиентов, связанных с cabinet_account, даже если их нет в панели.
		// Иначе после sync рвётся account<->customer link и кабинет bootstrap'ит synthetic web-only customer.
		_, err := cr.pool.Exec(ctx, `
			DELETE FROM customer c
			 WHERE c.is_web_only = FALSE
			   AND NOT EXISTS (
			     SELECT 1 FROM cabinet_account_customer_link l WHERE l.customer_id = c.id
			   )`)
		if err != nil {
			return fmt.Errorf("failed to delete customers: %w", err)
		}
		return nil
	}
	_, err := cr.pool.Exec(ctx, `
		DELETE FROM customer c
		 WHERE c.is_web_only = FALSE
		   AND c.telegram_id <> ALL($1::bigint[])
		   AND NOT EXISTS (
		     SELECT 1 FROM cabinet_account_customer_link l WHERE l.customer_id = c.id
		   )`, telegramIDs)
	if err != nil {
		return fmt.Errorf("failed to delete customers: %w", err)
	}
	return nil
}

// HasCabinetLink проверяет, привязан ли customer к cabinet_account.
func (cr *CustomerRepository) HasCabinetLink(ctx context.Context, customerID int64) (bool, error) {
	if customerID <= 0 {
		return false, fmt.Errorf("invalid customer id")
	}
	var exists bool
	if err := cr.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM cabinet_account_customer_link WHERE customer_id = $1
		)`, customerID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check cabinet link: %w", err)
	}
	return exists, nil
}

// DeleteByID удаляет клиента из БД бота (связанные purchase/promo и т.п. — по CASCADE в схеме).
func (cr *CustomerRepository) DeleteByID(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid customer id")
	}
	q, args, err := sq.Delete("customer").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete customer sql: %w", err)
	}
	ct, err := cr.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("failed to delete customer: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("customer not found")
	}
	return nil
}

// DeleteByIDWithCabinetCascade удаляет клиента и связанные cabinet_account (если есть link),
// чтобы пользователь мог заново зайти через Telegram как "новый".
func (cr *CustomerRepository) DeleteByIDWithCabinetCascade(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid customer id")
	}
	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete customer with cabinet tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM cabinet_account
		WHERE id IN (
			SELECT account_id
			FROM cabinet_account_customer_link
			WHERE customer_id = $1
		)
	`, id); err != nil {
		return fmt.Errorf("delete linked cabinet accounts: %w", err)
	}

	ct, err := tx.Exec(ctx, `DELETE FROM customer WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("customer not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete customer with cabinet tx: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) GetAllTelegramIds(ctx context.Context) ([]int64, error) {
	buildSelect := sq.Select("telegram_id").
		From("customer").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query telegram ids: %w", err)
	}
	defer rows.Close()

	var telegramIDs []int64
	for rows.Next() {
		var telegramID int64
		err := rows.Scan(&telegramID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan telegram id row: %w", err)
		}
		telegramIDs = append(telegramIDs, telegramID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over telegram id rows: %w", err)
	}

	return telegramIDs, nil
}

func (cr *CustomerRepository) GetActiveTelegramIds(ctx context.Context) ([]int64, error) {
	now := time.Now()
	buildSelect := sq.Select("telegram_id").
		From("customer").
		Where(
			sq.And{
				sq.NotEq{"expire_at": nil},
				sq.Gt{"expire_at": now},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query active telegram ids: %w", err)
	}
	defer rows.Close()

	var telegramIDs []int64
	for rows.Next() {
		var telegramID int64
		err := rows.Scan(&telegramID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan telegram id row: %w", err)
		}
		telegramIDs = append(telegramIDs, telegramID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over telegram id rows: %w", err)
	}

	return telegramIDs, nil
}

func (cr *CustomerRepository) GetInactiveTelegramIds(ctx context.Context) ([]int64, error) {
	now := time.Now()
	buildSelect := sq.Select("telegram_id").
		From("customer").
		Where(
			sq.Or{
				sq.Eq{"expire_at": nil},
				sq.LtOrEq{"expire_at": now},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query inactive telegram ids: %w", err)
	}
	defer rows.Close()

	var telegramIDs []int64
	for rows.Next() {
		var telegramID int64
		err := rows.Scan(&telegramID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan telegram id row: %w", err)
		}
		telegramIDs = append(telegramIDs, telegramID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over telegram id rows: %w", err)
	}

	return telegramIDs, nil
}

// GetBroadcastRecipients returns telegram_id and language for mass broadcast (button labels per user).
// tariffID ограничивает сегменты active_paid / inactive_paid по customer.current_tariff_id (режим tariffs).
// Web-only клиенты кабинета исключаются (нет доставки в Telegram).
func (cr *CustomerRepository) GetBroadcastRecipients(ctx context.Context, audience string, tariffID *int64) ([]BroadcastRecipient, error) {
	now := time.Now()
	buildSelect := sq.Select("telegram_id", "language").
		From("customer").
		PlaceholderFormat(sq.Dollar)

	activeVPN := sq.And{sq.NotEq{"expire_at": nil}, sq.Gt{"expire_at": now}}
	inactiveVPN := sq.Or{sq.Eq{"expire_at": nil}, sq.LtOrEq{"expire_at": now}}
	paidSubscription := sq.Expr(`EXISTS (SELECT 1 FROM purchase p WHERE p.customer_id = customer.id AND p.status = 'paid' AND p.month > 0)`)
	noPaidSubscription := sq.Expr(`NOT EXISTS (SELECT 1 FROM purchase p WHERE p.customer_id = customer.id AND p.status = 'paid' AND p.month > 0)`)

	switch audience {
	case BroadcastAudienceAll:
		// no extra filter
	case BroadcastAudienceActive, BroadcastAudienceActiveAll:
		buildSelect = buildSelect.Where(activeVPN)
	case BroadcastAudienceInactive, BroadcastAudienceInactiveAll:
		buildSelect = buildSelect.Where(inactiveVPN)
	case BroadcastAudienceActivePaid:
		ap := sq.And{activeVPN, paidSubscription}
		if tariffID != nil {
			ap = sq.And{ap, sq.Eq{"current_tariff_id": *tariffID}}
		}
		buildSelect = buildSelect.Where(ap)
	case BroadcastAudienceActiveTrial:
		buildSelect = buildSelect.Where(sq.And{activeVPN, noPaidSubscription})
	case BroadcastAudienceInactivePaid:
		ip := sq.And{inactiveVPN, paidSubscription}
		if tariffID != nil {
			ip = sq.And{ip, sq.Eq{"current_tariff_id": *tariffID}}
		}
		buildSelect = buildSelect.Where(ip)
	case BroadcastAudienceInactiveTrial:
		buildSelect = buildSelect.Where(sq.And{inactiveVPN, noPaidSubscription})
	case BroadcastAudienceTestBroadcast:
		// Тестовая рассылка уходит только админу (ADMIN_TELEGRAM_ID), минуя фильтры аудитории.
		return cr.testBroadcastRecipients(ctx)
	default:
		return nil, fmt.Errorf("unknown broadcast audience: %s", audience)
	}

	buildSelect = buildSelect.Where(sq.Eq{"is_web_only": false, "broadcast_opt_out": false})

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query broadcast recipients: %w", err)
	}
	defer rows.Close()

	var out []BroadcastRecipient
	for rows.Next() {
		var r BroadcastRecipient
		var lang string
		err := rows.Scan(&r.TelegramID, &lang)
		if err != nil {
			return nil, fmt.Errorf("failed to scan broadcast recipient: %w", err)
		}
		if lang != "" {
			r.Language = lang
		} else {
			r.Language = "en"
		}
		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating broadcast recipients: %w", err)
	}

	return out, nil
}

// testBroadcastRecipients возвращает единственного получателя тестовой рассылки — админа
// из ADMIN_TELEGRAM_ID. Запись в customer не обязательна: она нужна только чтобы взять язык
// для подписей кнопок. Пустой ADMIN_TELEGRAM_ID даёт пустой список получателей.
func (cr *CustomerRepository) testBroadcastRecipients(ctx context.Context) ([]BroadcastRecipient, error) {
	adminID := config.GetAdminTelegramId()
	if adminID == 0 {
		return nil, nil
	}

	lang := "en"
	var dbLang string
	err := cr.pool.QueryRow(ctx, "SELECT language FROM customer WHERE telegram_id = $1", adminID).Scan(&dbLang)
	switch {
	case err == nil:
		if strings.TrimSpace(dbLang) != "" {
			lang = dbLang
		}
	case errors.Is(err, pgx.ErrNoRows):
		// Админа нет в customer — шлём на дефолтном языке.
	default:
		return nil, fmt.Errorf("failed to query admin language for test broadcast: %w", err)
	}

	return []BroadcastRecipient{{TelegramID: adminID, Language: lang}}, nil
}

// FindActiveByCurrentTariffID возвращает клиентов с активной подпиской и указанным current_tariff_id.
// CustomerListScope задаёт фильтр для админских списков клиентов.
type CustomerListScope int

const (
	CustomerListScopeAll CustomerListScope = iota
	CustomerListScopeInactive
	CustomerListScopeExpiringSoon
	CustomerListScopeTrial
	CustomerListScopeActive
)

func customerListWhere(scope CustomerListScope) sq.Sqlizer {
	// Не сравнивать timestamptz с sq.Expr в LtOrEq — драйвер пытается присвоить Expr в timestamptz.
	switch scope {
	case CustomerListScopeInactive:
		return sq.Expr("(customer.expire_at IS NULL OR customer.expire_at <= NOW())")
	case CustomerListScopeExpiringSoon:
		return sq.Expr("(customer.expire_at IS NOT NULL AND customer.expire_at > NOW() AND customer.expire_at <= NOW() + INTERVAL '7 days')")
	case CustomerListScopeTrial:
		return sq.Expr(`(customer.expire_at IS NOT NULL AND customer.expire_at > NOW() AND customer.subscription_link IS NOT NULL AND TRIM(customer.subscription_link) <> '' AND NOT EXISTS (SELECT 1 FROM purchase p WHERE p.customer_id = customer.id AND p.status = 'paid' AND p.month > 0))`)
	case CustomerListScopeActive:
		return sq.Expr(`(customer.subscription_link IS NOT NULL AND TRIM(customer.subscription_link) <> '' AND customer.expire_at IS NOT NULL AND customer.expire_at > NOW())`)
	default:
		return nil
	}
}

func customerListOrder(scope CustomerListScope) string {
	switch scope {
	case CustomerListScopeInactive:
		return "expire_at ASC NULLS FIRST"
	case CustomerListScopeExpiringSoon:
		return "expire_at ASC"
	case CustomerListScopeTrial:
		return "id DESC"
	case CustomerListScopeActive:
		return "expire_at DESC"
	default:
		return "id DESC"
	}
}

// CountByListScope возвращает число строк для списка с фильтром.
func (cr *CustomerRepository) CountByListScope(ctx context.Context, scope CustomerListScope) (int64, error) {
	buildSelect := sq.Select("COUNT(*)").
		From("customer").
		PlaceholderFormat(sq.Dollar)
	if w := customerListWhere(scope); w != nil {
		buildSelect = buildSelect.Where(w)
	}
	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}
	var n int64
	err = cr.pool.QueryRow(ctx, sqlStr, args...).Scan(&n)
	return n, err
}

// ListPaged возвращает страницу клиентов для админских списков (Пользователи / неактивные / истекающие).
func (cr *CustomerRepository) ListPaged(ctx context.Context, scope CustomerListScope, offset, limit int) ([]Customer, error) {
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		PlaceholderFormat(sq.Dollar).
		OrderBy(customerListOrder(scope)).
		Limit(uint64(limit)).
		Offset(uint64(offset))
	if w := customerListWhere(scope); w != nil {
		buildSelect = buildSelect.Where(w)
	}
	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build list query: %w", err)
	}
	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := scanCustomer(rows, &customer)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customers: %w", err)
	}
	return customers, nil
}

func escapeSQLLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// AdminSearchMaxResults верхняя граница строк поиска в админке.
const AdminSearchMaxResults = 24

// SearchForAdmin ищет по telegram_username (ILIKE) и подстроке telegram_id (LIKE по тексту).
func (cr *CustomerRepository) SearchForAdmin(ctx context.Context, needle string, limit int) ([]Customer, error) {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = AdminSearchMaxResults
	}
	esc := escapeSQLLikePattern(needle)
	pattern := "%" + esc + "%"

	// Поиск по email аккаунта кабинета — единственный способ найти web-клиента
	// до первой покупки: у него нет ни @username, ни профиля в панели, а
	// синтетический telegram_id админ наизусть не помнит. Логин в панели
	// выглядит как "<id>_<local-part email>", поэтому подстрока email его находит.
	// EXISTS, а не JOIN: у клиента может быть несколько связанных аккаунтов,
	// и JOIN размножил бы строки.
	const cabinetEmailMatch = `EXISTS (
		SELECT 1 FROM cabinet_account_customer_link l
		JOIN cabinet_account a ON a.id = l.account_id
		WHERE l.customer_id = customer.id AND a.email ILIKE ?
	)`

	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.Or{
			sq.Expr("telegram_username ILIKE ?", pattern),
			sq.Expr("CAST(telegram_id AS TEXT) LIKE ?", pattern),
			sq.Expr(cabinetEmailMatch, pattern),
		}).
		OrderBy("id DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build admin search: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("admin search query: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := scanCustomer(rows, &customer)
		if err != nil {
			return nil, fmt.Errorf("scan admin search: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return customers, nil
}

func (cr *CustomerRepository) FindActiveByCurrentTariffID(ctx context.Context, tariffID int64) ([]Customer, error) {
	now := time.Now().UTC()
	buildSelect := sq.Select(customerSelectColumns).
		From("customer").
		Where(sq.And{
			sq.Eq{"current_tariff_id": tariffID},
			sq.NotEq{"expire_at": nil},
			sq.Gt{"expire_at": now},
		}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select active by tariff: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query active by tariff: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err = scanCustomer(rows, &customer)
		if err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return customers, nil
}

// CabinetAccountEmailsByCustomerIDs — email из cabinet_account по связке
// cabinet_account_customer_link (для подписей web-клиентов в админке бота).
func (cr *CustomerRepository) CabinetAccountEmailsByCustomerIDs(ctx context.Context, customerIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if len(customerIDs) == 0 {
		return out, nil
	}
	qb := sq.Select("l.customer_id", "a.email").
		From("cabinet_account_customer_link l").
		Join("cabinet_account a ON a.id = l.account_id").
		Where(sq.Eq{"l.customer_id": customerIDs}).
		PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := qb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build cabinet account emails: %w", err)
	}
	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query cabinet account emails: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int64
		var email *string
		if err := rows.Scan(&cid, &email); err != nil {
			return nil, fmt.Errorf("scan cabinet email: %w", err)
		}
		if email != nil && strings.TrimSpace(*email) != "" {
			out[cid] = strings.TrimSpace(*email)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
