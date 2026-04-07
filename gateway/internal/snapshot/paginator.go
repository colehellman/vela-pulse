package snapshot

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const TTL = 5 * time.Minute

// NewID generates a snapshot_id as Unix_milliseconds + 4 random hex chars.
// Format: "{unix_ms}{4hex}" e.g. "17438291234abcd"
func NewID() string {
	ms := time.Now().UnixMilli()
	b := make([]byte, 2)
	rand.Read(b) //nolint:errcheck // crypto/rand never errors on supported platforms
	return fmt.Sprintf("%d%04x", ms, b)
}

// EncodeCursor encodes Base64(published_at_unix | article_id) per TRD §4.2.
func EncodeCursor(publishedAt time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", publishedAt.Unix(), id)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes a cursor back to (published_at, article_id).
func DecodeCursor(cursor string) (time.Time, string, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	unix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor timestamp: %w", err)
	}
	return time.Unix(unix, 0), parts[1], nil
}
