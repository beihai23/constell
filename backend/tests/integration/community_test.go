package integration

import (
	"net/http"
	"testing"
)

func TestCreateAndGetCommunity(t *testing.T) {
	user := registerUser(t)

	// Create a community.
	communityName := "TestCommunity-" + uniqueNickname()
	createResp := doPost(t, apiURL("/api/v1/communities"), user.AccessToken, map[string]string{
		"name":        communityName,
		"description": "A test community",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var community struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		OwnerID     string `json:"owner_id"`
		CreatedAt   int64  `json:"created_at"`
	}
	parseJSON(t, createResp.Body, &community)

	if community.ID == "" {
		t.Fatal("expected non-empty community id")
	}
	if community.Name != communityName {
		t.Fatalf("community name mismatch: got %s, want %s", community.Name, communityName)
	}
	if community.OwnerID != user.UserID {
		t.Fatalf("owner mismatch: got %s, want %s", community.OwnerID, user.UserID)
	}
	t.Logf("create community OK: id=%s name=%s", community.ID, community.Name)

	// Get the community.
	getResp := doGet(t, apiURL("/api/v1/communities/"+community.ID), user.AccessToken)
	defer getResp.Body.Close()
	requireStatus(t, getResp, http.StatusOK)

	var fetched struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	parseJSON(t, getResp.Body, &fetched)

	if fetched.ID != community.ID {
		t.Fatalf("fetched community id mismatch: got %s, want %s", fetched.ID, community.ID)
	}
	t.Logf("get community OK")
}

func TestUpdateCommunity(t *testing.T) {
	user := registerUser(t)

	// Create a community first.
	createResp := doPost(t, apiURL("/api/v1/communities"), user.AccessToken, map[string]string{
		"name": "OriginalName",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var community struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &community)

	// Update the community.
	newName := "UpdatedName"
	updateResp := doPatch(t, apiURL("/api/v1/communities/"+community.ID), user.AccessToken, map[string]string{
		"name":        newName,
		"description": "Updated description",
	})
	defer updateResp.Body.Close()
	requireStatus(t, updateResp, http.StatusOK)

	var updated struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	parseJSON(t, updateResp.Body, &updated)

	if updated.Name != newName {
		t.Fatalf("community name not updated: got %s, want %s", updated.Name, newName)
	}
	t.Logf("update community OK")
}

func TestChannelCRUD(t *testing.T) {
	user := registerUser(t)

	// Create a community.
	createResp := doPost(t, apiURL("/api/v1/communities"), user.AccessToken, map[string]string{
		"name": "ChannelTestCommunity",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var community struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &community)

	// Create a channel.
	chName := "general"
	chResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), user.AccessToken, map[string]string{
		"name": chName,
	})
	defer chResp.Body.Close()
	requireStatus(t, chResp, http.StatusCreated)

	var channel struct {
		ID          string `json:"id"`
		CommunityID string `json:"community_id"`
		Name        string `json:"name"`
	}
	parseJSON(t, chResp.Body, &channel)

	if channel.ID == "" {
		t.Fatal("expected non-empty channel id")
	}
	if channel.Name != chName {
		t.Fatalf("channel name mismatch: got %s, want %s", channel.Name, chName)
	}
	t.Logf("create channel OK: id=%s name=%s", channel.ID, channel.Name)

	// List channels.
	listResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), user.AccessToken)
	defer listResp.Body.Close()
	requireStatus(t, listResp, http.StatusOK)

	var list struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	parseJSON(t, listResp.Body, &list)

	found := false
	for _, c := range list.Channels {
		if c.ID == channel.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created channel not found in channel list")
	}
	t.Logf("list channels OK: %d channels", len(list.Channels))

	// Update channel.
	updatedName := "general-updated"
	updResp := doPatch(t, apiURL("/api/v1/channels/"+channel.ID), user.AccessToken, map[string]string{
		"name":  updatedName,
		"topic": "New topic",
	})
	defer updResp.Body.Close()
	requireStatus(t, updResp, http.StatusOK)

	var updCh struct {
		Name  string `json:"name"`
		Topic string `json:"topic"`
	}
	parseJSON(t, updResp.Body, &updCh)

	if updCh.Name != updatedName {
		t.Fatalf("channel name not updated: got %s, want %s", updCh.Name, updatedName)
	}
	t.Logf("update channel OK")
}

