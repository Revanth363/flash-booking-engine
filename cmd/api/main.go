package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flash-booking-engine/application_2/api"
	"flash-booking-engine/application_2/repository"
	"flash-booking-engine/application_2/services"
	"flash-booking-engine/background_6"
	"flash-booking-engine/database_3"
	"flash-booking-engine/payment_5"

	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := godotenv.Load(".env"); err != nil {
		logger.Warn("no .env file found, reading from system env")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Error("DATABASE_URL is not set")
		os.Exit(1)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "fbe_super_secure_production_secret_key_2026"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbConfig := postgres.Config{
		URL:         dbURL,
		MaxConns:    10,
		MinConns:    2,
		MaxConnIdle: 5 * time.Minute,
		MaxConnLife: 30 * time.Minute,
	}

	pool, err := postgres.NewPool(ctx, dbConfig)
	if err != nil {
		logger.Error("failed to connect to postgres pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to PostgreSQL connection pool successfully")

	userRepo := repository.NewUserRepository(pool)
	authService := services.NewAuthService(userRepo, jwtSecret, 24*time.Hour)
	authHandler := api.NewAuthHandler(authService)

	bookingRepo := repository.NewBookingRepository(pool)
	bookingService := services.NewBookingService(pool, bookingRepo, 10)
	bookingHandler := api.NewBookingHandler(bookingService, logger, 8)

	mockGateway := payment.NewMockGateway(payment.ModeSuccess)
	idempMgr := payment.NewIdempotencyManager(pool)
	paymentService := payment.NewPaymentService(pool, bookingRepo, mockGateway, idempMgr)
	paymentHandler := payment.NewPaymentHandler(paymentService, logger)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	expWorker := background.NewExpirationWorker(bookingRepo, logger, 5*time.Second, 20)
	go expWorker.Start(workerCtx)

	reconWorker := background.NewReconciliationWorker(pool, bookingRepo, mockGateway, logger, 10*time.Second)
	go reconWorker.Start(workerCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", bookingHandler.HealthCheck)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	authMiddleware := api.AuthMiddleware(authService)
	mux.Handle("POST /api/v1/bookings/hold", authMiddleware(http.HandlerFunc(bookingHandler.ReserveSeat)))
	mux.Handle("POST /api/v1/payments/charge", authMiddleware(http.HandlerFunc(paymentHandler.ProcessPayment)))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 4 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server gracefully...")
	workerCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}
	logger.Info("server and workers exited cleanly")
}
