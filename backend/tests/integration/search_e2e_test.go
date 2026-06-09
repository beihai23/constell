package integration

import (
	"net/http"
	"testing"
)

// TestSearchMessages verifies that a sent channel message can be found via search.
func TestSearchMessages(t *testing.T) {
	user := registerUser(t)

	// Create community + channel, send a message with a unique keyword.
	community := createTestCommunity(t, user.AccessToken)
	channel := createTestChannel(t, user.AccessToken, community.ID)

	uniqueKeyword := "e2e_search_" + uniqueNickname()
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), user.AccessToken, map[string]string{
		"content": "find me " + uniqueKeyword,
	})
	requireStatus(t, msgResp, http.StatusCreated)
	msgResp.Body.Close()
	t.Logf("sent message with keyword %q", uniqueKeyword)

	// Search for the message.
	searchResp := doGet(t, apiURL("/api/v1/search?q="+uniqueKeyword), user.AccessToken)
	defer searchResp.Body.Close()
	requireStatus(t, searchResp, http.StatusOK)

	var results struct {
		Messages []struct {
			ID       string `json:"id"`
			Content  string `json:"content"`
			AuthorID string `json:"author_id"`
		} `json:"messages"`
	}
	parseJSON(t, searchResp.Body, &results)

	found := false
	for _, m := range results.Messages {
		if m.Content == "find me "+uniqueKeyword {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("search did not find message with keyword %q (got %d messages)", uniqueKeyword, len(results.Messages))
	}
	t.Logf("search found message with keyword %q", uniqueKeyword)
}

// TestSearchUsers verifies that a user can be found via search.
func TestSearchUsers(t *testing.T) {
	userA := registerUser(t)
	userB := registerUser(t)
	t.Logf("user A: id=%s nickname=%s", userA.UserID, userA.Nickname)
	t.Logf("user B: id=%s nickname=%s", userB.UserID, userB.Nickname)

	// Search for user B's nickname (use a partial match of the unique nickname).
	searchResp := doGet(t, apiURL("/api/v1/search?q="+userB.Nickname), userA.AccessToken)
	defer searchResp.Body.Close()
	requireStatus(t, searchResp, http.StatusOK)

	var results struct {
		Users []struct {
			ID       string `json:"id"`
			Nickname string `json:"nickname"`
		} `json:"users"`
	}
	parseJSON(t, searchResp.Body, &results)

	found := false
	for _, u := range results.Users {
		if u.ID == userB.UserID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("search did not find user B (got %d users)", len(results.Users))
	}
	t.Logf("search found user B: %s", userB.Nickname)
}

// TestSearchNoResults verifies that searching for a nonexistent term returns empty results.
func TestSearchNoResults(t *testing.T) {
	user := registerUser(t)

	searchResp := doGet(t, apiURL("/api/v1/search?q=nonexistent_zzz_not_a_real_term_99999"), user.AccessToken)
	defer searchResp.Body.Close()
	requireStatus(t, searchResp, http.StatusOK)

	var results struct {
		Users     []interface{} `json:"users"`
		Messages  []interface{} `json:"messages"`
		DMMessages []interface{} `json:"dm_messages"`
	}
	parseJSON(t, searchResp.Body, &results)

	if len(results.Users) > 0 || len(results.Messages) > 0 || len(results.DMMessages) > 0 {
		t.Fatalf("expected empty results, got users=%d messages=%d dm_messages=%d",
			len(results.Users), len(results.Messages), len(results.DMMessages))
	}
	t.Log("search returned empty results as expected")
}
