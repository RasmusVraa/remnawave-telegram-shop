-- Статус серверов: полоска истории считает замеры, а не сутки.
--
-- Раньше status_probe_day хранила один итог на день, и одна неудача из десятков проверок
-- красила весь столбик. Теперь запись — один замер, столбик — один замер.
--
-- Таблица новая и аддитивная: старый образ её не читает, откат образа без отката
-- миграции безопасен. status_probe_day остаётся нетронутой и больше не используется;
-- прежнюю историю по дням не переносим — её нельзя разложить на замеры.
BEGIN;

CREATE TABLE IF NOT EXISTS status_probe_run (
    probe_key     TEXT        NOT NULL,
    at            TIMESTAMPTZ NOT NULL,
    world_ok      INTEGER     NOT NULL DEFAULT 0,
    world_total   INTEGER     NOT NULL DEFAULT 0,
    russia_ok     INTEGER     NOT NULL DEFAULT 0,
    russia_total  INTEGER     NOT NULL DEFAULT 0,
    PRIMARY KEY (probe_key, at)
);

COMMIT;
