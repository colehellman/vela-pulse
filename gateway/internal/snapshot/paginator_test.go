package snapshot

import (
	"strings"
	"testing"
	"time"
)

func TestNewID_Format(t *testing.T) {
	id := NewID()
	// Must be numeric prefix (unix_ms, 13 digits in 2026) + 4 hex chars
	if len(id) < 17 {
		t.Errorf("snapshot ID too short: %q (len=%d)", id, len(id))
	}
	// Last 4 chars should be hex
	suffix := id[len(id)-4:]
	for _, c := range suffix {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("suffix %q contains non-hex char %c", suffix, c)
		}
	}
}

func TestNewID_Unique(t *testing.T) {
	ids := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := NewID()
		if ids[id] {
			t.Errorf("duplicate snapshot ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestEncodeCursor_RoundTrip(t *testing.T) {
	pub := time.Unix(1775595534, 0).UTC()
	id := "550e8400-e29b-41d4-a716-446655440000"

	encoded := EncodeCursor(pub, id)
	gotPub, gotID, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor error: %v", err)
	}
	if !gotPub.Equal(pub) {
		t.Errorf("published_at mismatch: got %v, want %v", gotPub, pub)
	}
	if gotID != id {
		t.Errorf("id mismatch: got %s, want %s", gotID, id)
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecodeCursor_MissingPipe(t *testing.T) {
	import64 := "dGhpcyBoYXMgbm8gcGlwZQ==" // "this has no pipe"
	_, _, err := DecodeCursor(import64)
	if err == nil {
		t.Error("expected error for missing pipe separator")
	}
}

func TestDecodeCursor_InvalidTimestamp(t *testing.T) {
	import64 := "bm90YW51bWJlcnxhcnRpY2xlLWlk" // "notanumber|article-id"
	_, _, err := DecodeCursor(import64)
	if err == nil {
		t.Error("expected error for non-numeric timestamp")
	}
}

func TestEncodeCursor_IsDeterministic(t *testing.T) {
	pub := time.Unix(1775595534, 0).UTC()
	id := "article-123"
	c1 := EncodeCursor(pub, id)
	c2 := EncodeCursor(pub, id)
	if c1 != c2 {
		t.Errorf("cursor encoding must be deterministic: %s != %s", c1, c2)
	}
}

func TestEncodeCursor_DifferentInputsDifferentCursors(t *testing.T) {
	pub := time.Unix(1775595534, 0).UTC()
	c1 := EncodeCursor(pub, "article-1")
	c2 := EncodeCursor(pub, "article-2")
	if c1 == c2 {
		t.Error("different article IDs must produce different cursors")
	}
}
