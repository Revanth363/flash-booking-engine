package payment

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"flash-booking-engine/application_2/services"
)

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PaymentHandler struct {
	paymentService *PaymentService
	logger         *slog.Logger
}

func NewPaymentHandler(paymentService *PaymentService, logger *slog.Logger) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		logger:         logger,
	}
}

type ChargePaymentRequest struct {
	BookingID     string `json:"booking_id"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	PaymentMethod string `json:"payment_method"`
}

func (h *PaymentHandler) ProcessPayment(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(services.UserContextKey).(*services.UserClaims)
	if !ok || userClaims == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "unauthorized"})
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "missing Idempotency-Key header"})
		return
	}

	var req ChargePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "invalid request payload"})
		return
	}

	if req.Currency == "" {
		req.Currency = "INR"
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "UPI"
	}

	result, err := h.paymentService.ProcessPayment(r.Context(), ChargeRequest{
		BookingID:      req.BookingID,
		UserID:         userClaims.UserID,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		IdempotencyKey: idempotencyKey,
		PaymentMethod:  req.PaymentMethod,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case errors.Is(err, ErrGatewayTimeout):
			w.WriteHeader(http.StatusGatewayTimeout)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "payment in-doubt: gateway timed out, background reconciliation scheduled"})
		case errors.Is(err, ErrPaymentFailed):
			w.WriteHeader(http.StatusPaymentRequired)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "payment declined by bank"})
		default:
			h.logger.Error("payment processing failed", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "internal payment processing error"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "payment processed successfully",
		Data:    result,
	})
}
