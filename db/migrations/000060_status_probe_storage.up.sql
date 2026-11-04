-- Статус серверов: замеры и история уходят из JSON в bot_runtime_settings в свои таблицы.
--
-- Раньше весь журнал лежал одной строкой ключ-значение и переписывался целиком
-- после каждой проверки. Теперь день и последний замер — это строки: запись
-- точечная, историю можно чистить и смотреть запросом.
--
-- Таблицы новые и аддитивные: старый образ их не читает, откат образа без
-- отката миграции безопасен. Список серверов остаётся настройкой в
-- bot_runtime_settings (ключ status_probe_targets): это конфиг, а не статистика.
BEGIN;

-- Итог дня по строке списка: сколько зондов ответило из мира и из России.
CREATE TABLE IF NOT EXISTS status_probe_day (
    probe_key     TEXT    NOT NULL,
    day           DATE    NOT NULL,
    world_ok      INTEGER NOT NULL DEFAULT 0,
    world_total   INTEGER NOT NULL DEFAULT 0,
    russia_ok     INTEGER NOT NULL DEFAULT 0,
    russia_total  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (probe_key, day)
);

-- Последний замер по строке: то, что показывает страница, пока не пришёл новый.
-- payload — JSON замера (зонды, операторы, серия неудач); адресов серверов в нём нет.
CREATE TABLE IF NOT EXISTS status_probe_sample (
    probe_key   TEXT        PRIMARY KEY,
    payload     JSONB       NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
