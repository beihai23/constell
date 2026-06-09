package integration

import (
	"net/http"
	"testing"
)

// TestErrorNoToken verifies that accessing a protected route without a token returns 401.
func TestErrorNoToken(t *testing.T) {
	resp, err := http.Get(apiURL("/api/v1/users/some-user-id"))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
	t.Log("401 returned for unauthenticated request")
}

// TestErrorInvalidToken verifies that an invalid JWT returns 401.
func TestErrorInvalidToken(t *testing.T) {
	resp := doGet(t, apiURL("/api/v1/users/some-user-id"), "invalid.jwt.token")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid token, got %d", resp.StatusCode)
	}
	t.Log("401 returned for invalid token")
}

// TestErrorResourceNotFound verifies 404 for nonexistent resources.
func TestErrorResourceNotFound(t *testing.T) {
	user := registerUser(t)

	resp := doGet(t, apiURL("/api/v1/communities/nonexistent-id-12345"), user.AccessToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent community, got %d", resp.StatusCode)
	}
	t.Log("404 returned for nonexistent community")
}

// TestErrorDuplicateRegistration verifies that registering with an existing email returns 409.
func TestErrorDuplicateRegistration(t *testing.T) {
	user := registerUser(t)

	resp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    user.Email,
		"password": "anotherPassword123!",
		"nickname": "AnotherNick",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate email, got %d", resp.StatusCode)
	}
	t.Logf("409 returned for duplicate email %q", user.Email)
}

// TestErrorInvalidCredentials verifies login with wrong password.
func TestErrorInvalidCredentials(t *testing.T) {
	user := registerUser(t)

	resp := doPost(t, apiURL("/api/v1/auth/login"), "", map[string]string{
		"email":    user.Email,
		"password": "wrongPassword123!",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}
	t.Log("401 returned for wrong password")
}

// TestErrorSendDMToSelf verifies that sending a DM to yourself returns an error.
func TestErrorSendDMToSelf(t *testing.T) {
	user := registerUser(t)

	resp := doPost(t, apiURL("/api/v1/dm/send"), user.AccessToken, map[string]string{
		"target_user_id": user.UserID,
		"content":        "hello myself",
	})
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Fatalf("expected error when DMing self, got %d", resp.StatusCode)
	}
	t.Logf("error returned for self-DM (status %d)", resp.StatusCode)
}

// TestErrorChannelMessageNonMember verifies that a non-member cannot send channel messages.
func TestErrorChannelMessageNonMember(t *testing.T) {
	owner := registerUser(t)
	outsider := registerUser(t)

	community := createTestCommunity(t, owner.AccessToken)
	channel := createTestChannel(t, owner.AccessToken, community.ID)

	// Outsider tries to send a message.
	resp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), outsider.AccessToken, map[string]string{
		"content": "unauthorized message",
	})
	defer resp.Body.Close()

	// Should be rejected with 403 Forbidden.
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member message, got %d", resp.StatusCode)
	}
	t.Log("403 returned for non-member channel message")
}

// TestErrorEmptyRegistrationFields verifies that missing required fields return 400.
func TestErrorEmptyRegistrationFields(t *testing.T) {
	resp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    "",
		"password": "",
		"nickname": "",
	})
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx for empty registration fields, got %d", resp.StatusCode)
	}
	t.Logf("error returned for empty fields (status %d)", resp.StatusCode)
}