func TestMemberJoinLeave(t *testing.T) {
	owner := registerUser(t)
	member := registerUser(t)

	// Owner creates a community.
	createResp := doPost(t, apiURL("/api/v1/communities"), owner.AccessToken, map[string]string{
		"name": "MemberTestCommunity",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var community struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &community)

	// Member joins the community (uses their own auth token).
	joinResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/members"), member.AccessToken, map[string]string{
		"user_id": member.UserID,
	})
	defer joinResp.Body.Close()
	requireStatus(t, joinResp, http.StatusCreated)

	var joinedMember struct {
		UserID      string `json:"user_id"`
		CommunityID string `json:"community_id"`
	}
	parseJSON(t, joinResp.Body, &joinedMember)

	if joinedMember.UserID != member.UserID {
		t.Fatalf("joined member id mismatch: got %s, want %s", joinedMember.UserID, member.UserID)
	}
	t.Logf("member join OK: user=%s", joinedMember.UserID)

	// List members — should contain at least owner and member.
	listResp := doGet(t, apiURL("/api/v1/communities/"+community.ID+"/members"), owner.AccessToken)
	defer listResp.Body.Close()
	requireStatus(t, listResp, http.StatusOK)

	var list struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}
	parseJSON(t, listResp.Body, &list)

	if len(list.Members) < 2 {
		t.Fatalf("expected at least 2 members, got %d", len(list.Members))
	}
	t.Logf("list members OK: %d members", len(list.Members))

	// Member leaves the community.
	leaveResp := doDelete(t, apiURL("/api/v1/communities/"+community.ID+"/members/"+member.UserID), member.AccessToken)
	defer leaveResp.Body.Close()
	requireStatus(t, leaveResp, http.StatusOK)
	t.Logf("member leave OK")
}

func TestChannelMessages(t *testing.T) {
	user := registerUser(t)

	// Create community + channel.
	srvResp := doPost(t, apiURL("/api/v1/communities"), user.AccessToken, map[string]string{
		"name": "MessageTestCommunity",
	})
	defer srvResp.Body.Close()
	requireStatus(t, srvResp, http.StatusCreated)
	var community struct{ ID string `json:"id"` }
	parseJSON(t, srvResp.Body, &community)

	chResp := doPost(t, apiURL("/api/v1/communities/"+community.ID+"/channels"), user.AccessToken, map[string]string{
		"name": "chat",
	})
	defer chResp.Body.Close()
	requireStatus(t, chResp, http.StatusCreated)
	var channel struct{ ID string `json:"id"` }
	parseJSON(t, chResp.Body, &channel)

	// Send a message.
	msgContent := "Hello channel!"
	sendResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), user.AccessToken, map[string]string{
		"content": msgContent,
	})
	defer sendResp.Body.Close()
	requireStatus(t, sendResp, http.StatusCreated)

	var msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		AuthorID  string `json:"author_id"`
		Content   string `json:"content"`
		CreatedAt int64  `json:"created_at"`
	}
	parseJSON(t, sendResp.Body, &msg)

	if msg.ID == "" {
		t.Fatal("expected non-empty message id")
	}
	if msg.AuthorID != user.UserID {
		t.Fatalf("author mismatch: got %s, want %s", msg.AuthorID, user.UserID)
	}
	if msg.Content != msgContent {
		t.Fatalf("content mismatch: got %s, want %s", msg.Content, msgContent)
	}
	t.Logf("send message OK: id=%s", msg.ID)

	// Get message history.
	histResp := doGet(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), user.AccessToken)
	defer histResp.Body.Close()
	requireStatus(t, histResp, http.StatusOK)

	var hist struct {
		Messages []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	parseJSON(t, histResp.Body, &hist)

	if len(hist.Messages) == 0 {
		t.Fatal("expected at least one message in history")
	}
	found := false
	for _, m := range hist.Messages {
		if m.ID == msg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sent message not found in history")
	}
	t.Logf("message history OK: %d messages", len(hist.Messages))
}
