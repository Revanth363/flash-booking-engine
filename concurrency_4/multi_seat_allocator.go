package concurrency

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmptySeatList     = errors.New("seat list cannot be empty")
	ErrPartialSeatHold   = errors.New("one or more requested seats are unavailable")
	ErrDeadlockPrevented = errors.New("deadlock avoided via canonical sort")
)

type MultiSeatAllocator struct {
	pool *pgxpool.Pool
}

func NewMultiSeatAllocator(pool *pgxpool.Pool) *MultiSeatAllocator {
	return &MultiSeatAllocator{pool: pool}
}

func (a *MultiSeatAllocator) AllocateMultiSeats(ctx context.Context, booking *domain.Booking, seatIDs []string, pricePerSeatCents int64) error {
	if len(seatIDs) == 0 {
		return ErrEmptySeatList
	}

	sortedSeatIDs := make([]string, len(seatIDs))
	copy(sortedSeatIDs, seatIDs)
	sort.Strings(sortedSeatIDs)

	for i := 0; i < len(sortedSeatIDs)-1; i++ {
		if sortedSeatIDs[i] == sortedSeatIDs[i+1] {
			return errors.New("duplicate seat id in request")
		}
	}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin multi-seat tx: %w", err)
	}
	defer tx.Rollback(ctx)

	type seatLockState struct {
		id      string
		version int
	}
	lockedSeats := make([]seatLockState, 0, len(sortedSeatIDs))

	// PESSIMISTIC ROW-LEVEL LOCKING: SELECT FOR UPDATE NOWAIT acquires an exclusive lock on the row, failing fast if contested
	for _, seatID := range sortedSeatIDs {
		var status string
		var version int
		query := `SELECT status, version FROM event_seats WHERE event_seat_id = $1 FOR UPDATE NOWAIT;`
		err := tx.QueryRow(ctx, query, seatID).Scan(&status, &version)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
				return repository.ErrLockNotAvailable
			}
			return fmt.Errorf("lock seat %s: %w", seatID, err)
		}

		if status != string(domain.SeatAvailable) {
			return ErrPartialSeatHold
		}
		lockedSeats = append(lockedSeats, seatLockState{id: seatID, version: version})
	}

	// OPTIMISTIC CONCURRENCY CONTROL: Version guard guarantees no concurrent modification occurred between read and commit
	for _, s := range lockedSeats {
		updateQuery := `
			UPDATE event_seats 
			SET status = $1, version = version + 1, updated_at = CURRENT_TIMESTAMP 
			WHERE event_seat_id = $2 AND version = $3;`
		cmdTag, err := tx.Exec(ctx, updateQuery, domain.SeatHeld, s.id, s.version)
		if err != nil {
			return fmt.Errorf("update seat %s: %w", s.id, err)
		}
		if cmdTag.RowsAffected() == 0 {
			return ErrPartialSeatHold
		}
	}

	totalAmount := int64(len(sortedSeatIDs)) * pricePerSeatCents
	booking.TotalAmountCents = totalAmount

	insertBookingQuery := `
		INSERT INTO bookings (booking_id, user_id, event_id, status, total_amount_cents, expires_at, version, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`

	_, err = tx.Exec(ctx, insertBookingQuery,
		booking.BookingID,
		booking.UserID,
		booking.EventID,
		booking.Status,
		booking.TotalAmountCents,
		booking.ExpiresAt,
		booking.Version,
	)
	if err != nil {
		return fmt.Errorf("insert multi-seat booking: %w", err)
	}

	for _, s := range lockedSeats {
		insertSeatQuery := `
			INSERT INTO booking_seats (booking_seat_id, booking_id, event_seat_id, booking_status, price_cents, created_at) 
			VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP);`
		bookingSeatID := fmt.Sprintf("%s-%s", booking.BookingID, s.id)
		_, err = tx.Exec(ctx, insertSeatQuery,
			bookingSeatID,
			booking.BookingID,
			s.id,
			booking.Status,
			pricePerSeatCents,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return repository.ErrDuplicateActive
			}
			return fmt.Errorf("insert booking_seat %s: %w", s.id, err)
		}
	}

	return tx.Commit(ctx)
}
