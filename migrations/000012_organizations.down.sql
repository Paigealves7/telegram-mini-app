BEGIN;

ALTER TABLE firewood_processing DROP CONSTRAINT firewood_processing_organization_id_kind_fkey;
ALTER TABLE order_firewood DROP CONSTRAINT order_firewood_organization_id_kind_fkey;

ALTER TABLE log_arrivals
    DROP CONSTRAINT log_arrivals_supplier_id_organization_id_fkey,
    ADD FOREIGN KEY (supplier_id) REFERENCES contractors(id),
    DROP CONSTRAINT log_arrivals_carrier_id_organization_id_fkey,
    ADD FOREIGN KEY (carrier_id) REFERENCES contractors(id);

ALTER TABLE log_items
    DROP CONSTRAINT log_items_arrival_id_organization_id_fkey,
    ADD FOREIGN KEY (arrival_id) REFERENCES log_arrivals(id) ON DELETE CASCADE;

ALTER TABLE boards
    DROP CONSTRAINT boards_sawing_id_organization_id_fkey,
    ADD FOREIGN KEY (sawing_id) REFERENCES sawing_operations(id);

ALTER TABLE sawed_logs
    DROP CONSTRAINT sawed_logs_sawing_id_organization_id_fkey,
    ADD FOREIGN KEY (sawing_id) REFERENCES sawing_operations(id) ON DELETE CASCADE,
    DROP CONSTRAINT sawed_logs_log_id_organization_id_fkey,
    ADD FOREIGN KEY (log_id) REFERENCES log_items(id);

ALTER TABLE orders
    DROP CONSTRAINT orders_contractor_id_organization_id_fkey,
    ADD FOREIGN KEY (contractor_id) REFERENCES contractors(id);

ALTER TABLE order_boards
    DROP CONSTRAINT order_boards_order_id_organization_id_fkey,
    ADD FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    DROP CONSTRAINT order_boards_board_id_organization_id_fkey,
    ADD FOREIGN KEY (board_id) REFERENCES boards(id);

ALTER TABLE order_firewood
    DROP CONSTRAINT order_firewood_order_id_organization_id_fkey,
    ADD FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

ALTER TABLE order_payments
    DROP CONSTRAINT order_payments_order_id_organization_id_fkey,
    ADD FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

ALTER TABLE sales
    DROP CONSTRAINT sales_contractor_id_organization_id_fkey,
    ADD FOREIGN KEY (contractor_id) REFERENCES contractors(id),
    DROP CONSTRAINT sales_order_id_organization_id_fkey,
    ADD FOREIGN KEY (order_id) REFERENCES orders(id);

ALTER TABLE sale_boards
    DROP CONSTRAINT sale_boards_sale_id_organization_id_fkey,
    ADD FOREIGN KEY (sale_id) REFERENCES sales(id) ON DELETE CASCADE,
    DROP CONSTRAINT sale_boards_board_id_organization_id_fkey,
    ADD FOREIGN KEY (board_id) REFERENCES boards(id);

ALTER TABLE firewood_arrivals
    DROP CONSTRAINT firewood_arrivals_supplier_id_organization_id_fkey,
    ADD FOREIGN KEY (supplier_id) REFERENCES contractors(id),
    DROP CONSTRAINT firewood_arrivals_carrier_id_organization_id_fkey,
    ADD FOREIGN KEY (carrier_id) REFERENCES contractors(id);

ALTER TABLE orders DROP COLUMN order_number;

ALTER TABLE contractors DROP COLUMN organization_id;
ALTER TABLE log_arrivals DROP COLUMN organization_id;
ALTER TABLE log_items DROP COLUMN organization_id;
ALTER TABLE sawing_operations DROP COLUMN organization_id;
ALTER TABLE boards DROP COLUMN organization_id;
ALTER TABLE sawed_logs DROP COLUMN organization_id;
ALTER TABLE orders DROP COLUMN organization_id;
ALTER TABLE order_boards DROP COLUMN organization_id;
ALTER TABLE order_firewood DROP COLUMN organization_id;
ALTER TABLE order_payments DROP COLUMN organization_id;
ALTER TABLE sales DROP COLUMN organization_id;
ALTER TABLE sale_boards DROP COLUMN organization_id;
ALTER TABLE stock_adjustments DROP COLUMN organization_id;
ALTER TABLE firewood_arrivals DROP COLUMN organization_id;
ALTER TABLE firewood_processing DROP COLUMN organization_id;
ALTER TABLE firewood_stock_adjustments DROP COLUMN organization_id;

ALTER TABLE firewood_settings
    DROP CONSTRAINT firewood_settings_pkey,
    DROP COLUMN organization_id,
    ADD PRIMARY KEY (kind);
ALTER TABLE firewood_processing ADD FOREIGN KEY (kind) REFERENCES firewood_settings(kind);
ALTER TABLE order_firewood ADD FOREIGN KEY (kind) REFERENCES firewood_settings(kind);

ALTER TABLE deleted_telegram_users
    DROP CONSTRAINT deleted_telegram_users_pkey,
    DROP COLUMN organization_id,
    ADD PRIMARY KEY (username);

DROP TABLE organization_invites;
DROP TABLE organization_members;
DROP TABLE organizations;

COMMIT;
