package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/colehellman/vela-pulse/gateway/internal/auth"
)

const testSecret = "test-secret-at-least-32-bytes-x!"

func makeToken(t *testing.T, userID string, ttl time.Duration) string {
	t.Helper()
	tok, err := auth.IssueInternalJWT(userID, []byte(testSecret), ttl)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	uid := UserIDFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(uid)) //nolint:errcheck
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	tok := makeToken(t, "user-uuid-1", 1*time.Hour)
	mw := RequireAuth([]byte(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "user-uuid-1" {
		t.Errorf("expected user ID in context, got %q", w.Body.String())
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	mw := RequireAuth([]byte(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidScheme(t *testing.T) {
	mw := RequireAuth([]byte(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	tok := makeToken(t, "user-uuid-1", -1*time.Minute)
	mw := RequireAuth([]byte(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongSecret(t *testing.T) {
	tok := makeToken(t, "user-uuid-1", 1*time.Hour)
	mw := RequireAuth([]byte("different-secret-at-least-32-bytes"))

	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong secret, got %d", w.Code)
	}
}

func TestOptionalAuth_NoHeader_Passes(t *testing.T) {
	// OptionalAuth should not block unauthenticated requests.
	mw := OptionalAuth([]byte(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "" {
		t.Errorf("expected empty user ID, got %q", w.Body.String())
	}
}

func TestOptionalAuth_ValidToken_SetsContext(t *testing.T) {
	tok := makeToken(t, "user-uuid-2", 1*time.Hour)
	mw := OptionalAuth([]byte(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	mw(http.HandlerFunc(okHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "user-uuid-2" {
		t.Errorf("expected user ID, got %q", w.Body.String())
	}
}
