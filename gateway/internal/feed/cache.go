package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const globalCacheTTL = 60 * time.Second

// CachedArticle is the JSON representation stored in the global top-200 Redis cache.
type CachedArticle struct {
	ID           string    `json:"id"`
	ContentHash  string    `json:"ch"`
	Title        string    `json:"t"`
	CanonicalURL string    `json:"u"`
	SourceDomain string    `json:"sd"`
	PublishedAt  time.Time `json:"pa"`
	PulseScore   float64   `json:"ps"`
}

// RebuildGlobalCache queries the DB for the top-200 global articles by pulse_score
// and stores them as a JSON blob in Redis with a 60-second TTL.
//
// This is called from the consumer goroutine after each successful article write.
// It is intentionally synchronous within the consumer loop to keep the cache
// consistent — the overhead is a single indexed Postgres query + one Redis SET.
func RebuildGlobalCache(ctx context.Context, rdb *goredis.Client, cacheKey string, articles []CachedArticle) error {
	if len(articles) == 0 {
		return nil
	}
	b, err := json.Marshal(articles)
	if err != nil {
		return fmt.Errorf("marshal global cache: %w", err)
	}
	return rdb.Set(ctx, cacheKey, b, globalCacheTTL).Err()
}

// LoadGlobalCache fetches the cached top-200 from Redis.
// Returns nil slice (not error) on cache miss — caller falls back to DB.
func LoadGlobalCache(ctx context.Context, rdb *goredis.Client, cacheKey string) ([]Article, error) {
	b, err := rdb.Get(ctx, cacheKey).Bytes()
	if err == goredis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("redis GET %s: %w", cacheKey, err)
	}

	var cached []CachedArticle
	if err := json.Unmarshal(b, &cached); err != nil {
		return nil, fmt.Errorf("unmarshal global cache: %w", err)
	}

	articles := make([]Article, len(cached))
	for i, c := range cached {
		articles[i] = Article{
			ID:           c.ID,
			ContentHash:  c.ContentHash,
			Title:        c.Title,
			CanonicalURL: c.CanonicalURL,
			SourceDomain: c.SourceDomain,
			PublishedAt:  c.PublishedAt,
			PulseScore:   c.PulseScore,
		}
	}
	return articles, nil
}
