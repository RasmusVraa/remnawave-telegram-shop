-- Тариф «Популярный»: админ отмечает один тариф, у клиентов на его карточке нашивка.
--
-- Колонка аддитивная, по умолчанию false: старый образ её не читает, откат образа
-- без отката миграции безопасен. Частичный уникальный индекс держит правило
-- «популярный не больше одного» на уровне базы — код снимает отметку с прежнего
-- тарифа в той же транзакции, а индекс не даст двум отметкам появиться при гонке.
BEGIN;

ALTER TABLE tariff ADD COLUMN IF NOT EXISTS is_popular BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS uq_tariff_single_popular ON tariff ((TRUE)) WHERE is_popular;

COMMIT;
