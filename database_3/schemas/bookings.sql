CREATE TABLE bookings (
    booking_id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    event_id UUID NOT NULL REFERENCES events(event_id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'HELD',
    total_amount_cents BIGINT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_bookings_status CHECK (status IN ('HELD', 'PAID', 'CONFIRMED', 'EXPIRED', 'CANCELLED', 'REFUND_PENDING', 'REFUNDED'))
);

CREATE INDEX idx_bookings_expiry ON bookings(expires_at) WHERE status = 'HELD';
CREATE INDEX idx_bookings_user_active_hold ON bookings(user_id) WHERE status = 'HELD';
