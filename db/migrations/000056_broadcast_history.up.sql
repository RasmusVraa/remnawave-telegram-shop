-- История рассылок: что ушло, кому и чем кончилось.
--
-- Раньше итог рассылки приходил только сообщением админу в Telegram, а сам
-- факт «рассылка идёт» жил в памяти процесса: после перезапуска бота посреди
-- отправки не оставалось ни следа. Строка заводится в начале, счётчики
-- обновляются после каждой пачки, итог пишется в конце.
--
-- Аудитории и кнопки хранятся как есть, без ссылок на тарифы: тариф могут
-- удалить, а запись о том, что рассылка по нему была, должна остаться.
BEGIN;

CREATE TABLE IF NOT EXISTS broadcast (
    id            BIGSERIAL PRIMARY KEY,
    -- cabinet | telegram — откуда запущена.
    source        TEXT        NOT NULL,
    audiences     TEXT[]      NOT NULL DEFAULT '{}',
    tariff_id     BIGINT,
    text          TEXT        NOT NULL DEFAULT '',
    -- HTML — разметка из редактора кабинета; пусто — простой текст
    -- (Telegram-админка шлёт форматирование entities, их не храним).
    parse_mode    TEXT        NOT NULL DEFAULT '',
    media_kind    TEXT        NOT NULL DEFAULT '',
    media_file_id TEXT        NOT NULL DEFAULT '',
    media_name    TEXT        NOT NULL DEFAULT '',
    buttons       JSONB       NOT NULL DEFAULT '{}',
    -- running | done | aborted | failed | interrupted
    status        TEXT        NOT NULL,
    total         INTEGER     NOT NULL DEFAULT 0,
    sent          INTEGER     NOT NULL DEFAULT 0,
    -- Получатель закрыл бота или удалил аккаунт (403 от Telegram).
    blocked       INTEGER     NOT NULL DEFAULT 0,
    -- Прочие отказы Telegram.
    failed        INTEGER     NOT NULL DEFAULT 0,
    error         TEXT        NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_broadcast_started_at ON broadcast (started_at DESC);

COMMIT;
