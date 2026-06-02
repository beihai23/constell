package integration

import (
	"net/http"
	"testing"
)

func TestCreateAndGetServer(t *testing.T) {
	user := registerUser(t)

	// Create a server.
	serverName := "TestServer-" + uniqueNickname()
	createResp := doPost(t, apiURL("/api/v1/servers"), user.AccessToken, map[string]string{
		"name":        serverName,
		"description": "A test server",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var server struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		OwnerID     string `json:"owner_id"`
		CreatedAt   int64  `json:"created_at"`
	}
	parseJSON(t, createResp.Body, &server)

	if server.ID == "" {
		t.Fatal("expected non-empty server id")
	}
	if server.Name != serverName {
		t.Fatalf("server name mismatch: got %s, want %s", server.Name, serverName)
	}
	if server.OwnerID != user.UserID {
		t.Fatalf("owner mismatch: got %s, want %s", server.OwnerID, user.UserID)
	}
	t.Logf("create server OK: id=%s name=%s", server.ID, server.Name)

	// Get the server.
	getResp := doGet(t, apiURL("/api/v1/servers/"+server.ID), user.AccessToken)
	defer getResp.Body.Close()
	requireStatus(t, getResp, http.StatusOK)

	var fetched struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OwnerID string `json:"owner_id"`
	}
	parseJSON(t, getResp.Body, &fetched)

	if fetched.ID != server.ID {
		t.Fatalf("fetched server id mismatch: got %s, want %s", fetched.ID, server.ID)
	}
	t.Logf("get server OK")
}

func TestUpdateServer(t *testing.T) {
	user := registerUser(t)

	// Create a server first.
	createResp := doPost(t, apiURL("/api/v1/servers"), user.AccessToken, map[string]string{
		"name": "OriginalName",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var server struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &server)

	// Update the server.
	newName := "UpdatedName"
	updateResp := doPatch(t, apiURL("/api/v1/servers/"+server.ID), user.AccessToken, map[string]string{
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
		t.Fatalf("server name not updated: got %s, want %s", updated.Name, newName)
	}
	t.Logf("update server OK")
}

func TestChannelCRUD(t *testing.T) {
	user := registerUser(t)

	// Create a server.
	createResp := doPost(t, apiURL("/api/v1/servers"), user.AccessToken, map[string]string{
		"name": "ChannelTestServer",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var server struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &server)

	// Create a channel.
	chName := "general"
	chResp := doPost(t, apiURL("/api/v1/servers/"+server.ID+"/channels"), user.AccessToken, map[string]string{
		"name": chName,
	})
	defer chResp.Body.Close()
	requireStatus(t, chResp, http.StatusCreated)

	var channel struct {
		ID       string `json:"id"`
		ServerID string `json:"server_id"`
		Name     string `json:"name"`
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
	listResp := doGet(t, apiURL("/api/v1/servers/"+server.ID+"/channels"), user.AccessToken)
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

	// Owner creates a server.
	createResp := doPost(t, apiURL("/api/v1/servers"), owner.AccessToken, map[string]string{
		"name": "MemberTestServer",
	})
	defer createResp.Body.Close()
	requireStatus(t, createResp, http.StatusCreated)

	var server struct {
		ID string `json:"id"`
	}
	parseJSON(t, createResp.Body, &server)

	// Member joins the server (uses their own auth token).
	joinResp := doPost(t, apiURL("/api/v1/servers/"+server.ID+"/members"), member.AccessToken, map[string]string{
		"user_id": member.UserID,
	})
	defer joinResp.Body.Close()
	requireStatus(t, joinResp, http.StatusCreated)

	var joinedMember struct {
		UserID   string `json:"user_id"`
		ServerID string `json:"server_id"`
	}
	parseJSON(t, joinResp.Body, &joinedMember)

	if joinedMember.UserID != member.UserID {
		t.Fatalf("joined member id mismatch: got %s, want %s", joinedMember.UserID, member.UserID)
	}
	t.Logf("member join OK: user=%s", joinedMember.UserID)

	// List members — should contain at least owner and member.
	listResp := doGet(t, apiURL("/api/v1/servers/"+server.ID+"/members"), owner.AccessToken)
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

	// Member leaves the server.
	leaveResp := doDelete(t, apiURL("/api/v1/servers/"+server.ID+"/members/"+member.UserID), member.AccessToken)
	defer leaveResp.Body.Close()
	requireStatus(t, leaveResp, http.StatusOK)
	t.Logf("member leave OK")
}

func TestChannelMessages(t *testing.T) {
	user := registerUser(t)

	// Create server + channel.
	srvResp := doPost(t, apiURL("/api/v1/servers"), user.AccessToken, map[string]string{
		"name": "MessageTestServer",
	})
	defer srvResp.Body.Close()
	requireStatus(t, srvResp, http.StatusCreated)
	var server struct{ ID string `json:"id"` }
	parseJSON(t, srvResp.Body, &server)

	chResp := doPost(t, apiURL("/api/v1/servers/"+server.ID+"/channels"), user.AccessToken, map[string]string{
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
