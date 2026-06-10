package config

import (
	"os"
	"testing"
)

type TestConfig struct {
	Host  string   `env:"HOST" default:"localhost"`
	Port  int      `env:"PORT" default:"8080"`
	Debug bool     `env:"DEBUG" default:"false"`
	Peers []string `env:"PEERS"`
	NoTag string
}

func TestLoadWithDefaults(t *testing.T) {
	os.Clearenv()
	var cfg TestConfig
	if err := NewLoader("").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port: got %d, want %d", cfg.Port, 8080)
	}
	if cfg.Debug != false {
		t.Errorf("Debug: got %v, want false", cfg.Debug)
	}
}

func TestLoadWithEnvOverride(t *testing.T) {
	os.Clearenv()
	os.Setenv("HOST", "0.0.0.0")
	os.Setenv("PORT", "3000")
	os.Setenv("DEBUG", "true")
	os.Setenv("PEERS", "a:8080,b:8080,c:8080")

	var cfg TestConfig
	if err := NewLoader("").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 3000 {
		t.Errorf("Port: got %d, want %d", cfg.Port, 3000)
	}
	if cfg.Debug != true {
		t.Errorf("Debug: got %v, want true", cfg.Debug)
	}
	if len(cfg.Peers) != 3 {
		t.Fatalf("Peers: got %d, want 3", len(cfg.Peers))
	}
	if cfg.Peers[0] != "a:8080" {
		t.Errorf("Peers[0]: got %q", cfg.Peers[0])
	}
}

func TestLoadWithPrefix(t *testing.T) {
	os.Clearenv()
	os.Setenv("MYAPP_HOST", "myapp-host")

	var cfg TestConfig
	if err := NewLoader("MYAPP_").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "myapp-host" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "myapp-host")
	}
}

func TestMustLoadPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	NewLoader("").MustLoad("not-a-struct-pointer")
}
