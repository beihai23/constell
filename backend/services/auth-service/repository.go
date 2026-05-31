package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User represents a row from the users table relevant to authentication.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Nickname     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserRepository defines the database operations needed by AuthService.
type UserRepository interface {
	CreateUser(ctx context.Context, nickname, email, hashedPassword string) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
}

// Repository handles database operations for the auth service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// CreateUser inserts a new user and returns the generated ID.
func (r *Repository) CreateUser(ctx context.Context, nickname, email, hashedPassword string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (nickname, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		nickname, email, hashedPassword,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return id, nil
}

// GetUserByEmail looks up a user by email. Returns pgx.ErrNoRows if not found.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, nickname, created_at, updated_at FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Nickname, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	return &u, nil
}
