package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Виды уведомлений кабинета. broadcast — из рассылки, остальные — личные.
const (
	CabinetNotificationBroadcast = "broadcast"
	CabinetNotificationPayment   = "payment"
	CabinetNotificationExpiring  = "expiring"
	CabinetNotificationReferral  = "referral"
	CabinetNotificationSupport   = "support"
	CabinetNotificationPartner   = "partner"
	CabinetNotificationDevices   = "devices"
	CabinetNotificationLoyalty   = "loyalty"
	CabinetNotificationTorrent   = "torrent"
)

// CabinetNotificationButton — кнопка-переход по кабинету под уведомлением.
// Key — ключ раздела (tariffs, connections…), путь по нему знает фронт.
type CabinetNotificationButton struct {
	Key string `json:"key"`
}

// CabinetNotification — уведомление, как его создают.
type CabinetNotification struct {
	Kind        string
	BroadcastID *int64
	Important   bool
	Title       string
	Body        string
	Buttons     []CabinetNotificationButton
	// DedupeKey — уникальный ключ события; повтор с тем же ключом не создаёт
	// второе уведомление. Пусто — без проверки.
	DedupeKey string
}

// CabinetNotificationItem — уведомление в ленте конкретного человека.
type CabinetNotificationItem struct {
	ID        int64
	Kind      string
	Important bool
	Title     string
	Body      string
	Buttons   []CabinetNotificationButton
	CreatedAt time.Time
	ReadAt    *time.Time
}

// CabinetUnread — счётчики колокольчика.
type CabinetUnread struct {
	Total     int
	Important int
}

// CabinetBroadcastStats — уведомление рассылки глазами админа.
type CabinetBroadcastStats struct {
	NotificationID int64
	BroadcastID    int64
	Important      bool
	Deleted        bool
	Recipients     int
	Read           int
}

// CabinetPushSubscription — подписка браузера на push.
type CabinetPushSubscription struct {
	ID         int64
	CustomerID int64
	Endpoint   string
	P256dh     string
	Auth       string
}

// CabinetNotificationRepository хранит уведомления кабинета и подписки на push.
type CabinetNotificationRepository struct {
	pool *pgxpool.Pool
}

func NewCabinetNotificationRepository(pool *pgxpool.Pool) *CabinetNotificationRepository {
	return &CabinetNotificationRepository{pool: pool}
}

func marshalCabinetButtons(buttons []CabinetNotificationButton) []byte {
	if len(buttons) == 0 {
		return []byte(`[]`)
	}
	raw, err := json.Marshal(buttons)
	if err != nil {
		return []byte(`[]`)
	}
	return raw
}

