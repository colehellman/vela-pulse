// Package auth handles Sign In With Apple (SIWA) token verification and
// internal JWT issuance for mobile clients.
package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppleClaims holds the fields we extract from a verified Apple id_token.
type AppleClaims struct {
	Sub   string // stable user identifier (apple_sub in users table)
	Email string // may be empty if user withheld it after first auth
}

// appleJWTClaims maps the Apple id_token JWT body.
type appleJWTClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// jwksCache caches Apple's public keys with a 24-hour TTL.
// Apple rotates keys infrequently; 24h is safe and avoids per-request fetches.
var (
	jwksMu      sync.RWMutex
	jwksKeys    map[string]*rsa.PublicKey
	jwksFetched time.Time
	jwksTTL     = 24 * time.Hour
)

// appleJWKSURL is a var so tests can point it at a local httptest.Server.
var appleJWKSURL = "https://appleid.apple.com/auth/keys"

// VerifyAppleToken validates an Apple id_token and returns the stable claims.
// It fetches Apple's JWKS on first call (or after TTL) and caches the keys.
func VerifyAppleToken(rawToken, clientID string) (*AppleClaims, error) {
	pub, err := getApplePublicKey(rawToken)
	if err != nil {
		return nil, fmt.Errorf("get apple public key: %w", err)
	}
	return verifyWithKey(rawToken, pub, clientID)
}

// verifyWithKey is the testable core — verifies a token against a given RSA public key.
func verifyWithKey(rawToken string, pub *rsa.PublicKey, clientID string) (*AppleClaims, error) {
	var claims appleJWTClaims
	_, err := jwt.ParseWithClaims(rawToken, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pub, nil
	},
		jwt.WithAudience(clientID),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt parse: %w", err)
	}

	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return nil, fmt.Errorf("missing sub claim")
	}

	return &AppleClaims{Sub: sub, Email: claims.Email}, nil
}

// getApplePublicKey parses the kid from the token header and returns the matching
// public key from Apple's JWKS (with 24h in-process cache).
func getApplePublicKey(rawToken string) (*rsa.PublicKey, error) {
	tok, _, err := jwt.NewParser().ParseUnverified(rawToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse unverified: %w", err)
	}
	kid, _ := tok.Header["kid"].(string)

	jwksMu.RLock()
	stale := time.Since(jwksFetched) > jwksTTL
	key, ok := jwksKeys[kid]
	jwksMu.RUnlock()

	if !stale && ok {
		return key, nil
	}

	if err := refreshJWKS(); err != nil {
		return nil, err
	}

	jwksMu.RLock()
	key, ok = jwksKeys[kid]
	jwksMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kid %q not found in Apple JWKS", kid)
	}
	return key, nil
}

// refreshJWKS fetches Apple's JWKS endpoint and updates the in-process cache.
func refreshJWKS() error {
	resp, err := http.Get(appleJWKSURL) //nolint:gosec
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			log.Printf("siwa: skipping malformed JWKS key kid=%q: %v", k.Kid, err)
			continue
		}
		keys[k.Kid] = pub
	}

	if len(keys) == 0 && len(jwks.Keys) > 0 {
		return fmt.Errorf("all %d JWKS keys failed to parse", len(jwks.Keys))
	}

	jwksMu.Lock()
	jwksKeys = keys
	jwksFetched = time.Now()
	jwksMu.Unlock()
	return nil
}

// rsaPublicKeyFromJWK decodes base64url-encoded modulus and exponent into an RSA public key.
func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	return &rsa.PublicKey{N: n, E: e}, nil
}

// ---------------------------------------------------------------------------
// Internal JWT (issued to mobile clients after SIWA verification)
// ---------------------------------------------------------------------------

type internalClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// IssueInternalJWT creates a HS256 JWT for the mobile client.
// The token encodes the internal user UUID and is signed with the gateway's JWTSecret.
func IssueInternalJWT(userID string, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := internalClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ValidateInternalJWT verifies the token signature and expiry, returning the user UUID.
func ValidateInternalJWT(rawToken string, secret []byte) (string, error) {
	var claims internalClaims
	_, err := jwt.ParseWithClaims(rawToken, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return "", fmt.Errorf("jwt validate: %w", err)
	}
	if claims.UserID == "" {
		return "", fmt.Errorf("missing uid claim")
	}
	return claims.UserID, nil
}
