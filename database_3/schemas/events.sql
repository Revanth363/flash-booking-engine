CREATE TABLE events (
    event_id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    venue_name VARCHAR(255) NOT NULL,
    booking_start_time TIMESTAMPTZ NOT NULL,
    event_start_time TIMESTAMPTZ NOT NULL,
    event_end_time TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'UPCOMING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_events_status CHECK (status IN ('UPCOMING', 'LIVE', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX idx_events_start_status ON events(booking_start_time, status);
