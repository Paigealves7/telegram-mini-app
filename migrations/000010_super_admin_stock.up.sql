BEGIN;

ALTER TABLE sales ADD COLUMN order_id BIGINT REFERENCES orders(id);
ALTER TABLE sales ADD COLUMN legacy_order_sale BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX sales_order_id_unique ON sales(order_id) WHERE order_id IS NOT NULL;

-- При завершении заказа продажа и updated_at получают NOW() одной транзакции.
-- Привязываем только пары, однозначные с обеих сторон.
WITH matches AS (
    SELECT s.id AS sale_id, o.id AS order_id,
           COUNT(*) OVER (PARTITION BY s.id) AS orders_per_sale,
           COUNT(*) OVER (PARTITION BY o.id) AS sales_per_order
    FROM sales s JOIN orders o ON s.contractor_id = o.contractor_id AND s.created_at = o.updated_at
    WHERE s.truck_number = 'Доставка по заявке' AND o.status = 'completed'
)
UPDATE sales s SET order_id = m.order_id FROM matches m
WHERE s.id = m.sale_id AND m.orders_per_sale = 1 AND m.sales_per_order = 1;

UPDATE sales SET legacy_order_sale = true
WHERE order_id IS NULL AND truck_number = 'Доставка по заявке';

ALTER TABLE sawed_logs ADD COLUMN consumed_count INT CHECK (consumed_count >= 0);
ALTER TABLE sawed_logs ADD COLUMN consumed_volume_m3 NUMERIC(12,3) CHECK (consumed_volume_m3 >= 0);

ALTER TABLE sawing_operations ALTER COLUMN worker_id DROP NOT NULL,
    DROP CONSTRAINT sawing_operations_worker_id_fkey,
    ADD FOREIGN KEY (worker_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE orders ALTER COLUMN master_id DROP NOT NULL,
    DROP CONSTRAINT orders_master_id_fkey,
    ADD FOREIGN KEY (master_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE firewood_processing ALTER COLUMN created_by DROP NOT NULL,
    DROP CONSTRAINT firewood_processing_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE log_arrivals DROP CONSTRAINT log_arrivals_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE sales DROP CONSTRAINT sales_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE order_payments DROP CONSTRAINT order_payments_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE stock_adjustments DROP CONSTRAINT stock_adjustments_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE firewood_arrivals DROP CONSTRAINT firewood_arrivals_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE firewood_stock_adjustments (
    id BIGSERIAL PRIMARY KEY,
    raw_delta_m3 NUMERIC(16,7) NOT NULL,
    split_delta_m3 NUMERIC(12,3) NOT NULL,
    round_delta_m3 NUMERIC(12,3) NOT NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE deleted_telegram_users (
    username TEXT PRIMARY KEY
);

COMMIT;
