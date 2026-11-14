-- Скидки колеса фортуны (3% и 5%) действуют на одну оплату, а не бессрочно.
-- Раньше они создавались с subscription_payments_remaining = -1 (безлимит) и не гасились.
UPDATE customer_pending_discount
SET subscription_payments_remaining = 1
WHERE subscription_payments_remaining = -1
  AND promo_code_id = (SELECT id FROM promo_code WHERE code = '__CABINET_FORTUNE__');
