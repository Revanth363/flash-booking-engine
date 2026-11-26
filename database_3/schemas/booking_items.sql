CREATE TABLE booking_items (
    booking_item_id UUID PRIMARY KEY,
    booking_id UUID NOT NULL REFERENCES bookings(booking_id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES event_ticket_categories(category_id) ON DELETE RESTRICT,
    quantity INT NOT NULL,
    unit_price_cents BIGINT NOT NULL,
    subtotal_cents BIGINT NOT NULL,
    CONSTRAINT chk_booking_items_quantity CHECK (quantity > 0)
);
