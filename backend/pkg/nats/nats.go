package nats

import (
	"fmt"
	"time"

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

// streamConfig is the desired configuration for the "constell" stream. The
// MaxAge bound is important: the stream captures all constell.> subjects,
// including high-frequency presence events (user.online/offline on every
// connect/disconnect). Without a retention limit those churn messages grow
// the stream without bound and eventually exhaust the disk, which leaves
// JetStream unable to persist any new message — silently breaking real-time
// delivery. Discarding messages older than the window keeps the stream bounded.
var streamConfig = nats.StreamConfig{
	Name:    "constell",
	Subjects: []string{"constell.>"},
	Storage: nats.FileStorage,
	Replicas: 1,
	MaxAge:  1 * time.Hour,
	Discard: nats.DiscardOld,
}

// New connects to NATS, creates a JetStream context, and ensures the
// "constell" stream exists with the desired configuration. If the stream
// already exists (e.g. created by an earlier version) its configuration is
// updated so retention limits are applied.
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

	streamName := streamConfig.Name
	if _, err := js.StreamInfo(streamName); err != nil {
		// Stream does not exist yet — create it.
		if _, err = js.AddStream(&streamConfig); err != nil {
			nc.Close()
			return nil, fmt.Errorf("add stream %q: %w", streamName, err)
		}
	} else {
		// Stream exists — reconcile its config (applies retention to streams
		// created before MaxAge was introduced).
		if _, err = js.UpdateStream(&streamConfig); err != nil {
			nc.Close()
			return nil, fmt.Errorf("update stream %q: %w", streamName, err)
		}
	}

	return &Result{Conn: nc, JS: js}, nil
}
