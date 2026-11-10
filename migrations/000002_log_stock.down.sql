BEGIN;
DROP TRIGGER IF EXISTS trg_log_items_remaining ON log_items;
DROP FUNCTION IF EXISTS log_items_set_remaining();
ALTER TABLE log_items DROP CONSTRAINT IF EXISTS log_items_remaining_nonneg;
ALTER TABLE log_items DROP COLUMN IF EXISTS remaining_volume_m3;
ALTER TABLE log_items DROP COLUMN IF EXISTS remaining_count;
COMMIT;