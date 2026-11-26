CREATE TABLE booking_seats (
    booking_seat_id UUID PRIMARY KEY,
    booking_id UUID NOT NULL REFERENCES bookings(booking_id) ON DELETE RESTRICT,
    event_seat_id UUID NOT NULL REFERENCES event_seats(event_seat_id) ON DELETE RESTRICT,
    booking_status VARCHAR(20) NOT NULL DEFAULT 'HELD',
    price_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_booking_seat UNIQUE (booking_id, event_seat_id),
    CONSTRAINT chk_booking_seats_status CHECK (booking_status IN ('HELD', 'PAID', 'CONFIRMED', 'EXPIRED', 'CANCELLED', 'REFUNDED'))
);

CREATE UNIQUE INDEX idx_unique_active_event_seat ON booking_seats(event_seat_id) WHERE booking_status IN ('HELD', 'PAID', 'CONFIRMED');
