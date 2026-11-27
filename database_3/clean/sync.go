package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"flash-booking-engine/database_3"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, postgres.Config{URL: os.Getenv("DATABASE_URL")})
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	seatID := "018f9e61-7e32-7230-bc2d-56543b570101"
	var seatStatus string
	_ = pool.QueryRow(ctx, "SELECT status FROM event_seats WHERE event_seat_id = $1;", seatID).Scan(&seatStatus)

	var activeAllocations int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM booking_seats WHERE event_seat_id = $1 AND booking_status IN ('HELD', 'PAID', 'CONFIRMED');", seatID).Scan(&activeAllocations)

	fmt.Printf("Seat A1 event_seats status: %s | Active in booking_seats: %d\n", seatStatus, activeAllocations)

	// Clean sync: update all booking_seats whose bookings are EXPIRED to EXPIRED
	_, _ = pool.Exec(ctx, `
		UPDATE booking_seats 
		SET booking_status = 'EXPIRED' 
		WHERE booking_id IN (SELECT booking_id FROM bookings WHERE status IN ('EXPIRED', 'REFUND_PENDING', 'REFUNDED'));
		
		UPDATE event_seats 
		SET status = 'AVAILABLE' 
		WHERE event_seat_id NOT IN (
			SELECT event_seat_id FROM booking_seats WHERE booking_status IN ('HELD', 'PAID', 'CONFIRMED')
		);
	`)

	fmt.Println("Cleaned and synchronized booking_seats and event_seats!")
}
