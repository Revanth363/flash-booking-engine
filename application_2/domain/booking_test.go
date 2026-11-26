package domain_test

import (
	"testing"
	"time"

	"flash-booking-engine/application_2/domain"
)

func TestBookingStateTransitions(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus domain.BookingStatus
		targetStatus  domain.BookingStatus
		shouldSucceed bool
	}{
		{"Held to Paid should succeed", domain.StatusHeld, domain.StatusPaid, true},
		{"Held to Expired should succeed", domain.StatusHeld, domain.StatusExpired, true},
		{"Held to Cancelled should succeed", domain.StatusHeld, domain.StatusCancelled, true},
		{"Held directly to Confirmed should fail", domain.StatusHeld, domain.StatusConfirmed, false},
		{"Held directly to Refunded should fail", domain.StatusHeld, domain.StatusRefunded, false},
		{"Paid to Confirmed should succeed", domain.StatusPaid, domain.StatusConfirmed, true},
		{"Paid to RefundPending should succeed", domain.StatusPaid, domain.StatusRefundPending, true},
		{"Confirmed to RefundPending should succeed", domain.StatusConfirmed, domain.StatusRefundPending, true},
		{"RefundPending to Refunded should succeed", domain.StatusRefundPending, domain.StatusRefunded, true},
		{"Expired to Paid should fail", domain.StatusExpired, domain.StatusPaid, false},
		{"Cancelled to Paid should fail", domain.StatusCancelled, domain.StatusPaid, false},
		{"Refunded to any status should fail", domain.StatusRefunded, domain.StatusConfirmed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &domain.Booking{
				BookingID: "test-booking-id",
				Status:    tt.initialStatus,
				Version:   1,
			}

			err := b.Transition(tt.targetStatus)
			if tt.shouldSucceed && err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Fatalf("expected failure from %s to %s, but transition succeeded", tt.initialStatus, tt.targetStatus)
			}
			if tt.shouldSucceed && b.Version != 2 {
				t.Fatalf("expected version increment to 2, got %d", b.Version)
			}
		})
	}
}

func TestBookingExpirationCheck(t *testing.T) {
	now := time.Now()
	expiredBooking := &domain.Booking{
		Status:    domain.StatusHeld,
		ExpiresAt: now.Add(-1 * time.Minute),
	}

	if !expiredBooking.IsExpiredAt(now) {
		t.Fatal("expected booking to be identified as expired")
	}

	activeBooking := &domain.Booking{
		Status:    domain.StatusHeld,
		ExpiresAt: now.Add(9 * time.Minute),
	}

	if activeBooking.IsExpiredAt(now) {
		t.Fatal("expected booking to be active, not expired")
	}
}
