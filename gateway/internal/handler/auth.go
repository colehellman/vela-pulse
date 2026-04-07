package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/colehellman/vela-pulse/gateway/internal/auth"
)

const internalJWTTTL = 30 * 24 * time.Hour // 30-day token

// SIWAHandler handles POST /v1/auth/siwa.
// Flow: verify Apple id_token → upsert user → issue internal JWT → return to client.
type SIWAHandler struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
	clientID  string // Apple app bundle ID
	log       *zap.Logger
}

func NewSIWAHandler(pool *pgxpool.Pool, jwtSecret []byte, clientID string, log *zap.Logger) *SIWAHandler {
	return &SIWAHandler{pool: pool, jwtSecret: jwtSecret, clientID: clientID, log: log}
}

type siwaRequest struct {
	IDToken string `json:"id_token"`
}

type siwaResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

func (h *SIWAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req siwaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		http.Error(w, "bad request: id_token required", http.StatusBadRequest)
		return
	}

	claims, err := auth.VerifyAppleToken(req.IDToken, h.clientID)
	if err != nil {
		h.log.Warn("siwa verify failed", zap.Error(err))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := h.upsertUser(r.Context(), claims)
	if err != nil {
		h.log.Error("upsert user", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	token, err := auth.IssueInternalJWT(userID, h.jwtSecret, internalJWTTTL)
	if err != nil {
		h.log.Error("issue jwt", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(siwaResponse{Token: token, UserID: userID}) //nolint:errcheck
}

// upsertUser inserts or finds the user by apple_sub.
// Email is only provided by Apple on first authentication; DO NOTHING preserves
// an existing email if the user later signs in with a private relay address.
func (h *SIWAHandler) upsertUser(ctx context.Context, claims *auth.AppleClaims) (string, error) {
	var userID string

	// Attempt insert first. If the apple_sub already exists, fall through to SELECT.
	err := h.pool.QueryRow(ctx, `
		INSERT INTO users (apple_sub, email)
		VALUES ($1, NULLIF($2, ''))
		ON CONFLICT (apple_sub) DO UPDATE
			SET updated_at = NOW()
		RETURNING id
	`, claims.Sub, claims.Email).Scan(&userID)

	return userID, err
}
