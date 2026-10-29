-- Код подтверждения email: счётчик попыток и уникальность хэша.
--
-- Код из шести цифр раньше искался по всей таблице, и подбор 000000…999999
-- попадал в любой живой код любого аккаунта. Теперь код проверяется только в
-- пределах одного аккаунта и сгорает после нескольких неверных попыток —
-- attempts считает их.
--
-- Глобальный UNIQUE(token_hash) снимаем: при 10^6 возможных кодах совпадение
-- хэшей у разных аккаунтов — вопрос времени, и выдача кода начала бы падать на
-- unique violation. Индекс по token_hash остаётся для старых ссылок из писем,
-- которые по-прежнему ищутся по хэшу.
--
-- Совместимо со старым кодом: он не знает attempts (у колонки есть DEFAULT), а
-- поиск по token_hash работает и без уникальности, поэтому откат версии не
-- требует отката БД.

ALTER TABLE cabinet_email_verification
    ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE cabinet_email_verification
    DROP CONSTRAINT IF EXISTS cabinet_email_verification_token_hash_key;

CREATE INDEX IF NOT EXISTS idx_cabinet_email_verification_token_hash
    ON cabinet_email_verification (token_hash);
