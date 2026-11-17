-- Половинки звёзд, снимок оригинала при правке, метка edited_at.

BEGIN;

ALTER TABLE cabinet_review
    ALTER COLUMN rating TYPE NUMERIC(2,1) USING rating::numeric;

ALTER TABLE cabinet_review
    DROP CONSTRAINT IF EXISTS cabinet_review_rating_chk;

ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_rating_chk
        CHECK (rating >= 0.5 AND rating <= 5 AND (rating * 2) = trunc(rating * 2));

ALTER TABLE cabinet_review
    ADD COLUMN IF NOT EXISTS original_body TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;

UPDATE cabinet_review
SET original_body = body
WHERE original_body = '';

COMMIT;
