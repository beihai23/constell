package middleware

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/constell/constell/backend/pkg/jwt"
)

type contextKey string

const userIDKey contextKey = "constell-user-id"

// UserIDFromContext extracts the authenticated user ID from the context.
// Returns empty string if no user ID is present.
func UserIDFromContext(ctx context.Context) string {
	val, _ := ctx.Value(userIDKey).(string)
	return val
}

// contextWithUserID returns a new context with the user ID embedded.
func contextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// AuthInterceptorConfig holds optional configuration for the auth interceptor.
type AuthInterceptorConfig struct {
	// SkipPaths is a list of RPC procedure paths that should skip authentication.
	// For example: []string{"/auth.v1.AuthService/Register", "/auth.v1.AuthService/Login"}
	SkipPaths []string
}

// NewAuthInterceptor returns a Connect-RPC unary interceptor that validates
// JWT tokens from the Authorization header. If the token is valid, the user
// ID is stored in the request context. If the token is missing or invalid,
// the request is rejected with connect.CodeUnauthenticated.
//
// If skipPaths is provided, requests whose procedure path matches any entry
// will bypass authentication (e.g. health checks, public endpoints).
func NewAuthInterceptor(jwtSecret string, skipPaths ...string) connect.UnaryInterceptorFunc {
	skipSet := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = struct{}{}
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			// Skip auth for configured paths.
			if _, ok := skipSet[req.Spec().Procedure]; ok {
				return next(ctx, req)
			}

			authHeader := req.Header().Get("Authorization")
			if authHeader == "" {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					fmt.Errorf("missing Authorization header"),
				)
			}

			// Expect "Bearer <token>".
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					fmt.Errorf("invalid Authorization header format, expected 'Bearer <token>'"),
				)
			}

			tokenString := parts[1]
			userID, err := jwt.ParseToken(jwtSecret, tokenString)
			if err != nil {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					fmt.Errorf("invalid token: %w", err),
				)
			}

			// Store user ID in context for downstream handlers.
			newCtx := contextWithUserID(ctx, userID)
			return next(newCtx, req)
		}
	}
}
