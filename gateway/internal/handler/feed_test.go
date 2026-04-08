package handler

import (
	"encoding/json"
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

func TestSnapshotOffset_RejectsUnknownArticle(t *testing.T) {
	articles := []feed.Article{{ID: "a1"}}

	if _, err := snapshotOffset(articles, "missing"); err == nil {
		t.Fatal("expected error for missing article")
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
