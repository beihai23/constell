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

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	pkgnats "github.com/constell/constell/backend/pkg/nats"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	pkgredis "github.com/constell/constell/backend/pkg/redis"
	"github.com/constell/constell/backend/pkg/registry"

	userv1 "github.com/constell/constell/backend/pkg/proto/user/v1"
	userv1connect "github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	communityv1 "github.com/constell/constell/backend/pkg/proto/community/v1"
	communityv1connect "github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go"
)

// Config holds the ws-gateway configuration, populated from environment variables.
type Config struct {
	GatewayID        string `env:"GATEWAY_ID" default:"gw-001"`
	JWTSecret        string `env:"JWT_SECRET" default:"constell-dev-secret"`
	ListenAddr       string `env:"LISTEN_ADDR" default:":8081"`
	RedisAddr        string `env:"REDIS_ADDR" default:"localhost:6379"`
	NatsURL          string `env:"NATS_URL" default:"nats://localhost:4222"`
	UserSvcAddr      string `env:"USER_SERVICE_ADDR" default:"http://localhost:9082"`
	CommunitySvcAddr string `env:"COMMUNITY_SERVICE_ADDR" default:"http://localhost:9083"`
	Env              string `env:"ENV" default:"dev"`

	RegistryType    string `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath string `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	// 1. Load config
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	// 2. Init OTel
	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "ws-gateway",
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
	logger := logging.Init("ws-gateway", cfg.Env)
	slog.SetDefault(logger)

	ctx := context.Background()

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
	userSvcAddr := cfg.UserSvcAddr
	communitySvcAddr := cfg.CommunitySvcAddr

	if reg != nil {
		if instances, err := reg.Discover(ctx, "user-service"); err == nil && len(instances) > 0 {
			userSvcAddr = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(ctx, "community-service"); err == nil && len(instances) > 0 {
			communitySvcAddr = "http://" + instances[0].Addr
		}
	}

	slog.Info("service discovery",
		"user", userSvcAddr,
		"community", communitySvcAddr,
	)

	// 6. Connect to infrastructure
	redisClient, err := pkgredis.New(ctx, pkgredis.Config{Addr: cfg.RedisAddr})
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	slog.Info("connected to redis", "addr", cfg.RedisAddr)

	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats", "url", cfg.NatsURL)

	// 7. Create Connect-RPC clients
	userSvcClient := userv1connect.NewUserServiceClient(http.DefaultClient, userSvcAddr)
	communitySvcClient := communityv1connect.NewCommunityServiceClient(http.DefaultClient, communitySvcAddr)

	userAdapter := &connectUserSvcClient{client: userSvcClient}
	communityAdapter := &connectCommunitySvcClient{client: communitySvcClient}

	// 8. Health checks
	hc := health.NewChecker()
	hc.RegisterCheck("redis", func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	})
	hc.RegisterCheck("nats", func(_ context.Context) error {
		if !natsResult.Conn.IsConnected() {
			return fmt.Errorf("nats not connected")
		}
		return nil
	})

	// 9. Create and wire up WS server
	srvCfg := ServerConfig{
		GatewayID:         cfg.GatewayID,
		JWTSecret:         cfg.JWTSecret,
		HeartbeatInterval: 30 * time.Second,
		RegistryTTL:       5 * time.Minute,
	}

	srv := NewServer(srvCfg, redisClient, natsResult.Conn)
	srv.SetRegistry(NewRegistry(redisClient, srvCfg.RegistryTTL))
	srv.SetRouter(NewRouter(userAdapter, communityAdapter, srv.ConnMgr))
	srv.SetPushSubscriber(NewPushSubscriber(natsResult.Conn, srv.ConnMgr))

	if err := srv.pushSub.Subscribe(cfg.GatewayID); err != nil {
		slog.Error("failed to subscribe to push topic", "error", err)
		os.Exit(1)
	}

	// 10. Wire up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleUpgrade)
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	// 11. Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutting down ws-gateway...", "signal", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("http server shutdown error", "error", err)
		}

		srv.pushSub.Unsubscribe()
		natsResult.Conn.Close()
		redisClient.Close()
	}()

	slog.Info("ws-gateway starting", "addr", cfg.ListenAddr, "gateway_id", cfg.GatewayID)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server error", "error", err)
		os.Exit(1)
	}

	slog.Info("ws-gateway stopped")
}

type connectUserSvcClient struct {
	client userv1connect.UserServiceClient
}

func (c *connectUserSvcClient) SendDM(ctx context.Context, senderID, receiverID, content string) (string, int64, error) {
	req := connect.NewRequest(&userv1.SendDMRequest{
		TargetUserId: receiverID,
		Content:      content,
	})
	if token := TokenFromContext(ctx); token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.SendDM(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("SendDM RPC: %w", err)
	}

	msg := resp.Msg.GetMessage()
	if msg == nil {
		return "", 0, fmt.Errorf("SendDM returned nil message")
	}

	return msg.Id, msg.Seq, nil
}

type connectCommunitySvcClient struct {
	client communityv1connect.CommunityServiceClient
}

func (c *connectCommunitySvcClient) SendMessage(ctx context.Context, senderID, channelID, content string) (string, int64, error) {
	req := connect.NewRequest(&communityv1.SendMessageRequest{
		ChannelId: channelID,
		Content:   content,
	})
	if token := TokenFromContext(ctx); token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.SendMessage(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("SendMessage RPC: %w", err)
	}

	msg := resp.Msg.GetMessage()
	if msg == nil {
		return "", 0, fmt.Errorf("SendMessage returned nil message")
	}

	return msg.Id, msg.Seq, nil
}

var (
	_ UserSvcClient      = (*connectUserSvcClient)(nil)
	_ CommunitySvcClient = (*connectCommunitySvcClient)(nil)
	_ interface{ Publish(subject string, data []byte) error } = (*nats.Conn)(nil)
)
