package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type bedolagaTariff struct {
	ID                int
	Name              string
	Description       *string
	DisplayOrder      int
	IsActive          bool
	TrafficLimitGB    int
	DeviceLimit       int
	AllowedSquadsJSON []byte
	PeriodPricesJSON  []byte
	TierLevel         int
	ExternalSquadUUID *string
	IsDaily           bool
	TrafficResetMode  *string
}

type bedolagaUser struct {
	ID            int
	TelegramID    *int64
	Username      *string
	Language      string
	BalanceKopeks int64
	Status        string
	AuthType      string
	Email         *string
	EmailVerified bool
	PasswordHash  *string
	ReferredByID  *int
	GoogleID      *string
	YandexID      *string
	VKID          *int64
	RemnawaveUUID *string
}

type bedolagaSub struct {
	ID                 int
	UserID             int
	Status             string
	EndDate            *time.Time
	SubscriptionURL    *string
	TariffID           *int
	RemnawaveUUID      *string
	RemnawaveShortUUID *string
	DeviceLimit        int
}

// sourceCaps describes optional columns across Bedolaga 3.x / 4.x schemas.
type sourceCaps struct {
	HasAuthType            bool
	HasRemnawaveUUIDUsers  bool
	HasRemnawaveIDUsers    bool // Bedolaga 4.x / RW 3.0 era marker
	HasRemnawaveUUIDSubs   bool
	HasRemnawaveShortUUID  bool
	HasIsDaily             bool
	HasTrafficResetMode    bool
	HasExternalSquadUUID   bool
	HasEmailVerified       bool
	HasGoogleID            bool
	HasYandexID            bool
	HasVKID                bool
	HasPasswordHash        bool
	HasTariffsTable        bool
	HasSubscriptionsTable  bool
	HintGeneration         string // "bedolaga_3x_rw2" | "bedolaga_4x_rw3" | "unknown"
}

type sourceDB struct {
	pool *pgxpool.Pool
	caps sourceCaps
}

func openSource(ctx context.Context, dsn string) (*sourceDB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect bedolaga db: %w", err)
	}
	s := &sourceDB{pool: pool}
	if err := s.detectCaps(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *sourceDB) Close() { s.pool.Close() }

func (s *sourceDB) hasColumn(ctx context.Context, table, column string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&n)
	return n > 0, err
}

func (s *sourceDB) hasTable(ctx context.Context, table string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&n)
	return n > 0, err
}

func (s *sourceDB) detectCaps(ctx context.Context) error {
	var err error
	mustCol := func(table, col string, dst *bool) error {
		ok, e := s.hasColumn(ctx, table, col)
		if e != nil {
			return e
		}
		*dst = ok
		return nil
	}
	if s.caps.HasTariffsTable, err = s.hasTable(ctx, "tariffs"); err != nil {
		return err
	}
	if s.caps.HasSubscriptionsTable, err = s.hasTable(ctx, "subscriptions"); err != nil {
		return err
	}
	usersOK, err := s.hasTable(ctx, "users")
	if err != nil {
		return err
	}
	if !usersOK {
		return fmt.Errorf("bedolaga source: table users not found — is this a Bedolaga database?")
	}

	_ = mustCol("users", "auth_type", &s.caps.HasAuthType)
	_ = mustCol("users", "remnawave_uuid", &s.caps.HasRemnawaveUUIDUsers)
	_ = mustCol("users", "remnawave_id", &s.caps.HasRemnawaveIDUsers)
	_ = mustCol("users", "email_verified", &s.caps.HasEmailVerified)
	_ = mustCol("users", "password_hash", &s.caps.HasPasswordHash)
	_ = mustCol("users", "google_id", &s.caps.HasGoogleID)
	_ = mustCol("users", "yandex_id", &s.caps.HasYandexID)
	_ = mustCol("users", "vk_id", &s.caps.HasVKID)
	_ = mustCol("subscriptions", "remnawave_uuid", &s.caps.HasRemnawaveUUIDSubs)
	_ = mustCol("subscriptions", "remnawave_short_uuid", &s.caps.HasRemnawaveShortUUID)
	_ = mustCol("tariffs", "is_daily", &s.caps.HasIsDaily)
	_ = mustCol("tariffs", "traffic_reset_mode", &s.caps.HasTrafficResetMode)
	_ = mustCol("tariffs", "external_squad_uuid", &s.caps.HasExternalSquadUUID)

	// remnawave_id (numeric panel id) is the Bedolaga 4.x / Remnawave 3.0 marker.
	if s.caps.HasRemnawaveIDUsers {
		s.caps.HintGeneration = "bedolaga_4x_rw3"
	} else if s.caps.HasRemnawaveUUIDUsers || s.caps.HasRemnawaveUUIDSubs {
		s.caps.HintGeneration = "bedolaga_3x_rw2"
	} else {
		s.caps.HintGeneration = "unknown"
	}
	return nil
}

