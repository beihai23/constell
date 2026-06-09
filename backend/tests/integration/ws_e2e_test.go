package integration

import (
	"net/http"
	"testing"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
)

// TestWSConnectAndAuth verifies a user can connect to the WS gateway with a valid JWT.
func TestWSConnectAndAuth(t *testing.T) {
	user := registerUser(t)
	t.Logf("registered user: id=%s", user.UserID)

	conn := connectWSDefault(t, user.AccessToken)
	defer conn.Close()

	t.Logf("WS connection established for user %s", user.UserID)
}

// TestWSInvalidToken verifies that an invalid JWT is rejected on WS upgrade.
func TestWSInvalidToken(t *testing.T) {
	host := "localhost"
	if h := wsBaseURL(); h != "ws://localhost:8081" {
		// Extract host from custom URL if needed.
		host = "localhost"
	}
	url := "ws://" + host + ":8081/ws?token=invalid-jwt-token"

	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("expected connection to be rejected with invalid token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("invalid token correctly rejected")
}

// TestWSHeartbeat verifies the heartbeat mechanism.
func TestWSHeartbeat(t *testing.T) {
	user := registerUser(t)
	conn := connectWSDefault(t, user.AccessToken)
	defer conn.Close()

	// Send a heartbeat.
	sendWSMessage(t, conn, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_HEARTBEAT,
		RequestId: "hb-001",
	})

	// Expect HEARTBEAT_ACK.
	event := readWSEvent(t, conn, 5*time.Second)
	if event.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_HEARTBEAT_ACK {
		t.Fatalf("expected HEARTBEAT_ACK, got %v", event.Type)
	}
	if event.RequestId != "hb-001" {
		t.Fatalf("expected request_id 'hb-001', got %q", event.RequestId)
	}
	t.Log("heartbeat ACK received")
}

// TestWSSubscribeUnsubscribeChannel verifies channel subscribe/unsubscribe flow.
func TestWSSubscribeUnsubscribeChannel(t *testing.T) {
	user := registerUser(t)

	// Create a community + channel via REST.
	community := createTestCommunity(t, user.AccessToken)
	channel := createTestChannel(t, user.AccessToken, community.ID)
	t.Logf("community=%s channel=%s", community.ID, channel.ID)

	conn := connectWSDefault(t, user.AccessToken)
	defer conn.Close()

	// Subscribe.
	sendWSMessage(t, conn, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
		RequestId: "sub-001",
		SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
			ChannelId: channel.ID,
		},
	})
	ack := readWSEvent(t, conn, 5*time.Second)
	if ack.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("expected ACK for subscribe, got %v", ack.Type)
	}
	t.Log("subscribe ACK received")

	// Unsubscribe.
	sendWSMessage(t, conn, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_UNSUBSCRIBE_CHANNEL,
		RequestId: "unsub-001",
		UnsubscribeChannelRequest: &gatewayv1.UnsubscribeChannelRequest{
			ChannelId: channel.ID,
		},
	})
	ack2 := readWSEvent(t, conn, 5*time.Second)
	if ack2.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("expected ACK for unsubscribe, got %v", ack2.Type)
	}
	t.Log("unsubscribe ACK received")
}

