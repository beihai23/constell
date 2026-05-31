package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/constell/constell/backend/pkg/proto/auth/v1/authv1connect"
)

func main() {
	databaseURL := envOrDefault("DATABASE_URL",
		"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable")
	redisURL := envOrDefault("REDIS_URL", "localhost:6379")
	jwtSecret := envOrDefault("JWT_SECRET", "dev-secret-change-me")
	port := envOrDefault("PORT", "9081")

	// Connect to PostgreSQL.
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("parse database URL: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("connected to postgres")

	// Connect to Redis.
	rdb := goredis.NewClient(&goredis.Options{Addr: redisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("ping redis: %v", err)
	}
	log.Println("connected to redis")

	// Wire up the service.
	repo := NewRepository(pool)
	authService := NewAuthService(repo, rdb, jwtSecret)

	// Build the HTTP mux with Connect-RPC handler.
	mux := http.NewServeMux()
	path, handler := authv1connect.NewAuthServiceHandler(authService)
	mux.Handle(path, handler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Printf("auth-service listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
