BEGIN;

-- Оплаты по заказам. Сумма заказа считается из позиций, доставки и допов; оплаты суммируются отдельно.
-- Скрипт можно запускать повторно.
CREATE TABLE IF NOT EXISTS order_payments (
    id         BIGSERIAL PRIMARY KEY,
    order_id   BIGINT        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    amount     NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    note       TEXT          NOT NULL DEFAULT '',
    created_by BIGINT        REFERENCES users(id),
    created_at TIMESTAMP     NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_order_payments_order_id ON order_payments(order_id);

COMMIT;