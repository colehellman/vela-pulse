package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// startJWKSServer serves a JWKS document containing pub under kid.
func startJWKSServer(t *testing.T, kid string, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"keys": []map[string]any{
				{"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig", "n": nB64, "e": eB64},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// resetJWKSCache clears the in-process cache so each test fetches fresh keys.
func resetJWKSCache() {
	jwksMu.Lock()
	jwksKeys = nil
	jwksFetched = time.Time{}
	jwksMu.Unlock()
}

// generateTestKey makes a throwaway RSA-256 key pair for test tokens.
func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return k
}

// mintAppleToken creates a JWT that looks like an Apple id_token.
func mintAppleToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-kid"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func validClaims(sub string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": "com.vela.pulse",
		"sub": sub,
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

// TestExtractAppleClaims_Valid verifies correct extraction of sub and email.
func TestExtractAppleClaims_Valid(t *testing.T) {
	key := generateTestKey(t)
	rawToken := mintAppleToken(t, key, jwt.MapClaims{
		"iss":   "https://appleid.apple.com",
		"aud":   "com.vela.pulse",
		"sub":   "001234.abc.5678",
		"email": "user@privaterelay.appleid.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	// Use the test verifier that accepts an in-process key (bypasses JWKS fetch).
	claims, err := verifyWithKey(rawToken, &key.PublicKey, "com.vela.pulse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Sub != "001234.abc.5678" {
		t.Errorf("sub: got %q", claims.Sub)
	}
	if claims.Email != "user@privaterelay.appleid.com" {
		t.Errorf("email: got %q", claims.Email)
	}
}

// TestExtractAppleClaims_Expired rejects expired tokens.
func TestExtractAppleClaims_Expired(t *testing.T) {
	key := generateTestKey(t)
	rawToken := mintAppleToken(t, key, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": "com.vela.pulse",
		"sub": "001234.abc.5678",
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // expired
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	})
	_, err := verifyWithKey(rawToken, &key.PublicKey, "com.vela.pulse")
	if err == nil {
		t.Error("expected error for expired token")
	}
}

// TestExtractAppleClaims_WrongAudience rejects tokens with a different aud.
func TestExtractAppleClaims_WrongAudience(t *testing.T) {
	key := generateTestKey(t)
	rawToken := mintAppleToken(t, key, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": "com.other.app",
		"sub": "001234.abc.5678",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := verifyWithKey(rawToken, &key.PublicKey, "com.vela.pulse")
	if err == nil {
		t.Error("expected error for wrong audience")
	}
}

// TestExtractAppleClaims_MissingSub rejects tokens without a subject.
func TestExtractAppleClaims_MissingSub(t *testing.T) {
	key := generateTestKey(t)
	rawToken := mintAppleToken(t, key, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": "com.vela.pulse",
		// no sub
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := verifyWithKey(rawToken, &key.PublicKey, "com.vela.pulse")
	if err == nil {
		t.Error("expected error for missing sub")
	}
}

// TestExtractAppleClaims_Tampered rejects a token signed with a different key.
func TestExtractAppleClaims_Tampered(t *testing.T) {
	signingKey := generateTestKey(t)
	verifyKey := generateTestKey(t) // different key

	rawToken := mintAppleToken(t, signingKey, validClaims("user-sub"))
	_, err := verifyWithKey(rawToken, &verifyKey.PublicKey, "com.vela.pulse")
	if err == nil {
		t.Error("expected error for wrong signing key")
	}
}

// TestInternalJWT_RoundTrip verifies the internal JWT (issued to mobile clients) is consistent.
func TestInternalJWT_RoundTrip(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-x!")
	userID := "550e8400-e29b-41d4-a716-446655440000"

	token, err := IssueInternalJWT(userID, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	gotUserID, err := ValidateInternalJWT(token, secret)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if gotUserID != userID {
		t.Errorf("got userID %q, want %q", gotUserID, userID)
	}
}

func TestInternalJWT_ExpiredRejected(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-x!")
	token, _ := IssueInternalJWT("user-1", secret, -1*time.Minute)
	_, err := ValidateInternalJWT(token, secret)
	if err == nil {
		t.Error("expected error for expired internal JWT")
	}
}

func TestInternalJWT_WrongSecretRejected(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-x!")
	other := []byte("other-secret-at-least-32-bytes-x")
	token, _ := IssueInternalJWT("user-1", secret, 1*time.Hour)
	_, err := ValidateInternalJWT(token, other)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

// --- Full VerifyAppleToken tests (exercises JWKS fetch path) ---

func TestVerifyAppleToken_ValidToken(t *testing.T) {
	resetJWKSCache()
	key := generateTestKey(t)
	const kid, clientID, subject = "kid-1", "com.vela.pulse", "apple-sub-abc"

	srv := startJWKSServer(t, kid, &key.PublicKey)
	appleJWKSURL = srv.URL

	token := mintAppleToken(t, key, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": clientID, "sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	// Override kid in token header to match JWKS.
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": clientID, "sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	tok.Header["kid"] = kid
	token, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	claims, err := VerifyAppleToken(token, clientID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Sub != subject {
		t.Fatalf("sub=%q want %q", claims.Sub, subject)
	}
}

func TestVerifyAppleToken_KidNotInJWKS(t *testing.T) {
	resetJWKSCache()
	key := generateTestKey(t)
	const clientID = "com.vela.pulse"

	// Server exposes "known-kid"; token uses "unknown-kid".
	srv := startJWKSServer(t, "known-kid", &key.PublicKey)
	appleJWKSURL = srv.URL

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://appleid.apple.com",
		"aud": clientID, "sub": "sub",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	tok.Header["kid"] = "unknown-kid"
	token, _ := tok.SignedString(key)

	_, err := VerifyAppleToken(token, clientID)
	if err == nil {
		t.Fatal("expected error: kid not in JWKS")
	}
}

func TestRsaPublicKeyFromJWK_RoundTrip(t *testing.T) {
	key := generateTestKey(t)
	pub := &key.PublicKey

	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	got, err := rsaPublicKeyFromJWK(nB64, eB64)
	if err != nil {
		t.Fatalf("rsaPublicKeyFromJWK: %v", err)
	}
	if got.N.Cmp(pub.N) != 0 {
		t.Fatal("N mismatch")
	}
	if got.E != pub.E {
		t.Fatalf("E=%d want %d", got.E, pub.E)
	}
}
