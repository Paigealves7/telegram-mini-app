CREATE TYPE role AS ENUM ('manager', 'master', 'worker');
CREATE TYPE warehouse_type AS ENUM ('logs', 'boards');
CREATE TYPE order_status AS ENUM ('new', 'in_progress', 'completed', 'canceled');
CREATE TYPE operation_type AS ENUM ('arrival', 'sawing', 'sale', 'order');
CREATE TYPE user_role AS ENUM ('user', 'admin', 'worker');

ALTER TABLE users ALTER COLUMN role SET DEFAULT 'user'::user_role;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role role NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE warehouses (
    id BIGSERIAL PRIMARY KEY,
    type warehouse_type NOT NULL,
    name TEXT NOT NULL
);

CREATE TABLE contractors (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE log_arrivals (
    id BIGSERIAL PRIMARY KEY,
    supplier_id BIGINT REFERENCES contractors(id),
    carrier_id BIGINT REFERENCES contractors(id),
    truck_number TEXT NOT NULL,
    arrival_date DATE NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE log_items (
    id BIGSERIAL PRIMARY KEY,
    arrival_id BIGINT NOT NULL REFERENCES log_arrivals(id) ON DELETE CASCADE,
    length_mm INT NOT NULL,
    diameter_mm INT NOT NULL,
    species TEXT NOT NULL,
    count INT NOT NULL,
    volume_m3 NUMERIC(12,3) NOT NULL
);
CREATE INDEX idx_log_items_arrival_id ON log_items(arrival_id);
CREATE INDEX idx_log_items_species ON log_items(species);

CREATE TABLE sawing_operations (
    id BIGSERIAL PRIMARY KEY,
    worker_id BIGINT NOT NULL REFERENCES users(id),
    date DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE sawed_logs (
    id BIGSERIAL PRIMARY KEY,
    sawing_id BIGINT NOT NULL REFERENCES sawing_operations(id) ON DELETE CASCADE,
    log_id BIGINT REFERENCES log_items(id),
    volume_m3 NUMERIC(12,3) NOT NULL
);

CREATE TABLE boards (
    id BIGSERIAL PRIMARY KEY,
    height_mm INT NOT NULL,
    width_mm INT NOT NULL,
    length_mm INT NOT NULL,
    species TEXT NOT NULL,
    grade TEXT NOT NULL,
    count INT NOT NULL,
    volume_m3 NUMERIC(12,3) NOT NULL,
    price_per_m3 NUMERIC(12,2)
);
CREATE INDEX idx_boards_species ON boards(species);
CREATE INDEX idx_boards_grade ON boards(grade);

CREATE TABLE sales (
    id BIGSERIAL PRIMARY KEY,
    contractor_id BIGINT NOT NULL REFERENCES contractors(id),
    truck_number TEXT NOT NULL,
    delivery_price NUMERIC(12,2) DEFAULT 0,
    extra_price NUMERIC(12,2) DEFAULT 0,
    total_volume NUMERIC(12,3) NOT NULL,
    total_price NUMERIC(12,2) NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE sale_boards (
    id BIGSERIAL PRIMARY KEY,
    sale_id BIGINT NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    board_id BIGINT NOT NULL REFERENCES boards(id),
    count INT NOT NULL,
    volume_m3 NUMERIC(12,3) NOT NULL,
    price NUMERIC(12,2) NOT NULL
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    master_id BIGINT NOT NULL REFERENCES users(id),
    contractor_id BIGINT NOT NULL REFERENCES contractors(id),
    notes TEXT,
    delivery_price NUMERIC(12,2) DEFAULT 0,
    extra_price NUMERIC(12,2) DEFAULT 0,
    status order_status NOT NULL DEFAULT 'new',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE order_boards (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    board_id BIGINT NOT NULL REFERENCES boards(id),
    count INT NOT NULL,
    volume_m3 NUMERIC(12,3) NOT NULL,
    price NUMERIC(12,2) NOT NULL
);