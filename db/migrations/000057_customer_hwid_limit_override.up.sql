-- Закреплённый админом лимит устройств.
--
-- Лимит в панели пересчитывается при каждой оплате, докупке, промокоде и смене
-- тарифа, поэтому поднятое админом вручную число жило только до следующей
-- оплаты. Если колонка заполнена, бот при любой записи лимита в панель подставляет
-- именно её; NULL — лимит считается как обычно (тариф + докупленные слоты).
ALTER TABLE customer ADD COLUMN IF NOT EXISTS hwid_limit_override INTEGER;

ALTER TABLE customer DROP CONSTRAINT IF EXISTS customer_hwid_limit_override_range;
ALTER TABLE customer ADD CONSTRAINT customer_hwid_limit_override_range
    CHECK (hwid_limit_override IS NULL OR hwid_limit_override BETWEEN 1 AND 100);
