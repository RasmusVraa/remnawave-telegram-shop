-- Список платежей в админке: последние покупки по created_at.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_created_at ON purchase (created_at DESC);
