-- «Требует внимания» в админке: что админ уже прочитал и скрыл. Хранится на
-- сервере по аккаунту, чтобы на телефоне и на компьютере было одинаково.
-- Пункт возвращается сам, если стало хуже: счётчик вырос, severity поднялась
-- до crit или событие случилось заново (since новее запомненного).

BEGIN;

CREATE TABLE cabinet_admin_attention_dismissed (
    account_id BIGINT       NOT NULL,
    item_key   VARCHAR(160) NOT NULL,
    count      INTEGER      NOT NULL DEFAULT 1,
    severity   VARCHAR(8)   NOT NULL DEFAULT 'warn',
    since      TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, item_key)
);

COMMIT;
