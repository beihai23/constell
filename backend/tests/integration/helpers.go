package integration

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"testing"
	"time"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
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

// =============================================================================
// WebSocket helpers
// =============================================================================

// wsBaseURL returns the WS Gateway base URL from the WS_GATEWAY_URL env var,
// defaulting to ws://localhost:8081.
func wsBaseURL() string {
	if v := os.Getenv("WS_GATEWAY_URL"); v != "" {
		return v
	}
	return "ws://localhost:8081"
}

// connectWS dials a WebSocket connection to the gateway with JWT token auth.
func connectWS(t *testing.T, token string, port int) *websocket.Conn {
	t.Helper()
	host := "localhost"
	if h := os.Getenv("WS_GATEWAY_HOST"); h != "" {
		host = h
	}
	url := fmt.Sprintf("ws://%s:%d/ws?token=%s", host, port, token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial WS %s: %v", url, err)
	}
	return conn
}

// connectWSDefault connects to the default WS gateway (port 8081).
func connectWSDefault(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	return connectWS(t, token, 8081)
}

// sendWSMessage encodes and sends a ClientMessage over the WebSocket connection.
func sendWSMessage(t *testing.T, conn *websocket.Conn, msg *gatewayv1.ClientMessage) {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal client message: %v", err)
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("write WS message: %v", err)
	}
}

// readWSEvent reads and decodes a ServerEvent from the WebSocket connection.
func readWSEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) *gatewayv1.ServerEvent {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read WS message: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("expected binary message, got type %d", msgType)
	}
	if len(data) < 4 {
		t.Fatalf("frame too short: %d bytes", len(data))
	}
	length := binary.BigEndian.Uint32(data[:4])
	payload := data[4:]
	if len(payload) < int(length) {
		t.Fatalf("payload length mismatch: header says %d, got %d", length, len(payload))
	}
	event := &gatewayv1.ServerEvent{}
	if err := proto.Unmarshal(payload[:length], event); err != nil {
		t.Fatalf("unmarshal server event: %v", err)
	}
	return event
}

// waitForEventType reads events from the connection until one matching the
// desired type is found or the timeout expires.
func waitForEventType(t *testing.T, conn *websocket.Conn, eventType gatewayv1.ServerEventType, timeout time.Duration) *gatewayv1.ServerEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
	 remaining := time.Until(deadline)
		if remaining < time.Second {
			remaining = time.Second
		}
		event := readWSEvent(t, conn, remaining)
		if event.Type == eventType {
			return event
		}
		// Skip unexpected event types (e.g. HEARTBEAT_ACK).
		t.Logf("skipping event type %v, waiting for %v", event.Type, eventType)
	}
	t.Fatalf("timed out waiting for event type %v", eventType)
	return nil
}

// =============================================================================
// File upload helpers
// =============================================================================

// uploadFile uploads a file via POST /api/v1/files/upload and returns the file ID.
func uploadFile(t *testing.T, token string, filename string, content []byte, contentType string) string {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Set Content-Type for the file part.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", contentType)
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL("/api/v1/files/upload"), &buf)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusCreated)

	var result struct {
		ID string `json:"id"`
	}
	parseJSON(t, resp.Body, &result)
	return result.ID
}

// =============================================================================
// Utility helpers
// =============================================================================

// eventually retries fn at the given interval until it returns true or timeout.
func eventually(t *testing.T, fn func() bool, timeout time.Duration, interval time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("eventually: condition not met within %v", timeout)
}

// doPut sends a PUT with Bearer token and JSON body.
func doPut(t *testing.T, url, token string, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create PUT request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}
