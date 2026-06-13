package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgjwt "github.com/constell/constell/backend/pkg/jwt"
)

const testJWTSecret = "test-ws-gateway-secret"

func TestAuthenticateUpgrade_ValidToken(t *testing.T) {
	token, _, err := pkgjwt.GenerateToken(testJWTSecret, "user-123", 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	auth := NewAuthenticator(testJWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	userID, gotToken, err := auth.AuthenticateUpgrade(req)
	if err != nil {
		t.Fatalf("AuthenticateUpgrade failed: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("expected userID 'user-123', got %q", userID)
	}
	if gotToken != token {
		t.Fatalf("expected returned token to match input")
	}

	t.Logf("valid token authenticated: userID=%s", userID)
}

func TestAuthenticateUpgrade_MissingToken(t *testing.T) {
	auth := NewAuthenticator(testJWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	_, _, err := auth.AuthenticateUpgrade(req)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}

	t.Logf("correctly rejected missing token: %v", err)
}

func TestAuthenticateUpgrade_ExpiredToken(t *testing.T) {
	token, _, err := pkgjwt.GenerateToken(testJWTSecret, "user-expired", -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	auth := NewAuthenticator(testJWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	_, _, err = auth.AuthenticateUpgrade(req)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}

	t.Logf("correctly rejected expired token: %v", err)
}

func TestAuthenticateUpgrade_InvalidToken(t *testing.T) {
	auth := NewAuthenticator(testJWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/ws?token=not.a.valid.token", nil)
	_, _, err := auth.AuthenticateUpgrade(req)
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}

	t.Logf("correctly rejected invalid token: %v", err)
}

func TestAuthenticateUpgrade_WrongSecret(t *testing.T) {
	token, _, err := pkgjwt.GenerateToken("correct-secret", "user-wrong", 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	auth := NewAuthenticator("wrong-secret")

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	_, _, err = auth.AuthenticateUpgrade(req)
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}

	t.Logf("correctly rejected wrong secret: %v", err)
}

func TestAuthenticateUpgrade_EmptyToken(t *testing.T) {
	auth := NewAuthenticator(testJWTSecret)

	req := httptest.NewRequest(http.MethodGet, "/ws?token=", nil)
	_, _, err := auth.AuthenticateUpgrade(req)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}

	t.Logf("correctly rejected empty token: %v", err)
}
