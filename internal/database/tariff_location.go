package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v4"
)

// TariffLocation — город или площадка, которую клиент выбирает при покупке тарифа
// «свой сервер». SoldOut скрывает локацию из доступных, но не удаляет её.
type TariffLocation struct {
	ID          int64   `json:"id"`
	TariffID    int64   `json:"tariff_id,omitempty"`
	Name        string  `json:"name"`
	SortOrder   int     `json:"sort_order"`
	SoldOut     bool    `json:"sold_out"`
	CountryCode *string `json:"country_code,omitempty"`
}

// ErrDuplicateLocationName — два одинаковых имени в одном тарифе.
var ErrDuplicateLocationName = errors.New("duplicate tariff location name")

const tariffLocationNameMax = 80

func (r *TariffRepository) ListLocations(ctx context.Context, tariffID int64) ([]TariffLocation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tariff_id, name, sort_order, sold_out, country_code
		FROM tariff_location
		WHERE tariff_id = $1
		ORDER BY sort_order ASC, id ASC`, tariffID)
	if err != nil {
		return nil, fmt.Errorf("list tariff locations: %w", err)
	}
	defer rows.Close()
	return scanTariffLocations(rows)
}

func scanTariffLocations(rows pgx.Rows) ([]TariffLocation, error) {
	out := make([]TariffLocation, 0)
	for rows.Next() {
		var loc TariffLocation
		if err := rows.Scan(&loc.ID, &loc.TariffID, &loc.Name, &loc.SortOrder, &loc.SoldOut, &loc.CountryCode); err != nil {
			return nil, fmt.Errorf("scan tariff location: %w", err)
		}
		out = append(out, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceLocations полностью заменяет список локаций тарифа.
// Пустое имя пропускается. id > 0 обновляет существующую строку этого тарифа,
// иначе создаётся новая. Строки, которых нет во входном списке, удаляются.
func (r *TariffRepository) ReplaceLocations(ctx context.Context, tariffID int64, items []TariffLocation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keep := make([]int64, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	order := 0
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > tariffLocationNameMax {
			return fmt.Errorf("location name too long")
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return ErrDuplicateLocationName
		}
		seen[key] = struct{}{}

		if item.ID > 0 {
			tag, err := tx.Exec(ctx, `
				UPDATE tariff_location
				SET name = $1, sort_order = $2, sold_out = $3, country_code = $4
				WHERE id = $5 AND tariff_id = $6`,
				name, order, item.SoldOut, locationCountryCode(item.CountryCode), item.ID, tariffID)
			if err != nil {
				return fmt.Errorf("update tariff location: %w", err)
			}
			if tag.RowsAffected() > 0 {
				keep = append(keep, item.ID)
				order++
				continue
			}
		}
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tariff_location (tariff_id, name, sort_order, sold_out, country_code)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id`, tariffID, name, order, item.SoldOut, locationCountryCode(item.CountryCode)).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert tariff location: %w", err)
		}
		keep = append(keep, id)
		order++
	}

	if len(keep) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM tariff_location WHERE tariff_id = $1`, tariffID); err != nil {
			return fmt.Errorf("clear tariff locations: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			DELETE FROM tariff_location
			WHERE tariff_id = $1 AND NOT (id = ANY($2))`, tariffID, keep); err != nil {
			return fmt.Errorf("delete stale tariff locations: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func locationCountryCode(code *string) any {
	if code == nil {
		return nil
	}
	c := strings.ToUpper(strings.TrimSpace(*code))
	if len(c) != 2 {
		return nil
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return nil
		}
	}
	return c
}

// InsertLocation добавляет площадку в конец списка тарифа.
func (r *TariffRepository) InsertLocation(ctx context.Context, tariffID int64, name string, country *string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > tariffLocationNameMax {
		return fmt.Errorf("location name too long")
	}
	var n int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tariff_location
		WHERE tariff_id = $1 AND lower(name) = lower($2)`, tariffID, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrDuplicateLocationName
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tariff_location (tariff_id, name, sort_order, sold_out, country_code)
		VALUES ($1, $2, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM tariff_location WHERE tariff_id = $1), FALSE, $3)`,
		tariffID, name, locationCountryCode(country))
	return err
}

// ToggleLocationSoldOut переключает «закончилась» у локации этого тарифа.
func (r *TariffRepository) ToggleLocationSoldOut(ctx context.Context, tariffID, locationID int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE tariff_location SET sold_out = NOT sold_out
		WHERE id = $1 AND tariff_id = $2`, locationID, tariffID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteLocation удаляет площадку тарифа.
func (r *TariffRepository) DeleteLocation(ctx context.Context, tariffID, locationID int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tariff_location WHERE id = $1 AND tariff_id = $2`, locationID, tariffID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
