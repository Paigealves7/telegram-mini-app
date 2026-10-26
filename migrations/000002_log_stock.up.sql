-- Остаток сырья: log_items.volume_m3 и count остаются «сколько пришло» (по ним считается статистика),
-- а сколько бревен ещё не распилено хранится в новых колонках.
-- Скрипт можно запускать повторно: он ничего не ломает.
BEGIN;

ALTER TABLE log_items ADD COLUMN IF NOT EXISTS remaining_volume_m3 NUMERIC(12,3);
ALTER TABLE log_items ADD COLUMN IF NOT EXISTS remaining_count INT;

-- Для уже существующих партий: остаток = пришло минус уже распилено (не меньше нуля)
UPDATE log_items li
SET remaining_volume_m3 = GREATEST(
      li.volume_m3 - COALESCE((SELECT SUM(sl.volume_m3) FROM sawed_logs sl WHERE sl.log_id = li.id), 0), 0)
WHERE li.remaining_volume_m3 IS NULL;

UPDATE log_items
SET remaining_count = CASE WHEN volume_m3 > 0 THEN ROUND(log_items.count * remaining_volume_m3 / volume_m3)::INT ELSE 0 END
WHERE remaining_count IS NULL;

ALTER TABLE log_items ALTER COLUMN remaining_volume_m3 SET NOT NULL;
ALTER TABLE log_items ALTER COLUMN remaining_count SET NOT NULL;

-- Новая партия при приходе автоматически получает остаток = пришедшему количеству
CREATE OR REPLACE FUNCTION log_items_set_remaining() RETURNS trigger AS $$
BEGIN
    IF NEW.remaining_volume_m3 IS NULL THEN NEW.remaining_volume_m3 := NEW.volume_m3; END IF;
    IF NEW.remaining_count IS NULL THEN NEW.remaining_count := NEW.count; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_log_items_remaining ON log_items;
CREATE TRIGGER trg_log_items_remaining BEFORE INSERT ON log_items
    FOR EACH ROW EXECUTE FUNCTION log_items_set_remaining();

-- Остаток не может быть отрицательным
ALTER TABLE log_items DROP CONSTRAINT IF EXISTS log_items_remaining_nonneg;
ALTER TABLE log_items ADD CONSTRAINT log_items_remaining_nonneg
    CHECK (remaining_volume_m3 >= 0 AND remaining_count >= 0);

COMMIT;