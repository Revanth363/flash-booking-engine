package background

import (
	"context"
	"log/slog"
	"time"

	"flash-booking-engine/application_2/repository"
)

type ExpirationWorker struct {
	bookingRepo *repository.BookingRepository
	logger      *slog.Logger
	interval    time.Duration
	batchSize   int
}

func NewExpirationWorker(repo *repository.BookingRepository, logger *slog.Logger, interval time.Duration, batchSize int) *ExpirationWorker {
	return &ExpirationWorker{
		bookingRepo: repo,
		logger:      logger,
		interval:    interval,
		batchSize:   batchSize,
	}
}

func (w *ExpirationWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("expiration worker started", "interval", w.interval.String(), "batch_size", w.batchSize)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("expiration worker shutting down gracefully")
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *ExpirationWorker) sweep(ctx context.Context) {
	sweepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	releasedCount, err := w.bookingRepo.ReleaseExpiredSeats(sweepCtx, w.batchSize)
	if err != nil {
		w.logger.Error("failed to release expired seats", "error", err)
		return
	}

	if releasedCount > 0 {
		w.logger.Info("released expired reservations safely", "released_count", releasedCount)
	}
}