// CreateForAudience заводит уведомление рассылки и раздаёт его всем, кто сейчас
// попадает в сегменты. Тот же фильтр, что у Telegram-рассылки, но без отсечки
// web-only: кабинет — как раз их канал. Тестовая рассылка уходит только админу.
//
// Получатели вставляются одним INSERT … SELECT в той же транзакции: даже на
// десятки тысяч человек это доли секунды, а полурозданного уведомления не бывает.
//
// Важная рассылка снимает важность со всех прежних (demoteOtherImportant):
// важное в сервисе одно — последнее, для какой бы аудитории его ни отправили.
// Тестовая рассылка прежних не трогает: проверка текста не должна снимать
// настоящее важное у всех.
func (r *CabinetNotificationRepository) CreateForAudience(ctx context.Context, n CabinetNotification, audiences []string, tariffID *int64, adminTelegramID int64) (id int64, recipients int, err error) {
	normalized, err := NormalizeBroadcastAudiences(audiences)
	if err != nil {
		return 0, 0, err
	}
	if len(normalized) == 0 {
		return 0, 0, errors.New("no broadcast audience selected")
	}

	var filter sq.Sqlizer
	testOnly := normalized[0] == BroadcastAudienceTestBroadcast
	if testOnly {
		if adminTelegramID == 0 {
			return 0, 0, nil
		}
		filter = sq.Eq{"telegram_id": adminTelegramID}
	} else {
		if filter, err = BroadcastAudienceFilter(normalized, tariffID, time.Now()); err != nil {
			return 0, 0, err
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin notification tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err = tx.QueryRow(ctx, `
		INSERT INTO cabinet_notification (kind, broadcast_id, important, title, body, buttons, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		n.Kind, n.BroadcastID, n.Important, n.Title, n.Body, marshalCabinetButtons(n.Buttons), nullableText(n.DedupeKey),
	).Scan(&id); err != nil {
		return 0, 0, fmt.Errorf("insert notification: %w", err)
	}
	if n.Important && n.Kind == CabinetNotificationBroadcast && !testOnly {
		if err = demoteOtherImportant(ctx, tx, id); err != nil {
			return 0, 0, err
		}
	}

	sel := sq.Select().Column(sq.Expr("?::bigint", id)).Column("customer.id").From("customer").PlaceholderFormat(sq.Dollar)
	if filter != nil {
		sel = sel.Where(filter)
	}
	selSQL, selArgs, err := sel.ToSql()
	if err != nil {
		return 0, 0, fmt.Errorf("build recipients select: %w", err)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO cabinet_notification_recipient (notification_id, customer_id) `+selSQL+`
		ON CONFLICT DO NOTHING`, selArgs...)
	if err != nil {
		return 0, 0, fmt.Errorf("insert notification recipients: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit notification: %w", err)
	}
	return id, int(tag.RowsAffected()), nil
}

// CountAudience — сколько человек получит уведомление рассылки: те же
// сегменты, что у CreateForAudience, включая web-only клиентов.
func (r *CabinetNotificationRepository) CountAudience(ctx context.Context, audiences []string, tariffID *int64, adminTelegramID int64) (int, error) {
	normalized, err := NormalizeBroadcastAudiences(audiences)
	if err != nil {
		return 0, err
	}
	if len(normalized) == 0 {
		return 0, errors.New("no broadcast audience selected")
	}
	sel := sq.Select("count(*)").From("customer").PlaceholderFormat(sq.Dollar)
	if normalized[0] == BroadcastAudienceTestBroadcast {
		if adminTelegramID == 0 {
			return 0, nil
		}
		sel = sel.Where(sq.Eq{"telegram_id": adminTelegramID})
	} else {
		filter, ferr := BroadcastAudienceFilter(normalized, tariffID, time.Now())
		if ferr != nil {
			return 0, ferr
		}
		if filter != nil {
			sel = sel.Where(filter)
		}
	}
	q, args, err := sel.ToSql()
	if err != nil {
		return 0, fmt.Errorf("build audience count: %w", err)
	}
	var n int
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count notification audience: %w", err)
	}
	return n, nil
}

// CreatePersonal заводит личное уведомление одному человеку. created = false —
// такое событие уже было (тот же DedupeKey), второе не заводится.
func (r *CabinetNotificationRepository) CreatePersonal(ctx context.Context, customerID int64, n CabinetNotification) (id int64, created bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin notification tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO cabinet_notification (kind, important, title, body, buttons, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
		RETURNING id`,
		n.Kind, n.Important, n.Title, n.Body, marshalCabinetButtons(n.Buttons), nullableText(n.DedupeKey),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("insert personal notification: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cabinet_notification_recipient (notification_id, customer_id) VALUES ($1, $2)`,
		id, customerID); err != nil {
		return 0, false, fmt.Errorf("insert personal recipient: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit personal notification: %w", err)
	}
	return id, true, nil
}

// currentImportantCTE — единственное важное уведомление человека ($1): самое
// свежее из отмеченных важными.
//
// Важных рассылок и так одна — новая снимает флаг с прежних (см.
// demoteOtherImportant). Но важными бывают и личные события («подписка
// кончается завтра»), и рядом с важной рассылкой их было бы два; правило при
// чтении оставляет одно — последнее. Скрытые крестиком учитываются: убрал
// человек свежее важное — прежнее важным не возвращается. Удалённые админом и
// со снятой важностью — нет.
const currentImportantCTE = `WITH cur AS (
		SELECT n.id FROM cabinet_notification_recipient r
		JOIN cabinet_notification n ON n.id = r.notification_id
		WHERE r.customer_id = $1 AND n.important AND n.deleted_at IS NULL
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT 1
	)`

// List — лента человека: непрочитанное важное закреплено сверху, дальше по
// времени. Удалённые админом не показываются.
func (r *CabinetNotificationRepository) List(ctx context.Context, customerID int64, limit, offset int) ([]CabinetNotificationItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM cabinet_notification_recipient r
		JOIN cabinet_notification n ON n.id = r.notification_id
		WHERE r.customer_id = $1 AND r.hidden_at IS NULL AND n.deleted_at IS NULL`, customerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}
	rows, err := r.pool.Query(ctx, currentImportantCTE+`
		SELECT n.id, n.kind, cur.id IS NOT NULL, n.title, n.body, n.buttons, n.created_at, r.read_at
		FROM cabinet_notification_recipient r
		JOIN cabinet_notification n ON n.id = r.notification_id
		LEFT JOIN cur ON cur.id = n.id
		WHERE r.customer_id = $1 AND r.hidden_at IS NULL AND n.deleted_at IS NULL
		ORDER BY (cur.id IS NOT NULL AND r.read_at IS NULL) DESC, n.created_at DESC, n.id DESC
		LIMIT $2 OFFSET $3`, customerID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := make([]CabinetNotificationItem, 0, limit)
	for rows.Next() {
		var it CabinetNotificationItem
		var buttons []byte
		if err := rows.Scan(&it.ID, &it.Kind, &it.Important, &it.Title, &it.Body, &buttons, &it.CreatedAt, &it.ReadAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		if len(buttons) > 0 {
			_ = json.Unmarshal(buttons, &it.Buttons)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notifications: %w", err)
	}
	return out, total, nil
}

// Unread — счётчики колокольчика и плашки важного.
func (r *CabinetNotificationRepository) Unread(ctx context.Context, customerID int64) (CabinetUnread, error) {
	var u CabinetUnread
	err := r.pool.QueryRow(ctx, currentImportantCTE+`
		SELECT count(*), count(*) FILTER (WHERE n.id IN (SELECT id FROM cur))
		FROM cabinet_notification_recipient r
		JOIN cabinet_notification n ON n.id = r.notification_id
		WHERE r.customer_id = $1 AND r.read_at IS NULL AND r.hidden_at IS NULL AND n.deleted_at IS NULL`, customerID).Scan(&u.Total, &u.Important)
	if err != nil {
		return u, fmt.Errorf("count unread notifications: %w", err)
	}
	return u, nil
}

// LatestUnreadImportant — важное человека, если оно не прочитано: его плашка
// висит на главной.
func (r *CabinetNotificationRepository) LatestUnreadImportant(ctx context.Context, customerID int64) (*CabinetNotificationItem, error) {
	var it CabinetNotificationItem
	var buttons []byte
	err := r.pool.QueryRow(ctx, currentImportantCTE+`
		SELECT n.id, n.kind, n.important, n.title, n.body, n.buttons, n.created_at, r.read_at
		FROM cur
		JOIN cabinet_notification n ON n.id = cur.id
		JOIN cabinet_notification_recipient r ON r.notification_id = n.id AND r.customer_id = $1
		WHERE r.read_at IS NULL AND r.hidden_at IS NULL`, customerID).Scan(&it.ID, &it.Kind, &it.Important, &it.Title, &it.Body, &buttons, &it.CreatedAt, &it.ReadAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest important notification: %w", err)
	}
	if len(buttons) > 0 {
		_ = json.Unmarshal(buttons, &it.Buttons)
	}
	return &it, nil
}

// MarkRead отмечает прочитанными уведомления человека. Чужие id молча
// игнорируются: условие по customer_id стоит в самом UPDATE.
func (r *CabinetNotificationRepository) MarkRead(ctx context.Context, customerID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE cabinet_notification_recipient SET read_at = now()
		WHERE customer_id = $1 AND notification_id = ANY($2) AND read_at IS NULL`, customerID, ids)
	if err != nil {
		return fmt.Errorf("mark notifications read: %w", err)
	}
	return nil
}

