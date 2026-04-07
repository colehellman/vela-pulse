-- +goose Up

-- Partitioned parent table. published_at is the partition key per TRD (weekly via pg_partman).
-- NOTE: PostgreSQL requires the partition column to be part of every unique constraint,
-- so the PK is (id, published_at). Cross-partition uniqueness on content_hash is enforced
-- via the shadow table article_content_hashes below.
CREATE TABLE articles (
    id               UUID         NOT NULL DEFAULT gen_random_uuid(),
    -- NULL = Global article (no sentinel UUID). user_id NOT NULL = private/user-added.
    user_id          UUID         NULL REFERENCES users(id) ON DELETE SET NULL,
    canonical_url    TEXT         NOT NULL,
    content_hash     TEXT         NOT NULL,   -- SHA-256 hex (64 chars): SHA256(canonical_url + normalize_lead(text))
    title            TEXT         NOT NULL,
    lead_normalized  TEXT         NOT NULL,   -- output of normalize_lead(); stored for audit
    source_domain    TEXT         NOT NULL,
    published_at     TIMESTAMPTZ  NOT NULL,   -- partition control column
    scraped_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    pulse_score      NUMERIC(6,2) NOT NULL DEFAULT 0,
    share_count      INTEGER      NOT NULL DEFAULT 0,   -- S_weight input in Pulse Score
    recency_bias     NUMERIC(4,2) NOT NULL DEFAULT 1.0, -- P_bias input in Pulse Score
    PRIMARY KEY (id, published_at)
) PARTITION BY RANGE (published_at);

-- Shadow table for cross-partition content_hash uniqueness.
-- The gateway performs INSERT ... ON CONFLICT DO NOTHING here before inserting into articles.
-- rowsAffected == 0 means duplicate; skip the article insert entirely.
CREATE TABLE article_content_hashes (
    content_hash TEXT        PRIMARY KEY,
    article_id   UUID        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Global feed: recent articles with no user (NULLs first in IS NULL filter).
CREATE INDEX idx_articles_global
    ON articles(published_at DESC)
    WHERE user_id IS NULL;

-- User-specific feed: descending recency per user.
CREATE INDEX idx_articles_user_published
    ON articles(user_id, published_at DESC);

-- Ranking index for Two-Pass Merge re-sort.
CREATE INDEX idx_articles_pulse_score
    ON articles(pulse_score DESC, published_at DESC);

-- Fast lookup by hash (e.g., conflict resolution reads).
CREATE INDEX idx_articles_content_hash
    ON articles(content_hash);

-- +goose Down
DROP TABLE IF EXISTS article_content_hashes;
DROP TABLE IF EXISTS articles;
