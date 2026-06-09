package integration

import (
	"net/http"
	"testing"
)

// TestNotifyDMUnread verifies that sending a DM creates an unread notification.
func TestNotifyDMUnread(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)
	t.Logf("user A: %s, user B: %s", userA.UserID, userB.UserID)

	// User B sends DM to User A.
	dmResp := doPost(t, apiURL("/api/v1/dm/send"), userB.AccessToken, map[string]string{
		"target_user_id": userA.UserID,
		"content":        "unread test DM",
	})
	requireStatus(t, dmResp, http.StatusCreated)
	var dm struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
	}
	parseJSON(t, dmResp.Body, &dm)
	dmResp.Body.Close()
	t.Logf("DM sent: conv=%s msg=%s", dm.ConversationID, dm.ID)

	// User A checks unread count.
	unreadResp := doGet(t, apiURL("/api/v1/notify/unread"), userA.AccessToken)
	defer unreadResp.Body.Close()
	requireStatus(t, unreadResp, http.StatusOK)

	var unread struct {
		DMTotal        int `json:"dm_total"`
		ChannelTotal   int `json:"channel_total"`
		DMConversations []struct {
			ConversationID string `json:"conversation_id"`
			Count          int    `json:"count"`
		} `json:"dm_conversations"`
	}
	parseJSON(t, unreadResp.Body, &unread)

	if unread.DMTotal < 1 {
		t.Fatalf("expected dm_total >= 1, got %d", unread.DMTotal)
	}
	t.Logf("unread: dm_total=%d, channel_total=%d", unread.DMTotal, unread.ChannelTotal)
}

// TestNotifyMarkDMRead verifies that marking a DM conversation as read clears the unread count.
func TestNotifyMarkDMRead(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)

	// User B sends DM to User A.
	dmResp := doPost(t, apiURL("/api/v1/dm/send"), userB.AccessToken, map[string]string{
		"target_user_id": userA.UserID,
		"content":        "mark read test",
	})
	requireStatus(t, dmResp, http.StatusCreated)
	var dm struct {
		ConversationID string `json:"conversation_id"`
	}
	parseJSON(t, dmResp.Body, &dm)
	dmResp.Body.Close()
	t.Logf("DM sent to conv=%s", dm.ConversationID)

	// Mark the DM conversation as read.
	readResp := doPost(t, apiURL("/api/v1/notify/dm/"+dm.ConversationID+"/read"), userA.AccessToken, nil)
	defer readResp.Body.Close()
	requireStatus(t, readResp, http.StatusOK)
	t.Logf("marked conv %s as read", dm.ConversationID)

	// Verify unread count is now 0 for that conversation.
	unreadResp := doGet(t, apiURL("/api/v1/notify/unread"), userA.AccessToken)
	defer unreadResp.Body.Close()
	requireStatus(t, unreadResp, http.StatusOK)

	var unread struct {
		DMConversations []struct {
			ConversationID string `json:"conversation_id"`
			Count          int    `json:"count"`
		} `json:"dm_conversations"`
	}
	parseJSON(t, unreadResp.Body, &unread)

	for _, c := range unread.DMConversations {
		if c.ConversationID == dm.ConversationID && c.Count > 0 {
			t.Fatalf("expected 0 unread for conv %s, got %d", dm.ConversationID, c.Count)
		}
	}
	t.Log("DM unread count confirmed as 0 after marking read")
}

// TestNotifyChannelUnread verifies that sending a channel message creates an unread notification.
func TestNotifyChannelUnread(t *testing.T) {
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

	// User A sends a channel message.
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": "channel unread test message",
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()

	// User B checks unread count.
	unreadResp := doGet(t, apiURL("/api/v1/notify/unread"), userB.AccessToken)
	defer unreadResp.Body.Close()
	requireStatus(t, unreadResp, http.StatusOK)

	var unread struct {
		ChannelTotal int `json:"channel_total"`
	}
	parseJSON(t, unreadResp.Body, &unread)

	if unread.ChannelTotal < 1 {
		t.Fatalf("expected channel_total >= 1, got %d", unread.ChannelTotal)
	}
	t.Logf("channel unread: total=%d", unread.ChannelTotal)
}

// TestNotifyMarkChannelRead verifies marking a channel as read.
func TestNotifyMarkChannelRead(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)

	community := createTestCommunity(t, userA.AccessToken)
	channel := createTestChannel(t, userA.AccessToken, community.ID)

	// User B joins.
	joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), userB.AccessToken, map[string]string{
		"user_id": userB.UserID,
	})
	requireStatus(t, joinResp, http.StatusCreated)
	joinResp.Body.Close()

	// User A sends a message.
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": "channel mark read test",
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()

	// User B marks the channel as read.
	readResp := doPost(t, apiURL("/api/v1/notify/channel/"+channel.ID+"/read"), userB.AccessToken, nil)
	defer readResp.Body.Close()
	requireStatus(t, readResp, http.StatusOK)

	// Verify unread for that channel is now 0.
	unreadResp := doGet(t, apiURL("/api/v1/notify/unread"), userB.AccessToken)
	defer unreadResp.Body.Close()
	requireStatus(t, unreadResp, http.StatusOK)

	var unread struct {
		Channels []struct {
			ChannelID string `json:"channel_id"`
			Count     int    `json:"count"`
		} `json:"channels"`
	}
	parseJSON(t, unreadResp.Body, &unread)

	for _, ch := range unread.Channels {
		if ch.ChannelID == channel.ID && ch.Count > 0 {
			t.Fatalf("expected 0 unread for channel %s, got %d", channel.ID, ch.Count)
		}
	}
	t.Log("channel unread count confirmed as 0 after marking read")
}
