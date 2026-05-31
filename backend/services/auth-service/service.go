package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	goredis "github.com/redis/go-redis/v9"

	pbv1 "github.com/constell/constell/backend/pkg/proto/auth/v1"
	"github.com/constell/constell/backend/pkg/proto/auth/v1/authv1connect"
	"github.com/constell/constell/backend/pkg/jwt"
)

const (
	accessTokenExpiry  = 15 * time.Minute
	refreshTokenExpiry = 7 * 24 * time.Hour
)

// TokenPair holds an access token and refresh token with expiry.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

// AuthService implements the Connect-RPC AuthServiceHandler.
type AuthService struct {
	repo      UserRepository
	rdb       *goredis.Client
	jwtSecret string
}

// NewAuthService creates a new AuthService.
func NewAuthService(repo UserRepository, rdb *goredis.Client, jwtSecret string) *AuthService {
	return &AuthService{
		repo:      repo,
		rdb:       rdb,
		jwtSecret: jwtSecret,
	}
}

// Ensure AuthService implements the generated service handler interface.
var _ authv1connect.AuthServiceHandler = (*AuthService)(nil)

// Register handles user registration.
func (s *AuthService) Register(
	ctx context.Context,
	req *connect.Request[pbv1.RegisterRequest],
) (*connect.Response[pbv1.RegisterResponse], error) {
	msg := req.Msg

	email := strings.TrimSpace(msg.Email)
	password := msg.Password
	nickname := strings.TrimSpace(msg.Nickname)

	if email == "" || password == "" || nickname == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email, password, and nickname are required"))
	}

	if len(password) < 8 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("password must be at least 8 characters"))
	}

	// Check if email is already taken.
	_, err := s.repo.GetUserByEmail(ctx, email)
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("email already registered"))
	}

	// Hash the password with bcrypt.
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to hash password: %w", err))
	}

	// Create the user.
	userID, err := s.repo.CreateUser(ctx, nickname, email, hashedPassword)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create user: %w", err))
	}

	// Generate JWT pair.
	pair, err := s.generateTokenPair(userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to generate tokens: %w", err))
	}

	resp := connect.NewResponse(&pbv1.RegisterResponse{
		UserId:       userID,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresAt:    pair.ExpiresAt,
	})
	return resp, nil
}

// Login handles user authentication.
func (s *AuthService) Login(
	ctx context.Context,
	req *connect.Request[pbv1.LoginRequest],
) (*connect.Response[pbv1.LoginResponse], error) {
	msg := req.Msg

	email := strings.TrimSpace(msg.Email)
	password := msg.Password

	if email == "" || password == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email and password are required"))
	}

	// Look up user by email.
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("invalid email or password"))
	}

	// Verify password.
	if !checkPassword(password, user.PasswordHash) {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("invalid email or password"))
	}

	// Generate JWT pair.
	pair, err := s.generateTokenPair(user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to generate tokens: %w", err))
	}

	resp := connect.NewResponse(&pbv1.LoginResponse{
		UserId:       user.ID,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresAt:    pair.ExpiresAt,
	})
	return resp, nil
}

// RefreshToken rotates an access/refresh token pair.
func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *connect.Request[pbv1.RefreshTokenRequest],
) (*connect.Response[pbv1.RefreshTokenResponse], error) {
	msg := req.Msg

	if msg.RefreshToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("refresh_token is required"))
	}

	// Look up the refresh token in Redis.
	tokenHash := hashToken(msg.RefreshToken)
	redisKey := fmt.Sprintf("refresh:%s", tokenHash)

	userID, err := s.rdb.Get(ctx, redisKey).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, connect.NewError(connect.CodeUnauthenticated,
				fmt.Errorf("invalid or expired refresh token"))
		}
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("redis lookup failed: %w", err))
	}

	// Delete the old refresh token (rotation).
	s.rdb.Del(ctx, redisKey)

	// Generate new JWT pair.
	pair, err := s.generateTokenPair(userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to generate tokens: %w", err))
	}

	resp := connect.NewResponse(&pbv1.RefreshTokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresAt:    pair.ExpiresAt,
	})
	return resp, nil
}

// generateTokenPair creates an access token and refresh token, stores the refresh
// token in Redis, and returns both.
func (s *AuthService) generateTokenPair(userID string) (*TokenPair, error) {
	accessToken, _, err := jwt.GenerateToken(s.jwtSecret, userID, accessTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, _, err := jwt.GenerateToken(s.jwtSecret, userID, refreshTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store refresh token in Redis: refresh:{sha256(token)} -> userID.
	tokenHash := hashToken(refreshToken)
	redisKey := fmt.Sprintf("refresh:%s", tokenHash)
	ctx := context.Background()
	if err := s.rdb.Set(ctx, redisKey, userID, refreshTokenExpiry).Err(); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	expiresAt := time.Now().Add(accessTokenExpiry).Unix()

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

// hashToken returns the hex-encoded SHA-256 of a token string.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
