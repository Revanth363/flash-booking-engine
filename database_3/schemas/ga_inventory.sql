CREATE TABLE ga_inventory (
    inventory_id UUID PRIMARY KEY,
    category_id UUID NOT NULL UNIQUE REFERENCES event_ticket_categories(category_id) ON DELETE RESTRICT,
    total_capacity INT NOT NULL,
    reserved_count INT NOT NULL DEFAULT 0,
    confirmed_count INT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_ga_total_capacity CHECK (total_capacity >= 0),
    CONSTRAINT chk_ga_reserved_count CHECK (reserved_count >= 0),
    CONSTRAINT chk_ga_confirmed_count CHECK (confirmed_count >= 0),
    CONSTRAINT chk_ga_no_oversell CHECK (reserved_count + confirmed_count <= total_capacity)
);
