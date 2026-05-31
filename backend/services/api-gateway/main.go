package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authv1connect "github.com/constell/constell/backend/pkg/proto/auth/v1/authv1connect"
	userv1connect "github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	communityv1connect "github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"
)

// Config holds the gateway's configuration, populated from environment variables.
type Config struct {
	Addr                string
	AuthServiceURL      string
	UserServiceURL      string
	CommunityServiceURL string
	JWTSecret           string
}

func loadConfig() Config {
	return Config{
		Addr:                getEnv("GATEWAY_ADDR", ":8080"),
		AuthServiceURL:      getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		UserServiceURL:      getEnv("USER_SERVICE_URL", "http://localhost:8082"),
		CommunityServiceURL: getEnv("COMMUNITY_SERVICE_URL", "http://localhost:8083"),
		JWTSecret:           getEnv("JWT_SECRET", "dev-secret-change-me"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Clients holds the Connect-RPC clients for all backend services.
type Clients struct {
	Auth      authv1connect.AuthServiceClient
	User      userv1connect.UserServiceClient
	Community communityv1connect.CommunityServiceClient
}

func newClients(cfg Config) *Clients {
	return &Clients{
		Auth: authv1connect.NewAuthServiceClient(
			http.DefaultClient,
			cfg.AuthServiceURL,
		),
		User: userv1connect.NewUserServiceClient(
			http.DefaultClient,
			cfg.UserServiceURL,
		),
		Community: communityv1connect.NewCommunityServiceClient(
			http.DefaultClient,
			cfg.CommunityServiceURL,
		),
	}
}

func main() {
	cfg := loadConfig()
	clients := newClients(cfg)

	r := chi.NewRouter()

	// Global middleware.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Register all routes.
	registerRoutes(r, clients, cfg.JWTSecret)

	// HTTP server with graceful shutdown.
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start listening in a goroutine.
	go func() {
		log.Printf("API Gateway listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down API Gateway...")

	// Graceful shutdown with 10-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}

	log.Println("API Gateway stopped")
}

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

const userIDKey contextKey = "constell-user-id"

// userIDFromContext extracts the user ID from the request context.
// This value is set by the auth middleware.
func userIDFromContext(r *http.Request) string {
	val, _ := r.Context().Value(userIDKey).(string)
	return val
}

// contextWithUserID returns a new context with the user ID embedded.
func contextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// connectErrorToHTTP maps a Connect error code to an HTTP status code.
func connectErrorToHTTP(err error) (int, string) {
	if connectErr := new(connect.Error); errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeInvalidArgument:
			return http.StatusBadRequest, connectErr.Message()
		case connect.CodeUnauthenticated:
			return http.StatusUnauthorized, connectErr.Message()
		case connect.CodePermissionDenied:
			return http.StatusForbidden, connectErr.Message()
		case connect.CodeNotFound:
			return http.StatusNotFound, connectErr.Message()
		case connect.CodeAlreadyExists:
			return http.StatusConflict, connectErr.Message()
		case connect.CodeInternal:
			return http.StatusInternalServerError, "internal server error"
		case connect.CodeUnavailable:
			return http.StatusServiceUnavailable, "service unavailable"
		case connect.CodeDeadlineExceeded:
			return http.StatusGatewayTimeout, "request timeout"
		default:
			return http.StatusInternalServerError, connectErr.Message()
		}
	}
	return http.StatusInternalServerError, err.Error()
}
