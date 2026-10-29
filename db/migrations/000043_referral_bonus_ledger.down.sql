-- Журнал — единственная запись о том, сколько дней кому начислено: пересчёт
-- задним числом удалён (см. 000043 up). Откат с данными стёр бы эту историю
-- безвозвратно, поэтому он отказывается. Если журнал действительно не нужен —
-- сохраните дамп и очистите таблицу вручную, затем повторите откат.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM referral_bonus_ledger) THEN
        RAISE EXCEPTION 'referral_bonus_ledger has rows: refusing to drop the bonus history (dump and TRUNCATE it manually first)';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_referral_bonus_ledger_created;
DROP INDEX IF EXISTS idx_referral_bonus_ledger_referrer;
DROP INDEX IF EXISTS idx_referral_bonus_ledger_recipient;
DROP INDEX IF EXISTS idx_referral_bonus_ledger_purchase_kind;
DROP TABLE IF EXISTS referral_bonus_ledger;
