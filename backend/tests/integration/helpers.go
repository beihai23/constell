package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// gatewayBaseURL returns the API Gateway base URL from the GATEWAY_URL env var,
// defaulting to http://localhost:8080.
func gatewayBaseURL() string {
	if v := os.Getenv("GATEWAY_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

// apiURL builds a full URL for the given path (e.g., /api/v1/auth/register).
func apiURL(path string) string {
	return gatewayBaseURL() + path
}

// uniqueEmail generates a unique email using the current nanosecond timestamp.
func uniqueEmail() string {
	return fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
}

// uniqueNickname generates a unique nickname using the current nanosecond timestamp.
func uniqueNickname() string {
	return fmt.Sprintf("TestUser%d", time.Now().UnixNano())
}

// testUser holds the credentials and tokens for a registered test user.
type testUser struct {
	Email        string
	Password     string
	Nickname     string
	UserID       string
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

// registerUser registers a new test user via POST /api/v1/auth/register.
func registerUser(t *testing.T) *testUser {
	t.Helper()

	email := uniqueEmail()
	password := "testPassword123!"
	nickname := uniqueNickname()

	resp := doPost(t, apiURL("/api/v1/auth/register"), "", map[string]string{
		"email":    email,
		"password": password,
		"nickname": nickname,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusCreated)

	var result struct {
		UserID       string `json:"user_id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, resp.Body, &result)

	return &testUser{
		Email:        email,
		Password:     password,
		Nickname:     nickname,
		UserID:       result.UserID,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	}
}

// loginUser logs in via POST /api/v1/auth/login and returns a refreshed testUser.
func loginUser(t *testing.T, email, password string) *testUser {
	t.Helper()

	resp := doPost(t, apiURL("/api/v1/auth/login"), "", map[string]string{
		"email":    email,
		"password": password,
	})
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		UserID       string `json:"user_id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	parseJSON(t, resp.Body, &result)

	return &testUser{
		Email:        email,
		Password:     password,
		UserID:       result.UserID,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
	}
}

// --- Low-level HTTP helpers ---

// doPost sends a POST with optional Bearer token and JSON body.
func doPost(t *testing.T, url, token string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// doGet sends a GET with Bearer token.
func doGet(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create GET request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// doPatch sends a PATCH with Bearer token and JSON body.
func doPatch(t *testing.T, url, token string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create PATCH request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}

// doDelete sends a DELETE with Bearer token.
func doDelete(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		t.Fatalf("create DELETE request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}

// --- Response helpers ---

// parseJSON decodes a JSON reader into v.
func parseJSON(t *testing.T, r io.Reader, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
}

// readBody reads and returns the response body as a string.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(b)
}

// requireStatus asserts the HTTP status code.
func requireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body := readBody(t, resp)
		t.Fatalf("expected status %d, got %d: %s", expected, resp.StatusCode, body)
	}
}
