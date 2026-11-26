package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"flash-booking-engine/application_2/domain"
	"flash-booking-engine/application_2/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserBlocked        = errors.New("user account is blocked")
)

type AuthService struct {
	userRepo  *repository.UserRepository
	jwtSecret []byte
	accessTTL time.Duration
}

func NewAuthService(repo *repository.UserRepository, jwtSecret string, accessTTL time.Duration) *AuthService {
	return &AuthService{
		userRepo:  repo,
		jwtSecret: []byte(jwtSecret),
		accessTTL: accessTTL,
	}
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type UserClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func (s *AuthService) Register(ctx context.Context, name, email, password, phone string) (*domain.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := fmt.Sprintf("usr-%d", time.Now().UnixNano())
	user := &domain.User{
		UserID:       userID,
		Name:         name,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Phone:        phone,
		Status:       "ACTIVE",
	}

	err = s.userRepo.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, *domain.User, error) {
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	if user.Status != "ACTIVE" {
		return nil, nil, ErrUserBlocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	now := time.Now()
	expiresAt := now.Add(s.accessTTL)
	claims := UserClaims{
		UserID: user.UserID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   user.UserID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("sign access token: %w", err)
	}

	rawRefreshToken := make([]byte, 32)
	_, _ = rand.Read(rawRefreshToken)
	refreshTokenStr := hex.EncodeToString(rawRefreshToken)

	h := sha256.New()
	h.Write([]byte(refreshTokenStr))
	tokenHash := hex.EncodeToString(h.Sum(nil))

	tokenID := fmt.Sprintf("tok-%d", now.UnixNano())
	err = s.userRepo.StoreRefreshToken(ctx, tokenID, user.UserID, tokenHash, now.Add(7*24*time.Hour))
	if err != nil {
		return nil, nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		ExpiresAt:    expiresAt,
	}, user, nil
}

func (s *AuthService) VerifyToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
