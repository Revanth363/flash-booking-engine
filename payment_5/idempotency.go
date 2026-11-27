package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IdempotencyManager struct {
	pool *pgxpool.Pool
}

func NewIdempotencyManager(pool *pgxpool.Pool) *IdempotencyManager {
	return &IdempotencyManager{pool: pool}
}

func HashPayload(payload []byte) string {
	h := sha256.New()
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

type IdempotencyRecord struct {
	Status       string
	ResponseCode int
	ResponseBody string
	BookingID    string
}

func (m *IdempotencyManager) AcquireOrGet(ctx context.Context, userID, key, path, reqHash string, ttl time.Duration) (*IdempotencyRecord, error) {
	var record IdempotencyRecord
	var storedHash string

	queryExisting := `
		SELECT status, request_hash, COALESCE(response_code, 0), COALESCE(response_body, ''), COALESCE(booking_id::text, '') 
		FROM idempotency_keys 
		WHERE user_id = $1 AND idempotency_key = $2 AND expires_at > CURRENT_TIMESTAMP;`

	err := m.pool.QueryRow(ctx, queryExisting, userID, key).Scan(
		&record.Status,
		&storedHash,
		&record.ResponseCode,
		&record.ResponseBody,
		&record.BookingID,
	)

	if err == nil {
		if storedHash != reqHash {
			return nil, errors.New("idempotency key reused with mismatched payload")
		}
		if record.Status == "IN_PROGRESS" {
			return nil, ErrIdempotencyConflict
		}
		return &record, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check idempotency key: %w", err)
	}

	idempotencyID := uuid.NewString()
	insertQuery := `
		INSERT INTO idempotency_keys (idempotency_id, user_id, idempotency_key, request_path, request_hash, status, created_at, expires_at) 
		VALUES ($1, $2, $3, $4, $5, 'IN_PROGRESS', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + $6::interval);`

	intervalStr := fmt.Sprintf("%d seconds", int(ttl.Seconds()))
	_, err = m.pool.Exec(ctx, insertQuery, idempotencyID, userID, key, path, reqHash, intervalStr)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrIdempotencyConflict
		}
		return nil, fmt.Errorf("insert in_progress idempotency key: %w", err)
	}

	return nil, nil
}

func (m *IdempotencyManager) Complete(ctx context.Context, userID, key string, responseCode int, responseBody string, bookingID string) error {
	updateQuery := `
		UPDATE idempotency_keys 
		SET status = 'COMPLETED', response_code = $1, response_body = $2, booking_id = $3 
		WHERE user_id = $4 AND idempotency_key = $5;`

	_, err := m.pool.Exec(ctx, updateQuery, responseCode, responseBody, bookingID, userID, key)
	if err != nil {
		return fmt.Errorf("complete idempotency record: %w", err)
	}
	return nil
}
