package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Виды трафика, которые можно докупить и считать отдельно.
const (
	TrafficKindMain   = "main"   // обычные серверы
	TrafficKindBypass = "bypass" // серверы обхода
)

// TrafficPeriodStart — начало календарного месяца (UTC), в который попадает t.
// Именно так сбрасывает трафик панель со стратегией MONTH.
func TrafficPeriodStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// TrafficPeriodEnd — начало следующего календарного месяца: до этого момента живут докупленные пакеты.
func TrafficPeriodEnd(t time.Time) time.Time {
	return TrafficPeriodStart(t).AddDate(0, 1, 0)
}

// TariffBypass — настройки обхода в тарифе. LimitBytes = 0: у тарифа нет отдельного лимита обхода.
type TariffBypass struct {
	LimitBytes int64
	SquadUUIDs string // через запятую, как active_internal_squad_uuids
	NodeUUIDs  string
}

// Enabled — включён ли у тарифа отдельный лимит обхода.
func (b TariffBypass) Enabled() bool { return b.LimitBytes > 0 }

// TrafficPack — пакет докупки трафика у тарифа.
type TrafficPack struct {
	ID          int64
	TariffID    int64
	Kind        string
	Bytes       int64
	AmountRub   int
	AmountStars *int
	SortOrder   int
	IsActive    bool
}

// BypassState — снимок настроек обхода клиента и накопленный расход за месяц.
type BypassState struct {
	CustomerID   int64
	TariffID     *int64
	LimitBytes   int64
	SquadUUIDs   string
	NodeUUIDs    string
	PeriodStart  time.Time
	UsedBytes    int64
	SquadRemoved bool
	CheckedAt    *time.Time
}

// TrafficExtra — купленный пакет трафика.
type TrafficExtra struct {
	ID         int64
	CustomerID int64
	Kind       string
	Bytes      int64
	PurchaseID *int64
	ValidUntil time.Time
}

// ExpiredExtra — клиент и вид трафика, у которых пакет только что истёк.
type ExpiredExtra struct {
	CustomerID int64
	Kind       string
}

// TrafficSplitRepository — всё, что связано с раздельным трафиком и докупкой.
type TrafficSplitRepository struct {
	pool *pgxpool.Pool
}

func NewTrafficSplitRepository(pool *pgxpool.Pool) *TrafficSplitRepository {
	return &TrafficSplitRepository{pool: pool}
}

// GetTariffBypass возвращает настройки обхода тарифа. Нет тарифа — нулевое значение (обход выключен).
func (r *TrafficSplitRepository) GetTariffBypass(ctx context.Context, tariffID int64) (TariffBypass, error) {
	var b TariffBypass
	err := r.pool.QueryRow(ctx,
		`SELECT bypass_traffic_limit_bytes, bypass_squad_uuids, bypass_node_uuids FROM tariff WHERE id = $1`, tariffID,
	).Scan(&b.LimitBytes, &b.SquadUUIDs, &b.NodeUUIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return TariffBypass{}, nil
	}
	return b, err
}

