CREATE TABLE event_seats (
    event_seat_id UUID PRIMARY KEY,
    event_id UUID NOT NULL,
    category_id UUID NOT NULL,
    seat_label VARCHAR(10) NOT NULL,
    row_label VARCHAR(5) NOT NULL,
    seat_number INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE',
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_event_seats_event FOREIGN KEY (event_id) REFERENCES events(event_id) ON DELETE RESTRICT,
    CONSTRAINT fk_event_seats_category FOREIGN KEY (event_id, category_id) REFERENCES event_ticket_categories(event_id, category_id) ON DELETE RESTRICT,
    CONSTRAINT uq_event_seat_label UNIQUE (event_id, seat_label),
    CONSTRAINT chk_event_seats_status CHECK (status IN ('AVAILABLE', 'HELD', 'BOOKED'))
);

CREATE INDEX idx_event_seats_lookup ON event_seats(event_id, status);