// Hide убирает уведомления из ленты человека (крестик). Заодно отмечает их
// прочитанными: удалённое не должно висеть в счётчике колокольчика.
func (r *CabinetNotificationRepository) Hide(ctx context.Context, customerID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE cabinet_notification_recipient
		SET hidden_at = now(), read_at = coalesce(read_at, now())
		WHERE customer_id = $1 AND notification_id = ANY($2) AND hidden_at IS NULL`, customerID, ids)
	if err != nil {
		return fmt.Errorf("hide notifications: %w", err)
	}
	return nil
}

// MarkAllRead — «Прочитать все».
func (r *CabinetNotificationRepository) MarkAllRead(ctx context.Context, customerID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE cabinet_notification_recipient SET read_at = now()
		WHERE customer_id = $1 AND read_at IS NULL`, customerID)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

// BroadcastStats — уведомления рассылок для истории: важность, удалено ли,
// скольким раздали и сколько прочитали. Ключ — id рассылки.
func (r *CabinetNotificationRepository) BroadcastStats(ctx context.Context, broadcastIDs []int64) (map[int64]CabinetBroadcastStats, error) {
	out := make(map[int64]CabinetBroadcastStats, len(broadcastIDs))
	if len(broadcastIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT n.id, n.broadcast_id, n.important, n.deleted_at IS NOT NULL,
			count(rc.customer_id), count(rc.read_at)
		FROM cabinet_notification n
		LEFT JOIN cabinet_notification_recipient rc ON rc.notification_id = n.id
		WHERE n.broadcast_id = ANY($1)
		GROUP BY n.id`, broadcastIDs)
	if err != nil {
		return nil, fmt.Errorf("notification stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s CabinetBroadcastStats
		if err := rows.Scan(&s.NotificationID, &s.BroadcastID, &s.Important, &s.Deleted, &s.Recipients, &s.Read); err != nil {
			return nil, fmt.Errorf("scan notification stats: %w", err)
		}
		out[s.BroadcastID] = s
	}
	return out, rows.Err()
}

// demoteOtherImportant снимает важность со всех рассылок, кроме keepID:
// важная рассылка в сервисе одна.
func demoteOtherImportant(ctx context.Context, tx pgx.Tx, keepID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE cabinet_notification SET important = false
		WHERE kind = $1 AND important AND id <> $2`, CabinetNotificationBroadcast, keepID); err != nil {
		return fmt.Errorf("demote previous important: %w", err)
	}
	return nil
}

