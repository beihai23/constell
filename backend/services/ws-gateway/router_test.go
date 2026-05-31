package main

import (
	"context"
	"testing"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
)

type mockUserSvcClient struct {
	sendDMCalled bool
	lastSenderID string
	lastReceiver string
	lastContent  string
}

func (m *mockUserSvcClient) SendDM(ctx context.Context, senderID, receiverID, content string) (messageID string, createdAt string, err error) {
	m.sendDMCalled = true
	m.lastSenderID = senderID
	m.lastReceiver = receiverID
	m.lastContent = content
	return "dm-msg-001", "1700000000", nil
}

type mockCommunitySvcClient struct {
	sendMsgCalled bool
	lastSenderID  string
	lastChannelID string
	lastContent   string
}

func (m *mockCommunitySvcClient) SendMessage(ctx context.Context, senderID, channelID, content string) (messageID string, createdAt string, err error) {
	m.sendMsgCalled = true
	m.lastSenderID = senderID
	m.lastChannelID = channelID
	m.lastContent = content
	return "ch-msg-001", "1700000001", nil
}

func TestRouter_Route_SendDM(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}
	mgr := NewConnManager()

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM,
		RequestId: "req-dm-1",
		SendDmRequest: &gatewayv1.SendDMRequest{
			ReceiverId: "user-b",
			Content:    "Hello user B",
		},
	}

	ack, err := router.Route(context.Background(), "user-a", msg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if !userClient.sendDMCalled {
		t.Fatal("expected SendDM to be called")
	}
	if userClient.lastSenderID != "user-a" {
		t.Fatalf("expected sender 'user-a', got %q", userClient.lastSenderID)
	}
	if userClient.lastReceiver != "user-b" {
		t.Fatalf("expected receiver 'user-b', got %q", userClient.lastReceiver)
	}
	if userClient.lastContent != "Hello user B" {
		t.Fatalf("expected content 'Hello user B', got %q", userClient.lastContent)
	}

	if ack == nil {
		t.Fatal("expected non-nil ack")
	}
	if ack.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("expected ACK type, got %v", ack.Type)
	}

	t.Logf("route send_dm OK: sender=%s receiver=%s", userClient.lastSenderID, userClient.lastReceiver)
}

func TestRouter_Route_SendChannelMessage(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}
	mgr := NewConnManager()

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_CHANNEL_MESSAGE,
		RequestId: "req-ch-1",
		SendChannelMessageRequest: &gatewayv1.SendChannelMessageRequest{
			ChannelId: "ch-123",
			Content:   "Hello channel",
		},
	}

	ack, err := router.Route(context.Background(), "user-x", msg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if !communityClient.sendMsgCalled {
		t.Fatal("expected SendMessage to be called")
	}
	if communityClient.lastSenderID != "user-x" {
		t.Fatalf("expected sender 'user-x', got %q", communityClient.lastSenderID)
	}
	if communityClient.lastChannelID != "ch-123" {
		t.Fatalf("expected channel 'ch-123', got %q", communityClient.lastChannelID)
	}

	if ack == nil {
		t.Fatal("expected non-nil ack")
	}

	t.Logf("route send_channel_message OK: sender=%s channel=%s", communityClient.lastSenderID, communityClient.lastChannelID)
}

func TestRouter_Route_SubscribeChannel(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}

	_, conn := setupTestWS(t)
	defer conn.Close()

	mgr := NewConnManager()
	mgr.Register("user-sub", conn)

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
		RequestId: "req-sub-1",
		SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
			ChannelId: "ch-456",
		},
	}

	ack, err := router.Route(context.Background(), "user-sub", msg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if ack == nil {
		t.Fatal("expected non-nil ack")
	}

	entry, ok := mgr.Get("user-sub")
	if !ok {
		t.Fatal("expected user-sub to exist")
	}
	if !entry.SubscribedChannels["ch-456"] {
		t.Fatal("expected ch-456 to be in subscribed channels")
	}

	t.Log("route subscribe_channel OK")
}

func TestRouter_Route_UnsubscribeChannel(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}

	_, conn := setupTestWS(t)
	defer conn.Close()

	mgr := NewConnManager()
	mgr.Register("user-unsub", conn)
	mgr.AddSubscribedChannel("user-unsub", "ch-789")

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_UNSUBSCRIBE_CHANNEL,
		RequestId: "req-unsub-1",
		UnsubscribeChannelRequest: &gatewayv1.UnsubscribeChannelRequest{
			ChannelId: "ch-789",
		},
	}

	ack, err := router.Route(context.Background(), "user-unsub", msg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if ack == nil {
		t.Fatal("expected non-nil ack")
	}

	entry, ok := mgr.Get("user-unsub")
	if !ok {
		t.Fatal("expected user-unsub to exist")
	}
	if entry.SubscribedChannels["ch-789"] {
		t.Fatal("expected ch-789 to be removed from subscribed channels")
	}

	t.Log("route unsubscribe_channel OK")
}

func TestRouter_Route_UnknownType(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}
	mgr := NewConnManager()

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_UNSPECIFIED,
		RequestId: "req-unknown",
	}

	ack, err := router.Route(context.Background(), "user-y", msg)
	if err == nil {
		t.Fatal("expected error for unknown message type, got nil")
	}
	if ack != nil {
		t.Fatal("expected nil ack for unknown type")
	}

	t.Logf("correctly rejected unknown type: %v", err)
}

func TestRouter_Route_SendDM_MissingFields(t *testing.T) {
	userClient := &mockUserSvcClient{}
	communityClient := &mockCommunitySvcClient{}
	mgr := NewConnManager()

	router := NewRouter(userClient, communityClient, mgr)

	msg := &gatewayv1.ClientMessage{
		Type:          gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM,
		RequestId:     "req-dm-missing",
		SendDmRequest: &gatewayv1.SendDMRequest{},
	}

	_, err := router.Route(context.Background(), "user-z", msg)
	if err == nil {
		t.Fatal("expected error for missing fields, got nil")
	}

	t.Logf("correctly rejected missing fields: %v", err)
}
