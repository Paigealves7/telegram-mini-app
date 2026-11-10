BEGIN;

-- Журнал корректировок склада: списание (брак, потери, свои нужды) и излишки.
-- Названия позиции сохраняются текстом, чтобы журнал читался, даже если позиция потом изменится.
-- Скрипт можно запускать повторно.
CREATE TABLE IF NOT EXISTS stock_adjustments (
    id           BIGSERIAL PRIMARY KEY,
    kind         TEXT          NOT NULL CHECK (kind IN ('board', 'log')),
    item_id      BIGINT        NOT NULL,
    item_label   TEXT          NOT NULL,
    delta_count  INT           NOT NULL,
    delta_volume NUMERIC(12,3) NOT NULL,
    reason       TEXT          NOT NULL CHECK (reason IN ('defect', 'loss', 'own_use', 'surplus', 'other')),
    note         TEXT          NOT NULL DEFAULT '',
    created_by   BIGINT        REFERENCES users(id),
    created_at   TIMESTAMP     NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_stock_adjustments_created_at ON stock_adjustments(created_at DESC);

COMMIT;