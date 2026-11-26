CREATE TABLE refunds (
    refund_id UUID PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES payments(payment_id) ON DELETE RESTRICT,
    gateway_refund_id VARCHAR(255) UNIQUE,
    amount_cents BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'REFUND_PENDING',
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_refunds_status CHECK (status IN ('REFUND_PENDING', 'REFUNDED', 'FAILED'))
);

CREATE INDEX idx_refunds_payment ON refunds(payment_id);
