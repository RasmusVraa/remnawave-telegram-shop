-- Поллеры касс каждые 5 секунд ищут открытые счета своего типа (status new/pending).
-- Без индекса это полный просмотр purchase на каждый тик. CONCURRENTLY — чтобы не
-- блокировать запись в таблицу при создании на большой базе; поэтому одна команда
-- на файл: CONCURRENTLY не работает внутри транзакции.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_open_by_type ON purchase (invoice_type, created_at) WHERE status IN ('new', 'pending');
