"""
Tests for normalize_lead and content_hash — TRD §3.1

These tests encode the exact algorithm spec. If any of them fail after a change
to normalize.py, the gateway's Go re-computation will diverge, causing silent
dedup failures across the entire pipeline.
"""

import hashlib

import pytest

from scraper.normalize import content_hash, normalize_lead


class TestNormalizeLead:
    def test_lowercase(self):
        assert normalize_lead("BREAKING NEWS") == "breaking news"

    def test_html_stripped(self):
        assert normalize_lead("<p>Hello <b>World</b></p>") == "hello world"

    def test_non_alphanum_removed(self):
        # Punctuation, special chars, and unicode are stripped
        assert normalize_lead("Hello, World! It's #1.") == "hello world its 1"

    def test_whitespace_collapsed(self):
        # Multiple spaces collapse to one; leading/trailing stripped
        assert normalize_lead("  hello   world  ") == "hello world"

    def test_whitespace_collapse_preserves_semantic_integrity(self):
        # Critical: collapsing to single space, NOT removing spaces entirely.
        # "lakers game" must not become "lakersgame".
        result = normalize_lead("lakers   game")
        assert result == "lakers game"
        assert "lakersgame" not in result

    def test_truncated_at_300_chars(self):
        long_text = "a " * 200   # 400 chars
        result = normalize_lead(long_text)
        assert len(result) == 300

    def test_truncation_applied_after_normalization(self):
        # Truncation is the LAST step — not applied to raw input.
        # 300 chars of repeated "ab " → normalized is "ab " repeated → slice at 300.
        raw = "ab " * 150      # 450 raw chars → 300 after normalize+truncate
        result = normalize_lead(raw)
        assert len(result) == 300

    def test_empty_string(self):
        assert normalize_lead("") == ""

    def test_html_entities_decoded_by_parser(self):
        # HTMLParser DOES decode entities: &amp; → &, then & is stripped by [^a-z0-9\s].
        # "&amp; hello" → text node "& hello" → strip non-alphanum → " hello" → strip → "hello"
        result = normalize_lead("&amp; hello")
        assert result == "hello"

    def test_newlines_treated_as_whitespace(self):
        # \n is not alphanumeric → removed by [^a-z0-9 ]; space remains.
        result = normalize_lead("hello\nworld")
        assert result == "hello world"

    def test_tabs_treated_as_whitespace(self):
        result = normalize_lead("hello\tworld")
        assert result == "hello world"

    def test_numbers_preserved(self):
        assert normalize_lead("Score: 42-17") == "score 4217"

    def test_nested_html(self):
        html = "<div><p>Top story: <a href='/'>Lakers win</a> tonight</p></div>"
        assert normalize_lead(html) == "top story lakers win tonight"

    def test_unicode_removed(self):
        assert normalize_lead("café résumé") == "caf rsum"

    def test_already_normalized_is_idempotent(self):
        raw = "hello world 123"
        assert normalize_lead(normalize_lead(raw)) == normalize_lead(raw)


class TestContentHash:
    def _expected(self, url: str, lead: str) -> str:
        payload = url + normalize_lead(lead)
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

    def test_basic(self):
        url = "https://espn.com/nba/story/lakers-win"
        lead = "The Lakers won 112-108 against the Celtics."
        assert content_hash(url, lead) == self._expected(url, lead)

    def test_hash_is_64_hex_chars(self):
        result = content_hash("https://example.com", "some text")
        assert len(result) == 64
        assert all(c in "0123456789abcdef" for c in result)

    def test_url_raw_no_normalization(self):
        # URL is concatenated raw — NOT normalized. Two URLs that differ only in
        # case must produce different hashes.
        h1 = content_hash("https://EXAMPLE.COM/path", "text")
        h2 = content_hash("https://example.com/path", "text")
        assert h1 != h2

    def test_different_urls_same_lead_differ(self):
        lead = "Same lead text"
        h1 = content_hash("https://espn.com/article/1", lead)
        h2 = content_hash("https://espn.com/article/2", lead)
        assert h1 != h2

    def test_same_url_different_lead_differ(self):
        url = "https://espn.com/article/1"
        h1 = content_hash(url, "First paragraph about sports.")
        h2 = content_hash(url, "Completely different content.")
        assert h1 != h2

    def test_html_in_lead_stripped_before_hash(self):
        url = "https://espn.com/article/1"
        h1 = content_hash(url, "<p>Lakers win</p>")
        h2 = content_hash(url, "Lakers win")
        # Both should produce the same hash because HTML is stripped in normalize_lead
        assert h1 == h2

    def test_whitespace_collapse_in_lead(self):
        url = "https://espn.com/article/1"
        h1 = content_hash(url, "lakers   win")
        h2 = content_hash(url, "lakers win")
        assert h1 == h2


class TestBackpressureIntegration:
    """Sanity-check that normalize_lead behavior matches the Go algorithm spec exactly."""

    def test_spec_example(self):
        # Walk through the TRD algorithm step by step on a concrete example.
        raw = "  Breaking: <b>Lakers WIN!</b> — Season opener.  "

        # Step 1: lowercase
        s = raw.lower()
        assert s == "  breaking: <b>lakers win!</b> — season opener.  "

        # Step 2: strip HTML
        # (HTMLParser extracts text nodes: "  breaking: ", "lakers win!", " — season opener.  ")
        # Result: "  breaking: lakers win! — season opener.  "

        # Step 3+4+5: normalize
        result = normalize_lead(raw)
        assert result == "breaking lakers win season opener"
