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

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9085"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
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
		ServiceName: "search-service",
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
	logger := logging.Init("search-service", cfg.Env)
	slog.SetDefault(logger)

	// 4. Connect to PostgreSQL
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

	// 5. Health checks
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error {
		return pool.Ping(ctx)
	})

	// 6. OTel Connect interceptor
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	// 7. Wire up service
	repo := NewRepository(pool)
	searchSvc := NewSearchService(repo)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(searchv1connect.NewSearchServiceHandler(
		searchSvc,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	// 8. Start HTTP server
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

	slog.Info("search-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