// TestWSChannelMessageRealtime verifies that a channel message sent via REST
// is received in real-time by a user subscribed to that channel via WS.
func TestWSChannelMessageRealtime(t *testing.T) {
	// Setup: register user, create community + channel, subscribe via WS.
	userA := registerUser(t)
	userB := registerUser(t)

	community := createTestCommunity(t, userA.AccessToken)
	channel := createTestChannel(t, userA.AccessToken, community.ID)

	// User B joins the community.
	joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), userB.AccessToken, map[string]string{
		"user_id": userB.UserID,
	})
	requireStatus(t, joinResp, http.StatusCreated)
	joinResp.Body.Close()

	// User B connects WS and subscribes to the channel.
	// (notify-service does not push messages back to the sender, so the receiver must subscribe.)
	conn := connectWSDefault(t, userB.AccessToken)
	defer conn.Close()

	sendWSMessage(t, conn, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
		RequestId: "sub-rt",
		SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
			ChannelId: channel.ID,
		},
	})
	// Consume subscribe ACK.
	ack := readWSEvent(t, conn, 5*time.Second)
	if ack.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("expected ACK for subscribe, got %v", ack.Type)
	}

	// User A (owner) sends a channel message via REST.
	msgContent := "realtime test message from A"
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": msgContent,
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()
	t.Logf("user A sent message via REST: %q", msgContent)

	// User B should receive the channel message event via WS.
	event := waitForEventType(t, conn, gatewayv1.ServerEventType_SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED, 10*time.Second)
	if event.ChannelMessageEvent == nil {
		t.Fatal("expected channel_message_event to be set")
	}
	if event.ChannelMessageEvent.Content != msgContent {
		t.Fatalf("expected content %q, got %q", msgContent, event.ChannelMessageEvent.Content)
	}
	if event.ChannelMessageEvent.SenderId != userA.UserID {
		t.Fatalf("expected sender %s, got %s", userA.UserID, event.ChannelMessageEvent.SenderId)
	}
	t.Logf("user B received real-time channel message from A")
}

// TestWSDMRealtime verifies DM delivery via WebSocket.
// User A is connected via WS, User B sends DM via REST, User A receives DM_RECEIVED.
func TestWSDMRealtime(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)

	// User A connects via WS.
	conn := connectWSDefault(t, userA.AccessToken)
	defer conn.Close()

	// Drain any initial events (e.g., USER_ONLINE).
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// User B sends DM to User A via REST.
	dmContent := "hello from B via DM"
	dmResp := doPost(t, apiURL("/api/v1/dm/send"), userB.AccessToken, map[string]string{
		"target_user_id": userA.UserID,
		"content":        dmContent,
	})
	requireStatus(t, dmResp, http.StatusCreated)
	var dmResult struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
	}
	parseJSON(t, dmResp.Body, &dmResult)
	dmResp.Body.Close()
	t.Logf("user B sent DM via REST: conv=%s msg=%s", dmResult.ConversationID, dmResult.ID)

	// User A should receive DM_RECEIVED via WS.
	event := waitForEventType(t, conn, gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED, 10*time.Second)
	if event.DmReceivedEvent == nil {
		t.Fatal("expected dm_received_event to be set")
	}
	if event.DmReceivedEvent.Content != dmContent {
		t.Fatalf("expected content %q, got %q", dmContent, event.DmReceivedEvent.Content)
	}
	if event.DmReceivedEvent.SenderId != userB.UserID {
		t.Fatalf("expected sender %s, got %s", userB.UserID, event.DmReceivedEvent.SenderId)
	}
	t.Logf("user A received real-time DM from user B")
}

// TestWSCrossGateway verifies that a message sent by a user on gateway-1
// is received by a user connected to gateway-2 (cross-gateway fan-out).
func TestWSCrossGateway(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)

	community := createTestCommunity(t, userA.AccessToken)
	channel := createTestChannel(t, userA.AccessToken, community.ID)

	// User B joins the community.
	joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), userB.AccessToken, map[string]string{
		"user_id": userB.UserID,
	})
	requireStatus(t, joinResp, http.StatusCreated)
	joinResp.Body.Close()

	// User A connects to gateway-1 (8081), User B connects to gateway-2 (8082).
	connA := connectWS(t, userA.AccessToken, 8081)
	defer connA.Close()
	connB := connectWS(t, userB.AccessToken, 8082)
	defer connB.Close()

	// Both subscribe to the channel.
	sendWSMessage(t, connA, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
		RequestId: "sub-a",
		SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
			ChannelId: channel.ID,
		},
	})
	ackA := readWSEvent(t, connA, 5*time.Second)
	if ackA.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("user A: expected ACK, got %v", ackA.Type)
	}

	sendWSMessage(t, connB, &gatewayv1.ClientMessage{
		Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
		RequestId: "sub-b",
		SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
			ChannelId: channel.ID,
		},
	})
	ackB := readWSEvent(t, connB, 5*time.Second)
	if ackB.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
		t.Fatalf("user B: expected ACK, got %v", ackB.Type)
	}

	// User A sends a channel message via REST.
	msgContent := "cross-gateway test message"
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": msgContent,
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()

	// User B (on gateway-2) should receive the message.
	event := waitForEventType(t, connB, gatewayv1.ServerEventType_SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED, 10*time.Second)
	if event.ChannelMessageEvent == nil {
		t.Fatal("expected channel_message_event to be set")
	}
	if event.ChannelMessageEvent.Content != msgContent {
		t.Fatalf("expected content %q, got %q", msgContent, event.ChannelMessageEvent.Content)
	}
	t.Log("cross-gateway message delivery verified")
}

