-- Изображения в чате поддержки кабинета. Файлы хранит Telegram (их держит
-- support-bot), здесь — описание вложения и его id в support-bot, по которому
-- кабинет скачивает файл через GET /cabinet/api/support/attachments/{id}.
--
-- Таблица новая и аддитивная: старый образ её не читает, откат образа без отката
-- миграции безопасен.
BEGIN;

CREATE TABLE IF NOT EXISTS cabinet_support_attachment (
    id                        BIGSERIAL PRIMARY KEY,
    message_id                BIGINT       NOT NULL REFERENCES cabinet_support_message (id) ON DELETE CASCADE,
    position                  SMALLINT     NOT NULL DEFAULT 0,
    support_bot_attachment_id BIGINT       NOT NULL,
    mime                      VARCHAR(64)  NOT NULL,
    size_bytes                INTEGER,
    width                     INTEGER,
    height                    INTEGER,
    name                      VARCHAR(255),
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cabinet_support_attachment_message
    ON cabinet_support_attachment (message_id, position);

COMMIT;
