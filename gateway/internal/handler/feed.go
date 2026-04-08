package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/colehellman/vela-pulse/gateway/internal/feed"
	"github.com/colehellman/vela-pulse/gateway/internal/middleware"
	"github.com/colehellman/vela-pulse/gateway/internal/snapshot"
)

// errCursorNotFound is returned by snapshotOffset when afterID is not present in the
// snapshot. The caller maps this to 410 Gone so the iOS client retries from page 1.
var errCursorNotFound = errors.New("cursor not found in snapshot")

const (
	defaultPageSize = 20
	maxPageSize     = 100
	snapshotPrefix  = "vela:snapshot:"
)

// FeedHandler serves GET /v1/feed using the Two-Pass Merge algorithm.
type FeedHandler struct {
	pool           *pgxpool.Pool
	rdb            *goredis.Client
	globalCacheKey string
	log            *zap.Logger
}

func NewFeedHandler(pool *pgxpool.Pool, rdb *goredis.Client, globalCacheKey string, log *zap.Logger) *FeedHandler {
	return &FeedHandler{pool: pool, rdb: rdb, globalCacheKey: globalCacheKey, log: log}
}

type feedResponse struct {
	Articles   []articleJSON `json:"articles"`
	NextCursor string        `json:"next_cursor,omitempty"`
	SnapshotID string        `json:"snapshot_id"`
	Total      int           `json:"total"`
}

type articleJSON struct {
	ID           string    `json:"id"`
	ContentHash  string    `json:"content_hash"`
	Title        string    `json:"title"`
	CanonicalURL string    `json:"canonical_url"`
	SourceDomain string    `json:"source_domain"`
	PublishedAt  time.Time `json:"published_at"`
	PulseScore   float64   `json:"pulse_score"`
}

