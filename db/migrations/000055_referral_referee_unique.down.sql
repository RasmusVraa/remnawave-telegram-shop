-- Удалённые дубли не восстанавливаются: они были ошибкой данных.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_referral_referee_id ON referral USING hash (referee_id);
DROP INDEX IF EXISTS uq_referral_referee_id;

COMMIT;
