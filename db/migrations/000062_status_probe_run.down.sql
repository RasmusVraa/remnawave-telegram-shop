-- Таблица хранит только статистику проверок, её можно собрать заново.
BEGIN;
DROP TABLE IF EXISTS status_probe_run;
COMMIT;
