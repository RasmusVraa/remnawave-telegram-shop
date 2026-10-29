ALTER TABLE customer DROP CONSTRAINT IF EXISTS customer_hwid_limit_override_range;
ALTER TABLE customer DROP COLUMN IF EXISTS hwid_limit_override;
