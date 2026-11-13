-- Журнал событий для страницы «Мониторинг» в админке и вкладки «История»
-- в карточке клиента: оплаты, регистрации, входы, промокоды, поддержка, ноды,
-- действия админов. level = 2 — нажатия кнопок и команды в боте (пишутся только
-- при MONITORING_BOT_ACTIONS=true и хранятся коротко).
--
-- Старые строки раз в сутки удаляет фоновая очистка (MONITORING_RETENTION_*).
-- customer_id без FK: удаление клиента не должно ломать ленту и упираться в журнал.
--
-- Таблица новая и аддитивная: старый образ её не читает, откат образа без отката
-- миграции безопасен.
BEGIN;

CREATE TABLE IF NOT EXISTS admin_event (
    id          BIGSERIAL    PRIMARY KEY,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    type        VARCHAR(40)  NOT NULL,
    level       SMALLINT     NOT NULL DEFAULT 1,
    customer_id BIGINT,
    actor_tg_id BIGINT,
    source      VARCHAR(16)  NOT NULL DEFAULT 'system',
    payload     JSONB        NOT NULL DEFAULT '{}'::jsonb
);

-- История клиента: последние события одного человека.
CREATE INDEX IF NOT EXISTS idx_admin_event_customer
    ON admin_event (customer_id, id DESC)
    WHERE customer_id IS NOT NULL;

-- Счётчики за период и очистка по возрасту. Строки пишутся по порядку времени,
-- BRIN здесь в сотни раз меньше btree и этого хватает.
CREATE INDEX IF NOT EXISTS idx_admin_event_created_brin
    ON admin_event USING BRIN (created_at);

COMMIT;
