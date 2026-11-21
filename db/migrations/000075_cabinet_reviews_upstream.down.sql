-- Откат к упрощённой схеме 073/074 (без модерации и ответа команды).

BEGIN;

DROP INDEX IF EXISTS idx_cabinet_review_published;
DROP INDEX IF EXISTS idx_cabinet_review_status_created;
DROP INDEX IF EXISTS idx_cabinet_review_customer_noacc;

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_reply_len_chk;
ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_status_chk;
ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_avatar_kind_chk;

ALTER TABLE cabinet_review
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS avatar_kind,
    DROP COLUMN IF EXISTS telegram_id,
    DROP COLUMN IF EXISTS by_admin,
    DROP COLUMN IF EXISTS reply_body,
    DROP COLUMN IF EXISTS reply_at,
    DROP COLUMN IF EXISTS published_at;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'cabinet_review' AND column_name = 'tariff_label'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'cabinet_review' AND column_name = 'tariff_name'
    ) THEN
        ALTER TABLE cabinet_review RENAME COLUMN tariff_label TO tariff_name;
    END IF;
END $$;

ALTER TABLE cabinet_review ALTER COLUMN account_id SET NOT NULL;

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_rating_chk;
ALTER TABLE cabinet_review
    ALTER COLUMN rating TYPE NUMERIC(2,1) USING rating::numeric;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_rating_chk
        CHECK (rating >= 0.5 AND rating <= 5 AND (rating * 2) = trunc(rating * 2));

ALTER TABLE cabinet_review DROP CONSTRAINT IF EXISTS cabinet_review_body_len_chk;
ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_body_len_chk CHECK (char_length(body) BETWEEN 10 AND 2000);

ALTER TABLE cabinet_review ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;
ALTER TABLE cabinet_review ADD COLUMN IF NOT EXISTS tariff_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_cabinet_review_created ON cabinet_review (created_at DESC, id DESC);

COMMIT;
