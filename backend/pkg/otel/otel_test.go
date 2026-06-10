package otel

import (
	"context"
	"testing"
	"time"
)

func TestInitDoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := Init(ctx, Config{
		ServiceName: "test-service",
		Environment: "test",
		Endpoint:    "http://localhost:9999/api/default/v1/otlp",
		Insecure:    true,
	})
	if err != nil {
		t.Logf("Init returned error (acceptable for invalid endpoint): %v", err)
		return
	}
	defer shutdown(ctx)
}

func TestShutdownWithTimeout(t *testing.T) {
	ShutdownWithTimeout(func(_ context.Context) error {
		return nil
	}, 0)
}
