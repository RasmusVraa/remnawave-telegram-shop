-- Локации собственного сервера у тарифа и отказ клиента от массовой рассылки.
-- Имя локации на покупке сохраняется отдельно: если строку локации потом удалят,
-- в истории платежа останется то, что выбрал клиент.

CREATE TABLE IF NOT EXISTS tariff_location
(
    id         BIGSERIAL PRIMARY KEY,
    tariff_id  BIGINT      NOT NULL REFERENCES tariff (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    sort_order   INT         NOT NULL DEFAULT 0,
    sold_out     BOOLEAN     NOT NULL DEFAULT FALSE,
    country_code TEXT
);

CREATE INDEX IF NOT EXISTS tariff_location_tariff_idx
    ON tariff_location (tariff_id, sort_order, id);

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS tariff_location_id BIGINT REFERENCES tariff_location (id) ON DELETE SET NULL;

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS tariff_location_name TEXT;

ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS broadcast_opt_out BOOLEAN NOT NULL DEFAULT FALSE;
