package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/colehellman/vela-pulse/gateway/internal/feed"
)

func TestHealthHandler(t *testing.T) {
	h := &HealthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
}

func TestHealthHandler_RejectsPost(t *testing.T) {
	h := &HealthHandler{}
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestLoadGlobalArticlesWithFallback_UsesPostgresOnCacheMiss(t *testing.T) {
	expected := []feed.Article{{ID: "a1"}}
	cacheCalls := 0
	fallbackCalls := 0

	got, err := loadGlobalArticlesWithFallback(
		func() ([]feed.Article, error) {
			cacheCalls++
			return nil, nil
		},
		func() ([]feed.Article, error) {
			fallbackCalls++
			return expected, nil
		},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cacheCalls != 1 {
		t.Fatalf("cacheCalls=%d want 1", cacheCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("fallbackCalls=%d want 1", fallbackCalls)
	}
	if len(got) != 1 || got[0].ID != expected[0].ID {
		t.Fatalf("unexpected articles: %+v", got)
	}
}

func TestLoadGlobalArticlesWithFallback_SkipsPostgresOnCacheHit(t *testing.T) {
	expected := []feed.Article{{ID: "a1"}}
	fallbackCalls := 0

	got, err := loadGlobalArticlesWithFallback(
		func() ([]feed.Article, error) {
			return expected, nil
		},
		func() ([]feed.Article, error) {
			fallbackCalls++
			return nil, nil
		},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallbackCalls=%d want 0", fallbackCalls)
	}
	if len(got) != 1 || got[0].ID != expected[0].ID {
		t.Fatalf("unexpected articles: %+v", got)
	}
}

func TestLoadGlobalArticlesWithFallback_BothInfraFailDegradesToEmpty(t *testing.T) {
	infraErr := fmt.Errorf("infra down")

	got, err := loadGlobalArticlesWithFallback(
		func() ([]feed.Article, error) { return nil, infraErr },
		func() ([]feed.Article, error) { return nil, infraErr },
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("expected nil error (degrade to empty), got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %+v", got)
	}
}

func TestLoadGlobalArticlesWithFallback_EmptySliceCacheHitSkipsFallback(t *testing.T) {
	fallbackCalls := 0

	got, err := loadGlobalArticlesWithFallback(
		func() ([]feed.Article, error) { return []feed.Article{}, nil },
		func() ([]feed.Article, error) {
			fallbackCalls++
			return []feed.Article{{ID: "a1"}}, nil
		},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallbackCalls=%d want 0: empty non-nil slice is a cache hit", fallbackCalls)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %+v", got)
	}
}

func TestSnapshotOffset_RejectsUnknownArticle(t *testing.T) {
	articles := []feed.Article{{ID: "a1"}}

	if _, err := snapshotOffset(articles, "missing"); err == nil {
		t.Fatal("expected error for missing article")
	}
}

func TestSnapshotOffset_HappyPath(t *testing.T) {
	articles := []feed.Article{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}

	cases := []struct {
		afterID string
		want    int
	}{
		{"a1", 1},
		{"a2", 2},
		{"a3", 3},
	}
	for _, tc := range cases {
		got, err := snapshotOffset(articles, tc.afterID)
		if err != nil {
			t.Fatalf("afterID=%s: unexpected error: %v", tc.afterID, err)
		}
		if got != tc.want {
			t.Fatalf("afterID=%s: got offset %d, want %d", tc.afterID, got, tc.want)
		}
	}
}

func TestWriteResponse_EmitsContentHash(t *testing.T) {
	h := &FeedHandler{}
	w := httptest.NewRecorder()
	articles := []feed.Article{{
		ID:           "a1",
		ContentHash:  "hash-1",
		Title:        "Title",
		CanonicalURL: "https://example.com",
		SourceDomain: "example.com",
		PublishedAt:  time.Unix(1, 0).UTC(),
		PulseScore:   1.5,
	}}

	h.writeResponse(w, articles, "snap-1", 20)

	var body struct {
		Articles []map[string]any `json:"articles"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body.Articles[0]["content_hash"]; got != "hash-1" {
		t.Fatalf("content_hash=%v want hash-1", got)
	}
}

func TestWriteResponse_NextCursorPresentWhenMorePages(t *testing.T) {
	h := &FeedHandler{}
	w := httptest.NewRecorder()

	articles := make([]feed.Article, 5)
	for i := range articles {
		articles[i] = feed.Article{
			ID:          fmt.Sprintf("a%d", i),
			PublishedAt: time.Unix(int64(i), 0).UTC(),
		}
	}

	h.writeResponse(w, articles, "snap-1", 2)

	var body struct {
		Articles   []map[string]any `json:"articles"`
		NextCursor string           `json:"next_cursor"`
		Total      int              `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.NextCursor == "" {
		t.Fatal("next_cursor must be present when articles > limit")
	}
	if len(body.Articles) != 2 {
		t.Fatalf("expected 2 articles in page, got %d", len(body.Articles))
	}
	if body.Total != 5 {
		t.Fatalf("total=%d want 5 (full slice length, not page length)", body.Total)
	}
}

func TestWriteResponse_NextCursorAbsentOnLastPage(t *testing.T) {
	h := &FeedHandler{}
	w := httptest.NewRecorder()

	articles := []feed.Article{
		{ID: "a1", PublishedAt: time.Unix(1, 0).UTC()},
		{ID: "a2", PublishedAt: time.Unix(2, 0).UTC()},
	}

	h.writeResponse(w, articles, "snap-1", 20)

	var body struct {
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.NextCursor != nil && *body.NextCursor != "" {
		t.Fatalf("next_cursor must be absent on last page, got %q", *body.NextCursor)
	}
}
