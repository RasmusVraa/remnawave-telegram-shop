-- У приглашённого одна реферальная связь. Без уникальности двойной /start
-- (или /start и регистрация в кабинете одновременно) создавал две строки, и
-- FindByReferee без ORDER BY отдавал случайного реферера — бонус получал то
-- один, то другой.
--
-- Дубли схлопываем: остаётся строка, по которой бонус уже начислен, иначе —
-- самая ранняя. Журнал referral_bonus_ledger при удалении строки получает
-- referral_id = NULL (ON DELETE SET NULL), факт начисления сохраняется.
BEGIN;

DELETE FROM referral r
USING (
    SELECT id,
           row_number() OVER (PARTITION BY referee_id ORDER BY bonus_granted DESC, id) AS rn
    FROM referral
) d
WHERE r.id = d.id
  AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_referral_referee_id ON referral (referee_id);

-- Hash-индекс по referee_id теперь дублирует уникальный.
DROP INDEX IF EXISTS idx_referral_referee_id;

COMMIT;
