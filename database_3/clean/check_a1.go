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
	var status string
	var ver int
	err = pool.QueryRow(ctx, "SELECT status, version FROM event_seats WHERE event_seat_id = $1;", seatID).Scan(&status, &ver)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("CURRENT DB STATUS FOR SEAT A1: status='%s', version=%d\n", status, ver)

	var bkgSeatsCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM booking_seats WHERE event_seat_id = $1 AND booking_status IN ('HELD', 'PAID', 'CONFIRMED');", seatID).Scan(&bkgSeatsCount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("CURRENT ACTIVE BOOKING_SEATS COUNT FOR A1: %d\n", bkgSeatsCount)
}
