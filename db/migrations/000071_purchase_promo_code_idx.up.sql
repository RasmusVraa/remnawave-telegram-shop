-- Отдача промокода в админке (оплаты со скидкой по коду) и каскад ON DELETE SET NULL
-- при удалении промокода иначе листают purchase целиком. Частичный: у большинства
-- покупок кода нет. CONCURRENTLY — чтобы не блокировать оплаты; одна команда на файл.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_promo_code_id ON purchase (promo_code_id) WHERE promo_code_id IS NOT NULL;
