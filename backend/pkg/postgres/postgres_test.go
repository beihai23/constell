package postgres

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	cfg := Config{
		Host:     "localhost",
		Port:     5432,
		Name:     "constell",
		User:     "constell",
		Password: "constell_dev",
		SSLMode:  "disable",
		MaxConns: 5,
	}

	pool, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	var result int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&result); err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1, got %d", result)
	}

	t.Log("postgres connection pool working")
}
