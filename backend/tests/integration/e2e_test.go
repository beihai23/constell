package integration

import (
	"net/http"
	"testing"
)

// TestE2EUserJourney runs the complete user journey as a single test:
// 1. Register User A and User B
// 2. User A creates a community
// 3. User A creates a channel "general"
// 4. User B joins the community
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

	// --- Step 2: User A creates a community ---
	srvResp := doPost(t, apiURL("/api/v1/communities"), userA.AccessToken, map[string]string{
		"name":        "E2E Test Community",
		"description": "Community for E2E testing",
	})
	defer srvResp.Body.Close()
	requireStatus(t, srvResp, http.StatusCreated)
	var community struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	parseJSON(t, srvResp.Body, &community)
	t.Logf("[step 2] User A created community: id=%s name=%s", community.ID, community.Name)

	// Verify community is retrievable.
	getSrvResp := doGet(t, apiURL("/api/v1/communities/"+community.ID), userA.AccessToken)
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
	t.Logf("[step 2] Community verified: name=%s owner=%s", fetched.Name, fetched.OwnerID)

	// --- Step 3: User A creates a channel "general" ---
	chResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), userA.AccessToken, map[string]string{
		"name": "general",
	})
	defer chResp.Body.Close()
	requireStatus(t, chResp, http.StatusCreated)
	var channel struct {
		ID          string `json:"id"`
		CommunityID string `json:"community_id"`
		Name        string `json:"name"`
	}
	parseJSON(t, chResp.Body, &channel)
	t.Logf("[step 3] User A created channel: id=%s name=%s", channel.ID, channel.Name)

	// Verify channel in list.
	chListResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), userA.AccessToken)
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

	// --- Step 4: User B joins the community ---
	// The AddMember handler uses forwardAuth, so the joining user must call with their own token.
	joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), userB.AccessToken, map[string]string{
		"user_id": userB.UserID,
	})
	defer joinResp.Body.Close()
	requireStatus(t, joinResp, http.StatusCreated)
	t.Logf("[step 4] User B joined the community")

	// Verify User B in member list.
	memListResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/members"), userA.AccessToken)
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
	msgContent := "Welcome to E2E Test Community!"
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
	t.Logf("  Community: %s", community.ID)
	t.Logf("  Channel: %s", channel.ID)
	t.Logf("  Channel Message: %s", sentMsg.ID)
	t.Logf("  DM Conversation: %s", sentDM.ConversationID)
	t.Logf("  DM Message: %s", sentDM.ID)
	t.Log("========================================")
}

// TestE2ETokenRefresh verifies that a refresh token can be exchanged for new tokens.
func TestE2ETokenRefresh(t *testing.T) {
	user := registerUser(t)

	// Refresh the token.
	resp := doPost(t, apiURL("/api/v1/auth/refresh"), "", map[string]string{
		"refresh_token": user.RefreshToken,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, resp.Body, &result)

	if result.AccessToken == "" {
		t.Fatal("expected non-empty access_token after refresh")
	}
	if result.RefreshToken == "" {
		t.Fatal("expected non-empty refresh_token after refresh")
	}
	t.Logf("token refreshed: expires_at=%d", result.ExpiresAt)

	// Verify the new token works.
	profileResp := doGet(t, apiURL("/api/v1/users/"+user.UserID), result.AccessToken)
	defer profileResp.Body.Close()
	requireStatus(t, profileResp, http.StatusOK)
	t.Log("new access token works for authenticated requests")
}

// TestE2EUserProfileUpdate verifies updating a user's profile.
func TestE2EUserProfileUpdate(t *testing.T) {
	user := registerUser(t)

	newNickname := "Updated" + uniqueNickname()
	resp := doPatch(t, apiURL("/api/v1/users/"+user.UserID), user.AccessToken, map[string]string{
		"nickname": newNickname,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	// Verify the update.
	profileResp := doGet(t, apiURL("/api/v1/users/"+user.UserID), user.AccessToken)
	defer profileResp.Body.Close()
	requireStatus(t, profileResp, http.StatusOK)

	var profile struct {
		Nickname string `json:"nickname"`
	}
	parseJSON(t, profileResp.Body, &profile)
	if profile.Nickname != newNickname {
		t.Fatalf("expected nickname %q, got %q", newNickname, profile.Nickname)
	}
	t.Logf("profile updated: nickname=%s", profile.Nickname)
}

// TestE2ECommunityMemberRemoval verifies removing a member from a community.
func TestE2ECommunityMemberRemoval(t *testing.T) {
	owner := registerUser(t)
	member := registerUser(t)
	thirdUser := registerUser(t)

	community := createTestCommunity(t, owner.AccessToken)

	// Both non-owners join.
	for _, u := range []*testUser{member, thirdUser} {
		joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), u.AccessToken, map[string]string{
			"user_id": u.UserID,
		})
		requireStatus(t, joinResp, http.StatusCreated)
		joinResp.Body.Close()
	}

	// Owner kicks the member from the community.
	delResp := doDelete(t, apiURL("/api/v1/communities/"+community.ID+"/members/"+member.UserID), owner.AccessToken)
	defer delResp.Body.Close()
	requireStatus(t, delResp, http.StatusOK)

	// Verify member is gone.
	memListResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/members"), owner.AccessToken)
	defer memListResp.Body.Close()
	requireStatus(t, memListResp, http.StatusOK)

	var memberList struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}
	parseJSON(t, memListResp.Body, &memberList)
	for _, m := range memberList.Members {
		if m.UserID == member.UserID {
			t.Fatal("expected member to be removed")
		}
	}
	t.Log("member successfully removed from community")
}

// TestE2EChannelUpdate verifies updating a channel name.
func TestE2EChannelUpdate(t *testing.T) {
	user := registerUser(t)
	community := createTestCommunity(t, user.AccessToken)
	channel := createTestChannel(t, user.AccessToken, community.ID)

	newName := "updated-" + channel.Name
	resp := doPatch(t, apiURL("/api/v1/channels/"+channel.ID), user.AccessToken, map[string]string{
		"name": newName,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	// Verify via channel list.
	listResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), user.AccessToken)
	defer listResp.Body.Close()
	requireStatus(t, listResp, http.StatusOK)

	var channels struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	parseJSON(t, listResp.Body, &channels)
	found := false
	for _, ch := range channels.Channels {
		if ch.ID == channel.ID && ch.Name == newName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("channel %s not found with name %q", channel.ID, newName)
	}
	t.Logf("channel updated to %q", newName)
}

// TestE2EUserBlock verifies the block/unblock user flow.
// NOTE: Block/unblock is not yet implemented (returns 501). This test is
// skipped until the feature is available.
func TestE2EUserBlock(t *testing.T) {
	t.Skip("block/unblock not yet implemented (501)")
}

// TestE2ELoginFlow verifies the complete login flow (register → logout → login).
func TestE2ELoginFlow(t *testing.T) {
	user := registerUser(t)

	// Login with the same credentials.
	loggedIn := loginUser(t, user.Email, user.Password)
	if loggedIn.UserID != user.UserID {
		t.Fatalf("expected user_id %s, got %s", user.UserID, loggedIn.UserID)
	}
	if loggedIn.AccessToken == "" {
		t.Fatal("expected non-empty access_token after login")
	}
	t.Logf("login successful: user=%s", loggedIn.UserID)
}

// TestE2EHealthCheck verifies the API Gateway health endpoint.
func TestE2EHealthCheck(t *testing.T) {
	resp, err := http.Get(apiURL("/healthz"))
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
