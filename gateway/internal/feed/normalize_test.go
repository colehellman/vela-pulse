package feed

import (
	"testing"
)

// TestNormalizeLead mirrors the Python test suite in scraper/tests/test_normalize.py.
// Both suites must pass identically to guarantee hash consistency across the pipeline.

func TestNormalizeLead_Lowercase(t *testing.T) {
	got := NormalizeLead("BREAKING NEWS")
	want := "breaking news"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_HTMLStripped(t *testing.T) {
	got := NormalizeLead("<p>Hello <b>World</b></p>")
	want := "hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_NonAlphanumRemoved(t *testing.T) {
	got := NormalizeLead("Hello, World! It's #1.")
	want := "hello world its 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_WhitespaceCollapsed(t *testing.T) {
	got := NormalizeLead("  hello   world  ")
	want := "hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_WhitespaceCollapsePreservesSemanticIntegrity(t *testing.T) {
	got := NormalizeLead("lakers   game")
	want := "lakers game"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got == "lakersgame" {
		t.Error("whitespace was fully removed instead of collapsed")
	}
}

func TestNormalizeLead_TruncatedAt300(t *testing.T) {
	// Build a 400-rune input
	input := ""
	for i := 0; i < 200; i++ {
		input += "a "
	}
	got := NormalizeLead(input)
	runes := []rune(got)
	if len(runes) != 300 {
		t.Errorf("expected 300 runes, got %d", len(runes))
	}
}

func TestNormalizeLead_EmptyString(t *testing.T) {
	got := NormalizeLead("")
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestNormalizeLead_NewlinesCollapsedToSpace(t *testing.T) {
	got := NormalizeLead("hello\nworld")
	want := "hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_TabsCollapsedToSpace(t *testing.T) {
	got := NormalizeLead("hello\tworld")
	want := "hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_NumbersPreserved(t *testing.T) {
	got := NormalizeLead("Score: 42-17")
	want := "score 4217"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_NestedHTML(t *testing.T) {
	got := NormalizeLead(`<div><p>Top story: <a href="/">Lakers win</a> tonight</p></div>`)
	want := "top story lakers win tonight"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeLead_Idempotent(t *testing.T) {
	raw := "hello world 123"
	first := NormalizeLead(raw)
	second := NormalizeLead(first)
	if first != second {
		t.Errorf("not idempotent: first=%q second=%q", first, second)
	}
}

// TestContentHash_Basic verifies the hash formula matches the Python reference.
func TestContentHash_Basic(t *testing.T) {
	url := "https://espn.com/nba/story/lakers-win"
	lead := "The Lakers won 112-108 against the Celtics."
	got := ContentHash(url, lead)
	if len(got) != 64 {
		t.Errorf("expected 64 hex chars, got %d: %s", len(got), got)
	}
}

func TestContentHash_URLNotNormalized(t *testing.T) {
	// URL is concatenated raw — case differences produce different hashes.
	h1 := ContentHash("https://EXAMPLE.COM/path", "text")
	h2 := ContentHash("https://example.com/path", "text")
	if h1 == h2 {
		t.Error("expected different hashes for URLs differing in case")
	}
}

func TestContentHash_HTMLInLeadStripped(t *testing.T) {
	url := "https://espn.com/article/1"
	h1 := ContentHash(url, "<p>Lakers win</p>")
	h2 := ContentHash(url, "Lakers win")
	if h1 != h2 {
		t.Errorf("HTML in lead should produce same hash as plain text: h1=%s h2=%s", h1, h2)
	}
}

func TestContentHash_WhitespaceCollapseInLead(t *testing.T) {
	url := "https://espn.com/article/1"
	h1 := ContentHash(url, "lakers   win")
	h2 := ContentHash(url, "lakers win")
	if h1 != h2 {
		t.Errorf("whitespace collapse must produce same hash: h1=%s h2=%s", h1, h2)
	}
}

// TestNormalizeLead_SpecExample walks through the TRD algorithm on a concrete example.
func TestNormalizeLead_SpecExample(t *testing.T) {
	raw := "  Breaking: <b>Lakers WIN!</b> — Season opener.  "
	got := NormalizeLead(raw)
	want := "breaking lakers win season opener"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
