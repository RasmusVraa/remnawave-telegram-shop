-- Статистика подписки на канал теряется при откате; на доступ клиентов не влияет.
ALTER TABLE customer
    DROP COLUMN IF EXISTS channel_gate_shown_at,
    DROP COLUMN IF EXISTS channel_gate_passed_at,
    DROP COLUMN IF EXISTS channel_gate_attempts;
