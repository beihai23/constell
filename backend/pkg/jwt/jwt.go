package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims used by Constell.
type Claims struct {
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT string for the given userID.
// The token contains sub=userID, iat=now, and exp=now+ttl.
// Returns the signed token string, the expiry timestamp (unix seconds), and any error.
func GenerateToken(secret string, userID string, ttl time.Duration) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", 0, fmt.Errorf("sign token: %w", err)
	}

	return signed, expiresAt.Unix(), nil
}

// ParseToken verifies a JWT string and returns the userID from the sub claim.
// Returns an error if the token is invalid, expired, or signed with a different secret.
func ParseToken(secret string, tokenString string) (string, error) {
	if tokenString == "" {
		return "", fmt.Errorf("empty token")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token claims")
	}

	return claims.Subject, nil
}
