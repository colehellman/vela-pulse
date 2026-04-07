package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/colehellman/vela-pulse/gateway/internal/auth"
)

type contextKey string

const userIDKey contextKey = "user_id"

// UserIDFromContext returns the authenticated user's UUID from the request context,
// or an empty string if the request is unauthenticated.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

// RequireAuth blocks requests that don't carry a valid Bearer JWT.
// Returns 401 on missing/invalid/expired tokens.
func RequireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := extractBearer(r)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID, err := auth.ValidateInternalJWT(tok, secret)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth attempts to extract and validate a Bearer JWT.
// If absent or invalid, the request proceeds without a user ID in context.
// The feed handler uses this so unauthenticated users get the global-only feed.
func OptionalAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tok, ok := extractBearer(r); ok {
				if userID, err := auth.ValidateInternalJWT(tok, secret); err == nil {
					ctx := context.WithValue(r.Context(), userIDKey, userID)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	tok := strings.TrimPrefix(h, "Bearer ")
	if tok == "" {
		return "", false
	}
	return tok, true
}
