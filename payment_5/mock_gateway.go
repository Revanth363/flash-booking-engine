package payment

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MockGatewayMode string

const (
	ModeSuccess MockGatewayMode = "SUCCESS"
	ModeFailure MockGatewayMode = "FAILURE"
	ModeTimeout MockGatewayMode = "TIMEOUT"
)

type MockGateway struct {
	mu           sync.Mutex
	mode         MockGatewayMode
	transactions map[string]string
}

func NewMockGateway(mode MockGatewayMode) *MockGateway {
	return &MockGateway{
		mode:         mode,
		transactions: make(map[string]string),
	}
}

func (m *MockGateway) SetMode(mode MockGatewayMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode
}

func (m *MockGateway) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	m.mu.Lock()
	mode := m.mode
	m.mu.Unlock()

	switch mode {
	case ModeTimeout:
		select {
		case <-time.After(2 * time.Second):
			return nil, ErrGatewayTimeout
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case ModeFailure:
		return nil, ErrPaymentFailed
	case ModeSuccess:
		fallthrough
	default:
		txID := fmt.Sprintf("gtx_%d", time.Now().UnixNano())
		m.mu.Lock()
		m.transactions[txID] = "SUCCESS"
		m.mu.Unlock()

		return &ChargeResponse{
			GatewayTransactionID: txID,
			Status:               "SUCCESS",
			ProcessedAt:          time.Now(),
		}, nil
	}
}

func (m *MockGateway) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	refID := fmt.Sprintf("rfnd_%d", time.Now().UnixNano())
	m.transactions[refID] = "REFUNDED"

	return &RefundResponse{
		GatewayRefundID: refID,
		Status:          "REFUNDED",
		ProcessedAt:     time.Now(),
	}, nil
}

func (m *MockGateway) Inquire(ctx context.Context, gatewayTransactionID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, exists := m.transactions[gatewayTransactionID]
	if !exists {
		return "UNKNOWN", nil
	}
	return status, nil
}
