package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgnats "github.com/constell/constell/backend/pkg/nats"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9086"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	RedisURL  string `env:"REDIS_URL" default:"localhost:6379"`
	NatsURL   string `env:"NATS_URL" default:"nats://localhost:4222"`
	Env       string `env:"ENV" default:"dev"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	// 1. Load config
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	// 2. Init OTel
	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "notify-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	// 3. Init logging
	logger := logging.Init("notify-service", cfg.Env)
	slog.SetDefault(logger)

	// 4. Connect to PostgreSQL (used for health check)
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("parse database URL", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("create pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("ping postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to postgres")

	// 5. Connect to Redis
	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("ping redis", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	// 6. Connect to NATS
	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("nats init", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats")

	// 7. Health checks
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error {
		return pool.Ping(ctx)
	})
	hc.RegisterCheck("redis", func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})

	// 8. Wire up store, subscriber, and service
	store := NewStore(rdb)

	subscriber := NewSubscriber(natsResult.Conn, natsResult.JS, store, pool)
	if err := subscriber.SubscribeAll(); err != nil {
		slog.Error("subscribe to NATS subjects", "error", err)
		os.Exit(1)
	}

	notifySvc := NewNotifyService(store)

	// 9. OTel Connect interceptor
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	// 10. HTTP mux
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(notifyv1connect.NewNotifyServiceHandler(
		notifySvc,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	// 11. Start HTTP server with graceful shutdown
	server := &http.Server{Addr: ":" + cfg.Port, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("notify-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
