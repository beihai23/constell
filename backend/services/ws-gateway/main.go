package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkgnats "github.com/constell/constell/backend/pkg/nats"
	pkgredis "github.com/constell/constell/backend/pkg/redis"
	userv1 "github.com/constell/constell/backend/pkg/proto/user/v1"
	userv1connect "github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	communityv1 "github.com/constell/constell/backend/pkg/proto/community/v1"
	communityv1connect "github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go"
)

func main() {
	gatewayID := envOr("GATEWAY_ID", "gw-001")
	jwtSecret := envOr("JWT_SECRET", "constell-dev-secret")
	listenAddr := envOr("LISTEN_ADDR", ":8081")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	natsURL := envOr("NATS_URL", "nats://localhost:4222")
	userSvcAddr := envOr("USER_SERVICE_ADDR", "http://localhost:9082")
	communitySvcAddr := envOr("COMMUNITY_SERVICE_ADDR", "http://localhost:9083")

	ctx := context.Background()

	redisClient, err := pkgredis.New(ctx, pkgredis.Config{Addr: redisAddr})
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisClient.Close()
	log.Printf("connected to redis at %s", redisAddr)

	natsResult, err := pkgnats.New(pkgnats.Config{URL: natsURL})
	if err != nil {
		log.Fatalf("failed to connect to nats: %v", err)
	}
	defer natsResult.Conn.Close()
	log.Printf("connected to nats at %s", natsURL)

	userSvcClient := userv1connect.NewUserServiceClient(http.DefaultClient, userSvcAddr)
	communitySvcClient := communityv1connect.NewCommunityServiceClient(http.DefaultClient, communitySvcAddr)

	userAdapter := &connectUserSvcClient{client: userSvcClient}
	communityAdapter := &connectCommunitySvcClient{client: communitySvcClient}

	cfg := ServerConfig{
		GatewayID:         gatewayID,
		JWTSecret:         jwtSecret,
		HeartbeatInterval: 30 * time.Second,
		RegistryTTL:       5 * time.Minute,
	}

	srv := NewServer(cfg, redisClient, natsResult.Conn)
	srv.SetRegistry(NewRegistry(redisClient, cfg.RegistryTTL))
	srv.SetRouter(NewRouter(userAdapter, communityAdapter, srv.ConnMgr))
	srv.SetPushSubscriber(NewPushSubscriber(natsResult.Conn, srv.ConnMgr))

	if err := srv.pushSub.Subscribe(gatewayID); err != nil {
		log.Fatalf("failed to subscribe to push topic: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleUpgrade)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","connections":%d,"gateway_id":"%s"}`,
			srv.ConnectionCount(), gatewayID)
	})

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("http server shutdown error: %v", err)
		}

		srv.pushSub.Unsubscribe()
		natsResult.Conn.Close()
		redisClient.Close()
	}()

	log.Printf("WS Gateway starting on %s (gateway_id=%s)", listenAddr, gatewayID)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %v", err)
	}

	log.Println("WS Gateway stopped")
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

type connectUserSvcClient struct {
	client userv1connect.UserServiceClient
}

func (c *connectUserSvcClient) SendDM(ctx context.Context, senderID, receiverID, content string) (string, string, error) {
	resp, err := c.client.SendDM(ctx, connect.NewRequest(&userv1.SendDMRequest{
		TargetUserId: receiverID,
		Content:      content,
	}))
	if err != nil {
		return "", "", fmt.Errorf("SendDM RPC: %w", err)
	}

	msg := resp.Msg.GetMessage()
	if msg == nil {
		return "", "", fmt.Errorf("SendDM returned nil message")
	}

	return msg.Id, time.Unix(msg.CreatedAt, 0).Format(time.RFC3339), nil
}

type connectCommunitySvcClient struct {
	client communityv1connect.CommunityServiceClient
}

func (c *connectCommunitySvcClient) SendMessage(ctx context.Context, senderID, channelID, content string) (string, string, error) {
	resp, err := c.client.SendMessage(ctx, connect.NewRequest(&communityv1.SendMessageRequest{
		ChannelId: channelID,
		Content:   content,
	}))
	if err != nil {
		return "", "", fmt.Errorf("SendMessage RPC: %w", err)
	}

	msg := resp.Msg.GetMessage()
	if msg == nil {
		return "", "", fmt.Errorf("SendMessage returned nil message")
	}

	return msg.Id, time.Unix(msg.CreatedAt, 0).Format(time.RFC3339), nil
}

var (
	_ UserSvcClient       = (*connectUserSvcClient)(nil)
	_ CommunitySvcClient  = (*connectCommunitySvcClient)(nil)
	_ interface{ Publish(subject string, data []byte) error } = (*nats.Conn)(nil)
)
