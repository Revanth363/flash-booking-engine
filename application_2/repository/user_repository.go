package repository

import (
	"context"
	"errors"
	"fmt"

	"flash-booking-engine/application_2/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserAlreadyExists = errors.New("user with this email already exists")
	ErrUserNotFound      = errors.New("user not found")
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (user_id, name, email, password_hash, phone, status, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`

	_, err := r.pool.Exec(ctx, query,
		user.UserID,
		user.Name,
		user.Email,
		user.PasswordHash,
		user.Phone,
		user.Status,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT user_id, name, email, password_hash, phone, status, created_at, updated_at 
		FROM users 
		WHERE email = $1;`

	var u domain.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.UserID,
		&u.Name,
		&u.Email,
		&u.PasswordHash,
		&u.Phone,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) StoreRefreshToken(ctx context.Context, tokenID, userID, tokenHash string, expiresAt any) error {
	query := `
		INSERT INTO refresh_tokens (token_id, user_id, token_hash, expires_at, created_at) 
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP);`

	_, err := r.pool.Exec(ctx, query, tokenID, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}
