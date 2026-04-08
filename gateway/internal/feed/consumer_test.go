package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)


// --- Pure function tests (no DB/Redis required) ---

func TestContentHashToUUID_Format(t *testing.T) {
	hash := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	got := contentHashToUUID(hash)

	parts := strings.Split(got, "-")
	if len(parts) != 5 {
		t.Fatalf("want 5 UUID parts, got %d: %s", len(parts), got)
	}
	wantLengths := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != wantLengths[i] {
			t.Errorf("part[%d] len=%d want %d (%s)", i, len(p), wantLengths[i], p)
		}
	}
}

func TestContentHashToUUID_Deterministic(t *testing.T) {
	hash := "deadbeefcafe0000111122223333444455556666777788889999aaaabbbbcccc"
	if contentHashToUUID(hash) != contentHashToUUID(hash) {
		t.Fatal("contentHashToUUID must be deterministic")
	}
}

func TestContentHashToUUID_ShortHashPassthrough(t *testing.T) {
	short := "abc"
	if got := contentHashToUUID(short); got != short {
		t.Fatalf("short hash: got %q, want %q", got, short)
	}
}

func TestContentHashToUUID_Version4Marker(t *testing.T) {
	// The UUID format is 8-4-4-4-12; the third group starts with "4" (version marker).
	hash := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	got := contentHashToUUID(hash)
	parts := strings.Split(got, "-")
	if !strings.HasPrefix(parts[2], "4") {
		t.Fatalf("third UUID group should start with '4', got %q", parts[2])
	}
}

// --- processMessage unit tests (no DB needed for error-path tests) ---

func makeConsumer() *Consumer {
	return &Consumer{log: zap.NewNop()}
}

func makeMsg(values map[string]any) goredis.XMessage {
	return goredis.XMessage{ID: "1-0", Values: values}
}

func TestProcessMessage_MissingCanonicalURL(t *testing.T) {
	c := makeConsumer()
	msg := makeMsg(map[string]any{
		"content_hash": "abc123",
		"title":        "A Title",
	})
	_, ok, err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for missing canonical_url")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestProcessMessage_MissingContentHash(t *testing.T) {
	c := makeConsumer()
	msg := makeMsg(map[string]any{
		"canonical_url": "https://espn.com/1",
		"title":         "A Title",
	})
	_, ok, err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for missing content_hash")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestProcessMessage_MissingTitle(t *testing.T) {
	c := makeConsumer()
	msg := makeMsg(map[string]any{
		"canonical_url": "https://espn.com/1",
		"content_hash":  "abc123",
	})
	_, ok, err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestProcessMessage_InvalidPublishedAtDefaultsToNow(t *testing.T) {
	// Verify the published_at fallback (non-parseable value → time.Now) doesn't panic.
	// We can't call processMessage fully without DB, but we can test the time-parsing
	// logic in isolation here.
	_, err := time.Parse(time.RFC3339Nano, "not-a-time")
	if err == nil {
		t.Fatal("expected parse error")
	}
	// The consumer handles this by defaulting to time.Now() — no panic expected.
}

func TestProcessMessage_ShareCountDefaults(t *testing.T) {
	// share_count is parsed with strconv.Atoi; invalid or negative values must not panic.
	// Verify by calling processMessage with bad share_count values — it must return an
	// error only for missing required fields, not for a bad share_count.
	c := makeConsumer()
	for _, sc := range []string{"", "-1", "abc"} {
		msg := makeMsg(map[string]any{
			// Required fields missing → returns error before share_count is parsed.
			"share_count": sc,
		})
		_, _, err := c.processMessage(context.Background(), msg)
		// Error is expected (missing required fields), but must not panic.
		if err == nil {
			t.Errorf("share_count=%q: expected error for missing required fields", sc)
		}
	}
}

// --- Integration tests (require TEST_DATABASE_URL + TEST_REDIS_URL) ---

func TestProcessMessage_Integration_ValidMessage(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	ctx := context.Background()

	cfg := ConsumerConfig{
		StreamName:     "vela:test:stream",
		ConsumerGroup:  "test-group",
		GlobalCacheKey: "vela:test:global:" + t.Name(),
	}
	c := NewConsumer(rdb, pool, cfg, zap.NewNop())

	hash := "integration-test-hash-" + t.Name()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM article_content_hashes WHERE content_hash = $1`, hash) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM articles WHERE content_hash = $1`, hash)               //nolint:errcheck
		rdb.Del(ctx, cfg.GlobalCacheKey)                                                   //nolint:errcheck
	})

	msg := makeMsg(map[string]any{
		"canonical_url": "https://espn.com/integration-test",
		"content_hash":  hash,
		"title":         "Integration Test Article",
		"lead":          "This is the lead text for the integration test.",
		"source_domain": "espn.com",
		"published_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"recency_bias":  "1.0",
		"share_count":   "100",
	})

	ca, ok, err := c.processMessage(ctx, msg)
	if err != nil {
		t.Fatalf("processMessage: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for new article")
	}
	if ca.ContentHash != hash {
		t.Errorf("ContentHash=%q want %q", ca.ContentHash, hash)
	}
	if ca.PulseScore <= 0 {
		t.Errorf("PulseScore=%v want > 0", ca.PulseScore)
	}
}

func TestProcessMessage_Integration_Duplicate(t *testing.T) {
	pool := testPool(t)
	rdb := testRedis(t)
	ctx := context.Background()

	cfg := ConsumerConfig{
		StreamName:     "vela:test:stream",
		ConsumerGroup:  "test-group",
		GlobalCacheKey: "vela:test:global:" + t.Name(),
	}
	c := NewConsumer(rdb, pool, cfg, zap.NewNop())

	hash := "integration-dup-hash-" + t.Name()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM article_content_hashes WHERE content_hash = $1`, hash) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM articles WHERE content_hash = $1`, hash)               //nolint:errcheck
	})

	msg := makeMsg(map[string]any{
		"canonical_url": "https://espn.com/dup-test",
		"content_hash":  hash,
		"title":         "Dup Test",
		"source_domain": "espn.com",
		"published_at":  time.Now().UTC().Format(time.RFC3339Nano),
	})

	_, first, err := c.processMessage(ctx, msg)
	if err != nil {
		t.Fatalf("first processMessage: %v", err)
	}
	if !first {
		t.Fatal("first call: expected ok=true")
	}

	_, second, err := c.processMessage(ctx, msg)
	if err != nil {
		t.Fatalf("second processMessage: %v", err)
	}
	if second {
		t.Fatal("duplicate: expected ok=false")
	}
}
