-- Отметка «Популярный» — только витрина, без неё всё продаётся как раньше.
BEGIN;
DROP INDEX IF EXISTS uq_tariff_single_popular;
ALTER TABLE tariff DROP COLUMN IF EXISTS is_popular;
COMMIT;
