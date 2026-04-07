-- +goose Up
-- Composite index for Two-Pass Merge Pass 2: user-specific articles from the last 24h.
-- Note: CONCURRENTLY is not supported on partitioned tables in PostgreSQL.
CREATE INDEX IF NOT EXISTS idx_articles_user_recent
    ON articles(user_id, published_at DESC)
    WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_articles_user_recent;
