package payment

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPaymentFailed      = errors.New("payment was declined by the gateway")
	ErrGatewayTimeout     = errors.New("payment gateway timed out (in-doubt state)")
	ErrIdempotencyConflict = errors.New("concurrent in-flight operation for idempotency key")
	ErrInvalidAmount      = errors.New("payment amount must be greater than zero")
	ErrRefundDeclined     = errors.New("refund was rejected by the gateway")
)

type ChargeRequest struct {
	BookingID      string
	UserID         string
	AmountCents    int64
	Currency       string
	IdempotencyKey string
	PaymentMethod  string
}

type ChargeResponse struct {
	GatewayTransactionID string
	Status               string
	ProcessedAt          time.Time
}

type RefundRequest struct {
	PaymentID            string
	GatewayTransactionID string
	AmountCents          int64
	Reason               string
}

type RefundResponse struct {
	GatewayRefundID string
	Status          string
	ProcessedAt     time.Time
}

type Gateway interface {
	Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
	Inquire(ctx context.Context, gatewayTransactionID string) (string, error)
}
