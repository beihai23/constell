package nats

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

// Config holds NATS connection parameters.
type Config struct {
	URL string
}

// Result holds the NATS connection and JetStream context returned by New.
type Result struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

// New connects to NATS, creates a JetStream context, and ensures the
// "constell" stream exists. The stream is created with sensible defaults
// if it does not already exist.
func New(cfg Config) (*Result, error) {
	nc, err := nats.Connect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}

	// Ensure the "constell" stream exists. If it already exists this is a no-op.
	streamName := "constell"
	_, err = js.StreamInfo(streamName)
	if err != nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{"constell.>"},
			Storage:  nats.FileStorage,
			Replicas: 1,
		})
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("add stream %q: %w", streamName, err)
		}
	}

	return &Result{Conn: nc, JS: js}, nil
}
