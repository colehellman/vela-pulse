package feed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deduplicator enforces cross-partition content_hash uniqueness via the
// article_content_hashes shadow table. PostgreSQL UNIQUE constraints do not
// span partition boundaries, so this shadow table is the authoritative dedup layer.
type Deduplicator struct {
	pool *pgxpool.Pool
}

func NewDeduplicator(pool *pgxpool.Pool) *Deduplicator {
	return &Deduplicator{pool: pool}
}

// TryInsert attempts to claim content_hash for articleID.
// Returns (true, nil) if the hash was inserted (new article).
// Returns (false, nil) if the hash already exists (duplicate; caller should skip insert).
func (d *Deduplicator) TryInsert(ctx context.Context, hash, articleID string) (bool, error) {
	tag, err := d.pool.Exec(ctx, `
		INSERT INTO article_content_hashes (content_hash, article_id)
		VALUES ($1, $2)
		ON CONFLICT (content_hash) DO NOTHING
	`, hash, articleID)
	if err != nil {
		return false, fmt.Errorf("dedup insert: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
