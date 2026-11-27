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

	cfg := postgres.Config{
		URL:         dbURL,
		MaxConns:    5,
		MinConns:    1,
		MaxConnIdle: 5 * time.Minute,
		MaxConnLife: 30 * time.Minute,
	}

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	fmt.Println("Connected to Neon PostgreSQL for seeding...")

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	eventID := "018f9e61-7e32-7230-bc2d-56543b578c01"
	eventTitle := "Coldplay Live: Music of the Spheres 2026"
	venueName := "Narendra Modi Stadium, Ahmedabad"
	bookingStartTime := time.Now().Add(-1 * time.Hour)
	eventStartTime := time.Now().Add(48 * time.Hour)
	eventEndTime := time.Now().Add(52 * time.Hour)

	insertEventQuery := `
		INSERT INTO events (event_id, title, venue_name, booking_start_time, event_start_time, event_end_time, status, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6, 'LIVE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (event_id) DO UPDATE 
		SET status = 'LIVE', booking_start_time = EXCLUDED.booking_start_time;`

	_, err = tx.Exec(ctx, insertEventQuery, eventID, eventTitle, venueName, bookingStartTime, eventStartTime, eventEndTime)
	if err != nil {
		log.Fatalf("Failed to insert event: %v", err)
	}
	fmt.Printf("Inserted Event: %s (ID: %s)\n", eventTitle, eventID)

	vipCategoryID := "018f9e61-7e32-7230-bc2d-56543b578c02"
	insertCategoryVIP := `
		INSERT INTO event_ticket_categories (category_id, event_id, name, inventory_type, price_cents, created_at) 
		VALUES ($1, $2, 'VIP', 'RESERVED_SEAT', 1500000, CURRENT_TIMESTAMP)
		ON CONFLICT (event_id, name) DO NOTHING;`

	_, err = tx.Exec(ctx, insertCategoryVIP, vipCategoryID, eventID)
	if err != nil {
		log.Fatalf("Failed to insert VIP category: %v", err)
	}

	gaCategoryID := "018f9e61-7e32-7230-bc2d-56543b578c03"
	insertCategoryGA := `
		INSERT INTO event_ticket_categories (category_id, event_id, name, inventory_type, price_cents, created_at) 
		VALUES ($1, $2, 'STANDING', 'GENERAL_ADMISSION', 450000, CURRENT_TIMESTAMP)
		ON CONFLICT (event_id, name) DO NOTHING;`

	_, err = tx.Exec(ctx, insertCategoryGA, gaCategoryID, eventID)
	if err != nil {
		log.Fatalf("Failed to insert GA category: %v", err)
	}
	fmt.Println("Inserted Categories: VIP (Reserved) & STANDING (General Admission)")

	rows := []string{"A", "B", "C", "D", "E", "F"}
	seatsPerRow := 10
	totalSeats := 0

	for rIdx, row := range rows {
		for num := 1; num <= seatsPerRow; num++ {
			seatLabel := fmt.Sprintf("%s%d", row, num)
			seatID := fmt.Sprintf("018f9e61-7e32-7230-bc2d-56543b57%02d%02d", rIdx+1, num)

			insertSeatQuery := `
				INSERT INTO event_seats (event_seat_id, event_id, category_id, seat_label, row_label, seat_number, status, version, updated_at) 
				VALUES ($1, $2, $3, $4, $5, $6, 'AVAILABLE', 1, CURRENT_TIMESTAMP)
				ON CONFLICT (event_id, seat_label) DO UPDATE 
				SET status = 'AVAILABLE', version = 1;`

			_, err = tx.Exec(ctx, insertSeatQuery, seatID, eventID, vipCategoryID, seatLabel, row, num)
			if err != nil {
				log.Fatalf("Failed to insert seat %s: %v", seatLabel, err)
			}
			totalSeats++
		}
	}
	fmt.Printf("Inserted %d Reserved VIP Seats (Rows A1 through F10)\n", totalSeats)

	gaInventoryID := "018f9e61-7e32-7230-bc2d-56543b578c04"
	insertGAInventory := `
		INSERT INTO ga_inventory (inventory_id, category_id, total_capacity, reserved_count, confirmed_count, version, updated_at) 
		VALUES ($1, $2, 500, 0, 0, 1, CURRENT_TIMESTAMP)
		ON CONFLICT (category_id) DO UPDATE 
		SET total_capacity = 500, reserved_count = 0, confirmed_count = 0, version = 1;`

	_, err = tx.Exec(ctx, insertGAInventory, gaInventoryID, gaCategoryID)
	if err != nil {
		log.Fatalf("Failed to insert GA inventory: %v", err)
	}
	fmt.Println("Inserted GA Inventory: 500 Standing capacity pool")

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("Failed to commit seed transaction: %v", err)
	}

	fmt.Println("\nSeeding completed successfully! Refresh DBeaver to inspect the rows.")
}
