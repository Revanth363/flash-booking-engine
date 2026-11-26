package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"
	"flash-booking-engine/application_2/services"
)

type BookingHandler struct {
	service   *services.BookingService
	logger    *slog.Logger
	semaphore chan struct{}
}

func NewBookingHandler(service *services.BookingService, logger *slog.Logger, maxConcurrentSlots int) *BookingHandler {
	return &BookingHandler{
		service:   service,
		logger:    logger,
		semaphore: make(chan struct{}, maxConcurrentSlots),
	}
}

type ReserveSeatRequest struct {
	UserID     string `json:"user_id"`
	EventID    string `json:"event_id"`
	SeatID     string `json:"seat_id"`
	PriceCents int64  `json:"price_cents"`
}

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (h *BookingHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "service healthy"})
}

func (h *BookingHandler) ReserveSeat(w http.ResponseWriter, r *http.Request) {
	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()
	default:
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "admission gate full: high server load, retry shortly",
		})
		return
	}

	var req ReserveSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "invalid request payload"})
		return
	}

	userClaims, ok := GetAuthenticatedUser(r.Context())
	if !ok || userClaims == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "unauthorized"})
		return
	}
	req.UserID = userClaims.UserID

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(4 * time.Second))

	booking, err := h.service.ReserveSeat(r.Context(), req.UserID, req.EventID, req.SeatID, req.PriceCents)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case errors.Is(err, repository.ErrLockNotAvailable):
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "seat currently locked by another customer"})
		case errors.Is(err, domain.ErrSeatUnavailable), errors.Is(err, repository.ErrDuplicateActive):
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "seat is already held or booked"})
		case errors.Is(err, services.ErrSaleNotStarted):
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "ticket sale has not started yet"})
		case errors.Is(err, services.ErrMaxHoldReached):
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "user quota exceeded: active reservation exists"})
		default:
			h.logger.Error("failed to reserve seat", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "internal reservation error"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "seat reserved successfully",
		Data:    booking,
	})
}
