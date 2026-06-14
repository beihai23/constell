package main

import (
	"fmt"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
)

// HeartbeatHandler manages heartbeat detection and response.
type HeartbeatHandler struct {
	interval time.Duration
}

// NewHeartbeatHandler creates a new HeartbeatHandler with the given interval.
func NewHeartbeatHandler(interval time.Duration) *HeartbeatHandler {
	return &HeartbeatHandler{interval: interval}
}

// Interval returns the configured heartbeat interval.
func (h *HeartbeatHandler) Interval() time.Duration {
	return h.interval
}

// HandleHeartbeat processes a heartbeat message and returns a HEARTBEAT_ACK.
func (h *HeartbeatHandler) HandleHeartbeat(msg *gatewayv1.ClientMessage) *gatewayv1.ServerEvent {
	return h.BuildHeartbeatAck(msg.RequestId)
}

// BuildHeartbeatAck creates a HEARTBEAT_ACK ServerEvent for the given request ID.
func (h *HeartbeatHandler) BuildHeartbeatAck(requestID string) *gatewayv1.ServerEvent {
	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK,
		RequestId: requestID,
	}
}

// IsHeartbeatMessage returns true if the given client message is a heartbeat.
func (h *HeartbeatHandler) IsHeartbeatMessage(msg *gatewayv1.ClientMessage) bool {
	return msg.Type == gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_HEARTBEAT
}

// ResetDeadline extends the read deadline on the WebSocket connection. The
// deadline must be strictly greater than the client's heartbeat interval:
// the client sends a heartbeat every `interval`, and if the deadline equals
// that same `interval` the two race (JS timers drift late), so the connection
// churns — disconnects and reconnects on every cycle, dropping any push
// delivered during the brief disconnect window. Using 2x gives a full
// interval of slack while still detecting truly-dead peers.
func (h *HeartbeatHandler) ResetDeadline(conn *websocket.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(h.interval * 2)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	return nil
}
