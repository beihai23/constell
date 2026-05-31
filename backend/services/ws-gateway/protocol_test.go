package main

import (
	"bytes"
	"testing"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"google.golang.org/protobuf/proto"
)

func TestEncodeDecodeFrame_RoundTrip(t *testing.T) {
	original := &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED,
		DmReceivedEvent: &gatewayv1.DMReceivedEvent{
			MessageId:      "msg-001",
			SenderId:       "user-abc",
			SenderNickname: "Alice",
			Content:        "Hello!",
			CreatedAt:      1700000000,
		},
	}

	encoded, err := EncodeFrame(original)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}
	if len(encoded) < 4 {
		t.Fatalf("encoded frame too short: %d bytes", len(encoded))
	}

	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Fatalf("expected type %v, got %v", original.Type, decoded.Type)
	}
	if decoded.DmReceivedEvent.MessageId != original.DmReceivedEvent.MessageId {
		t.Fatalf("expected message_id %q, got %q", original.DmReceivedEvent.MessageId, decoded.DmReceivedEvent.MessageId)
	}
	if decoded.DmReceivedEvent.Content != original.DmReceivedEvent.Content {
		t.Fatalf("expected content %q, got %q", original.DmReceivedEvent.Content, decoded.DmReceivedEvent.Content)
	}

	t.Logf("round-trip OK: type=%v message_id=%s", decoded.Type, decoded.DmReceivedEvent.MessageId)
}

func TestDecodeFrame_TooShort(t *testing.T) {
	_, err := DecodeFrame([]byte{0x00, 0x01})
	if err == nil {
		t.Fatal("expected error for frame too short, got nil")
	}
	t.Logf("correctly rejected short frame: %v", err)
}

func TestDecodeFrame_LengthMismatch(t *testing.T) {
	frame := []byte{0x00, 0x00, 0x00, 0x64, 0x01, 0x02, 0x03, 0x04}
	_, err := DecodeFrame(frame)
	if err == nil {
		t.Fatal("expected error for length mismatch, got nil")
	}
	t.Logf("correctly rejected mismatched length: %v", err)
}

func TestEncodeFrame_EmptyMessage(t *testing.T) {
	msg := &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK,
	}

	encoded, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame failed: %v", err)
	}

	if decoded.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK {
		t.Fatalf("expected HEARTBEAT_ACK, got %v", decoded.Type)
	}

	t.Log("empty message round-trip OK")
}

func TestEncodeDecodeClientMessage_RoundTrip(t *testing.T) {
	original := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM,
		RequestId: "req-123",
		SendDmRequest: &gatewayv1.SendDMRequest{
			ReceiverId: "user-def",
			Content:    "Hey there",
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}

	parsed := &gatewayv1.ClientMessage{}
	if err := proto.Unmarshal(data, parsed); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}

	if parsed.Type != original.Type {
		t.Fatalf("expected type %v, got %v", original.Type, parsed.Type)
	}
	if parsed.RequestId != original.RequestId {
		t.Fatalf("expected request_id %q, got %q", original.RequestId, parsed.RequestId)
	}
	if parsed.SendDmRequest.ReceiverId != original.SendDmRequest.ReceiverId {
		t.Fatalf("expected receiver_id %q, got %q", original.SendDmRequest.ReceiverId, parsed.SendDmRequest.ReceiverId)
	}

	t.Logf("client message round-trip OK: type=%v request_id=%s", parsed.Type, parsed.RequestId)
}

func TestDecodeFrame_InvalidProtobuf(t *testing.T) {
	frame := []byte{0x00, 0x00, 0x00, 0x04, 0xFF, 0xFF, 0xFF, 0xFF}
	_, err := DecodeFrame(frame)
	if err == nil {
		t.Fatal("expected error for invalid protobuf, got nil")
	}
	t.Logf("correctly rejected invalid protobuf: %v", err)
}

func TestEncodeFrame_LengthPrefixMatches(t *testing.T) {
	msg := &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: "req-999",
	}

	encoded, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	length := int(encoded[0])<<24 | int(encoded[1])<<16 | int(encoded[2])<<8 | int(encoded[3])
	payload := encoded[4:]

	if length != len(payload) {
		t.Fatalf("length prefix says %d but payload is %d bytes", length, len(payload))
	}

	serverEvent := &gatewayv1.ServerEvent{}
	if err := proto.Unmarshal(payload, serverEvent); err != nil {
		t.Fatalf("payload is not valid protobuf: %v", err)
	}

	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame failed: %v", err)
	}
	if decoded.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("expected ACK type, got %v", decoded.Type)
	}

	t.Logf("length prefix=%d, payload=%d bytes, round-trip OK", length, len(payload))
}

func TestDecodeFrame_BinaryWriterReader(t *testing.T) {
	msg := &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_USER_ONLINE,
		UserOnlineEvent: &gatewayv1.UserOnlineEvent{
			UserId: "user-online-1",
		},
	}

	encoded, err := EncodeFrame(msg)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	buf := bytes.NewReader(encoded)

	lenBuf := make([]byte, 4)
	if _, err := buf.Read(lenBuf); err != nil {
		t.Fatalf("read length prefix: %v", err)
	}

	length := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
	payload := make([]byte, length)
	if _, err := buf.Read(payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}

	parsed := &gatewayv1.ServerEvent{}
	if err := proto.Unmarshal(payload, parsed); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}

	if parsed.UserOnlineEvent.UserId != "user-online-1" {
		t.Fatalf("expected user_id 'user-online-1', got %q", parsed.UserOnlineEvent.UserId)
	}

	t.Log("binary buffer round-trip OK")
}
