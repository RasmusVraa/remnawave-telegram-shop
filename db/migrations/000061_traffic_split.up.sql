-- Раздельный трафик (обычные серверы + обход) и докупка трафика.
-- Всё аддитивное: у существующих тарифов и клиентов поведение не меняется.

BEGIN;

-- Настройки обхода в тарифе. 0 = у тарифа нет отдельного лимита обхода.
ALTER TABLE tariff
    ADD COLUMN IF NOT EXISTS bypass_traffic_limit_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS bypass_squad_uuids         TEXT   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS bypass_node_uuids          TEXT   NOT NULL DEFAULT '';

-- Пакеты докупки: свои у каждого тарифа и отдельно для обычного трафика и обхода.
CREATE TABLE IF NOT EXISTS tariff_traffic_pack
(
    id           BIGSERIAL PRIMARY KEY,
    tariff_id    BIGINT      NOT NULL REFERENCES tariff (id) ON DELETE CASCADE,
    kind         VARCHAR(16) NOT NULL CHECK (kind IN ('main', 'bypass')),
    bytes        BIGINT      NOT NULL CHECK (bytes > 0),
    amount_rub   INTEGER     NOT NULL CHECK (amount_rub >= 0),
    amount_stars INTEGER,
    sort_order   INTEGER     NOT NULL DEFAULT 0,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_tariff_traffic_pack_tariff ON tariff_traffic_pack (tariff_id, kind, sort_order);

-- Снимок настроек обхода у клиента. Берётся из тарифа при покупке и продлении:
-- изменение тарифа не трогает тех, кто уже на нём, только будущие покупки.
-- limit_bytes = 0 — у клиента нет отдельного лимита обхода, воркер его пропускает.
CREATE TABLE IF NOT EXISTS customer_bypass_state
(
    customer_id   BIGINT PRIMARY KEY REFERENCES customer (id) ON DELETE CASCADE,
    tariff_id     BIGINT REFERENCES tariff (id) ON DELETE SET NULL,
    limit_bytes   BIGINT      NOT NULL DEFAULT 0,
    squad_uuids   TEXT        NOT NULL DEFAULT '',
    node_uuids    TEXT        NOT NULL DEFAULT '',
    period_start  DATE        NOT NULL,
    used_bytes    BIGINT      NOT NULL DEFAULT 0,
    squad_removed BOOLEAN     NOT NULL DEFAULT FALSE,
    checked_at    TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Купленные пакеты. Живут до valid_until (1-е число следующего месяца).
CREATE TABLE IF NOT EXISTS customer_traffic_extra
(
    id          BIGSERIAL PRIMARY KEY,
    customer_id BIGINT      NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    kind        VARCHAR(16) NOT NULL CHECK (kind IN ('main', 'bypass')),
    bytes       BIGINT      NOT NULL CHECK (bytes > 0),
    purchase_id BIGINT      REFERENCES purchase (id) ON DELETE SET NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_customer_traffic_extra_customer ON customer_traffic_extra (customer_id, kind, valid_until);
CREATE INDEX IF NOT EXISTS idx_customer_traffic_extra_valid_until ON customer_traffic_extra (valid_until);
-- Один и тот же платёж не должен выдать пакет дважды.
CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_traffic_extra_purchase ON customer_traffic_extra (purchase_id) WHERE purchase_id IS NOT NULL;

-- Антидубль уведомлений: уровень 80/100 раз за месяц на каждый вид трафика.
CREATE TABLE IF NOT EXISTS customer_traffic_alert
(
    customer_id  BIGINT      NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    kind         VARCHAR(16) NOT NULL,
    period_start DATE        NOT NULL,
    level        SMALLINT    NOT NULL,
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, kind, period_start, level)
);

-- Покупка пакета трафика (purchase_kind = 'extra_traffic').
ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS traffic_pack_id    BIGINT,
    ADD COLUMN IF NOT EXISTS traffic_pack_kind  VARCHAR(16),
    ADD COLUMN IF NOT EXISTS traffic_pack_bytes BIGINT NOT NULL DEFAULT 0;

COMMIT;
