-- Статистика и выручка: оплаченные покупки за период по paid_at.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_paid_at ON purchase (paid_at) WHERE status = 'paid';
