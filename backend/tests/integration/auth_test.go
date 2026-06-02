package integration

import (
	"net/http"
	"testing"
)

func TestRegisterLoginRefresh(t *testing.T) {
	// Step 1: Register a new user.
	email := uniqueEmail()
	password := "testPassword123!"
	nickname := uniqueNickname()

	regResp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    email,
		"password": password,
		"nickname": nickname,
	})
	defer regResp.Body.Close()
	requireStatus(t, regResp, http.StatusCreated)

	var regResult struct {
		UserID       string `json:"user_id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, regResp.Body, &regResult)

	if regResult.UserID == "" {
		t.Fatal("expected non-empty user_id after registration")
	}
	if regResult.AccessToken == "" {
		t.Fatal("expected non-empty access_token after registration")
	}
	if regResult.RefreshToken == "" {
		t.Fatal("expected non-empty refresh_token after registration")
	}
	if regResult.ExpiresAt == 0 {
		t.Fatal("expected non-zero expires_at after registration")
	}
	t.Logf("registered user %s (id=%s)", email, regResult.UserID)

	// Step 2: Login with the same credentials.
	loginResp := doPost(t, apiURL("/api/v1/auth/login"), "", map[string]string{
		"email":    email,
		"password": password,
	})
	defer loginResp.Body.Close()
	requireStatus(t, loginResp, http.StatusOK)

	var loginResult struct {
		UserID       string `json:"user_id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, loginResp.Body, &loginResult)

	if loginResult.UserID != regResult.UserID {
		t.Fatalf("login user_id mismatch: got %s, want %s", loginResult.UserID, regResult.UserID)
	}
	if loginResult.AccessToken == "" {
		t.Fatal("expected non-empty access_token after login")
	}
	t.Logf("login OK, new access token issued")

	// Step 3: Refresh the token using the login refresh token.
	refreshResp := doPost(t, apiURL("/api/v1/auth/refresh"), "", map[string]string{
		"refresh_token": loginResult.RefreshToken,
	})
	defer refreshResp.Body.Close()
	requireStatus(t, refreshResp, http.StatusOK)

	var refreshResult struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, refreshResp.Body, &refreshResult)

	if refreshResult.AccessToken == "" {
		t.Fatal("expected non-empty access_token after refresh")
	}
	if refreshResult.RefreshToken == "" {
		t.Fatal("expected non-empty refresh_token after refresh")
	}
	t.Logf("token refresh OK")
}

func TestRegisterDuplicateEmail(t *testing.T) {
	email := uniqueEmail()
	password := "testPassword123!"
	nickname := uniqueNickname()

	// First registration succeeds.
	regResp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    email,
		"password": password,
		"nickname": nickname,
	})
	regResp.Body.Close()
	requireStatus(t, regResp, http.StatusCreated)

	// Second registration with same email fails.
	dupResp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    email,
		"password": password,
		"nickname": uniqueNickname(),
	})
	defer dupResp.Body.Close()

	if dupResp.StatusCode == http.StatusCreated {
		t.Fatal("expected duplicate email registration to fail, but got 201")
	}
	t.Logf("duplicate email correctly rejected with status %d", dupResp.StatusCode)
}

func TestLoginBadCredentials(t *testing.T) {
	// Login with non-existent email should fail.
	resp := doPost(t, apiURL("/api/v1/auth/login"), "", map[string]string{
		"email":    "nonexistent-" + uniqueEmail(),
		"password": "wrongPassword!",
	})
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected login with bad credentials to fail, but got 200")
	}
	t.Logf("bad credentials correctly rejected with status %d", resp.StatusCode)
}

func TestRefreshWithInvalidToken(t *testing.T) {
	resp := doPost(t, apiURL("/api/v1/auth/refresh"), "", map[string]string{
		"refresh_token": "this-is-not-a-valid-token",
	})
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected refresh with invalid token to fail, but got 200")
	}
	t.Logf("invalid refresh token correctly rejected with status %d", resp.StatusCode)
}

func TestAuthenticatedEndpointRejectsBadToken(t *testing.T) {
	// Accessing a protected endpoint with an invalid token should return 401.
	resp := doGet(t, apiURL("/api/v1/users/me"), "invalid-token-value")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d", resp.StatusCode)
	}
	t.Logf("protected endpoint correctly returned 401")
}
