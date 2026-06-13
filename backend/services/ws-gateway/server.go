package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
	goredis "github.com/redis/go-redis/v9"
)

// Server is the top-level WS Gateway server.
type Server struct {
	gwID      string
	auth      *Authenticator
	ConnMgr   *ConnManager
	registry  *Registry
	router    *Router
	pushSub   *PushSubscriber
	heartbeat *HeartbeatHandler
	natsConn  interface{ Publish(subject string, data []byte) error }

	upgrader websocket.Upgrader
}

// ServerConfig holds configuration for the WS Gateway server.
type ServerConfig struct {
	GatewayID         string
	JWTSecret         string
	HeartbeatInterval time.Duration
	RegistryTTL       time.Duration
}

// NewServer creates a new Server with the given configuration and dependencies.
func NewServer(cfg ServerConfig, _ *goredis.Client, natsConn interface{ Publish(subject string, data []byte) error }) *Server {
	auth := NewAuthenticator(cfg.JWTSecret)
	connMgr := NewConnManager()
	heartbeat := NewHeartbeatHandler(cfg.HeartbeatInterval)

	return &Server{
		gwID:      cfg.GatewayID,
		auth:      auth,
		ConnMgr:   connMgr,
		heartbeat: heartbeat,
		natsConn:  natsConn,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// SetRegistry sets the Redis connection registry.
func (s *Server) SetRegistry(reg *Registry) {
	s.registry = reg
}

// SetRouter sets the message router.
func (s *Server) SetRouter(router *Router) {
	s.router = router
}

// SetPushSubscriber sets the NATS push subscriber.
func (s *Server) SetPushSubscriber(ps *PushSubscriber) {
	s.pushSub = ps
}

// HandleUpgrade handles the WebSocket upgrade request.
func (s *Server) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	userID, token, err := s.auth.AuthenticateUpgrade(r)
	if err != nil {
		log.Printf("auth failed for %s: %v", r.RemoteAddr, err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade failed for user %s: %v", userID, err)
		return
	}

	s.ConnMgr.Register(userID, conn)
	log.Printf("user %s connected (gw=%s)", userID, s.gwID)

	if s.registry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := s.registry.RegisterConnection(ctx, userID, s.gwID); err != nil {
			log.Printf("failed to register user %s in redis: %v", userID, err)
		}
		cancel()
	}

	s.broadcastUserOnline(userID)

	go s.readPump(userID, token, conn)
}

func (s *Server) readPump(userID string, token string, conn *websocket.Conn) {
	defer s.cleanupDisconnect(userID)

	s.heartbeat.ResetDeadline(conn)

	for {
		msg, err := ReadMessage(conn)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("read error for user %s: %v", userID, err)
			}
			return
		}

		if s.heartbeat.IsHeartbeatMessage(msg) {
			ack := s.heartbeat.HandleHeartbeat(msg)
			if writeErr := WriteMessage(conn, ack); writeErr != nil {
				log.Printf("failed to send heartbeat ack to user %s: %v", userID, writeErr)
				return
			}

			s.heartbeat.ResetDeadline(conn)

			if s.registry != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if regErr := s.registry.RegisterConnection(ctx, userID, s.gwID); regErr != nil {
					log.Printf("failed to refresh redis TTL for user %s: %v", userID, regErr)
				}
				cancel()
			}

			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ctx = WithToken(ctx, token)
		ack, routeErr := s.router.Route(ctx, userID, msg)
		cancel()

		if routeErr != nil {
			log.Printf("route error for user %s: %v", userID, routeErr)
			errEvent := &gatewayv1.ServerEvent{
				Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ERROR,
				ErrorEvent: &gatewayv1.ErrorEvent{
					Code:    "ROUTE_ERROR",
					Message: fmt.Sprintf("failed to process message: %v", routeErr),
				},
				RequestId: msg.RequestId,
			}
			if writeErr := WriteMessage(conn, errEvent); writeErr != nil {
				log.Printf("failed to send error to user %s: %v", userID, writeErr)
				return
			}
			continue
		}

		if ack != nil {
			if writeErr := WriteMessage(conn, ack); writeErr != nil {
				log.Printf("failed to send ack to user %s: %v", userID, writeErr)
				return
			}
		}
	}
}

func (s *Server) cleanupDisconnect(userID string) {
	s.ConnMgr.Unregister(userID)
	log.Printf("user %s disconnected (gw=%s)", userID, s.gwID)

	if s.registry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := s.registry.UnregisterConnection(ctx, userID); err != nil {
			log.Printf("failed to unregister user %s from redis: %v", userID, err)
		}
		cancel()
	}

	s.broadcastUserOffline(userID)
}

func (s *Server) broadcastUserOnline(userID string) {
	if s.natsConn == nil {
		return
	}
	data, _ := json.Marshal(map[string]string{
		"user_id": userID,
		"gw_id":   s.gwID,
	})
	if err := s.natsConn.Publish("constell.user.online", data); err != nil {
		log.Printf("failed to broadcast user_online for %s: %v", userID, err)
	}
}

func (s *Server) broadcastUserOffline(userID string) {
	if s.natsConn == nil {
		return
	}
	data, _ := json.Marshal(map[string]string{
		"user_id": userID,
	})
	if err := s.natsConn.Publish("constell.user.offline", data); err != nil {
		log.Printf("failed to broadcast user_offline for %s: %v", userID, err)
	}
}

// ConnectionCount returns the number of active connections.
func (s *Server) ConnectionCount() int {
	return s.ConnMgr.Count()
}
