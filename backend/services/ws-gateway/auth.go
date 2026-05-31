package main

import (
	"fmt"
	"net/http"

	pkgjwt "github.com/constell/constell/backend/pkg/jwt"
)

// Authenticator validates JWT tokens on WebSocket upgrade requests.
type Authenticator struct {
	secret string
}

// NewAuthenticator creates a new Authenticator with the given JWT secret.
func NewAuthenticator(secret string) *Authenticator {
	return &Authenticator{secret: secret}
}

// AuthenticateUpgrade extracts and validates the JWT token from the
// WebSocket upgrade request's query parameter "token".
func (a *Authenticator) AuthenticateUpgrade(r *http.Request) (string, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		return "", fmt.Errorf("missing token query parameter")
	}

	userID, err := pkgjwt.ParseToken(a.secret, token)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	return userID, nil
}
