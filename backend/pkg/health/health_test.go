package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzAlwaysOK(t *testing.T) {
	c := NewChecker()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	c.HealthzHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyAllChecksPass(t *testing.T) {
	c := NewChecker()
	c.RegisterCheck("db", func(ctx context.Context) error { return nil })
	c.RegisterCheck("cache", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyCheckFails(t *testing.T) {
	c := NewChecker()
	c.RegisterCheck("db", func(ctx context.Context) error { return nil })
	c.RegisterCheck("redis", func(ctx context.Context) error {
		return errors.New("connection refused")
	})

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyNoChecks(t *testing.T) {
	c := NewChecker()
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz with no checks: got %d, want %d", rec.Code, http.StatusOK)
	}
}
