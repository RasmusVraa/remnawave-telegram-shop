-- Обязательная подписка на канал: для статистики «увидели экран → подписались → ушли».
-- shown_at — когда клиент впервые увидел экран подписки; passed_at — когда впервые
-- прошёл проверку после этого; attempts — сколько раз нажал «Да, я подписался».
-- Колонки аддитивные, у старых клиентов пустые (функция по умолчанию выключена).
ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS channel_gate_shown_at  TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS channel_gate_passed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS channel_gate_attempts  INT NOT NULL DEFAULT 0;