func (s *sourceDB) LoadTariffs(ctx context.Context) ([]bedolagaTariff, error) {
	if !s.caps.HasTariffsTable {
		return nil, fmt.Errorf("table tariffs not found")
	}
	extExpr := "NULL::text"
	if s.caps.HasExternalSquadUUID {
		extExpr = "external_squad_uuid"
	}
	dailyExpr := "false"
	if s.caps.HasIsDaily {
		dailyExpr = "COALESCE(is_daily,false)"
	}
	resetExpr := "NULL::text"
	if s.caps.HasTrafficResetMode {
		resetExpr = "traffic_reset_mode"
	}
	q := fmt.Sprintf(`
		SELECT id, name, description, COALESCE(display_order,0), COALESCE(is_active,true),
		       COALESCE(traffic_limit_gb,0), COALESCE(device_limit,1),
		       COALESCE(allowed_squads::text, '[]'), COALESCE(period_prices::text, '{}'),
		       COALESCE(tier_level,1), %s, %s, %s
		FROM tariffs
		ORDER BY display_order ASC, id ASC`, extExpr, dailyExpr, resetExpr)
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("select tariffs: %w", err)
	}
	defer rows.Close()
	var out []bedolagaTariff
	for rows.Next() {
		var t bedolagaTariff
		var allowed, prices string
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.DisplayOrder, &t.IsActive,
			&t.TrafficLimitGB, &t.DeviceLimit, &allowed, &prices, &t.TierLevel,
			&t.ExternalSquadUUID, &t.IsDaily, &t.TrafficResetMode); err != nil {
			return nil, err
		}
		t.AllowedSquadsJSON = []byte(allowed)
		t.PeriodPricesJSON = []byte(prices)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *sourceDB) LoadUsers(ctx context.Context) ([]bedolagaUser, error) {
	authExpr := "'telegram'"
	if s.caps.HasAuthType {
		authExpr = "COALESCE(auth_type,'telegram')"
	}
	emailVerExpr := "false"
	if s.caps.HasEmailVerified {
		emailVerExpr = "COALESCE(email_verified,false)"
	}
	passExpr := "NULL::text"
	if s.caps.HasPasswordHash {
		passExpr = "password_hash"
	}
	googleExpr := "NULL::text"
	if s.caps.HasGoogleID {
		googleExpr = "google_id"
	}
	yandexExpr := "NULL::text"
	if s.caps.HasYandexID {
		yandexExpr = "yandex_id"
	}
	vkExpr := "NULL::bigint"
	if s.caps.HasVKID {
		vkExpr = "vk_id"
	}
	rwUUIDExpr := "NULL::text"
	if s.caps.HasRemnawaveUUIDUsers {
		rwUUIDExpr = "remnawave_uuid"
	}
	q := fmt.Sprintf(`
		SELECT id, telegram_id, username, COALESCE(language,'ru'), COALESCE(balance_kopeks,0),
		       COALESCE(status,'active'), %s,
		       email, %s, %s, referred_by_id,
		       %s, %s, %s, %s
		FROM users
		ORDER BY id ASC`,
		authExpr, emailVerExpr, passExpr, googleExpr, yandexExpr, vkExpr, rwUUIDExpr)
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", err)
	}
	defer rows.Close()
	var out []bedolagaUser
	for rows.Next() {
		var u bedolagaUser
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Username, &u.Language, &u.BalanceKopeks,
			&u.Status, &u.AuthType, &u.Email, &u.EmailVerified, &u.PasswordHash, &u.ReferredByID,
			&u.GoogleID, &u.YandexID, &u.VKID, &u.RemnawaveUUID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *sourceDB) LoadSubscriptions(ctx context.Context) ([]bedolagaSub, error) {
	if !s.caps.HasSubscriptionsTable {
		return nil, fmt.Errorf("table subscriptions not found")
	}
	rwUUID := "NULL::text"
	if s.caps.HasRemnawaveUUIDSubs {
		rwUUID = "remnawave_uuid"
	}
	shortUUID := "NULL::text"
	if s.caps.HasRemnawaveShortUUID {
		shortUUID = "remnawave_short_uuid"
	}
	q := fmt.Sprintf(`
		SELECT id, user_id, COALESCE(status,''), end_date, subscription_url, tariff_id,
		       %s, %s, COALESCE(device_limit,1)
		FROM subscriptions
		ORDER BY user_id ASC, end_date DESC NULLS LAST`, rwUUID, shortUUID)
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("select subscriptions: %w", err)
	}
	defer rows.Close()
	var out []bedolagaSub
	for rows.Next() {
		var sub bedolagaSub
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Status, &sub.EndDate, &sub.SubscriptionURL,
			&sub.TariffID, &sub.RemnawaveUUID, &sub.RemnawaveShortUUID, &sub.DeviceLimit); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func parseAllowedSquads(raw []byte) []string {
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}
	return list
}

