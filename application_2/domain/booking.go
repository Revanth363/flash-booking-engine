package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidStateTransition = errors.New("invalid booking state transition")
	ErrBookingExpired         = errors.New("booking has expired")
	ErrSeatUnavailable        = errors.New("seat is not available for booking")
	ErrCapacityExhausted      = errors.New("general admission capacity is fully exhausted")
	ErrUnauthorizedActor      = errors.New("actor is not authorized for this state transition")
)

type BookingStatus string

const (
	StatusHeld           BookingStatus = "HELD"
	StatusPaid           BookingStatus = "PAID"
	StatusConfirmed      BookingStatus = "CONFIRMED"
	StatusExpired        BookingStatus = "EXPIRED"
	StatusCancelled      BookingStatus = "CANCELLED"
	StatusRefundPending  BookingStatus = "REFUND_PENDING"
	StatusRefunded       BookingStatus = "REFUNDED"
)

type SeatStatus string

const (
	SeatAvailable SeatStatus = "AVAILABLE"
	SeatHeld      SeatStatus = "HELD"
	SeatBooked    SeatStatus = "BOOKED"
)

type User struct {
	UserID       string
	Name         string
	Email        string
	PasswordHash string
	Phone        string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Event struct {
	EventID          string
	Title            string
	VenueName        string
	BookingStartTime time.Time
	EventStartTime   time.Time
	EventEndTime     time.Time
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type EventSeat struct {
	EventSeatID string
	EventID     string
	CategoryID  string
	SeatLabel   string
	RowLabel    string
	SeatNumber  int
	Status      SeatStatus
	Version     int
	UpdatedAt   time.Time
}

type Booking struct {
	BookingID        string
	UserID           string
	EventID          string
	Status           BookingStatus
	TotalAmountCents int64
	ExpiresAt        time.Time
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type BookingSeat struct {
	BookingSeatID string
	BookingID     string
	EventSeatID   string
	BookingStatus BookingStatus
	PriceCents    int64
	CreatedAt     time.Time
}

type GAInventory struct {
	InventoryID    string
	CategoryID     string
	TotalCapacity  int
	ReservedCount  int
	ConfirmedCount int
	Version        int
	UpdatedAt      time.Time
}

func (b *Booking) CanTransitionTo(next BookingStatus) bool {
	switch b.Status {
	case StatusHeld:
		return next == StatusPaid || next == StatusExpired || next == StatusCancelled
	case StatusPaid:
		return next == StatusConfirmed || next == StatusRefundPending
	case StatusConfirmed:
		return next == StatusRefundPending
	case StatusRefundPending:
		return next == StatusRefunded
	default:
		return false
	}
}

func (b *Booking) Transition(next BookingStatus) error {
	if !b.CanTransitionTo(next) {
		return ErrInvalidStateTransition
	}
	b.Status = next
	b.Version++
	return nil
}

func (b *Booking) IsExpiredAt(currentTime time.Time) bool {
	return b.Status == StatusHeld && currentTime.After(b.ExpiresAt)
}
