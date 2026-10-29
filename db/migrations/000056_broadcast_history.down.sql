-- История рассылок есть только здесь: откат с данными стёр бы её
-- безвозвратно, поэтому он отказывается. Если история действительно не
-- нужна — сохраните дамп и очистите таблицу вручную, затем повторите откат.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM broadcast) THEN
        RAISE EXCEPTION 'broadcast has rows: refusing to drop the broadcast history (dump and TRUNCATE it manually first)';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_broadcast_started_at;
DROP TABLE IF EXISTS broadcast;
