package main

import "context"

// authTokenKey carries the caller's raw JWT through the request lifecycle so
// outgoing Connect-RPC calls (SendDM, SendMessage) can forward it as the
// Authorization header. The ws-gateway already validated this token at WS
// upgrade; forwarding it lets user/community services re-validate the same
// identity rather than trusting an internal header.
type authTokenKey struct{}

// WithToken returns a context carrying the raw JWT.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, authTokenKey{}, token)
}

// TokenFromContext returns the raw JWT stored in ctx, or "" if absent.
func TokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(authTokenKey{}).(string)
	return v
}
