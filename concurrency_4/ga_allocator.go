package concurrency

import (
	"context"
	"errors"
	"fmt"

	"flash-booking-engine/application_2/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidQuantity = errors.New("requested quantity must be greater than zero")
	ErrPoolSoldOut     = errors.New("requested quantity exceeds available general admission capacity")
)

type GAAllocator struct {
	pool *pgxpool.Pool
}

func NewGAAllocator(pool *pgxpool.Pool) *GAAllocator {
	return &GAAllocator{pool: pool}
}

func (a *GAAllocator) HoldGATickets(ctx context.Context, booking *domain.Booking, categoryID string, quantity int, unitPriceCents int64) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ga tx: %w", err)
	}
	defer tx.Rollback(ctx)

	updateQuery := `
		UPDATE ga_inventory 
		SET reserved_count = reserved_count + $1,
		    version = version + 1,
		    updated_at = CURRENT_TIMESTAMP 
		WHERE category_id = $2 
		  AND (total_capacity - reserved_count - confirmed_count) >= $1;`

	cmdTag, err := tx.Exec(ctx, updateQuery, quantity, categoryID)
	if err != nil {
		return fmt.Errorf("atomic ga hold update: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrPoolSoldOut
	}

	subtotal := int64(quantity) * unitPriceCents
	booking.TotalAmountCents = subtotal

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
		return fmt.Errorf("insert ga booking: %w", err)
	}

	insertItemQuery := `
		INSERT INTO booking_items (booking_item_id, booking_id, category_id, quantity, unit_price_cents, subtotal_cents) 
		VALUES ($1, $2, $3, $4, $5, $6);`

	bookingItemID := fmt.Sprintf("%s-item", booking.BookingID)
	_, err = tx.Exec(ctx, insertItemQuery,
		bookingItemID,
		booking.BookingID,
		categoryID,
		quantity,
		unitPriceCents,
		subtotal,
	)
	if err != nil {
		return fmt.Errorf("insert ga booking item: %w", err)
	}

	return tx.Commit(ctx)
}

func (a *GAAllocator) ConfirmGATickets(ctx context.Context, tx pgx.Tx, categoryID string, quantity int) error {
	confirmQuery := `
		UPDATE ga_inventory 
		SET reserved_count = reserved_count - $1,
		    confirmed_count = confirmed_count + $1,
		    version = version + 1,
		    updated_at = CURRENT_TIMESTAMP 
		WHERE category_id = $2 AND reserved_count >= $1;`

	cmdTag, err := tx.Exec(ctx, confirmQuery, quantity, categoryID)
	if err != nil {
		return fmt.Errorf("confirm ga inventory: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.New("cannot confirm: insufficient reserved count in pool")
	}
	return nil
}

func (a *GAAllocator) ReleaseExpiredGATickets(ctx context.Context, tx pgx.Tx, categoryID string, quantity int) error {
	releaseQuery := `
		UPDATE ga_inventory 
		SET reserved_count = reserved_count - $1,
		    version = version + 1,
		    updated_at = CURRENT_TIMESTAMP 
		WHERE category_id = $2 AND reserved_count >= $1;`

	cmdTag, err := tx.Exec(ctx, releaseQuery, quantity, categoryID)
	if err != nil {
		return fmt.Errorf("release expired ga inventory: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.New("cannot release: insufficient reserved count in pool")
	}
	return nil
}
