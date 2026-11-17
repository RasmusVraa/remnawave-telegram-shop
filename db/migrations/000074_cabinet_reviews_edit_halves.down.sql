BEGIN;

ALTER TABLE cabinet_review
    DROP CONSTRAINT IF EXISTS cabinet_review_rating_chk;

-- Половинки округляем вверх до целых при откате.
ALTER TABLE cabinet_review
    ALTER COLUMN rating TYPE SMALLINT USING CEIL(rating)::smallint;

ALTER TABLE cabinet_review
    ADD CONSTRAINT cabinet_review_rating_chk CHECK (rating BETWEEN 1 AND 5);

ALTER TABLE cabinet_review
    DROP COLUMN IF EXISTS original_body,
    DROP COLUMN IF EXISTS edited_at;

COMMIT;
