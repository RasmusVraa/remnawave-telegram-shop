-- Начисления и выплаты партнёрам — это деньги. Откат с данными стёр бы
-- балансы и историю выплат безвозвратно, поэтому он отказывается. Если данные
-- действительно не нужны — сохраните дамп и очистите таблицы вручную.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM partner_earning) OR EXISTS (SELECT 1 FROM partner_payout) THEN
        RAISE EXCEPTION 'partner_earning/partner_payout have rows: refusing to drop partner money history (dump and TRUNCATE them manually first)';
    END IF;
END $$;

-- Порядок обратный созданию: сначала таблицы со ссылками наружу.
DROP TABLE IF EXISTS partner_payout;
DROP TABLE IF EXISTS partner_earning;
DROP TABLE IF EXISTS partner_attribution;
DROP TABLE IF EXISTS partner_link;
DROP TABLE IF EXISTS partner;
