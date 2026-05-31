package integration

import (
	"testing"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"google.golang.org/protobuf/proto"
)

// TestGatewayProtoRoundTrip verifies the gateway proto types serialize/deserialize correctly.
func TestGatewayProtoRoundTrip(t *testing.T) {
	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM,
		RequestId: "test-001",
		SendDmRequest: &gatewayv1.SendDMRequest{
			ReceiverId: "user-b",
			Content:    "Hello from integration test",
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed := &gatewayv1.ClientMessage{}
	if err := proto.Unmarshal(data, parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Type != gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM {
		t.Fatalf("expected SEND_DM, got %v", parsed.Type)
	}
	if parsed.SendDmRequest.ReceiverId != "user-b" {
		t.Fatalf("expected receiver_id 'user-b', got %q", parsed.SendDmRequest.ReceiverId)
	}

	t.Logf("gateway proto round-trip OK: type=%v request_id=%s", parsed.Type, parsed.RequestId)
}

// TestServerEventRoundTrip verifies server event serialization.
func TestServerEventRoundTrip(t *testing.T) {
	event := &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED,
		RequestId: "ack-001",
		DmReceivedEvent: &gatewayv1.DMReceivedEvent{
			MessageId:      "msg-001",
			SenderId:       "user-a",
			SenderNickname: "Alice",
			Content:        "Hello Bob!",
			CreatedAt:      1700000000,
		},
	}

	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed := &gatewayv1.ServerEvent{}
	if err := proto.Unmarshal(data, parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED {
		t.Fatalf("expected DM_RECEIVED, got %v", parsed.Type)
	}
	if parsed.DmReceivedEvent.MessageId != "msg-001" {
		t.Fatalf("expected message_id 'msg-001', got %q", parsed.DmReceivedEvent.MessageId)
	}

	t.Logf("server event round-trip OK: type=%v message_id=%s", parsed.Type, parsed.DmReceivedEvent.MessageId)
}
