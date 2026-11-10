BEGIN;

-- Имя и @username из Telegram для списка сотрудников. Заполняются при входе через Telegram.
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS tg_username TEXT NOT NULL DEFAULT '';

COMMIT;