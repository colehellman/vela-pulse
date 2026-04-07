package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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
