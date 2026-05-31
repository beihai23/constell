package handlers

import (
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"

	authv1 "github.com/constell/constell/backend/pkg/proto/auth/v1"
	authv1connect "github.com/constell/constell/backend/pkg/proto/auth/v1/authv1connect"
)

// AuthHandler handles REST API requests for authentication.
type AuthHandler struct {
	client authv1connect.AuthServiceClient
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(client authv1connect.AuthServiceClient) *AuthHandler {
	return &AuthHandler{client: client}
}

// registerRequest is the JSON body for POST /api/v1/auth/register.
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// registerResponse is the JSON response for successful registration.
type registerResponse struct {
	UserID       string `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Nickname == "" {
		writeError(w, http.StatusBadRequest, "email, password, and nickname are required")
		return
	}

	resp, err := h.client.Register(r.Context(), connect.NewRequest(&authv1.RegisterRequest{
		Email:    req.Email,
		Password: req.Password,
		Nickname: req.Nickname,
	}))
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	writeJSON(w, http.StatusCreated, registerResponse{
		UserID:       msg.UserId,
		AccessToken:  msg.AccessToken,
		RefreshToken: msg.RefreshToken,
		ExpiresAt:    msg.ExpiresAt,
	})
}

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the JSON response for successful login.
type loginResponse struct {
	UserID       string `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	resp, err := h.client.Login(r.Context(), connect.NewRequest(&authv1.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}))
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	writeJSON(w, http.StatusOK, loginResponse{
		UserID:       msg.UserId,
		AccessToken:  msg.AccessToken,
		RefreshToken: msg.RefreshToken,
		ExpiresAt:    msg.ExpiresAt,
	})
}

// refreshTokenRequest is the JSON body for POST /api/v1/auth/refresh.
type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// refreshTokenResponse is the JSON response for successful token refresh.
type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// RefreshToken handles POST /api/v1/auth/refresh.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req refreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	resp, err := h.client.RefreshToken(r.Context(), connect.NewRequest(&authv1.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	}))
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	writeJSON(w, http.StatusOK, refreshTokenResponse{
		AccessToken:  msg.AccessToken,
		RefreshToken: msg.RefreshToken,
		ExpiresAt:    msg.ExpiresAt,
	})
}
