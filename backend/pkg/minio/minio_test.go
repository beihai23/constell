package minio

import (
	"testing"
)

func TestConfigFields(t *testing.T) {
	cfg := Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
		Bucket:    "constell",
	}
	if cfg.Endpoint != "localhost:9000" {
		t.Errorf("expected localhost:9000, got %s", cfg.Endpoint)
	}
	if cfg.Bucket != "constell" {
		t.Errorf("expected constell, got %s", cfg.Bucket)
	}
}
