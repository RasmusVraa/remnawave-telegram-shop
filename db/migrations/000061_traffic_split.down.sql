-- Откат отказывается, если уже есть купленные пакеты трафика: это оплаченные деньги,
-- стирать их молча нельзя. Остальное (снимки, антидубль) собирается заново.
BEGIN;

DO
$$
    BEGIN
        IF EXISTS (SELECT 1 FROM customer_traffic_extra) THEN
            RAISE EXCEPTION 'customer_traffic_extra не пуста: откат стёр бы купленные пакеты трафика';
        END IF;
    END
$$;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS traffic_pack_bytes,
    DROP COLUMN IF EXISTS traffic_pack_kind,
    DROP COLUMN IF EXISTS traffic_pack_id;

DROP TABLE IF EXISTS customer_traffic_alert;
DROP TABLE IF EXISTS customer_traffic_extra;
DROP TABLE IF EXISTS customer_bypass_state;
DROP TABLE IF EXISTS tariff_traffic_pack;

ALTER TABLE tariff
    DROP COLUMN IF EXISTS bypass_node_uuids,
    DROP COLUMN IF EXISTS bypass_squad_uuids,
    DROP COLUMN IF EXISTS bypass_traffic_limit_bytes;

COMMIT;
