package feed

import (
	"testing"
	"time"
)

func makeArticle(id, hash string, score float64, minsAgo int, userID *string) Article {
	return Article{
		ID:           id,
		ContentHash:  hash,
		Title:        "Test: " + id,
		CanonicalURL: "https://example.com/" + id,
		SourceDomain: "example.com",
		PublishedAt:  time.Now().Add(-time.Duration(minsAgo) * time.Minute),
		PulseScore:   score,
		UserID:       userID,
	}
}

func ptr(s string) *string { return &s }

func TestTwoPassMerge_GlobalOnlyWhenNoUser(t *testing.T) {
	global := []Article{
		makeArticle("g1", "hash1", 90.0, 5, nil),
		makeArticle("g2", "hash2", 80.0, 10, nil),
	}
	merged := TwoPassMerge(global, nil)
	if len(merged) != 2 {
		t.Fatalf("expected 2, got %d", len(merged))
	}
}

func TestTwoPassMerge_UserArticlesAppended(t *testing.T) {
	global := []Article{makeArticle("g1", "hash1", 90.0, 5, nil)}
	user := []Article{makeArticle("u1", "hash2", 70.0, 3, ptr("user-a"))}
	merged := TwoPassMerge(global, user)
	if len(merged) != 2 {
		t.Fatalf("expected 2, got %d", len(merged))
	}
}

func TestTwoPassMerge_DeduplicatesByContentHash(t *testing.T) {
	// Same content_hash in both global and user — should only appear once.
	global := []Article{makeArticle("g1", "shared-hash", 90.0, 5, nil)}
	user := []Article{makeArticle("u1", "shared-hash", 85.0, 3, ptr("user-a"))}
	merged := TwoPassMerge(global, user)
	if len(merged) != 1 {
		t.Fatalf("expected 1 after dedup, got %d", len(merged))
	}
	// Global wins on collision — the merged item should be the global one.
	if merged[0].ID != "g1" {
		t.Errorf("global article should win on hash collision, got ID=%s", merged[0].ID)
	}
}

func TestTwoPassMerge_SortedByPulseScoreDesc(t *testing.T) {
	global := []Article{
		makeArticle("g1", "h1", 50.0, 5, nil),
		makeArticle("g2", "h2", 90.0, 10, nil),
	}
	user := []Article{
		makeArticle("u1", "h3", 70.0, 3, ptr("user-a")),
	}
	merged := TwoPassMerge(global, user)
	for i := 1; i < len(merged); i++ {
		if merged[i].PulseScore > merged[i-1].PulseScore {
			t.Errorf("not sorted desc at index %d: %.2f > %.2f", i, merged[i].PulseScore, merged[i-1].PulseScore)
		}
	}
}

func TestTwoPassMerge_TiebreakByPublishedAtDesc(t *testing.T) {
	// Two articles with identical pulse scores — newer published_at wins.
	global := []Article{
		makeArticle("old", "h1", 75.0, 60, nil),
		makeArticle("new", "h2", 75.0, 5, nil),
	}
	merged := TwoPassMerge(global, nil)
	if merged[0].ID != "new" {
		t.Errorf("newer article should sort first on score tie, got %s", merged[0].ID)
	}
}

func TestTwoPassMerge_EmptyGlobal(t *testing.T) {
	user := []Article{makeArticle("u1", "h1", 60.0, 5, ptr("user-a"))}
	merged := TwoPassMerge(nil, user)
	if len(merged) != 1 {
		t.Fatalf("expected 1, got %d", len(merged))
	}
}

func TestTwoPassMerge_EmptyBoth(t *testing.T) {
	merged := TwoPassMerge(nil, nil)
	if len(merged) != 0 {
		t.Fatalf("expected 0, got %d", len(merged))
	}
}

func TestTwoPassMerge_MultipleHashCollisions(t *testing.T) {
	global := []Article{
		makeArticle("g1", "shared", 80.0, 5, nil),
		makeArticle("g2", "unique", 70.0, 8, nil),
	}
	user := []Article{
		makeArticle("u1", "shared", 75.0, 3, ptr("u")), // dup
		makeArticle("u2", "also-unique", 60.0, 1, ptr("u")),
	}
	merged := TwoPassMerge(global, user)
	if len(merged) != 3 {
		t.Fatalf("expected 3 (2 global + 1 new user), got %d", len(merged))
	}
}
