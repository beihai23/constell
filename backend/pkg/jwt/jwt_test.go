package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndParse(t *testing.T) {
	secret := "test-secret-key"
	userID := "user-abc-123"
	ttl := 1 * time.Hour

	tokenString, expiresAt, err := GenerateToken(secret, userID, ttl)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if tokenString == "" {
		t.Fatal("expected non-empty token string")
	}

	now := time.Now().Unix()
	if expiresAt <= now {
		t.Fatalf("expected expiresAt in the future, got %d (now=%d)", expiresAt, now)
	}
	// expiresAt should be approximately now + ttl
	expectedExpiry := now + int64(ttl.Seconds())
	delta := expiresAt - expectedExpiry
	if delta < -1 || delta > 1 {
		t.Fatalf("expected expiresAt ~%d, got %d (delta=%d)", expectedExpiry, expiresAt, delta)
	}

	parsedUserID, err := ParseToken(secret, tokenString)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if parsedUserID != userID {
		t.Fatalf("expected userID %q, got %q", userID, parsedUserID)
	}

	t.Logf("token generated and parsed successfully, userID=%s", parsedUserID)
}

func TestExpiredToken(t *testing.T) {
	secret := "test-secret-key"
	userID := "user-expired"

	// Token that expired 1 hour ago (negative ttl).
	tokenString, _, err := GenerateToken(secret, userID, -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ParseToken(secret, tokenString)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}

	t.Logf("expired token correctly rejected: %v", err)
}

func TestInvalidSecret(t *testing.T) {
	secret := "correct-secret"
	wrongSecret := "wrong-secret"
	userID := "user-wrong"

	tokenString, _, err := GenerateToken(secret, userID, 1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ParseToken(wrongSecret, tokenString)
	if err == nil {
		t.Fatal("expected error when parsing token with wrong secret, got nil")
	}

	t.Logf("wrong secret correctly rejected: %v", err)
}

func TestEmptyToken(t *testing.T) {
	secret := "test-secret-key"

	_, err := ParseToken(secret, "")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}

	t.Logf("empty token correctly rejected: %v", err)
}
