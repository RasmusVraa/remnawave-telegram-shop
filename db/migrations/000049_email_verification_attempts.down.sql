DROP INDEX IF EXISTS idx_cabinet_email_verification_token_hash;

-- Пока уникальности не было, хэши могли совпасть. Коды одноразовые, поэтому
-- из совпавших оставляем самый свежий: худшее, что теряется, — старый код,
-- который пользователь всё равно может запросить заново.
DELETE FROM cabinet_email_verification a
 USING cabinet_email_verification b
 WHERE a.token_hash = b.token_hash
   AND a.id < b.id;

ALTER TABLE cabinet_email_verification
    ADD CONSTRAINT cabinet_email_verification_token_hash_key UNIQUE (token_hash);

ALTER TABLE cabinet_email_verification
    DROP COLUMN IF EXISTS attempts;
