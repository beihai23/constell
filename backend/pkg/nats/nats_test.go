package nats

import (
	"testing"
)

func TestNew(t *testing.T) {
	cfg := Config{
		URL: "nats://localhost:4222",
	}

	result, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to connect to nats: %v", err)
	}
	defer result.Conn.Close()

	if result.Conn == nil {
		t.Fatal("expected non-nil conn")
	}
	if result.JS == nil {
		t.Fatal("expected non-nil jetstream context")
	}

	if !result.Conn.IsConnected() {
		t.Fatal("expected nats connection to be active")
	}

	// Verify the stream was created.
	info, err := result.JS.StreamInfo("constell")
	if err != nil {
		t.Fatalf("failed to get stream info: %v", err)
	}
	if info.Config.Name != "constell" {
		t.Fatalf("expected stream name 'constell', got %q", info.Config.Name)
	}

	t.Log("nats client and jetstream stream working")
}
