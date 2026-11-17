BEGIN;

-- Приход дров: считается общей кубатурой (м³), без партий и брёвен. Порода всегда берёза.
-- Остаток сырья (дров) = приходы минус переработка (переработка появится следующим шагом).
-- Скрипт можно запускать повторно.
CREATE TABLE IF NOT EXISTS firewood_arrivals (
    id           BIGSERIAL PRIMARY KEY,
    supplier_id  BIGINT        NOT NULL REFERENCES contractors(id),
    carrier_id   BIGINT        NOT NULL REFERENCES contractors(id),
    truck_number TEXT          NOT NULL,
    volume_m3    NUMERIC(12,3) NOT NULL CHECK (volume_m3 > 0),
    arrival_date DATE          NOT NULL DEFAULT CURRENT_DATE,
    created_by   BIGINT        REFERENCES users(id),
    created_at   TIMESTAMP     NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_firewood_arrivals_date ON firewood_arrivals(arrival_date DESC, id DESC);

COMMIT;