// SetImportantByBroadcast меняет важность уведомления рассылки. Снятая важность
// убирает плашку с главной и закрепление в ленте у всех сразу; поставленная —
// снимает её с прежней важной рассылки: важное одно.
func (r *CabinetNotificationRepository) SetImportantByBroadcast(ctx context.Context, broadcastID int64, important bool) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin importance tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, `UPDATE cabinet_notification SET important = $2 WHERE broadcast_id = $1 RETURNING id`,
		broadcastID, important).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("set notification importance: %w", err)
	}
	if important {
		if err = demoteOtherImportant(ctx, tx, id); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit importance: %w", err)
	}
	return true, nil
}

// DeleteByBroadcast убирает уведомление рассылки из кабинета у всех.
func (r *CabinetNotificationRepository) DeleteByBroadcast(ctx context.Context, broadcastID int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE cabinet_notification SET deleted_at = now() WHERE broadcast_id = $1 AND deleted_at IS NULL`, broadcastID)
	if err != nil {
		return false, fmt.Errorf("delete notification: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// UpsertPushSubscription сохраняет подписку браузера. Тот же endpoint у другого
// аккаунта (вышли и вошли другим) переезжает к новому владельцу.
func (r *CabinetNotificationRepository) UpsertPushSubscription(ctx context.Context, sub CabinetPushSubscription, userAgent string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO cabinet_push_subscription (customer_id, endpoint, p256dh, auth, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (endpoint) DO UPDATE
		SET customer_id = EXCLUDED.customer_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
			user_agent = EXCLUDED.user_agent`,
		sub.CustomerID, sub.Endpoint, sub.P256dh, sub.Auth, userAgent)
	if err != nil {
		return fmt.Errorf("upsert push subscription: %w", err)
	}
	return nil
}

