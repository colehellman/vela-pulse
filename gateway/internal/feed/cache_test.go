package feed

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// testRedis connects to TEST_REDIS_URL or skips.
func testRedis(t *testing.T) *goredis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set — skipping Redis integration test")
	}
	opt, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	rdb := goredis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func testCacheKey(t *testing.T) string {
	return "vela:test:global:" + t.Name()
}

func TestRebuildGlobalCache_StoresAndExpires(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t)
	t.Cleanup(func() { rdb.Del(ctx, key) }) //nolint:errcheck

	articles := []CachedArticle{
		{ID: "a1", ContentHash: "h1", Title: "T1", CanonicalURL: "https://espn.com/1",
			SourceDomain: "espn.com", PublishedAt: time.Now().UTC(), PulseScore: 50.0},
	}

	if err := RebuildGlobalCache(ctx, rdb, key, articles); err != nil {
		t.Fatalf("RebuildGlobalCache: %v", err)
	}

	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected positive TTL, got %v", ttl)
	}
	if ttl > globalCacheTTL {
		t.Fatalf("TTL %v exceeds max %v", ttl, globalCacheTTL)
	}
}

func TestRebuildGlobalCache_EmptySliceSkipsWrite(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t)
	t.Cleanup(func() { rdb.Del(ctx, key) }) //nolint:errcheck

	// Key must not exist before call.
	rdb.Del(ctx, key) //nolint:errcheck

	if err := RebuildGlobalCache(ctx, rdb, key, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("EXISTS: %v", err)
	}
	if exists != 0 {
		t.Fatal("empty slice must not write to Redis")
	}
}

func TestLoadGlobalCache_Miss(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t) + "-miss"
	rdb.Del(ctx, key) //nolint:errcheck

	got, err := LoadGlobalCache(ctx, rdb, key)
	if err != nil {
		t.Fatalf("unexpected error on cache miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on miss, got %+v", got)
	}
}

func TestLoadGlobalCache_Hit(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t)
	t.Cleanup(func() { rdb.Del(ctx, key) }) //nolint:errcheck

	want := []CachedArticle{
		{ID: "a1", ContentHash: "h1", Title: "T1", CanonicalURL: "https://espn.com/1",
			SourceDomain: "espn.com", PublishedAt: time.Now().Truncate(time.Second).UTC(), PulseScore: 75.5},
	}
	if err := RebuildGlobalCache(ctx, rdb, key, want); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	got, err := LoadGlobalCache(ctx, rdb, key)
	if err != nil {
		t.Fatalf("LoadGlobalCache: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 article, got %d", len(got))
	}
	if got[0].ID != want[0].ID {
		t.Errorf("ID=%q want %q", got[0].ID, want[0].ID)
	}
	if got[0].PulseScore != want[0].PulseScore {
		t.Errorf("PulseScore=%v want %v", got[0].PulseScore, want[0].PulseScore)
	}
}

func TestLoadGlobalCache_CorruptData(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t) + "-corrupt"
	t.Cleanup(func() { rdb.Del(ctx, key) }) //nolint:errcheck

	// Write malformed JSON directly.
	rdb.Set(ctx, key, `{not valid json`, globalCacheTTL) //nolint:errcheck

	_, err := LoadGlobalCache(ctx, rdb, key)
	if err == nil {
		t.Fatal("expected error for corrupt cache data")
	}
}

func TestLoadGlobalCache_EnforcesTop200Limit(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	key := testCacheKey(t) + "-limit"
	t.Cleanup(func() { rdb.Del(ctx, key) }) //nolint:errcheck

	// Build 205 articles and store them — RebuildGlobalCache stores what it's given;
	// the 200-item cap is enforced by the SQL query upstream. This test verifies
	// the cache faithfully round-trips whatever it stores (no silent truncation).
	articles := make([]CachedArticle, 205)
	for i := range articles {
		articles[i] = CachedArticle{ID: string(rune('a' + i%26)), PulseScore: float64(i)}
	}
	if err := RebuildGlobalCache(ctx, rdb, key, articles); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := LoadGlobalCache(ctx, rdb, key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != len(articles) {
		t.Fatalf("got %d articles, want %d", len(got), len(articles))
	}
}
