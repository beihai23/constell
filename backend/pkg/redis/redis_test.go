package redis

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	cfg := Config{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}

	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	pong, err := client.Ping(context.Background()).Result()
	if err != nil {
		t.Fatalf("failed to ping redis: %v", err)
	}
	if pong != "PONG" {
		t.Fatalf("expected PONG, got %s", pong)
	}

	t.Log("redis client working")
}
