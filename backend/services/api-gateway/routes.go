package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/constell/constell/backend/pkg/jwt"
	"github.com/constell/constell/backend/services/api-gateway/handlers"
)

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

const userIDKey contextKey = "constell-user-id"

// contextWithUserID returns a new context with the user ID embedded.
func contextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// jwtAuthMiddleware validates the JWT token from the Authorization header
// and stores the user ID in the request context.
func jwtAuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"invalid Authorization header format"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]
			userID, err := jwt.ParseToken(jwtSecret, tokenString)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// Store user ID in context.
			ctx := contextWithUserID(r.Context(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// notImplemented returns a handler that responds with 501 Not Implemented.
// Used for routes whose backing RPC will be added in a later task.
func notImplemented() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(`{"error":"not implemented"}`))
	}
}

// registerRoutes sets up all REST routes on the chi router.
func registerRoutes(r chi.Router, clients *handlers.Clients, jwtSecret string) {
	// Create handler instances.
	authHandler := handlers.NewAuthHandler(clients.Auth)
	userHandler := handlers.NewUserHandler(clients.User)
	communityHandler := handlers.NewCommunityHandler(clients.Community)

	// Auth routes -- no JWT auth required.
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.RefreshToken)
	})

	// All routes below require JWT authentication.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(jwtAuthMiddleware(jwtSecret))

		// User routes.
		r.Route("/users", func(r chi.Router) {
			r.Get("/{id}", userHandler.GetUser)
			r.Patch("/{id}", userHandler.UpdateProfile)
			r.Get("/{id}/friends", userHandler.ListFriends)
			r.Post("/{id}/block", notImplemented())         // BlockUser RPC added in Task 16
			r.Delete("/{id}/block/{tid}", notImplemented()) // UnblockUser RPC added in Task 16
			r.Get("/{id}/relation/{tid}", notImplemented()) // GetRelation RPC added in Task 16
		})

		// DM routes.
		r.Route("/dm", func(r chi.Router) {
			r.Post("/send", userHandler.SendDM)
			r.Get("/history/{peerId}", userHandler.GetDMHistory)
			r.Get("/conversations", notImplemented()) // GetDMConversations RPC added in Task 16
		})

		// Server (community) routes.
		r.Post("/servers", communityHandler.CreateServer)
		r.Route("/servers/{id}", func(r chi.Router) {
			r.Get("/", communityHandler.GetServer)
			r.Patch("/", communityHandler.UpdateServer)

			// Channels under a server.
			r.Post("/channels", communityHandler.CreateChannel)
			r.Get("/channels", communityHandler.GetChannels)

			// Members under a server.
			r.Post("/members", communityHandler.AddMember)
			r.Delete("/members/{uid}", communityHandler.RemoveMember)
			r.Get("/members", communityHandler.ListMembers)

			// Roles under a server.
			r.Post("/roles", notImplemented())                      // CreateRole RPC added in Task 17
			r.Post("/members/{uid}/roles/{rid}", notImplemented()) // AssignRole RPC added in Task 17
		})

		// Channel routes (not nested under /servers).
		r.Patch("/channels/{id}", communityHandler.UpdateChannel)

		// Channel message routes.
		r.Post("/channels/{id}/messages", communityHandler.SendMessage)
		r.Get("/channels/{id}/messages", communityHandler.GetHistory)
	})
}
