package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSaleNotStarted = errors.New("ticket sales for this event have not started yet")
	ErrMaxHoldReached = errors.New("user already has an active held reservation")
)

type BookingService struct {
	pool        *pgxpool.Pool
	bookingRepo *repository.BookingRepository
	holdMinutes int
}

func NewBookingService(pool *pgxpool.Pool, repo *repository.BookingRepository, holdMinutes int) *BookingService {
	return &BookingService{
		pool:        pool,
		bookingRepo: repo,
		holdMinutes: holdMinutes,
	}
}

func (s *BookingService) ReserveSeat(ctx context.Context, userID, eventID, seatID string, priceCents int64) (*domain.Booking, error) {
	var bookingStartTime time.Time
	var eventStatus string
	eventQuery := `SELECT booking_start_time, status FROM events WHERE event_id = $1;`
	err := s.pool.QueryRow(ctx, eventQuery, eventID).Scan(&bookingStartTime, &eventStatus)
	if err != nil {
		return nil, fmt.Errorf("check event status: %w", err)
	}

	if eventStatus != "LIVE" && time.Now().Before(bookingStartTime) {
		return nil, ErrSaleNotStarted
	}

	var activeHoldCount int
	holdCheckQuery := `SELECT count(*) FROM bookings WHERE user_id = $1 AND status = 'HELD' AND expires_at > clock_timestamp();`
	err = s.pool.QueryRow(ctx, holdCheckQuery, userID).Scan(&activeHoldCount)
	if err != nil {
		return nil, fmt.Errorf("check active holds: %w", err)
	}
	if activeHoldCount >= 1 {
		return nil, ErrMaxHoldReached
	}

	now := time.Now()
	booking := &domain.Booking{
		BookingID:        uuid.NewString(),
		UserID:           userID,
		EventID:          eventID,
		Status:           domain.StatusHeld,
		TotalAmountCents: priceCents,
		ExpiresAt:        now.Add(time.Duration(s.holdMinutes) * time.Minute),
		Version:          1,
	}

	err = s.bookingRepo.HoldSeat(ctx, booking, seatID, priceCents)
	if err != nil {
		return nil, err
	}

	return booking, nil
}
