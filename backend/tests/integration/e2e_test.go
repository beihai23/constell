package integration

import (
	"net/http"
	"testing"
)

// TestE2EUserJourney runs the complete user journey as a single test:
// 1. Register User A and User B
// 2. User A creates a server
// 3. User A creates a channel "general"
// 4. User B joins the server
// 5. User A sends a channel message
// 6. User B reads the channel message
// 7. User A sends a DM to User B
// 8. User B reads the DM
func TestE2EUserJourney(t *testing.T) {
	// --- Step 1: Register User A and User B ---
	userA := registerUser(t)
	t.Logf("[step 1] Registered User A: id=%s email=%s", userA.UserID, userA.Email)

	userB := registerUser(t)
	t.Logf("[step 1] Registered User B: id=%s email=%s", userB.UserID, userB.Email)

	// Verify User A can fetch User B's profile.
	resp := doGet(t, apiURL("/api/v1/users/"+userB.UserID), userA.AccessToken)
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)
	var profileB struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
	}
	parseJSON(t, resp.Body, &profileB)
	if profileB.ID != userB.UserID {
		t.Fatalf("[step 1] expected user B id %q, got %q", userB.UserID, profileB.ID)
	}
	t.Logf("[step 1] User A can see User B's profile: nickname=%s", profileB.Nickname)

	// --- Step 2: User A creates a server ---
	srvResp := doPost(t, apiURL("/api/v1/servers"), userA.AccessToken, map[string]string{
		"name":        "E2E Test Server",
		"description": "Server for E2E testing",
	})
	defer srvResp.Body.Close()
	requireStatus(t, srvResp, http.StatusCreated)
	var server struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	parseJSON(t, srvResp.Body, &server)
	t.Logf("[step 2] User A created server: id=%s name=%s", server.ID, server.Name)

	// Verify server is retrievable.
	getSrvResp := doGet(t, apiURL("/api/v1/servers/"+server.ID), userA.AccessToken)
	defer getSrvResp.Body.Close()
	requireStatus(t, getSrvResp, http.StatusOK)
	var fetched struct {
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	parseJSON(t, getSrvResp.Body, &fetched)
	if fetched.OwnerID != userA.UserID {
		t.Fatalf("[step 2] expected owner %q, got %q", userA.UserID, fetched.OwnerID)
	}
	t.Logf("[step 2] Server verified: name=%s owner=%s", fetched.Name, fetched.OwnerID)

	// --- Step 3: User A creates a channel "general" ---
	chResp := doPost(t, apiURL("/api/v1/servers/"+server.ID+"/channels"), userA.AccessToken, map[string]string{
		"name": "general",
	})
	defer chResp.Body.Close()
	requireStatus(t, chResp, http.StatusCreated)
	var channel struct {
		ID       string `json:"id"`
		ServerID string `json:"server_id"`
		Name     string `json:"name"`
	}
	parseJSON(t, chResp.Body, &channel)
	t.Logf("[step 3] User A created channel: id=%s name=%s", channel.ID, channel.Name)

	// Verify channel in list.
	chListResp := doGet(t, apiURL("/api/v1/servers/"+server.ID+"/channels"), userA.AccessToken)
	defer chListResp.Body.Close()
	requireStatus(t, chListResp, http.StatusOK)
	var channelList struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	parseJSON(t, chListResp.Body, &channelList)
	chFound := false
	for _, c := range channelList.Channels {
		if c.ID == channel.ID && c.Name == "general" {
			chFound = true
			break
		}
	}
	if !chFound {
		t.Fatal("[step 3] expected to find 'general' in channel list")
	}
	t.Logf("[step 3] Channel verified in channel list")

	// --- Step 4: User B joins the server ---
	// The AddMember handler uses forwardAuth, so the joining user must call with their own token.
	joinResp := doPost(t, apiURL("/api/v1/servers/"+server.ID+"/members"), userB.AccessToken, map[string]string{
		"user_id": userB.UserID,
	})
	defer joinResp.Body.Close()
	requireStatus(t, joinResp, http.StatusCreated)
	t.Logf("[step 4] User B joined the server")

	// Verify User B in member list.
	memListResp := doGet(t, apiURL("/api/v1/servers/"+server.ID+"/members"), userA.AccessToken)
	defer memListResp.Body.Close()
	requireStatus(t, memListResp, http.StatusOK)
	var memberList struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}
	parseJSON(t, memListResp.Body, &memberList)
	memberFound := false
	for _, m := range memberList.Members {
		if m.UserID == userB.UserID {
			memberFound = true
			break
		}
	}
	if !memberFound {
		t.Fatal("[step 4] expected User B in member list")
	}
	t.Logf("[step 4] User B verified in member list (%d total)", len(memberList.Members))

	// --- Step 5: User A sends a message in #general ---
	msgContent := "Welcome to E2E Test Server!"
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userA.AccessToken, map[string]string{
		"content": msgContent,
	})
	defer msgResp.Body.Close()
	requireStatus(t, msgResp, http.StatusCreated)
	var sentMsg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		AuthorID  string `json:"author_id"`
		Content   string `json:"content"`
	}
	parseJSON(t, msgResp.Body, &sentMsg)
	if sentMsg.Content != msgContent {
		t.Fatalf("[step 5] content mismatch: got %q, want %q", sentMsg.Content, msgContent)
	}
	t.Logf("[step 5] User A sent message: id=%s", sentMsg.ID)

	// --- Step 6: User B reads the message from #general ---
	histResp := doGet(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), userB.AccessToken)
	defer histResp.Body.Close()
	requireStatus(t, histResp, http.StatusOK)
	var messages struct {
		Messages []struct {
			ID       string `json:"id"`
			AuthorID string `json:"author_id"`
			Content  string `json:"content"`
		} `json:"messages"`
	}
	parseJSON(t, histResp.Body, &messages)
	msgFound := false
	for _, m := range messages.Messages {
		if m.ID == sentMsg.ID && m.Content == msgContent {
			msgFound = true
			break
		}
	}
	if !msgFound {
		t.Fatalf("[step 6] message %s not found in channel history", sentMsg.ID)
	}
	t.Logf("[step 6] User B read the message (%d in channel)", len(messages.Messages))

	// --- Step 7: User A sends a DM to User B ---
	dmContent := "Hey User B, welcome!"
	dmResp := doPost(t, apiURL("/api/v1/dm/send"), userA.AccessToken, map[string]string{
		"target_user_id": userB.UserID,
		"content":        dmContent,
	})
	defer dmResp.Body.Close()
	requireStatus(t, dmResp, http.StatusCreated)
	var sentDM struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
		SenderID       string `json:"sender_id"`
		Content        string `json:"content"`
	}
	parseJSON(t, dmResp.Body, &sentDM)
	if sentDM.Content != dmContent {
		t.Fatalf("[step 7] DM content mismatch: got %q, want %q", sentDM.Content, dmContent)
	}
	t.Logf("[step 7] User A sent DM: id=%s conv=%s", sentDM.ID, sentDM.ConversationID)

	// --- Step 8: User B reads the DM ---
	dmHistResp := doGet(t, apiURL("/api/v1/dm/history/"+userA.UserID), userB.AccessToken)
	defer dmHistResp.Body.Close()
	requireStatus(t, dmHistResp, http.StatusOK)
	var dmHistory struct {
		Messages []struct {
			ID       string `json:"id"`
			SenderID string `json:"sender_id"`
			Content  string `json:"content"`
		} `json:"messages"`
	}
	parseJSON(t, dmHistResp.Body, &dmHistory)
	dmFound := false
	for _, m := range dmHistory.Messages {
		if m.ID == sentDM.ID && m.Content == dmContent {
			dmFound = true
			break
		}
	}
	if !dmFound {
		t.Fatalf("[step 8] DM %s not found in User B's history", sentDM.ID)
	}
	t.Logf("[step 8] User B read the DM (%d in conversation)", len(dmHistory.Messages))

	// --- Summary ---
	t.Log("========================================")
	t.Log("E2E User Journey Completed Successfully")
	t.Logf("  User A: %s (%s)", userA.UserID, userA.Email)
	t.Logf("  User B: %s (%s)", userB.UserID, userB.Email)
	t.Logf("  Server: %s", server.ID)
	t.Logf("  Channel: %s", channel.ID)
	t.Logf("  Channel Message: %s", sentMsg.ID)
	t.Logf("  DM Conversation: %s", sentDM.ConversationID)
	t.Logf("  DM Message: %s", sentDM.ID)
	t.Log("========================================")
}

// TestE2EHealthCheck verifies the API Gateway health endpoint.
func TestE2EHealthCheck(t *testing.T) {
	resp, err := http.Get(apiURL("/health"))
	if err != nil {
		t.Fatalf("gateway health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gateway health check returned status %d", resp.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
	}
	parseJSON(t, resp.Body, &result)

	if result.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", result.Status)
	}
	t.Log("API Gateway health check OK")
}
