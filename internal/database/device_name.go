package database

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
)

// DeviceNameMaxLen — предел длины пользовательского названия устройства
// в символах; совпадает с VARCHAR(64) в таблице device_name.
const DeviceNameMaxLen = 64

// DeviceNameRepository хранит названия, которые пользователь дал своим
// устройствам (HWID) в кабинете.
type DeviceNameRepository struct {
	pool *pgxpool.Pool
}

func NewDeviceNameRepository(pool *pgxpool.Pool) *DeviceNameRepository {
	return &DeviceNameRepository{pool: pool}
}

// ListByCustomer возвращает названия устройств клиента: hwid → name.
func (r *DeviceNameRepository) ListByCustomer(ctx context.Context, customerID int64) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT hwid, name FROM device_name WHERE customer_id = $1`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var hwid, name string
		if err := rows.Scan(&hwid, &name); err != nil {
			return nil, err
		}
		out[hwid] = name
	}
	return out, rows.Err()
}

// Upsert задаёт или меняет название устройства.
func (r *DeviceNameRepository) Upsert(ctx context.Context, customerID int64, hwid, name string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO device_name (customer_id, hwid, name, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (customer_id, hwid) DO UPDATE
		SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at`,
		customerID, hwid, name)
	return err
}

// Delete убирает название: устройство снова показывается под исходным именем.
func (r *DeviceNameRepository) Delete(ctx context.Context, customerID int64, hwid string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM device_name WHERE customer_id = $1 AND hwid = $2`, customerID, hwid)
	return err
}
