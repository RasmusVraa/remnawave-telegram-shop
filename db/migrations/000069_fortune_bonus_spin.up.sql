-- Приз «бесплатный спин»: спин, сыгранный за выигранную попытку, помечается is_bonus_spin.
-- Баланс попыток = сумма наград free_spin минус число таких спинов (отдельной таблицы нет).
ALTER TABLE fortune_spins ADD COLUMN IF NOT EXISTS is_bonus_spin BOOLEAN NOT NULL DEFAULT FALSE;
