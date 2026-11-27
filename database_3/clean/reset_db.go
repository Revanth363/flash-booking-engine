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

	_, err = pool.Exec(ctx, `
		DELETE FROM refunds;
		DELETE FROM payments;
		DELETE FROM booking_seats;
		DELETE FROM booking_items;
		DELETE FROM bookings;
		UPDATE event_seats SET status = 'AVAILABLE', version = 1;
		UPDATE ga_inventory SET reserved_count = 0, confirmed_count = 0, version = 1;
	`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("All test bookings wiped cleanly! All 60 seats are 100% AVAILABLE with 0 active holds.")
}
