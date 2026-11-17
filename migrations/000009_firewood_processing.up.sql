BEGIN;
CREATE TABLE IF NOT EXISTS firewood_settings (
    kind TEXT PRIMARY KEY CHECK (kind IN ('split', 'round')),
    coefficient NUMERIC(6,4) NOT NULL CHECK (coefficient > 0 AND coefficient <= 1)
);
INSERT INTO firewood_settings VALUES ('split', 0.55), ('round', 0.60) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS firewood_processing (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL REFERENCES firewood_settings(kind),
    volume_m3 NUMERIC(12,3) NOT NULL CHECK (volume_m3 > 0),
    coefficient NUMERIC(6,4) NOT NULL CHECK (coefficient > 0 AND coefficient <= 1),
    raw_m3 NUMERIC(16,7) NOT NULL CHECK (raw_m3 = volume_m3 * coefficient),
    performer TEXT NOT NULL CHECK (length(trim(performer)) BETWEEN 1 AND 120),
    date DATE NOT NULL,
    comment TEXT NOT NULL DEFAULT '' CHECK (length(comment) <= 1000),
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_firewood_processing_date ON firewood_processing(date DESC, id DESC);
CREATE TABLE IF NOT EXISTS order_firewood (
 id BIGSERIAL PRIMARY KEY,
 order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
 kind TEXT NOT NULL REFERENCES firewood_settings(kind),
 volume_m3 NUMERIC(12,3) NOT NULL CHECK(volume_m3>0),
 price NUMERIC(12,2) NOT NULL CHECK(price>=0)
);
CREATE INDEX IF NOT EXISTS idx_order_firewood_order ON order_firewood(order_id);
COMMIT;