func (h *FeedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	q := r.URL.Query()

	limit := defaultPageSize
	if ls := q.Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= maxPageSize {
			limit = n
		}
	}

	snapID := q.Get("snapshot_id")
	cursor := q.Get("cursor")

	// If a snapshot_id is provided (page 2+), serve from the cached snapshot.
	if snapID != "" && cursor != "" {
		h.serveFromSnapshot(ctx, w, snapID, cursor, limit)
		return
	}

	// Page 1: build a fresh merged feed. User ID may be empty for anonymous requests.
	userID := middleware.UserIDFromContext(ctx)
	articles, err := h.buildFeed(ctx, userID, limit)
	if err != nil {
		h.log.Error("build feed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	snapID = snapshot.NewID()
	if err := h.cacheSnapshot(ctx, snapID, articles); err != nil {
		h.log.Warn("cache snapshot", zap.Error(err))
		// Non-fatal: pagination on page 2+ will fail gracefully with a cache miss.
	}

	h.writeResponse(w, articles, snapID, limit)
}

// buildFeed runs the Two-Pass Merge against Redis cache + Postgres.
// userID is empty for anonymous (global-only) requests.
func (h *FeedHandler) buildFeed(ctx context.Context, userID string, limit int) ([]feed.Article, error) {
	global, err := h.loadGlobalArticles(ctx)
	if err != nil {
		return nil, err
	}

	// Pass 2: user-specific articles from last 24h (only when authenticated).
	var userArticles []feed.Article
	if userID != "" {
		userArticles, err = h.fetchUserArticles(ctx, userID)
		if err != nil {
			// Non-fatal: degrade to global-only feed. Log at Error so outages page.
			h.log.Error("user articles fetch failed, degrading to global-only feed",
				zap.String("user_id", userID), zap.Error(err))
		}
	}

	return feed.TwoPassMerge(global, userArticles), nil
}

func (h *FeedHandler) loadGlobalArticles(ctx context.Context) ([]feed.Article, error) {
	return loadGlobalArticlesWithFallback(
		func() ([]feed.Article, error) {
			return feed.LoadGlobalCache(ctx, h.rdb, h.globalCacheKey)
		},
		func() ([]feed.Article, error) {
			return h.fetchGlobalArticles(ctx)
		},
		h.log,
	)
}

func loadGlobalArticlesWithFallback(
	loadCache func() ([]feed.Article, error),
	fallback func() ([]feed.Article, error),
	log *zap.Logger,
) ([]feed.Article, error) {
	global, err := loadCache()
	if err != nil {
		log.Error("global cache load failed, falling back to postgres", zap.Error(err))
	} else if global != nil {
		return global, nil
	} else {
		log.Info("global cache miss, falling back to postgres")
	}

	// Reach here on cache error or cache miss. Try Postgres; degrade to empty on failure.
	articles, fbErr := fallback()
	if fbErr != nil {
		log.Error("postgres fallback failed, degrading to empty global set", zap.Error(fbErr))
		return nil, nil
	}
	return articles, nil
}

func (h *FeedHandler) fetchGlobalArticles(ctx context.Context) ([]feed.Article, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, content_hash, title, canonical_url, source_domain, published_at, pulse_score
		FROM articles
		WHERE user_id IS NULL
		ORDER BY pulse_score DESC, published_at DESC
		LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []feed.Article
	for rows.Next() {
		var a feed.Article
		if err := rows.Scan(&a.ID, &a.ContentHash, &a.Title, &a.CanonicalURL,
			&a.SourceDomain, &a.PublishedAt, &a.PulseScore); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// fetchUserArticles queries Postgres for articles belonging to the user in the last 24h.
func (h *FeedHandler) fetchUserArticles(ctx context.Context, userID string) ([]feed.Article, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, content_hash, title, canonical_url, source_domain, published_at, pulse_score
		FROM articles
		WHERE user_id = $1
		  AND published_at > NOW() - INTERVAL '24 hours'
		ORDER BY pulse_score DESC, published_at DESC
		LIMIT 200
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []feed.Article
	for rows.Next() {
		var a feed.Article
		uid := userID
		if err := rows.Scan(&a.ID, &a.ContentHash, &a.Title, &a.CanonicalURL,
			&a.SourceDomain, &a.PublishedAt, &a.PulseScore); err != nil {
			return nil, err
		}
		a.UserID = &uid
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// serveFromSnapshot reads a cached page from Redis. On TTL expiry returns 410 Gone,
// signalling the client to re-fetch page 1.
func (h *FeedHandler) serveFromSnapshot(ctx context.Context, w http.ResponseWriter, snapID, cursor string, limit int) {
	key := snapshotPrefix + snapID
	b, err := h.rdb.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		// Snapshot expired (> 5min TTL). Force client to page 1.
		http.Error(w, "snapshot expired", http.StatusGone)
		return
	}
	if err != nil {
		h.log.Error("snapshot redis GET failed", zap.String("snap_id", snapID), zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var all []feed.Article
	if err := json.Unmarshal(b, &all); err != nil {
		h.log.Error("snapshot unmarshal failed", zap.String("snap_id", snapID), zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Decode cursor and find offset.
	_, afterID, err := snapshot.DecodeCursor(cursor)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	offset, err := snapshotOffset(all, afterID)
	if err != nil {
		// Cursor not in snapshot — snapshot was rebuilt since page 1. Force client to re-fetch.
		http.Error(w, "snapshot expired", http.StatusGone)
		return
	}

	page := paginate(all, offset, limit)
	h.writeResponse(w, page, snapID, limit)
}

func (h *FeedHandler) cacheSnapshot(ctx context.Context, snapID string, articles []feed.Article) error {
	b, err := json.Marshal(articles)
	if err != nil {
		return err
	}
	return h.rdb.Set(ctx, snapshotPrefix+snapID, b, snapshot.TTL).Err()
}

func (h *FeedHandler) writeResponse(w http.ResponseWriter, articles []feed.Article, snapID string, limit int) {
	page := articles
	if len(page) > limit {
		page = page[:limit]
	}

	resp := feedResponse{
		Articles:   make([]articleJSON, len(page)),
		SnapshotID: snapID,
		Total:      len(articles),
	}

	for i, a := range page {
		resp.Articles[i] = articleJSON{
			ID:           a.ID,
			ContentHash:  a.ContentHash,
			Title:        a.Title,
			CanonicalURL: a.CanonicalURL,
			SourceDomain: a.SourceDomain,
			PublishedAt:  a.PublishedAt,
			PulseScore:   a.PulseScore,
		}
	}

	// Attach next_cursor if there are more pages.
	if len(articles) > limit {
		last := page[len(page)-1]
		resp.NextCursor = snapshot.EncodeCursor(last.PublishedAt, last.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Warn("write feed response", zap.Error(err))
	}
}

// paginate returns a slice of articles starting at offset.
func paginate(articles []feed.Article, offset, limit int) []feed.Article {
	if offset >= len(articles) {
		return nil
	}
	end := offset + limit
	if end > len(articles) {
		end = len(articles)
	}
	return articles[offset:end]
}

func snapshotOffset(articles []feed.Article, afterID string) (int, error) {
	for i, a := range articles {
		if a.ID == afterID {
			return i + 1, nil
		}
	}
	return 0, errCursorNotFound
}
