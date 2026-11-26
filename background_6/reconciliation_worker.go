package background

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"
	"flash-booking-engine/payment_5"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ReconciliationWorker struct {
	pool        *pgxpool.Pool
	bookingRepo *repository.BookingRepository
	gateway     payment.Gateway
	logger      *slog.Logger
	interval    time.Duration
}

func NewReconciliationWorker(pool *pgxpool.Pool, repo *repository.BookingRepository, gateway payment.Gateway, logger *slog.Logger, interval time.Duration) *ReconciliationWorker {
	return &ReconciliationWorker{
		pool:        pool,
		bookingRepo: repo,
		gateway:     gateway,
		logger:      logger,
		interval:    interval,
	}
}

func (w *ReconciliationWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("payment reconciliation worker started", "interval", w.interval.String())

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("payment reconciliation worker stopping")
			return
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

func (w *ReconciliationWorker) reconcile(ctx context.Context) {
	reconcileCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		SELECT payment_id, booking_id, amount_cents 
		FROM payments 
		WHERE status = 'IN_DOUBT' 
		LIMIT 10 
		FOR UPDATE SKIP LOCKED;`

	tx, err := w.pool.Begin(reconcileCtx)
	if err != nil {
		w.logger.Error("reconcile begin tx error", "error", err)
		return
	}
	defer tx.Rollback(reconcileCtx)

	rows, err := tx.Query(reconcileCtx, query)
	if err != nil {
		w.logger.Error("query in_doubt payments error", "error", err)
		return
	}
	defer rows.Close()

	type pendingItem struct {
		paymentID string
		bookingID string
		amount    int64
	}
	var items []pendingItem

	for rows.Next() {
		var it pendingItem
		if err := rows.Scan(&it.paymentID, &it.bookingID, &it.amount); err == nil {
			items = append(items, it)
		}
	}
	rows.Close()

	for _, it := range items {
		status, err := w.gateway.Inquire(reconcileCtx, it.paymentID)
		if err != nil {
			w.logger.Warn("gateway inquiry failed, will retry next cycle", "payment_id", it.paymentID, "error", err)
			continue
		}

		if status == "SUCCESS" {
			booking, err := w.bookingRepo.GetBookingForUpdate(reconcileCtx, tx, it.bookingID)
			if err != nil {
				w.logger.Error("failed to lock booking for reconciliation", "booking_id", it.bookingID, "error", err)
				continue
			}

			if booking.Status == domain.StatusHeld && !booking.IsExpiredAt(time.Now()) {
				_ = w.bookingRepo.UpdateBookingStatus(reconcileCtx, tx, it.bookingID, domain.StatusPaid, booking.Version)
				_ = w.bookingRepo.UpdateBookingStatus(reconcileCtx, tx, it.bookingID, domain.StatusConfirmed, booking.Version+1)
				_, _ = tx.Exec(reconcileCtx, `
					UPDATE event_seats 
					SET status = 'BOOKED', version = version + 1, updated_at = CURRENT_TIMESTAMP 
					WHERE event_seat_id IN (
						SELECT event_seat_id FROM booking_seats WHERE booking_id = $1
					);`, it.bookingID)
				_, _ = tx.Exec(reconcileCtx, `UPDATE payments SET status = 'SUCCESS', updated_at = CURRENT_TIMESTAMP WHERE payment_id = $1;`, it.paymentID)
				w.logger.Info("reconciled in_doubt payment to CONFIRMED", "booking_id", it.bookingID)
			} else {
				refundID := fmt.Sprintf("rfnd-%d", time.Now().UnixNano())
				_, _ = tx.Exec(reconcileCtx, `
					INSERT INTO refunds (refund_id, payment_id, amount_cents, status, reason, created_at, updated_at) 
					VALUES ($1, $2, $3, 'REFUND_PENDING', 'Reconciliation: payment charged after expiration', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`,
					refundID, it.paymentID, it.amount,
				)
				_, _ = tx.Exec(reconcileCtx, `UPDATE payments SET status = 'SUCCESS', updated_at = CURRENT_TIMESTAMP WHERE payment_id = $1;`, it.paymentID)
				w.logger.Info("reconciled in_doubt payment to REFUND_PENDING (expired hold)", "booking_id", it.bookingID)
			}
		} else {
			_, _ = tx.Exec(reconcileCtx, `UPDATE payments SET status = 'FAILED', updated_at = CURRENT_TIMESTAMP WHERE payment_id = $1;`, it.paymentID)
			w.logger.Info("reconciled in_doubt payment to FAILED", "payment_id", it.paymentID)
		}
	}

	_ = tx.Commit(reconcileCtx)
}
