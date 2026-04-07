package feed

import (
	"sort"
	"time"
)

// Article is the in-memory representation used during Two-Pass Merge.
// It is intentionally separate from any DB-generated type to keep the merge
// logic decoupled from the storage layer.
type Article struct {
	ID           string
	ContentHash  string
	Title        string
	CanonicalURL string
	SourceDomain string
	PublishedAt  time.Time
	PulseScore   float64
	UserID       *string // nil = global
}

// TwoPassMerge merges the global top-200 (from Redis) with user-specific articles
// (from Postgres, last 24h), deduplicates by content_hash, and re-sorts by
// pulse_score DESC with published_at as the tiebreak.
//
// Must complete in < 50ms per TRD §4.1. Both slices are already fetched before
// this call; the merge itself is O((n+m) log(n+m)) in-memory only.
func TwoPassMerge(globalTop200 []Article, userArticles []Article) []Article {
	seen := make(map[string]struct{}, len(globalTop200))
	merged := make([]Article, 0, len(globalTop200)+len(userArticles))

	// Pass 1: accept all global articles.
	for _, a := range globalTop200 {
		seen[a.ContentHash] = struct{}{}
		merged = append(merged, a)
	}

	// Pass 2: add user-specific articles, skipping content_hash duplicates.
	// Per TRD §2.2: on collision, Global metadata wins but source attribution is retained.
	for _, a := range userArticles {
		if _, dup := seen[a.ContentHash]; !dup {
			merged = append(merged, a)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].PulseScore != merged[j].PulseScore {
			return merged[i].PulseScore > merged[j].PulseScore
		}
		return merged[i].PublishedAt.After(merged[j].PublishedAt)
	})

	return merged
}
