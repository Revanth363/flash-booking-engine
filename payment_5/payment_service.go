package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentService struct {
	pool        *pgxpool.Pool
	bookingRepo *repository.BookingRepository
	gateway     Gateway
	idempMgr    *IdempotencyManager
}

func NewPaymentService(pool *pgxpool.Pool, repo *repository.BookingRepository, gateway Gateway, idempMgr *IdempotencyManager) *PaymentService {
	return &PaymentService{
		pool:        pool,
		bookingRepo: repo,
		gateway:     gateway,
		idempMgr:    idempMgr,
	}
}

type ProcessPaymentResult struct {
	PaymentID            string `json:"payment_id"`
	BookingID            string `json:"booking_id"`
	Status               string `json:"status"`
	GatewayTransactionID string `json:"gateway_transaction_id"`
	RefundTriggered      bool   `json:"refund_triggered,omitempty"`
}

func (s *PaymentService) ProcessPayment(ctx context.Context, req ChargeRequest) (*ProcessPaymentResult, error) {
	reqBytes, _ := json.Marshal(req)
	reqHash := HashPayload(reqBytes)

	// Check Idempotency Key first (ADR-06: return cached response if retried)
	record, err := s.idempMgr.AcquireOrGet(ctx, req.UserID, req.IdempotencyKey, "/api/v1/payments/charge", reqHash, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == "COMPLETED" {
		var cachedResult ProcessPaymentResult
		if err := json.Unmarshal([]byte(record.ResponseBody), &cachedResult); err == nil {
			return &cachedResult, nil
		}
	}

	chargeResp, err := s.gateway.Charge(ctx, req)
	if err != nil {
		if err == ErrGatewayTimeout {
			paymentID := fmt.Sprintf("pay-%d", time.Now().UnixNano())
			_, _ = s.pool.Exec(ctx, `
				INSERT INTO payments (payment_id, booking_id, amount_cents, currency, status, payment_method, created_at, updated_at) 
				VALUES ($1, $2, $3, $4, 'IN_DOUBT', $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`,
				paymentID, req.BookingID, req.AmountCents, req.Currency, req.PaymentMethod,
			)
			return nil, ErrGatewayTimeout
		}
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin payment settlement tx: %w", err)
	}
	defer tx.Rollback(ctx)

	booking, err := s.bookingRepo.GetBookingForUpdate(ctx, tx, req.BookingID)
	if err != nil {
		return nil, fmt.Errorf("fetch booking for payment: %w", err)
	}

	paymentID := uuid.NewString()

	if booking.Status != domain.StatusHeld || booking.IsExpiredAt(time.Now()) {
		_, _ = tx.Exec(ctx, `
			INSERT INTO payments (payment_id, booking_id, gateway_transaction_id, amount_cents, currency, status, payment_method, created_at, updated_at) 
			VALUES ($1, $2, $3, $4, $5, 'SUCCESS', $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`,
			paymentID, req.BookingID, chargeResp.GatewayTransactionID, req.AmountCents, req.Currency, req.PaymentMethod,
		)

		refundID := uuid.NewString()
		_, _ = tx.Exec(ctx, `
			INSERT INTO refunds (refund_id, payment_id, amount_cents, status, reason, created_at, updated_at) 
			VALUES ($1, $2, $3, 'REFUND_PENDING', 'Payment settled after hold expiration or cancellation', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`,
			refundID, paymentID, req.AmountCents,
		)

		_, _ = tx.Exec(ctx, `
			UPDATE bookings 
			SET status = 'REFUND_PENDING', version = version + 1, updated_at = CURRENT_TIMESTAMP 
			WHERE booking_id = $1;`, req.BookingID)

		_ = tx.Commit(ctx)

		_, _ = s.gateway.Refund(ctx, RefundRequest{
			PaymentID:            paymentID,
			GatewayTransactionID: chargeResp.GatewayTransactionID,
			AmountCents:          req.AmountCents,
			Reason:               "Auto-refund: hold window expired",
		})

		refundResult := &ProcessPaymentResult{
			PaymentID:            paymentID,
			BookingID:            req.BookingID,
			Status:               "REFUND_TRIGGERED",
			GatewayTransactionID: chargeResp.GatewayTransactionID,
			RefundTriggered:      true,
		}

		respBytes, _ := json.Marshal(refundResult)
		_ = s.idempMgr.Complete(ctx, req.UserID, req.IdempotencyKey, 200, string(respBytes), req.BookingID)

		return refundResult, nil
	}

	err = s.bookingRepo.UpdateBookingStatus(ctx, tx, req.BookingID, domain.StatusPaid, booking.Version)
	if err != nil {
		return nil, fmt.Errorf("update booking to PAID: %w", err)
	}

	err = s.bookingRepo.UpdateBookingStatus(ctx, tx, req.BookingID, domain.StatusConfirmed, booking.Version+1)
	if err != nil {
		return nil, fmt.Errorf("update booking to CONFIRMED: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE event_seats 
		SET status = 'BOOKED', version = version + 1, updated_at = CURRENT_TIMESTAMP 
		WHERE event_seat_id IN (
			SELECT event_seat_id FROM booking_seats WHERE booking_id = $1
		);`, req.BookingID)
	if err != nil {
		return nil, fmt.Errorf("mark event seats BOOKED: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payments (payment_id, booking_id, gateway_transaction_id, amount_cents, currency, status, payment_method, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, 'SUCCESS', $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`,
		paymentID, req.BookingID, chargeResp.GatewayTransactionID, req.AmountCents, req.Currency, req.PaymentMethod,
	)
	if err != nil {
		return nil, fmt.Errorf("insert successful payment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit payment transaction: %w", err)
	}

	finalResult := &ProcessPaymentResult{
		PaymentID:            paymentID,
		BookingID:            req.BookingID,
		Status:               "CONFIRMED",
		GatewayTransactionID: chargeResp.GatewayTransactionID,
	}

	respBytes, _ := json.Marshal(finalResult)
	_ = s.idempMgr.Complete(ctx, req.UserID, req.IdempotencyKey, 200, string(respBytes), req.BookingID)

	return finalResult, nil
}
