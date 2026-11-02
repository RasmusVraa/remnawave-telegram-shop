-- Уведомления в кабинете: колокольчик в шапке, лента, push в браузер.
--
-- Рассылка доходила только до Telegram, а клиенты, зарегистрированные через
-- сайт, не получали её вовсе. Теперь у рассылки второй канал — кабинет, и туда
-- же пишутся личные события (оплата, конец подписки, ответ поддержки).
--
-- Получатели фиксируются в момент отправки: строка на человека. Так «новость
-- для активных» остаётся у того, чья подписка закончилась позже, а новый
-- пользователь старых рассылок не видит. Всё аддитивно: старый образ этих
-- таблиц не читает, откат образа без отката миграции безопасен.
BEGIN;

CREATE TABLE IF NOT EXISTS cabinet_notification (
    id           BIGSERIAL PRIMARY KEY,
    -- broadcast — из рассылки; остальное — личные события (payment, expiring…).
    kind         TEXT        NOT NULL,
    broadcast_id BIGINT REFERENCES broadcast (id) ON DELETE SET NULL,
    -- Важное закреплено вверху ленты и висит плашкой на главной, пока не прочитано.
    important    BOOLEAN     NOT NULL DEFAULT FALSE,
    title        TEXT        NOT NULL DEFAULT '',
    -- Telegram HTML (b, i, u, s, a, code, pre, blockquote, tg-spoiler).
    body         TEXT        NOT NULL DEFAULT '',
    -- Кнопки-переходы по кабинету: [{"key":"tariffs"}, …].
    buttons      JSONB       NOT NULL DEFAULT '[]',
    -- Защита от дублей личных событий (крон напоминаний идёт каждый день).
    dedupe_key   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Админ удалил уведомление у всех: строки остаются, но в ленте их нет.
    deleted_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cabinet_notification_dedupe
    ON cabinet_notification (dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cabinet_notification_broadcast
    ON cabinet_notification (broadcast_id) WHERE broadcast_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS cabinet_notification_recipient (
    customer_id     BIGINT NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    notification_id BIGINT NOT NULL REFERENCES cabinet_notification (id) ON DELETE CASCADE,
    read_at         TIMESTAMPTZ,
    -- Человек удалил уведомление у себя крестиком. Строка остаётся: по ней
    -- админ видит, скольким рассылка дошла.
    hidden_at       TIMESTAMPTZ,
    PRIMARY KEY (customer_id, notification_id)
);

-- Счётчик колокольчика читает только непрочитанные одного человека.
CREATE INDEX IF NOT EXISTS idx_cabinet_notification_unread
    ON cabinet_notification_recipient (customer_id) WHERE read_at IS NULL AND hidden_at IS NULL;
-- Статистика «прочитали N из M» в истории рассылок.
CREATE INDEX IF NOT EXISTS idx_cabinet_notification_recipient_notification
    ON cabinet_notification_recipient (notification_id);

-- Подписки браузеров на push: по одной на устройство и браузер.
CREATE TABLE IF NOT EXISTS cabinet_push_subscription (
    id              BIGSERIAL PRIMARY KEY,
    customer_id     BIGINT      NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    endpoint        TEXT        NOT NULL UNIQUE,
    p256dh          TEXT        NOT NULL,
    auth            TEXT        NOT NULL,
    user_agent      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cabinet_push_subscription_customer
    ON cabinet_push_subscription (customer_id);

-- VAPID-ключи push: генерируются при первом запуске, одна строка на инсталляцию.
-- Смена ключей отписала бы все браузеры, поэтому они живут в базе, а не в .env.
CREATE TABLE IF NOT EXISTS cabinet_vapid_key (
    id          SMALLINT PRIMARY KEY CHECK (id = 1),
    public_key  TEXT        NOT NULL,
    private_key TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Каналы рассылки: telegram, cabinet. У старых записей — только Telegram.
ALTER TABLE broadcast ADD COLUMN IF NOT EXISTS channels TEXT[] NOT NULL DEFAULT '{telegram}';

COMMIT;