// SetTariffBypass сохраняет настройки обхода тарифа. Существующих клиентов не трогает.
func (r *TrafficSplitRepository) SetTariffBypass(ctx context.Context, tariffID int64, b TariffBypass) error {
	if b.LimitBytes < 0 {
		b.LimitBytes = 0
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE tariff SET bypass_traffic_limit_bytes = $2, bypass_squad_uuids = $3, bypass_node_uuids = $4, updated_at = NOW() WHERE id = $1`,
		tariffID, b.LimitBytes, b.SquadUUIDs, b.NodeUUIDs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tariff %d not found", tariffID)
	}
	return nil
}

// ListPacks — пакеты тарифа. kind пустой — оба вида; onlyActive — только включённые.
func (r *TrafficSplitRepository) ListPacks(ctx context.Context, tariffID int64, kind string, onlyActive bool) ([]TrafficPack, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tariff_id, kind, bytes, amount_rub, amount_stars, sort_order, is_active
		FROM tariff_traffic_pack
		WHERE tariff_id = $1 AND ($2 = '' OR kind = $2) AND (NOT $3 OR is_active)
		ORDER BY kind, sort_order, bytes, id`, tariffID, kind, onlyActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrafficPack
	for rows.Next() {
		var p TrafficPack
		if err := rows.Scan(&p.ID, &p.TariffID, &p.Kind, &p.Bytes, &p.AmountRub, &p.AmountStars, &p.SortOrder, &p.IsActive); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPack возвращает один пакет по id (nil, если нет).
func (r *TrafficSplitRepository) GetPack(ctx context.Context, id int64) (*TrafficPack, error) {
	var p TrafficPack
	err := r.pool.QueryRow(ctx, `
		SELECT id, tariff_id, kind, bytes, amount_rub, amount_stars, sort_order, is_active
		FROM tariff_traffic_pack WHERE id = $1`, id).
		Scan(&p.ID, &p.TariffID, &p.Kind, &p.Bytes, &p.AmountRub, &p.AmountStars, &p.SortOrder, &p.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ReplacePacks заменяет пакеты тарифа списком из админки (в одной транзакции).
func (r *TrafficSplitRepository) ReplacePacks(ctx context.Context, tariffID int64, packs []TrafficPack) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := replacePacksTx(ctx, tx, tariffID, packs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SaveTariffTraffic сохраняет настройки обхода и пакеты тарифа одной транзакцией.
func (r *TrafficSplitRepository) SaveTariffTraffic(ctx context.Context, tariffID int64, b TariffBypass, packs []TrafficPack) error {
	if b.LimitBytes < 0 {
		b.LimitBytes = 0
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx,
		`UPDATE tariff SET bypass_traffic_limit_bytes = $2, bypass_squad_uuids = $3, bypass_node_uuids = $4, updated_at = NOW() WHERE id = $1`,
		tariffID, b.LimitBytes, b.SquadUUIDs, b.NodeUUIDs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tariff %d not found", tariffID)
	}
	if err := replacePacksTx(ctx, tx, tariffID, packs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// replacePacksTx сопоставляет пакеты по (вид, размер): существующие обновляются на месте и сохраняют id
// (страница оплаты, открытая у клиента, остаётся рабочей), лишние удаляются, новые добавляются.
// Купленные пакеты (customer_traffic_extra) не страдают: они хранят свои байты.
func replacePacksTx(ctx context.Context, tx pgx.Tx, tariffID int64, packs []TrafficPack) error {
	rows, err := tx.Query(ctx, `SELECT id, kind, bytes FROM tariff_traffic_pack WHERE tariff_id = $1`, tariffID)
	if err != nil {
		return err
	}
	existing := map[string]int64{}
	for rows.Next() {
		var id, b int64
		var kind string
		if err := rows.Scan(&id, &kind, &b); err != nil {
			rows.Close()
			return err
		}
		existing[fmt.Sprintf("%s|%d", kind, b)] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	keep := map[int64]struct{}{}
	seen := map[string]struct{}{}
	for i, p := range packs {
		if p.Kind != TrafficKindMain && p.Kind != TrafficKindBypass {
			return fmt.Errorf("pack %d: unknown kind %q", i, p.Kind)
		}
		if p.Bytes <= 0 || p.AmountRub < 0 {
			return fmt.Errorf("pack %d: bytes must be > 0 and price >= 0", i)
		}
		key := fmt.Sprintf("%s|%d", p.Kind, p.Bytes)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("pack %d: duplicate size", i)
		}
		seen[key] = struct{}{}
		if id, ok := existing[key]; ok {
			keep[id] = struct{}{}
			if _, err := tx.Exec(ctx, `
				UPDATE tariff_traffic_pack SET amount_rub = $2, amount_stars = $3, sort_order = $4, is_active = $5 WHERE id = $1`,
				id, p.AmountRub, p.AmountStars, i, p.IsActive); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tariff_traffic_pack (tariff_id, kind, bytes, amount_rub, amount_stars, sort_order, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			tariffID, p.Kind, p.Bytes, p.AmountRub, p.AmountStars, i, p.IsActive); err != nil {
			return err
		}
	}
	for _, id := range existing {
		if _, ok := keep[id]; ok {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM tariff_traffic_pack WHERE id = $1`, id); err != nil {
			return err
		}
	}
	return nil
}

const bypassStateColumns = "customer_id, tariff_id, limit_bytes, squad_uuids, node_uuids, period_start, used_bytes, squad_removed, checked_at"

func scanBypassState(row rowScanner) (*BypassState, error) {
	var s BypassState
	if err := row.Scan(&s.CustomerID, &s.TariffID, &s.LimitBytes, &s.SquadUUIDs, &s.NodeUUIDs, &s.PeriodStart, &s.UsedBytes, &s.SquadRemoved, &s.CheckedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetState — снимок и расход обхода клиента; nil, если клиент никогда не покупал тариф с обходом.
func (r *TrafficSplitRepository) GetState(ctx context.Context, customerID int64) (*BypassState, error) {
	s, err := scanBypassState(r.pool.QueryRow(ctx,
		`SELECT `+bypassStateColumns+` FROM customer_bypass_state WHERE customer_id = $1`, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

// SaveSnapshot запоминает настройки обхода тарифа у клиента (вызывается при покупке и продлении).
// Расход за текущий месяц и признак «сквад снят» сохраняются. Если у нового тарифа обхода нет,
// признак снимается: профиль тарифа выставит сквады заново.
func (r *TrafficSplitRepository) SaveSnapshot(ctx context.Context, customerID int64, tariffID *int64, b TariffBypass, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO customer_bypass_state (customer_id, tariff_id, limit_bytes, squad_uuids, node_uuids, period_start)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (customer_id) DO UPDATE SET
			tariff_id = EXCLUDED.tariff_id,
			limit_bytes = EXCLUDED.limit_bytes,
			squad_uuids = EXCLUDED.squad_uuids,
			node_uuids = EXCLUDED.node_uuids,
			squad_removed = CASE WHEN EXCLUDED.limit_bytes > 0 THEN customer_bypass_state.squad_removed ELSE FALSE END,
			checked_at = NULL,
			updated_at = NOW()`,
		customerID, tariffID, b.LimitBytes, b.SquadUUIDs, b.NodeUUIDs, TrafficPeriodStart(now))
	return err
}

// SaveUsage записывает накопленный расход и признак «сквад снят».
func (r *TrafficSplitRepository) SaveUsage(ctx context.Context, customerID int64, periodStart time.Time, used int64, squadRemoved bool, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE customer_bypass_state
		SET period_start = $2, used_bytes = $3, squad_removed = $4, checked_at = $5, updated_at = NOW()
		WHERE customer_id = $1`, customerID, periodStart, used, squadRemoved, now)
	return err
}

// ListTrackedStates — клиенты, у которых есть отдельный лимит обхода.
func (r *TrafficSplitRepository) ListTrackedStates(ctx context.Context) ([]BypassState, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.customer_id, s.tariff_id, s.limit_bytes, s.squad_uuids, s.node_uuids, s.period_start, s.used_bytes, s.squad_removed, s.checked_at
		FROM customer_bypass_state s
		JOIN customer c ON c.id = s.customer_id
		WHERE s.limit_bytes > 0 AND c.expire_at IS NOT NULL AND c.expire_at > NOW()
		ORDER BY s.customer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BypassState
	for rows.Next() {
		s, err := scanBypassState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// AddExtra сохраняет купленный пакет. Повторный вызов с тем же purchase_id ничего не делает (false).
func (r *TrafficSplitRepository) AddExtra(ctx context.Context, e TrafficExtra) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO customer_traffic_extra (customer_id, kind, bytes, purchase_id, valid_until)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`, e.CustomerID, e.Kind, e.Bytes, e.PurchaseID, e.ValidUntil)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ActiveExtraBytes — сумма не истёкших пакетов клиента данного вида.
func (r *TrafficSplitRepository) ActiveExtraBytes(ctx context.Context, customerID int64, kind string, now time.Time) (int64, error) {
	var sum int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(bytes), 0)::bigint FROM customer_traffic_extra
		WHERE customer_id = $1 AND kind = $2 AND valid_until > $3`, customerID, kind, now).Scan(&sum)
	return sum, err
}

// ListActiveExtras — не истёкшие пакеты клиента (для окна расхода).
func (r *TrafficSplitRepository) ListActiveExtras(ctx context.Context, customerID int64, now time.Time) ([]TrafficExtra, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, kind, bytes, purchase_id, valid_until FROM customer_traffic_extra
		WHERE customer_id = $1 AND valid_until > $2 ORDER BY valid_until, id`, customerID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrafficExtra
	for rows.Next() {
		var e TrafficExtra
		if err := rows.Scan(&e.ID, &e.CustomerID, &e.Kind, &e.Bytes, &e.PurchaseID, &e.ValidUntil); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteExtra убирает пакет клиента (отзыв админом) и возвращает его вид. Чужой или несуществующий — ошибка.
func (r *TrafficSplitRepository) DeleteExtra(ctx context.Context, customerID, extraID int64) (string, error) {
	var kind string
	err := r.pool.QueryRow(ctx, `DELETE FROM customer_traffic_extra WHERE id = $1 AND customer_id = $2 RETURNING kind`, extraID, customerID).Scan(&kind)
	return kind, err
}

// ListExpiredExtras — у кого и какого вида пакеты уже истекли (но ещё не удалены).
func (r *TrafficSplitRepository) ListExpiredExtras(ctx context.Context, now time.Time) ([]ExpiredExtra, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT customer_id, kind FROM customer_traffic_extra WHERE valid_until <= $1`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiredExtra
	for rows.Next() {
		var e ExpiredExtra
		if err := rows.Scan(&e.CustomerID, &e.Kind); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteExpiredExtrasFor удаляет истёкшие пакеты клиента данного вида. Вызывается после того, как
// лимит в панели уже приведён в порядок: упадёт панель — пакеты останутся и повторим на следующем проходе.
func (r *TrafficSplitRepository) DeleteExpiredExtrasFor(ctx context.Context, customerID int64, kind string, now time.Time) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM customer_traffic_extra WHERE customer_id = $1 AND kind = $2 AND valid_until <= $3`, customerID, kind, now)
	return err
}

// ActiveMainExtraSums — клиенты с действующими пакетами обычного трафика и их сумма.
func (r *TrafficSplitRepository) ActiveMainExtraSums(ctx context.Context, now time.Time) (map[int64]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT customer_id, SUM(bytes)::bigint FROM customer_traffic_extra
		WHERE kind = 'main' AND valid_until > $1 GROUP BY customer_id`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, sum int64
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, err
		}
		out[id] = sum
	}
	return out, rows.Err()
}

// TariffMismatch — клиент, у которого тариф сменился мимо покупки (админ), а снимок остался от прежнего.
type TariffMismatch struct {
	CustomerID int64
	TariffID   int64
}

// ListTariffMismatches — клиенты со снимком, чей текущий тариф не совпадает с тарифом снимка.
// Клиентов без снимка (покупали до появления функции) не трогаем: для них действует правило «только для будущих покупок».
func (r *TrafficSplitRepository) ListTariffMismatches(ctx context.Context) ([]TariffMismatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.current_tariff_id
		FROM customer_bypass_state s
		JOIN customer c ON c.id = s.customer_id
		WHERE c.current_tariff_id IS NOT NULL
		  AND s.tariff_id IS DISTINCT FROM c.current_tariff_id
		  AND c.expire_at IS NOT NULL AND c.expire_at > NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TariffMismatch
	for rows.Next() {
		var m TariffMismatch
		if err := rows.Scan(&m.CustomerID, &m.TariffID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListRemovedStates — клиенты, у которых воркер снял сквад обхода (нужно вернуть при выключении функции).
func (r *TrafficSplitRepository) ListRemovedStates(ctx context.Context) ([]BypassState, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+bypassStateColumns+` FROM customer_bypass_state WHERE squad_removed`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BypassState
	for rows.Next() {
		s, err := scanBypassState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// ResetAlerts снимает отметки об отправленных предупреждениях клиента по виду трафика:
// после докупки пакета лимит вырос, и приближение к нему снова стоит заметить.
func (r *TrafficSplitRepository) ResetAlerts(ctx context.Context, customerID int64, kind string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM customer_traffic_alert WHERE customer_id = $1 AND kind = $2`, customerID, kind)
	return err
}

// TryRecordAlert отмечает, что уведомление данного уровня за месяц отправляется. false — уже было.
func (r *TrafficSplitRepository) TryRecordAlert(ctx context.Context, customerID int64, kind string, periodStart time.Time, level int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO customer_traffic_alert (customer_id, kind, period_start, level)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, customerID, kind, periodStart, level)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ForgetAlert снимает отметку (если отправка не удалась — попробуем снова на следующем проходе).
func (r *TrafficSplitRepository) ForgetAlert(ctx context.Context, customerID int64, kind string, periodStart time.Time, level int) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM customer_traffic_alert WHERE customer_id = $1 AND kind = $2 AND period_start = $3 AND level = $4`,
		customerID, kind, periodStart, level)
	return err
}

// CustomersByPanelUserIDs — клиенты по числовым id профилей панели.
func (r *TrafficSplitRepository) CustomersByPanelUserIDs(ctx context.Context, ids []int64) (map[int64]Customer, error) {
	out := make(map[int64]Customer, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+customerSelectColumns+` FROM customer WHERE remnawave_user_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Customer
		if err := scanCustomer(rows, &c); err != nil {
			return nil, err
		}
		if c.RemnawaveUserID != nil {
			out[*c.RemnawaveUserID] = c
		}
	}
	return out, rows.Err()
}
