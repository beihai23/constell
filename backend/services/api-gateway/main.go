package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	goredis "github.com/redis/go-redis/v9"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/redis"
	"github.com/constell/constell/backend/pkg/registry"
	"github.com/constell/constell/backend/services/api-gateway/handlers"
)

// Config holds the gateway's configuration, populated from environment variables.
type Config struct {
	Addr      string `env:"ADDR" default:":8080"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`
	Env       string `env:"ENV" default:"dev"`

	AuthServiceURL      string `env:"AUTH_SERVICE_URL" default:"http://auth-service:9081"`
	UserServiceURL      string `env:"USER_SERVICE_URL" default:"http://user-service:9082"`
	CommunityServiceURL string `env:"COMMUNITY_SERVICE_URL" default:"http://community-service:9083"`
	FileServiceURL      string `env:"FILE_SERVICE_URL" default:"http://file-service:9084"`
	SearchServiceURL    string `env:"SEARCH_SERVICE_URL" default:"http://search-service:9085"`
	NotifyServiceURL    string `env:"NOTIFY_SERVICE_URL" default:"http://notify-service:9086"`

	RegistryType    string `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath string `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`

	RedisAddr     string `env:"REDIS_URL" default:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD" default:""`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	// 1. Load config
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	// 2. Init OTel
	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "api-gateway",
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
	logger := logging.Init("api-gateway", cfg.Env)
	slog.SetDefault(logger)

	// 4. Init Registry (optional)
	var reg registry.Registry
	if cfg.RegistryType == "static" {
		staticReg, err := registry.NewStaticRegistry(registry.StaticConfig{
			ConfigPath: cfg.ServicesCfgPath,
		})
		if err != nil {
			slog.Warn("static registry unavailable, using env var URLs", "error", err)
		} else {
			reg = staticReg
		}
	}

	// 5. Discover service addresses (with fallback to env vars)
	authURL := cfg.AuthServiceURL
	userURL := cfg.UserServiceURL
	communityURL := cfg.CommunityServiceURL
	fileURL := cfg.FileServiceURL
	searchURL := cfg.SearchServiceURL
	notifyURL := cfg.NotifyServiceURL

	if reg != nil {
		if instances, err := reg.Discover(context.Background(), "auth-service"); err == nil && len(instances) > 0 {
			authURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "user-service"); err == nil && len(instances) > 0 {
			userURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "community-service"); err == nil && len(instances) > 0 {
			communityURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "file-service"); err == nil && len(instances) > 0 {
			fileURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "search-service"); err == nil && len(instances) > 0 {
			searchURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "notify-service"); err == nil && len(instances) > 0 {
			notifyURL = "http://" + instances[0].Addr
		}
	}

	slog.Info("service discovery",
		"auth", authURL,
		"user", userURL,
		"community", communityURL,
		"file", fileURL,
		"search", searchURL,
		"notify", notifyURL,
	)

	// 6. Init clients
	clients := handlers.NewClientsFromURLs(authURL, userURL, communityURL, fileURL, searchURL, notifyURL)

	// 6b. Init Redis (optional — used for presence lookups)
	var redisClient *goredis.Client
	rdb, err := redis.New(context.Background(), redis.Config{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	if err != nil {
		slog.Warn("redis unavailable, presence lookups disabled", "error", err)
	} else {
		redisClient = rdb
		slog.Info("redis connected", "addr", cfg.RedisAddr)
	}

	// 7. Health checks
	hc := health.NewChecker()

	// 8. Wire up routes
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health endpoints
	r.Get("/healthz", hc.HealthzHandler())
	r.Get("/readyz", hc.ReadyHandler())

	// Register all REST routes
	registerRoutes(r, clients, cfg.JWTSecret, redisClient)

	// Wrap mux with metrics middleware
	var handler http.Handler = r
	handler = metrics.HTTPMiddleware(handler)

	// 9. Start HTTP server
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down api-gateway...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("api-gateway listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
