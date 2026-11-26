package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
		log.Fatal("DATABASE_URL is not set in environment")
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

	fmt.Println("Successfully connected to Neon PostgreSQL!")

	schemaFiles := []string{
		"users.sql",
		"refresh_tokens.sql",
		"events.sql",
		"event_ticket_categories.sql",
		"event_seats.sql",
		"ga_inventory.sql",
		"bookings.sql",
		"booking_seats.sql",
		"booking_items.sql",
		"idempotency_keys.sql",
		"payments.sql",
		"refunds.sql",
	}

	schemaDir := filepath.Join("database_3", "schemas")

	for _, file := range schemaFiles {
		filePath := filepath.Join(schemaDir, file)
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Failed to read %s: %v", file, err)
		}

		fmt.Printf("Applying %s... ", file)
		execCtx, execCancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err = pool.Exec(execCtx, string(content))
		execCancel()
		if err != nil {
			log.Fatalf("\nError applying %s: %v", file, err)
		}
		fmt.Println("OK")
	}

	fmt.Println("\nAll 12 tables and constraints applied successfully to Neon!")
}
