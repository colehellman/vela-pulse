package feed

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// normalizeRegexp removes any character that is not a-z, 0-9, or whitespace.
// Must stay in sync with Python: re.sub(r"[^a-z0-9\s]", "", text)
var normalizeRegexp = regexp.MustCompile(`[^a-z0-9\s]`)

// whitespaceRegexp collapses all runs of whitespace (space, tab, newline) to a single space.
// Must stay in sync with Python: re.sub(r"\s+", " ", text)
var whitespaceRegexp = regexp.MustCompile(`\s+`)

// NormalizeLead implements the canonical normalization algorithm from TRD §3.1.
// Output must be bit-for-bit identical to the Python scraper's normalize_lead().
//
// Steps:
//  1. Lowercase
//  2. Strip HTML tags (extract text nodes only)
//  3. Remove non-alphanumeric / non-whitespace characters
//  4. Collapse all whitespace to single spaces and strip
//  5. Take first 300 characters (by rune, not byte)
func NormalizeLead(text string) string {
	text = strings.ToLower(text)
	text = stripHTML(text)
	text = normalizeRegexp.ReplaceAllString(text, "")
	text = whitespaceRegexp.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	runes := []rune(text)
	if len(runes) > 300 {
		runes = runes[:300]
	}
	return string(runes)
}

// ContentHash computes SHA-256(canonical_url + NormalizeLead(leadText)) per TRD §3.1.
func ContentHash(canonicalURL, leadText string) string {
	payload := canonicalURL + NormalizeLead(leadText)
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

// stripHTML extracts text nodes from an HTML fragment, discarding all tags.
func stripHTML(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		// Fallback: return as-is; downstream normalization will handle it.
		return s
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return b.String()
}
