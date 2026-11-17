BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM firewood_stock_adjustments) OR EXISTS (SELECT 1 FROM deleted_telegram_users) THEN
        RAISE EXCEPTION 'Откат требует переноса корректировок дров и списка отключённых аккаунтов';
    END IF;
END
$$;

-- Если сотрудники уже удалены, прежняя схема с NOT NULL неприменима без восстановления авторов.
ALTER TABLE sawing_operations ALTER COLUMN worker_id SET NOT NULL,
    DROP CONSTRAINT sawing_operations_worker_id_fkey,
    ADD FOREIGN KEY (worker_id) REFERENCES users(id);
ALTER TABLE orders ALTER COLUMN master_id SET NOT NULL,
    DROP CONSTRAINT orders_master_id_fkey,
    ADD FOREIGN KEY (master_id) REFERENCES users(id);
ALTER TABLE firewood_processing ALTER COLUMN created_by SET NOT NULL,
    DROP CONSTRAINT firewood_processing_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE log_arrivals DROP CONSTRAINT log_arrivals_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE sales DROP CONSTRAINT sales_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE order_payments DROP CONSTRAINT order_payments_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE stock_adjustments DROP CONSTRAINT stock_adjustments_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE firewood_arrivals DROP CONSTRAINT firewood_arrivals_created_by_fkey,
    ADD FOREIGN KEY (created_by) REFERENCES users(id);

DROP TABLE firewood_stock_adjustments;
DROP TABLE deleted_telegram_users;
ALTER TABLE sawed_logs DROP COLUMN consumed_count, DROP COLUMN consumed_volume_m3;
ALTER TABLE sales DROP COLUMN order_id, DROP COLUMN legacy_order_sale;

COMMIT;
