package feed

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/colehellman/vela-pulse/gateway/internal/scoring"
)

const (
	consumerName = "gateway-1"
	blockTimeout = 2 * time.Second
	batchSize    = 50
)

// ConsumerConfig holds everything the consumer needs to run.
type ConsumerConfig struct {
	StreamName     string
	ConsumerGroup  string
	GlobalCacheKey string
}

// Consumer reads articles from the Redis Stream, deduplicates, scores, and
// writes them to Postgres. After each batch it rebuilds the global top-200 cache.
type Consumer struct {
	rdb    *goredis.Client
	pool   *pgxpool.Pool
	dedup  *Deduplicator
	cfg    ConsumerConfig
	log    *zap.Logger
}

func NewConsumer(rdb *goredis.Client, pool *pgxpool.Pool, cfg ConsumerConfig, log *zap.Logger) *Consumer {
	return &Consumer{
		rdb:   rdb,
		pool:  pool,
		dedup: NewDeduplicator(pool),
		cfg:   cfg,
		log:   log,
	}
}

// Run starts the XREADGROUP loop. Blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}
	c.log.Info("consumer started",
		zap.String("stream", c.cfg.StreamName),
		zap.String("group", c.cfg.ConsumerGroup),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := c.rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    c.cfg.ConsumerGroup,
			Consumer: consumerName,
			Streams:  []string{c.cfg.StreamName, ">"},
			Count:    batchSize,
			Block:    blockTimeout,
		}).Result()

		if err == goredis.Nil || (err != nil && ctx.Err() != nil) {
			continue
		}
		if err != nil {
			c.log.Error("xreadgroup error", zap.Error(err))
			continue
		}

		var written []CachedArticle
		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				ca, ok, err := c.processMessage(ctx, msg)
				if err != nil {
					c.log.Error("process message", zap.String("id", msg.ID), zap.Error(err))
					continue
				}
				if ok {
					written = append(written, ca)
				}
				// ACK regardless — on error we log and move on (at-least-once, dedup handles retries).
				c.rdb.XAck(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, msg.ID) //nolint:errcheck
			}
		}

		if len(written) > 0 {
			if err := c.refreshCache(ctx); err != nil {
				c.log.Warn("cache refresh failed", zap.Error(err))
			}
		}
	}
}

// processMessage parses one stream message, deduplicates, scores, and inserts.
// Returns (CachedArticle, true, nil) on successful new write, (_, false, nil) on dup.
func (c *Consumer) processMessage(ctx context.Context, msg goredis.XMessage) (CachedArticle, bool, error) {
	v := msg.Values

	canonicalURL, _ := v["canonical_url"].(string)
	contentHash, _ := v["content_hash"].(string)
	title, _ := v["title"].(string)
	lead, _ := v["lead"].(string)
	sourceDomain, _ := v["source_domain"].(string)
	publishedAtStr, _ := v["published_at"].(string)
	pBiasStr, _ := v["recency_bias"].(string)

	if canonicalURL == "" || contentHash == "" || title == "" {
		return CachedArticle{}, false, fmt.Errorf("missing required fields in message %s", msg.ID)
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, publishedAtStr)
	if err != nil {
		publishedAt = time.Now().UTC()
	}

	pBias := 1.0
	if pBiasStr != "" {
		if v, err := strconv.ParseFloat(pBiasStr, 64); err == nil {
			pBias = v
		}
	}

	// share_count is scraped by the site extractor and published in the stream.
	// Defaults to 0 when absent (most scrapers don't surface engagement metrics yet).
	shareCountStr, _ := v["share_count"].(string)
	shareCount := 0
	if shareCountStr != "" {
		if n, err := strconv.Atoi(shareCountStr); err == nil && n >= 0 {
			shareCount = n
		}
	}

	pulseScore := scoring.PulseScore(shareCount, pBias, publishedAt)

	// Dedup: claim the content_hash. If already claimed, skip.
	// We need an ID to claim — use a deterministic approach: hash of content_hash.
	articleID := contentHashToUUID(contentHash)

	inserted, err := c.dedup.TryInsert(ctx, contentHash, articleID)
	if err != nil {
		return CachedArticle{}, false, fmt.Errorf("dedup: %w", err)
	}
	if !inserted {
		return CachedArticle{}, false, nil // duplicate
	}

	leadNorm := NormalizeLead(lead)

	_, err = c.pool.Exec(ctx, `
		INSERT INTO articles
			(id, canonical_url, content_hash, title, lead_normalized,
			 source_domain, published_at, pulse_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
	`, articleID, canonicalURL, contentHash, title, leadNorm,
		sourceDomain, publishedAt, pulseScore)
	if err != nil {
		// Rollback the dedup claim so a retry can succeed.
		c.pool.Exec(ctx, `DELETE FROM article_content_hashes WHERE content_hash = $1`, contentHash) //nolint:errcheck
		return CachedArticle{}, false, fmt.Errorf("insert article: %w", err)
	}

	c.log.Debug("article written",
		zap.String("id", articleID),
		zap.Float64("score", pulseScore),
		zap.String("domain", sourceDomain),
	)

	return CachedArticle{
		ID:           articleID,
		ContentHash:  contentHash,
		Title:        title,
		CanonicalURL: canonicalURL,
		SourceDomain: sourceDomain,
		PublishedAt:  publishedAt,
		PulseScore:   pulseScore,
	}, true, nil
}

// refreshCache queries the top-200 global articles and stores them in Redis.
func (c *Consumer) refreshCache(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, `
		SELECT id, content_hash, title, canonical_url, source_domain, published_at, pulse_score
		FROM articles
		WHERE user_id IS NULL
		ORDER BY pulse_score DESC, published_at DESC
		LIMIT 200
	`)
	if err != nil {
		return fmt.Errorf("query top 200: %w", err)
	}
	defer rows.Close()

	var articles []CachedArticle
	for rows.Next() {
		var a CachedArticle
		if err := rows.Scan(&a.ID, &a.ContentHash, &a.Title, &a.CanonicalURL,
			&a.SourceDomain, &a.PublishedAt, &a.PulseScore); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return RebuildGlobalCache(ctx, c.rdb, c.cfg.GlobalCacheKey, articles)
}

// ensureGroup creates the consumer group if it doesn't already exist.
func (c *Consumer) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// contentHashToUUID converts a SHA-256 hex string to a Postgres-compatible UUID v4 format.
// This makes article IDs deterministic and stable across retries.
func contentHashToUUID(hash string) string {
	if len(hash) < 32 {
		return hash
	}
	// Use first 32 hex chars, formatted as UUID: 8-4-4-4-12
	h := hash[:32]
	return fmt.Sprintf("%s-%s-4%s-%s-%s",
		h[0:8], h[8:12], h[13:16], h[16:20], h[20:32])
}

// Ensure pgx.Rows is imported (used in refreshCache).
var _ pgx.Rows // compile-time import check
