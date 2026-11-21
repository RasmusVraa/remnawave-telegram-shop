-- Перевод нашей схемы отзывов (073/074) на модель MrMe0ws 5.35:
-- модерация, ответ команды, стабильный аватар без протухающего токена.

BEGIN;

-- Целые звёзды 1–5 (половинки из 074 округляем).
ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_rating_chk;
ALTER TABLE cabinet_review
    ALTER COLUMN rating TYPE SMALLINT USING GREATEST(1, LEAST(5, ROUND(rating)::integer));
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_rating_chk CHECK (rating BETWEEN 1 AND 5);

-- account_id может быть NULL (отзыв от имени бот-клиента без кабинета).
ALTER TABLE cabinet_review ALTER COLUMN account_id DROP NOT NULL;

-- tariff_name → tariff_label
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'cabinet_review' AND column_name = 'tariff_name'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'cabinet_review' AND column_name = 'tariff_label'
    ) THEN
        ALTER TABLE cabinet_review RENAME COLUMN tariff_name TO tariff_label;
    END IF;
END $$;

ALTER TABLE cabinet_review DROP COLUMN IF EXISTS tariff_id;
ALTER TABLE cabinet_review DROP COLUMN IF EXISTS edited_at;

ALTER TABLE cabinet_review
    ADD COLUMN IF NOT EXISTS status VARCHAR(16),
    ADD COLUMN IF NOT EXISTS avatar_kind VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS telegram_id BIGINT,
    ADD COLUMN IF NOT EXISTS by_admin BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS reply_body TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reply_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS original_body TEXT NOT NULL DEFAULT '';

UPDATE cabinet_review
SET original_body = body
WHERE original_body = '';

-- Старые отзывы считаем опубликованными.
UPDATE cabinet_review
SET status = 'published',
    published_at = COALESCE(published_at, created_at)
WHERE status IS NULL OR status = '';

ALTER TABLE cabinet_review ALTER COLUMN status SET NOT NULL;

-- Протухающие /cabinet/api/avatar?t=… больше не храним: публичный прокси по id отзыва.
UPDATE cabinet_review
SET avatar_kind = '',
    avatar_url = ''
WHERE avatar_url LIKE '/cabinet/api/avatar%'
   OR avatar_url LIKE '%/cabinet/api/avatar%';

UPDATE cabinet_review
SET avatar_kind = 'url'
WHERE avatar_url ~* '^https?://' AND (avatar_kind = '' OR avatar_kind IS NULL);

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_status_chk;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_status_chk CHECK (status IN ('pending', 'published', 'hidden'));

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_avatar_kind_chk;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_avatar_kind_chk CHECK (avatar_kind IN ('', 'telegram', 'url'));

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_body_len_chk;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_body_len_chk CHECK (char_length(body) BETWEEN 5 AND 1000);

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_reply_len_chk;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_reply_len_chk CHECK (char_length(reply_body) <= 1000);

DROP INDEX IF EXISTS idx_cabinet_review_created;
CREATE UNIQUE INDEX IF NOT EXISTS idx_cabinet_review_account ON cabinet_review (account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cabinet_review_customer_noacc ON cabinet_review (customer_id) WHERE account_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_cabinet_review_status_created ON cabinet_review (status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_cabinet_review_published ON cabinet_review (published_at DESC, id DESC) WHERE status = 'published';

COMMIT;
