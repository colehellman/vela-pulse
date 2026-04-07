"""
normalize_lead and content_hash — TRD §3.1

CRITICAL: This implementation must produce bit-for-bit identical output to
any Go re-computation in the gateway. The algorithm is:
  1. Lowercase entire string
  2. Remove HTML tags
  3. Remove non-alphanumeric / non-space characters  ([^a-z0-9 ])
  4. Collapse all whitespace to single spaces and strip()
  5. Take the first 300 characters

content_hash = SHA-256(canonical_url + normalize_lead(text))
"""

import hashlib
import re
from html.parser import HTMLParser


class _HTMLStripper(HTMLParser):
    """Minimal HTML stripper that collects text nodes only."""

    def __init__(self) -> None:
        super().__init__()
        self._parts: list[str] = []

    def handle_data(self, data: str) -> None:
        self._parts.append(data)

    def result(self) -> str:
        return "".join(self._parts)


def _strip_html(text: str) -> str:
    s = _HTMLStripper()
    s.feed(text)
    return s.result()


def normalize_lead(text: str) -> str:
    """
    Canonical normalization per TRD §3.1.
    Returns the first 300 chars of the normalized string.
    """
    text = text.lower()
    text = _strip_html(text)
    text = re.sub(r"[^a-z0-9\s]", "", text)  # keep lowercase alphanum + any whitespace
    text = re.sub(r"\s+", " ", text)           # collapse all whitespace (tabs, newlines) to one space
    text = text.strip()
    return text[:300]


def content_hash(canonical_url: str, lead_text: str) -> str:
    """
    SHA-256(canonical_url + normalize_lead(lead_text)) as a lowercase hex digest.
    The canonical_url is concatenated raw (no separator) before hashing.
    """
    payload = canonical_url + normalize_lead(lead_text)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()
