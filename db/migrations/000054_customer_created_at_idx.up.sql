-- Статистика: новые клиенты за период по created_at.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_customer_created_at ON customer (created_at);
