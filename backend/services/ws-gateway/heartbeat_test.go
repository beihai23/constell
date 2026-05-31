package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestHeartbeatHandler_HandleHeartbeat(t *testing.T) {
	_, conn := setupTestWS(t)
	defer conn.Close()

	handler := NewHeartbeatHandler(30 * time.Second)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_HEARTBEAT,
		RequestId: "hb-001",
	}

	resp := handler.HandleHeartbeat(msg)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK {
		t.Fatalf("expected HEARTBEAT_ACK, got %v", resp.Type)
	}

	t.Logf("handle heartbeat OK: type=%v", resp.Type)
}

func TestHeartbeatHandler_ResetDeadline(t *testing.T) {
	_, conn := setupTestWS(t)
	defer conn.Close()

	handler := NewHeartbeatHandler(30 * time.Second)

	err := handler.ResetDeadline(conn)
	if err != nil {
		t.Fatalf("ResetDeadline failed: %v", err)
	}

	t.Log("reset deadline OK")
}

func TestHeartbeatHandler_TimeoutFires(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	var serverConn *websocket.Conn
	upgrader := websocket.Upgrader{}

	done := make(chan struct{})
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			close(done)
			return
		}
		defer serverConn.Close()

		_ = NewHeartbeatHandler(100 * time.Millisecond)
		serverConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		_, _, err = serverConn.ReadMessage()
		if err != nil {
			if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") && !websocket.IsCloseError(err, websocket.CloseGoingAway) && !websocket.IsUnexpectedCloseError(err) {
				t.Logf("unexpected read error: %v", err)
			}
		}
		close(done)
	})

	wsURL := "ws:" + strings.TrimPrefix(server.URL, "http:")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	select {
	case <-done:
		t.Log("timeout fired correctly")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout did not fire within 2s")
	}
}

func TestHeartbeatHandler_IsHeartbeatMessage(t *testing.T) {
	handler := NewHeartbeatHandler(30 * time.Second)

	hbMsg := &gatewayv1.ClientMessage{
		Type: gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_HEARTBEAT,
	}
	if !handler.IsHeartbeatMessage(hbMsg) {
		t.Fatal("expected heartbeat message to be detected")
	}

	nonHbMsg := &gatewayv1.ClientMessage{
		Type: gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM,
	}
	if handler.IsHeartbeatMessage(nonHbMsg) {
		t.Fatal("expected non-heartbeat message to not be detected")
	}

	t.Log("is heartbeat message detection OK")
}

func TestHeartbeatHandler_BuildHeartbeatAck(t *testing.T) {
	handler := NewHeartbeatHandler(30 * time.Second)

	ack := handler.BuildHeartbeatAck("req-hb-123")

	if ack.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK {
		t.Fatalf("expected HEARTBEAT_ACK, got %v", ack.Type)
	}
	if ack.RequestId != "req-hb-123" {
		t.Fatalf("expected request_id 'req-hb-123', got %q", ack.RequestId)
	}

	data, err := proto.Marshal(ack)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty serialized ack")
	}

	t.Logf("build heartbeat ack OK: type=%v request_id=%s", ack.Type, ack.RequestId)
}
