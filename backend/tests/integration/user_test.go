package integration

import (
	"net/http"
	"testing"
)

func TestGetUserProfile(t *testing.T) {
	user := registerUser(t)

	resp := doGet(t, apiURL("/api/v1/users/"+user.UserID), user.AccessToken)
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	var profile struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		Nickname      string `json:"nickname"`
		AvatarURL     string `json:"avatar_url"`
		StatusMessage string `json:"status_message"`
		CreatedAt     int64  `json:"created_at"`
		UpdatedAt     int64  `json:"updated_at"`
	}
	parseJSON(t, resp.Body, &profile)

	if profile.ID != user.UserID {
		t.Fatalf("profile id mismatch: got %s, want %s", profile.ID, user.UserID)
	}
	if profile.Email != user.Email {
		t.Fatalf("profile email mismatch: got %s, want %s", profile.Email, user.Email)
	}
	if profile.Nickname != user.Nickname {
		t.Fatalf("profile nickname mismatch: got %s, want %s", profile.Nickname, user.Nickname)
	}
	t.Logf("get user profile OK: id=%s nickname=%s", profile.ID, profile.Nickname)
}

func TestUpdateProfile(t *testing.T) {
	user := registerUser(t)

	newNickname := uniqueNickname() + "-updated"
	newStatus := "testing status message"

	resp := doPatch(t, apiURL("/api/v1/users/"+user.UserID), user.AccessToken, map[string]string{
		"nickname":       newNickname,
		"status_message": newStatus,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		ID            string `json:"id"`
		Nickname      string `json:"nickname"`
		StatusMessage string `json:"status_message"`
	}
	parseJSON(t, resp.Body, &updated)

	if updated.Nickname != newNickname {
		t.Fatalf("nickname not updated: got %s, want %s", updated.Nickname, newNickname)
	}
	if updated.StatusMessage != newStatus {
		t.Fatalf("status_message not updated: got %s, want %s", updated.StatusMessage, newStatus)
	}
	t.Logf("update profile OK: nickname=%s status=%s", updated.Nickname, updated.StatusMessage)
}

func TestListFriendsEmpty(t *testing.T) {
	user := registerUser(t)

	resp := doGet(t, apiURL("/api/v1/users/"+user.UserID+"/friends"), user.AccessToken)
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		Friends []struct {
			ID       string `json:"id"`
			Nickname string `json:"nickname"`
		} `json:"friends"`
		HasMore bool `json:"has_more"`
	}
	parseJSON(t, resp.Body, &result)

	if len(result.Friends) != 0 {
		t.Fatalf("new user should have no friends, got %d", len(result.Friends))
	}
	t.Logf("list friends OK: empty list for new user")
}

func TestDMFlow(t *testing.T) {
	// Register two users.
	userA := registerUser(t)
	userB := registerUser(t)
	t.Logf("userA=%s userB=%s", userA.UserID, userB.UserID)

	// User A sends a DM to User B.
	content := "Hello from user A!"
	sendResp := doPost(t, apiURL("/api/v1/dm/send"), userA.AccessToken, map[string]string{
		"target_user_id": userB.UserID,
		"content":        content,
	})
	defer sendResp.Body.Close()
	requireStatus(t, sendResp, http.StatusCreated)

	var dm struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
		SenderID       string `json:"sender_id"`
		Content        string `json:"content"`
		CreatedAt      int64  `json:"created_at"`
	}
	parseJSON(t, sendResp.Body, &dm)

	if dm.ID == "" {
		t.Fatal("expected non-empty DM id")
	}
	if dm.SenderID != userA.UserID {
		t.Fatalf("sender mismatch: got %s, want %s", dm.SenderID, userA.UserID)
	}
	if dm.Content != content {
		t.Fatalf("content mismatch: got %s, want %s", dm.Content, content)
	}
	t.Logf("send DM OK: id=%s conv=%s", dm.ID, dm.ConversationID)

	// User A retrieves DM history with User B.
	histResp := doGet(t, apiURL("/api/v1/dm/history/"+userB.UserID), userA.AccessToken)
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
		t.Fatal("expected at least one DM in history")
	}
	found := false
	for _, m := range hist.Messages {
		if m.ID == dm.ID && m.Content == content {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sent DM not found in history")
	}
	t.Logf("DM history OK: %d messages", len(hist.Messages))

	// User B sends a reply to User A.
	replyContent := "Hi from user B!"
	replyResp := doPost(t, apiURL("/api/v1/dm/send"), userB.AccessToken, map[string]string{
		"target_user_id": userA.UserID,
		"content":        replyContent,
	})
	defer replyResp.Body.Close()
	requireStatus(t, replyResp, http.StatusCreated)

	var reply struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
		SenderID       string `json:"sender_id"`
		Content        string `json:"content"`
		CreatedAt      int64  `json:"created_at"`
	}
	parseJSON(t, replyResp.Body, &reply)

	if reply.SenderID != userB.UserID {
		t.Fatalf("reply sender mismatch: got %s, want %s", reply.SenderID, userB.UserID)
	}
	if reply.ConversationID != dm.ConversationID {
		t.Fatalf("reply should be in same conversation: got %s, want %s", reply.ConversationID, dm.ConversationID)
	}
	t.Logf("DM reply OK: same conversation_id=%s", reply.ConversationID)
}