// periodPrices: keys are day counts as strings, values kopeks.
func parsePeriodPrices(raw []byte) map[int]int {
	out := map[int]int{}
	var m map[string]int
	if err := json.Unmarshal(raw, &m); err != nil {
		var mf map[string]float64
		if err2 := json.Unmarshal(raw, &mf); err2 != nil {
			return out
		}
		for k, v := range mf {
			var days int
			if _, err := fmt.Sscanf(k, "%d", &days); err == nil && days > 0 {
				out[days] = int(v)
			}
		}
		return out
	}
	for k, v := range m {
		var days int
		if _, err := fmt.Sscanf(k, "%d", &days); err == nil && days > 0 {
			out[days] = v
		}
	}
	return out
}

func pickPrimarySub(subs []bedolagaSub) (*bedolagaSub, int) {
	if len(subs) == 0 {
		return nil, 0
	}
	preferred := map[string]bool{"active": true, "trial": true, "limited": true}
	bestIdx := -1
	for i := range subs {
		if !preferred[subs[i].Status] {
			continue
		}
		if bestIdx < 0 {
			bestIdx = i
			continue
		}
		a, b := subs[i].EndDate, subs[bestIdx].EndDate
		if a != nil && (b == nil || a.After(*b)) {
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return &subs[0], len(subs) - 1
	}
	orphans := 0
	for i := range subs {
		if preferred[subs[i].Status] && i != bestIdx {
			orphans++
		}
	}
	return &subs[bestIdx], orphans
}

func (c sourceCaps) WarningLines() []string {
	var out []string
	out = append(out, "Meows bot 5.x targets Remnawave 3.3.*-3.4.* — point remnawave.* in migrate.yaml at that same panel.")
	switch c.HintGeneration {
	case "bedolaga_4x_rw3":
		out = append(out, "Source looks like Bedolaga 4.x (column users.remnawave_id). That generation usually sits on Remnawave 3.0+.")
		out = append(out, "Good match for Meows 5.x: if that panel is 3.3.*-3.4.*, this is the plain same-panel case.")
	case "bedolaga_3x_rw2":
		out = append(out, "Source looks like Bedolaga 3.x (e.g. 3.60) without users.remnawave_id — typical for Remnawave 2.x panels.")
		out = append(out, "Shop DB data migrates fine, but Meows 5.x cannot drive a 2.x panel: it needs Remnawave 3.3.*-3.4.*, so this is a panel switch, not a same-panel move.")
	default:
		out = append(out, "Could not detect Bedolaga 3.x vs 4.x generation from schema; check problems.csv after dry-run.")
	}
	var missing []string
	if !c.HasAuthType {
		missing = append(missing, "users.auth_type")
	}
	if !c.HasIsDaily {
		missing = append(missing, "tariffs.is_daily")
	}
	if !c.HasRemnawaveShortUUID {
		missing = append(missing, "subscriptions.remnawave_short_uuid")
	}
	if len(missing) > 0 {
		out = append(out, "Optional columns missing (OK, skipped): "+strings.Join(missing, ", "))
	}
	return out
}
