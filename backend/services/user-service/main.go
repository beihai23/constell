package main

import (
	"context"
	"fmt"
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
	pkgnats "github.com/constell/constell/backend/pkg/nats"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/middleware"
	"github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	"github.com/constell/constell/backend/pkg/registry"
)

type Config struct {
	Port      string `env:"PORT" default:"9082"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	RedisURL  string `env:"REDIS_URL" default:"localhost:6379"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`
	NatsURL   string `env:"NATS_URL" default:"nats://localhost:4222"`
	Env       string `env:"ENV" default:"dev"`

	RegistryType    string   `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath string   `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`
	Peers           []string `env:"GROUPCACHE_PEERS"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "user-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("user-service", cfg.Env)
	slog.SetDefault(logger)

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

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("ping redis", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats")

	// Init Registry (optional, for groupcache peer discovery)
	var reg registry.Registry
	if cfg.RegistryType == "static" && cfg.ServicesCfgPath != "" {
		var err error
		reg, err = registry.NewStaticRegistry(registry.StaticConfig{
			ConfigPath: cfg.ServicesCfgPath,
		})
		if err != nil {
			slog.Warn("registry init failed, using env peers", "error", err)
		}
	}

	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error { return pool.Ping(ctx) })
	hc.RegisterCheck("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
	hc.RegisterCheck("nats", func(ctx context.Context) error {
		if !natsResult.Conn.IsConnected() {
			return fmt.Errorf("nats not connected")
		}
		return nil
	})

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	repo := NewRepository(pool)
	userCache := NewUserCache(10000, cfg.Peers, reg, "user-service", repo)
	relationCache := NewRelationCache(10000, cfg.Peers, reg, "user-service", repo)
	userService := NewUserService(repo, userCache, relationCache, natsResult.Conn)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(userv1connect.NewUserServiceHandler(
		userService,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

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

	slog.Info("user-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
