-- Покупки клиента («есть ли оплата», история, лояльность, пробный период) и
-- проверка внешнего ключа purchase.customer_id при удалении клиента.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_customer_status ON purchase (customer_id, status);
