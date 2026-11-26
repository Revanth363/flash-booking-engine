package repository

import (
	"context"
	"errors"
	"fmt"

	"flash-booking-engine/application_2/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrLockNotAvailable = errors.New("seat row lock not available (NOWAIT)")
	ErrDuplicateActive  = errors.New("seat is already actively reserved or confirmed")
	ErrBookingNotFound  = errors.New("booking not found")
	ErrEventNotFound    = errors.New("event not found")
	ErrSeatNotFound     = errors.New("seat not found")
)

type BookingRepository struct {
	pool *pgxpool.Pool
}

func NewBookingRepository(pool *pgxpool.Pool) *BookingRepository {
	return &BookingRepository{pool: pool}
}

func (r *BookingRepository) HoldSeat(ctx context.Context, booking *domain.Booking, seatID string, priceCents int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var currentVersion int
	// PESSIMISTIC ROW-LEVEL LOCKING: SELECT FOR UPDATE NOWAIT prevents lock queuing and fails fast on contended seats
	lockQuery := `
		SELECT status, version 
		FROM event_seats 
		WHERE event_seat_id = $1 
		FOR UPDATE NOWAIT;`

	err = tx.QueryRow(ctx, lockQuery, seatID).Scan(&currentStatus, &currentVersion)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return ErrLockNotAvailable
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSeatNotFound
		}
		return fmt.Errorf("lock seat: %w", err)
	}

	if currentStatus != string(domain.SeatAvailable) {
		return domain.ErrSeatUnavailable
	}

	// OPTIMISTIC CONCURRENCY CONTROL: Version guard (CAS) ensures state mutation only succeeds if version matches
	updateSeatQuery := `
		UPDATE event_seats 
		SET status = $1, version = version + 1, updated_at = CURRENT_TIMESTAMP 
		WHERE event_seat_id = $2 AND version = $3;`

	cmdTag, err := tx.Exec(ctx, updateSeatQuery, domain.SeatHeld, seatID, currentVersion)
	if err != nil {
		return fmt.Errorf("update seat status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domain.ErrSeatUnavailable
	}

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
		return fmt.Errorf("insert booking: %w", err)
	}

	insertSeatQuery := `
		INSERT INTO booking_seats (booking_seat_id, booking_id, event_seat_id, booking_status, price_cents, created_at) 
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP);`

	bookingSeatID := booking.BookingID + "-seat"
	_, err = tx.Exec(ctx, insertSeatQuery,
		bookingSeatID,
		booking.BookingID,
		seatID,
		booking.Status,
		priceCents,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateActive
		}
		return fmt.Errorf("insert booking seat: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *BookingRepository) GetBookingForUpdate(ctx context.Context, tx pgx.Tx, bookingID string) (*domain.Booking, error) {
	query := `
		SELECT booking_id, user_id, event_id, status, total_amount_cents, expires_at, version, created_at, updated_at 
		FROM bookings 
		WHERE booking_id = $1 
		FOR UPDATE;`

	var b domain.Booking
	var status string
	err := tx.QueryRow(ctx, query, bookingID).Scan(
		&b.BookingID,
		&b.UserID,
		&b.EventID,
		&status,
		&b.TotalAmountCents,
		&b.ExpiresAt,
		&b.Version,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("get booking: %w", err)
	}
	b.Status = domain.BookingStatus(status)
	return &b, nil
}

func (r *BookingRepository) UpdateBookingStatus(ctx context.Context, tx pgx.Tx, bookingID string, newStatus domain.BookingStatus, expectedVersion int) error {
	query := `
		UPDATE bookings 
		SET status = $1, version = version + 1, updated_at = CURRENT_TIMESTAMP 
		WHERE booking_id = $2 AND version = $3;`

	cmdTag, err := tx.Exec(ctx, query, string(newStatus), bookingID, expectedVersion)
	if err != nil {
		return fmt.Errorf("update booking status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domain.ErrInvalidStateTransition
	}

	syncSeatsQuery := `
		UPDATE booking_seats 
		SET booking_status = $1 
		WHERE booking_id = $2;`
	_, err = tx.Exec(ctx, syncSeatsQuery, string(newStatus), bookingID)
	if err != nil {
		return fmt.Errorf("sync booking seats: %w", err)
	}

	return nil
}

func (r *BookingRepository) ReleaseExpiredSeats(ctx context.Context, batchSize int) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin sweep tx: %w", err)
	}
	defer tx.Rollback(ctx)

	findExpiredQuery := `
		SELECT booking_id 
		FROM bookings 
		WHERE status = 'HELD' AND expires_at < clock_timestamp() 
		ORDER BY expires_at ASC 
		LIMIT $1 
		FOR UPDATE SKIP LOCKED;`

	rows, err := tx.Query(ctx, findExpiredQuery, batchSize)
	if err != nil {
		return 0, fmt.Errorf("find expired: %w", err)
	}
	defer rows.Close()

	var expiredIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		expiredIDs = append(expiredIDs, id)
	}

	if len(expiredIDs) == 0 {
		return 0, nil
	}

	for _, id := range expiredIDs {
		_, err = tx.Exec(ctx, `
			UPDATE event_seats 
			SET status = 'AVAILABLE', version = version + 1, updated_at = CURRENT_TIMESTAMP 
			WHERE event_seat_id IN (
				SELECT event_seat_id FROM booking_seats WHERE booking_id = $1
			);`, id)
		if err != nil {
			return 0, fmt.Errorf("release event seats for %s: %w", id, err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE booking_seats 
			SET booking_status = 'EXPIRED' 
			WHERE booking_id = $1;`, id)
		if err != nil {
			return 0, fmt.Errorf("update booking_seats for %s: %w", id, err)
		}

		_, err = tx.Exec(ctx, `
			UPDATE bookings 
			SET status = 'EXPIRED', version = version + 1, updated_at = CURRENT_TIMESTAMP 
			WHERE booking_id = $1;`, id)
		if err != nil {
			return 0, fmt.Errorf("expire booking %s: %w", id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit sweep: %w", err)
	}

	return int64(len(expiredIDs)), nil
}
