package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

// CheckFunc checks if a dependency is healthy. Returns nil if healthy.
type CheckFunc func(ctx context.Context) error

// Checker manages liveness and readiness checks.
type Checker struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// NewChecker creates a health checker.
func NewChecker() *Checker {
	return &Checker{
		checks: make(map[string]CheckFunc),
	}
}

// RegisterCheck registers a readiness check.
func (c *Checker) RegisterCheck(name string, fn CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = fn
}

// HealthzHandler returns a liveness check handler. Always returns 200.
func (c *Checker) HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ReadyHandler returns a readiness check handler. Runs all registered CheckFuncs.
func (c *Checker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c.mu.RLock()
		defer c.mu.RUnlock()

		failures := make(map[string]string)
		for name, fn := range c.checks {
			if err := fn(ctx); err != nil {
				failures[name] = err.Error()
			}
		}

		if len(failures) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status":   "not ready",
				"failures": failures,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
