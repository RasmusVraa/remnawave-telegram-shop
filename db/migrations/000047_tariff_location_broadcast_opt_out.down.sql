ALTER TABLE customer DROP COLUMN IF EXISTS broadcast_opt_out;

ALTER TABLE purchase DROP COLUMN IF EXISTS tariff_location_name;
ALTER TABLE purchase DROP COLUMN IF EXISTS tariff_location_id;

DROP TABLE IF EXISTS tariff_location;
