-- Уведомления и подписки на push есть только здесь: откат с данными стёр бы
-- их безвозвратно, поэтому он отказывается. Если они действительно не нужны —
-- сохраните дамп и очистите таблицы вручную, затем повторите откат.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cabinet_notification) THEN
        RAISE EXCEPTION 'cabinet_notification has rows: refusing to drop notifications (dump and TRUNCATE them manually first)';
    END IF;
END $$;

ALTER TABLE broadcast DROP COLUMN IF EXISTS channels;
DROP TABLE IF EXISTS cabinet_vapid_key;
DROP TABLE IF EXISTS cabinet_push_subscription;
DROP TABLE IF EXISTS cabinet_notification_recipient;
DROP TABLE IF EXISTS cabinet_notification;
