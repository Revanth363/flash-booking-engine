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

	cmdTag, err := pool.Exec(ctx, `
		UPDATE bookings 
		SET status = 'EXPIRED', updated_at = CURRENT_TIMESTAMP 
		WHERE status = 'HELD';
	`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Expired %d lingering HELD bookings successfully!\n", cmdTag.RowsAffected())
}