// DeletePushSubscription — отписка браузера. customerID 0 — без проверки
// владельца (браузер сообщил, что подписка мертва: 404/410 от push-сервиса).
func (r *CabinetNotificationRepository) DeletePushSubscription(ctx context.Context, customerID int64, endpoint string) error {
	var err error
	if customerID == 0 {
		_, err = r.pool.Exec(ctx, `DELETE FROM cabinet_push_subscription WHERE endpoint = $1`, endpoint)
	} else {
		_, err = r.pool.Exec(ctx, `DELETE FROM cabinet_push_subscription WHERE endpoint = $1 AND customer_id = $2`, endpoint, customerID)
	}
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// TouchPushSubscription отмечает успешную доставку.
func (r *CabinetNotificationRepository) TouchPushSubscription(ctx context.Context, id int64) {
	_, _ = r.pool.Exec(ctx, `UPDATE cabinet_push_subscription SET last_success_at = now() WHERE id = $1`, id)
}

// PushSubscriptionsForNotification — подписки всех получателей уведомления.
func (r *CabinetNotificationRepository) PushSubscriptionsForNotification(ctx context.Context, notificationID int64) ([]CabinetPushSubscription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.customer_id, s.endpoint, s.p256dh, s.auth
		FROM cabinet_push_subscription s
		JOIN cabinet_notification_recipient rc ON rc.customer_id = s.customer_id
		WHERE rc.notification_id = $1`, notificationID)
	if err != nil {
		return nil, fmt.Errorf("push subscriptions: %w", err)
	}
	defer rows.Close()
	var out []CabinetPushSubscription
	for rows.Next() {
		var s CabinetPushSubscription
		if err := rows.Scan(&s.ID, &s.CustomerID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// HasPushSubscription — подписан ли этот браузер (по endpoint) у человека.
func (r *CabinetNotificationRepository) HasPushSubscription(ctx context.Context, customerID int64, endpoint string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cabinet_push_subscription WHERE customer_id = $1 AND endpoint = $2)`,
		customerID, endpoint).Scan(&ok)
	return ok, err
}

// VapidKeys — ключи push. Нет в базе — генерирует gen и сохраняет; при гонке
// двух процессов побеждает первая запись, и ключи у обоих одинаковые.
func (r *CabinetNotificationRepository) VapidKeys(ctx context.Context, gen func() (public, private string, err error)) (public, private string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT public_key, private_key FROM cabinet_vapid_key WHERE id = 1`).Scan(&public, &private)
	if err == nil {
		return public, private, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("read vapid keys: %w", err)
	}
	pub, priv, err := gen()
	if err != nil {
		return "", "", fmt.Errorf("generate vapid keys: %w", err)
	}
	if _, err = r.pool.Exec(ctx, `INSERT INTO cabinet_vapid_key (id, public_key, private_key) VALUES (1, $1, $2)
		ON CONFLICT (id) DO NOTHING`, pub, priv); err != nil {
		return "", "", fmt.Errorf("save vapid keys: %w", err)
	}
	err = r.pool.QueryRow(ctx, `SELECT public_key, private_key FROM cabinet_vapid_key WHERE id = 1`).Scan(&public, &private)
	if err != nil {
		return "", "", fmt.Errorf("reread vapid keys: %w", err)
	}
	return public, private, nil
}
