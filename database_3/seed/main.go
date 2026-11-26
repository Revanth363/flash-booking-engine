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
	if err := godotenv.Load(".env"); err != nil {
		log.Println("No .env file found, using system environment")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, postgres.Config{URL: dbURL, MaxConns: 5})
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	fmt.Println("Seeding test data into Neon...")

	userID := "01936c50-0001-7000-8000-000000000001"
	_, err = pool.Exec(ctx, `
		INSERT INTO users (user_id, name, email, password_hash, phone, status) 
		VALUES ($1, 'John Doe', 'john@example.com', 'hashed_pwd_placeholder', '+919999999999', 'ACTIVE')
		ON CONFLICT (email) DO NOTHING;`, userID)
	if err != nil {
		log.Fatalf("seed user: %v", err)
	}

	eventID := "01936c50-0002-7000-8000-000000000002"
	now := time.Now()
	_, err = pool.Exec(ctx, `
		INSERT INTO events (event_id, title, venue_name, booking_start_time, event_start_time, event_end_time, status)
		VALUES ($1, 'Coldplay Live VIP Arena', 'Ahmedabad Stadium', $2, $3, $4, 'LIVE')
		ON CONFLICT (event_id) DO NOTHING;`,
		eventID, now.Add(-1*time.Hour), now.Add(24*time.Hour), now.Add(28*time.Hour),
	)
	if err != nil {
		log.Fatalf("seed event: %v", err)
	}

	categoryID := "01936c50-0003-7000-8000-000000000003"
	_, err = pool.Exec(ctx, `
		INSERT INTO event_ticket_categories (category_id, event_id, name, inventory_type, price_cents)
		VALUES ($1, $2, 'VIP', 'RESERVED_SEAT', 150000)
		ON CONFLICT (event_id, category_id) DO NOTHING;`,
		categoryID, eventID,
	)
	if err != nil {
		log.Fatalf("seed category: %v", err)
	}

	rows := []string{"A", "B", "C", "D", "E", "F"}
	seatCount := 0
	for _, r := range rows {
		for s := 1; s <= 10; s++ {
			seatCount++
			seatLabel := fmt.Sprintf("%s%d", r, s)
			seatID := fmt.Sprintf("01936c50-0004-7000-8000-%012d", seatCount)

			_, err = pool.Exec(ctx, `
				INSERT INTO event_seats (event_seat_id, event_id, category_id, seat_label, row_label, seat_number, status, version)
				VALUES ($1, $2, $3, $4, $5, $6, 'AVAILABLE', 1)
				ON CONFLICT (event_id, seat_label) DO NOTHING;`,
				seatID, eventID, categoryID, seatLabel, r, s,
			)
			if err != nil {
				log.Fatalf("seed seat %s: %v", seatLabel, err)
			}
		}
	}

	fmt.Printf("Seeded 1 User, 1 Event ('Coldplay Live VIP Arena'), 1 Category ('VIP'), and exactly %d seats (A1 to F10)!\n", seatCount)
	fmt.Printf("\nEvent ID: %s\nUser ID: %s\nSeat A1 ID: 01936c50-0004-7000-8000-000000000001\n", eventID, userID)
}
