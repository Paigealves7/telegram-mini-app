BEGIN;

-- Типы контрагента и признак «скрыт». Для существующих записей включены все типы и они активны,
-- поэтому после миграции ничего не пропадает из списков. Скрипт можно запускать повторно.
ALTER TABLE contractors ADD COLUMN IF NOT EXISTS is_supplier BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE contractors ADD COLUMN IF NOT EXISTS is_carrier  BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE contractors ADD COLUMN IF NOT EXISTS is_client   BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE contractors ADD COLUMN IF NOT EXISTS is_active   BOOLEAN NOT NULL DEFAULT TRUE;

COMMIT;