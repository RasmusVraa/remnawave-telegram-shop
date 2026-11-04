-- Таблицы хранят только статистику проверок, её можно собрать заново.
BEGIN;
DROP TABLE IF EXISTS status_probe_sample;
DROP TABLE IF EXISTS status_probe_day;
COMMIT;
