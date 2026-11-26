CREATE TABLE event_ticket_categories (
    category_id UUID PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES events(event_id) ON DELETE RESTRICT,
    name VARCHAR(50) NOT NULL,
    inventory_type VARCHAR(20) NOT NULL,
    price_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_event_category UNIQUE (event_id, category_id),
    CONSTRAINT uq_event_category_name UNIQUE (event_id, name),
    CONSTRAINT chk_category_price CHECK (price_cents >= 0),
    CONSTRAINT chk_inventory_type CHECK (inventory_type IN ('RESERVED_SEAT', 'GENERAL_ADMISSION'))
);