// TestWSFanout verifies that a channel message is delivered to all subscribers.
func TestWSFanout(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)
	userC := registerUser(t)

	community := createTestCommunity(t, userA.AccessToken)
	channel := createTestChannel(t, userA.AccessToken, community.ID)

	// User B and C join the community.
	for _, u := range []*testUser{userB, userC} {
		joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), u.AccessToken, map[string]string{
			"user_id": u.UserID,
		})
		requireStatus(t, joinResp, http.StatusCreated)
		joinResp.Body.Close()
	}

	// All three subscribe via WS.
	conns := make([]*websocket.Conn, 3)
	users := []*testUser{userA, userB, userC}
	for i, u := range users {
		conns[i] = connectWSDefault(t, u.AccessToken)
		defer conns[i].Close()

		sendWSMessage(t, conns[i], &gatewayv1.ClientMessage{
			Type:      gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL,
			RequestId: "sub-fanout",
			SubscribeChannelRequest: &gatewayv1.SubscribeChannelRequest{
				ChannelId: channel.ID,
			},
		})
		// Consume subscribe ACK.
		ack := readWSEvent(t, conns[i], 5*time.Second)
		if ack.Type != gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK {
			t.Fatalf("user %d: expected ACK, got %v", i, ack.Type)
		}
	}

	// User A sends a channel message via REST.
	msgContent := "fanout test message"
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": msgContent,
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()

	// User B and C should both receive the message.
	for i := 1; i < 3; i++ {
		event := waitForEventType(t, conns[i], gatewayv1.ServerEventType_SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED, 10*time.Second)
		if event.ChannelMessageEvent == nil || event.ChannelMessageEvent.Content != msgContent {
			t.Fatalf("user %d: expected content %q", i, msgContent)
		}
		t.Logf("user %d received fanout message", i)
	}
	t.Log("fanout to all subscribers verified")
}

// =============================================================================
// Test helpers for creating community + channel
// =============================================================================

// createTestCommunity creates a community and returns its ID and name.
func createTestCommunity(t *testing.T, token string) struct{ ID, Name string } {
	t.Helper()
	resp := doPost(t, apiURL("/api/v1/communities"), token, map[string]string{
		"name":        uniqueNickname() + "-Community",
		"description": "test community",
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusCreated)
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	parseJSON(t, resp.Body, &result)
	return struct{ ID, Name string }{ID: result.ID, Name: result.Name}
}

// createTestChannel creates a channel in the given community and returns its ID and name.
func createTestChannel(t *testing.T, token, communityID string) struct{ ID, Name string } {
	t.Helper()
	resp := doPost(t, apiURL("/api/v1/communities/"+communityID+"/channels"), token, map[string]string{
		"name": "test-channel-" + uniqueNickname(),
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusCreated)
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	parseJSON(t, resp.Body, &result)
	return struct{ ID, Name string }{ID: result.ID, Name: result.Name}
}
