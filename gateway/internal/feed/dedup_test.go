package feed

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to TEST_DATABASE_URL or skips.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// cleanupHash removes a test content_hash so tests are idempotent.
func cleanupHash(t *testing.T, pool *pgxpool.Pool, hash string) {
	t.Helper()
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM article_content_hashes WHERE content_hash = $1`, hash)
	})
}

func TestTryInsert_NewHash(t *testing.T) {
	pool := testPool(t)
	d := NewDeduplicator(pool)
	ctx := context.Background()

	hash := "dedup-test-new-" + t.Name()
	cleanupHash(t, pool, hash)

	inserted, err := d.TryInsert(ctx, hash, "article-id-1")
	if err != nil {
		t.Fatalf("TryInsert: %v", err)
	}
	if !inserted {
		t.Fatal("expected inserted=true for new hash")
	}
}

func TestTryInsert_DuplicateHashReturnsFalse(t *testing.T) {
	pool := testPool(t)
	d := NewDeduplicator(pool)
	ctx := context.Background()

	hash := "dedup-test-dup-" + t.Name()
	cleanupHash(t, pool, hash)

	first, err := d.TryInsert(ctx, hash, "article-id-1")
	if err != nil {
		t.Fatalf("first TryInsert: %v", err)
	}
	if !first {
		t.Fatal("first insert should return true")
	}

	second, err := d.TryInsert(ctx, hash, "article-id-2")
	if err != nil {
		t.Fatalf("second TryInsert: %v", err)
	}
	if second {
		t.Fatal("duplicate hash should return false")
	}
}

func TestTryInsert_DifferentHashesAreBothInserted(t *testing.T) {
	pool := testPool(t)
	d := NewDeduplicator(pool)
	ctx := context.Background()

	hashA := "dedup-test-a-" + t.Name()
	hashB := "dedup-test-b-" + t.Name()
	cleanupHash(t, pool, hashA)
	cleanupHash(t, pool, hashB)

	for _, tc := range []struct{ hash, id string }{
		{hashA, "article-a"},
		{hashB, "article-b"},
	} {
		inserted, err := d.TryInsert(ctx, tc.hash, tc.id)
		if err != nil {
			t.Fatalf("TryInsert(%s): %v", tc.hash, err)
		}
		if !inserted {
			t.Fatalf("expected inserted=true for %s", tc.hash)
		}
	}
}
