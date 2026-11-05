BEGIN;

ALTER TABLE contractors DROP COLUMN IF EXISTS is_active;
ALTER TABLE contractors DROP COLUMN IF EXISTS is_client;
ALTER TABLE contractors DROP COLUMN IF EXISTS is_carrier;
ALTER TABLE contractors DROP COLUMN IF EXISTS is_supplier;

COMMIT;