BEGIN;

CREATE TABLE organizations (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    company_info TEXT NOT NULL DEFAULT '',
    is_demo BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_order_number BIGINT NOT NULL DEFAULT 0 CHECK (last_order_number >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE organization_members (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role role NOT NULL CHECK (role IN ('manager', 'master', 'worker')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE organization_invites (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL UNIQUE,
    role role NOT NULL CHECK (role IN ('manager', 'master', 'worker')),
    max_uses INT NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    used_count INT NOT NULL DEFAULT 0 CHECK (used_count >= 0 AND used_count <= max_uses),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO organizations (id, name, is_demo, is_active)
VALUES (1, 'Лесопилка', FALSE, TRUE);
SELECT setval(pg_get_serial_sequence('organizations', 'id'),
              GREATEST((SELECT COALESCE(MAX(id), 1) FROM organizations), 1), true);

INSERT INTO organization_members (organization_id, user_id, role)
SELECT 1, id, role FROM users WHERE role IN ('manager', 'master', 'worker')
ON CONFLICT DO NOTHING;

ALTER TABLE deleted_telegram_users
    ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id) ON DELETE CASCADE,
    DROP CONSTRAINT deleted_telegram_users_pkey,
    ADD PRIMARY KEY (organization_id, username);
ALTER TABLE deleted_telegram_users ALTER COLUMN organization_id DROP DEFAULT;

ALTER TABLE firewood_processing DROP CONSTRAINT firewood_processing_kind_fkey;
ALTER TABLE order_firewood DROP CONSTRAINT order_firewood_kind_fkey;
ALTER TABLE firewood_settings
    ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id) ON DELETE CASCADE,
    DROP CONSTRAINT firewood_settings_pkey,
    ADD PRIMARY KEY (organization_id, kind);
ALTER TABLE firewood_settings ALTER COLUMN organization_id DROP DEFAULT;

INSERT INTO firewood_settings (organization_id, kind, coefficient)
VALUES (1, 'split', 0.55), (1, 'round', 0.60)
ON CONFLICT (organization_id, kind) DO NOTHING;

ALTER TABLE contractors ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE log_arrivals ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE log_items ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE sawing_operations ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE boards ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE sawed_logs ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE orders ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE order_boards ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE order_firewood ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE order_payments ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE sales ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE sale_boards ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE stock_adjustments ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE firewood_arrivals ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE firewood_processing ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);
ALTER TABLE firewood_stock_adjustments ADD COLUMN organization_id BIGINT NOT NULL DEFAULT 1 REFERENCES organizations(id);

ALTER TABLE contractors ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE log_arrivals ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE log_items ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE sawing_operations ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE boards ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE sawed_logs ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE orders ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE order_boards ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE order_firewood ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE order_payments ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE sales ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE sale_boards ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE stock_adjustments ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE firewood_arrivals ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE firewood_processing ALTER COLUMN organization_id DROP DEFAULT;
ALTER TABLE firewood_stock_adjustments ALTER COLUMN organization_id DROP DEFAULT;

ALTER TABLE orders ADD COLUMN order_number BIGINT;
UPDATE orders SET order_number = id WHERE order_number IS NULL;
ALTER TABLE orders
    ALTER COLUMN order_number SET NOT NULL,
    ADD CHECK (order_number > 0),
    ADD UNIQUE (organization_id, order_number);
UPDATE organizations
SET last_order_number = COALESCE((SELECT MAX(order_number) FROM orders WHERE organization_id = 1), 0)
WHERE id = 1;

ALTER TABLE contractors ADD UNIQUE (id, organization_id);
ALTER TABLE log_arrivals ADD UNIQUE (id, organization_id);
ALTER TABLE log_items ADD UNIQUE (id, organization_id);
ALTER TABLE sawing_operations ADD UNIQUE (id, organization_id);
ALTER TABLE boards ADD UNIQUE (id, organization_id);
ALTER TABLE orders ADD UNIQUE (id, organization_id);
ALTER TABLE sales ADD UNIQUE (id, organization_id);

ALTER TABLE log_arrivals
    DROP CONSTRAINT log_arrivals_supplier_id_fkey,
    ADD FOREIGN KEY (supplier_id, organization_id) REFERENCES contractors(id, organization_id),
    DROP CONSTRAINT log_arrivals_carrier_id_fkey,
    ADD FOREIGN KEY (carrier_id, organization_id) REFERENCES contractors(id, organization_id);

ALTER TABLE log_items
    DROP CONSTRAINT log_items_arrival_id_fkey,
    ADD FOREIGN KEY (arrival_id, organization_id) REFERENCES log_arrivals(id, organization_id) ON DELETE CASCADE;

ALTER TABLE boards
    DROP CONSTRAINT boards_sawing_id_fkey,
    ADD FOREIGN KEY (sawing_id, organization_id) REFERENCES sawing_operations(id, organization_id);

ALTER TABLE sawed_logs
    DROP CONSTRAINT sawed_logs_sawing_id_fkey,
    ADD FOREIGN KEY (sawing_id, organization_id) REFERENCES sawing_operations(id, organization_id) ON DELETE CASCADE,
    DROP CONSTRAINT sawed_logs_log_id_fkey,
    ADD FOREIGN KEY (log_id, organization_id) REFERENCES log_items(id, organization_id);

ALTER TABLE orders
    DROP CONSTRAINT orders_contractor_id_fkey,
    ADD FOREIGN KEY (contractor_id, organization_id) REFERENCES contractors(id, organization_id);

ALTER TABLE order_boards
    DROP CONSTRAINT order_boards_order_id_fkey,
    ADD FOREIGN KEY (order_id, organization_id) REFERENCES orders(id, organization_id) ON DELETE CASCADE,
    DROP CONSTRAINT order_boards_board_id_fkey,
    ADD FOREIGN KEY (board_id, organization_id) REFERENCES boards(id, organization_id);

ALTER TABLE order_firewood
    DROP CONSTRAINT order_firewood_order_id_fkey,
    ADD FOREIGN KEY (order_id, organization_id) REFERENCES orders(id, organization_id) ON DELETE CASCADE;

ALTER TABLE order_payments
    DROP CONSTRAINT order_payments_order_id_fkey,
    ADD FOREIGN KEY (order_id, organization_id) REFERENCES orders(id, organization_id) ON DELETE CASCADE;

ALTER TABLE sales
    DROP CONSTRAINT sales_contractor_id_fkey,
    ADD FOREIGN KEY (contractor_id, organization_id) REFERENCES contractors(id, organization_id),
    DROP CONSTRAINT sales_order_id_fkey,
    ADD FOREIGN KEY (order_id, organization_id) REFERENCES orders(id, organization_id);

ALTER TABLE sale_boards
    DROP CONSTRAINT sale_boards_sale_id_fkey,
    ADD FOREIGN KEY (sale_id, organization_id) REFERENCES sales(id, organization_id) ON DELETE CASCADE,
    DROP CONSTRAINT sale_boards_board_id_fkey,
    ADD FOREIGN KEY (board_id, organization_id) REFERENCES boards(id, organization_id);

ALTER TABLE firewood_arrivals
    DROP CONSTRAINT firewood_arrivals_supplier_id_fkey,
    ADD FOREIGN KEY (supplier_id, organization_id) REFERENCES contractors(id, organization_id),
    DROP CONSTRAINT firewood_arrivals_carrier_id_fkey,
    ADD FOREIGN KEY (carrier_id, organization_id) REFERENCES contractors(id, organization_id);

ALTER TABLE firewood_processing
    ADD FOREIGN KEY (organization_id, kind) REFERENCES firewood_settings(organization_id, kind);
ALTER TABLE order_firewood
    ADD FOREIGN KEY (organization_id, kind) REFERENCES firewood_settings(organization_id, kind);

CREATE INDEX idx_orders_organization_id ON orders(organization_id);
CREATE INDEX idx_sales_organization_id ON sales(organization_id);
CREATE INDEX idx_boards_organization_id ON boards(organization_id);
CREATE INDEX idx_log_items_organization_id ON log_items(organization_id);
CREATE INDEX idx_log_arrivals_organization_id ON log_arrivals(organization_id);
CREATE INDEX idx_sawing_operations_organization_id ON sawing_operations(organization_id);
CREATE INDEX idx_firewood_arrivals_organization_id ON firewood_arrivals(organization_id);
CREATE INDEX idx_firewood_processing_organization_id ON firewood_processing(organization_id);
CREATE INDEX idx_stock_adjustments_organization_id ON stock_adjustments(organization_id);
CREATE INDEX idx_contractors_organization_id ON contractors(organization_id);
CREATE INDEX idx_organization_members_user_id ON organization_members(user_id);

COMMIT;
