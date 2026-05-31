package main

import (
	"encoding/json"
	"testing"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/nats-io/nats.go"
)

func TestPushPayload_Parse(t *testing.T) {
	raw := `{
		"targets": ["user-1", "user-2"],
		"event_type": "DM_RECEIVED",
		"payload": {
			"message_id": "msg-001",
			"sender_id": "user-3",
			"content": "Hello"
		}
	}`

	var pp PushPayload
	if err := json.Unmarshal([]byte(raw), &pp); err != nil {
		t.Fatalf("unmarshal push payload: %v", err)
	}

	if len(pp.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(pp.Targets))
	}
	if pp.Targets[0] != "user-1" {
		t.Fatalf("expected target[0] 'user-1', got %q", pp.Targets[0])
	}
	if pp.EventType != "DM_RECEIVED" {
		t.Fatalf("expected event_type 'DM_RECEIVED', got %q", pp.EventType)
	}

	t.Logf("push payload parse OK: targets=%v type=%s", pp.Targets, pp.EventType)
}

func TestPushSubscriber_topic(t *testing.T) {
	gwID := "gw-instance-005"
	expected := "gw.push.gw-instance-005"
	result := pushTopic(gwID)
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
	t.Logf("push topic OK: %s", result)
}

func TestPushSubscriber_DeliverToLocal(t *testing.T) {
	mgr := NewConnManager()

	client1, serverConn1 := setupTestWS(t)
	defer serverConn1.Close()
	client2, serverConn2 := setupTestWS(t)
	_ = client2
	defer serverConn2.Close()

	mgr.Register("user-local-1", serverConn1)
	mgr.Register("user-local-2", serverConn2)

	sub := NewPushSubscriber(nil, mgr)

	payload := PushPayload{
		Targets:   []string{"user-local-1", "user-local-2", "user-local-3"},
		EventType: "DM_RECEIVED",
		Payload: map[string]interface{}{
			"message_id": "msg-push-001",
			"sender_id":  "user-sender",
			"content":    "Push message!",
		},
	}

	delivered := sub.DeliverToLocal(payload)
	if delivered != 2 {
		t.Fatalf("expected 2 deliveries, got %d", delivered)
	}

	client1.SetReadDeadline(time.Now().Add(1 * time.Second))
	msgType, data, err := client1.ReadMessage()
	if err != nil {
		t.Fatalf("read from conn1: %v", err)
	}
	if msgType != 2 {
		t.Fatalf("expected binary message, got type %d", msgType)
	}

	serverEvent, err := DecodeFrame(data)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if serverEvent.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED {
		t.Fatalf("expected DM_RECEIVED, got %v", serverEvent.Type)
	}
	if serverEvent.DmReceivedEvent.MessageId != "msg-push-001" {
		t.Fatalf("expected message_id 'msg-push-001', got %q", serverEvent.DmReceivedEvent.MessageId)
	}

	t.Logf("deliver to local OK: delivered=%d", delivered)
}

func TestPushSubscriber_buildServerEvent(t *testing.T) {
	sub := NewPushSubscriber(nil, nil)

	payload := PushPayload{
		EventType: "CHANNEL_MESSAGE_RECEIVED",
		Payload: map[string]interface{}{
			"message_id": "ch-msg-001",
			"channel_id": "ch-1",
			"sender_id":  "user-s",
			"content":    "Channel msg",
			"created_at": float64(1700000000),
		},
	}

	event, err := sub.buildServerEvent(payload)
	if err != nil {
		t.Fatalf("buildServerEvent failed: %v", err)
	}

	if event.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED {
		t.Fatalf("expected CHANNEL_MESSAGE_RECEIVED, got %v", event.Type)
	}
	if event.ChannelMessageEvent.MessageId != "ch-msg-001" {
		t.Fatalf("expected message_id 'ch-msg-001', got %q", event.ChannelMessageEvent.MessageId)
	}
	if event.ChannelMessageEvent.ChannelId != "ch-1" {
		t.Fatalf("expected channel_id 'ch-1', got %q", event.ChannelMessageEvent.ChannelId)
	}

	t.Logf("build server event OK: type=%v", event.Type)
}

func TestPushSubscriber_buildServerEvent_UnknownType(t *testing.T) {
	sub := NewPushSubscriber(nil, nil)

	payload := PushPayload{
		EventType: "UNKNOWN_EVENT",
		Payload:   map[string]interface{}{},
	}

	_, err := sub.buildServerEvent(payload)
	if err == nil {
		t.Fatal("expected error for unknown event type, got nil")
	}

	t.Logf("correctly rejected unknown event type: %v", err)
}

func TestPushSubscriber_buildServerEvent_UserOnline(t *testing.T) {
	sub := NewPushSubscriber(nil, nil)

	payload := PushPayload{
		EventType: "USER_ONLINE",
		Payload: map[string]interface{}{
			"user_id": "user-online-1",
		},
	}

	event, err := sub.buildServerEvent(payload)
	if err != nil {
		t.Fatalf("buildServerEvent failed: %v", err)
	}

	if event.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_USER_ONLINE {
		t.Fatalf("expected USER_ONLINE, got %v", event.Type)
	}
	if event.UserOnlineEvent.UserId != "user-online-1" {
		t.Fatalf("expected user_id 'user-online-1', got %q", event.UserOnlineEvent.UserId)
	}

	t.Logf("build user online event OK: type=%v user_id=%s", event.Type, event.UserOnlineEvent.UserId)
}

func TestPushSubscriber_parseNATSMessage(t *testing.T) {
	mgr := NewConnManager()

	clientConn, serverConn := setupTestWS(t)
	defer serverConn.Close()

	mgr.Register("user-nats-test", serverConn)

	sub := NewPushSubscriber(nil, mgr)

	pushData := PushPayload{
		Targets:   []string{"user-nats-test"},
		EventType: "USER_OFFLINE",
		Payload: map[string]interface{}{
			"user_id": "user-offline-1",
		},
	}

	data, err := json.Marshal(pushData)
	if err != nil {
		t.Fatalf("marshal push data: %v", err)
	}

	natsMsg := &nats.Msg{Data: data}
	sub.handleNATSMessage(natsMsg)

	clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, frameData, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}

	serverEvent, err := DecodeFrame(frameData)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}

	if serverEvent.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_USER_OFFLINE {
		t.Fatalf("expected USER_OFFLINE, got %v", serverEvent.Type)
	}
	if serverEvent.UserOfflineEvent.UserId != "user-offline-1" {
		t.Fatalf("expected user_id 'user-offline-1', got %q", serverEvent.UserOfflineEvent.UserId)
	}

	t.Logf("NATS message handling OK: type=%v user_id=%s", serverEvent.Type, serverEvent.UserOfflineEvent.UserId)
}
