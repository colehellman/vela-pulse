"""
Redis Streams publisher — TRD §3.2

Publishes a scraped article to the vela:articles stream via XADD.
All fields are strings (Redis Streams require string values).
The content_hash is computed here so the gateway can trust it on ingest.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone

from redis.asyncio import Redis

from .normalize import content_hash as compute_hash


@dataclass
class RawArticle:
    canonical_url: str
    title: str
    lead: str            # raw lead text; normalize_lead applied inside content_hash
    source_domain: str
    published_at: datetime
    tier: int            # 1 / 2 / 3 — scrape tier the article came from
    share_count: int = 0  # engagement signal; 0 when site doesn't surface it
    recency_bias: float = 1.0  # P_bias; override per source for known high-quality outlets


async def publish(rdb: Redis, article: RawArticle, stream: str, maxlen: int) -> str:
    """
    XADD article to *stream* with approximate MAXLEN cap.
    Returns the Redis stream entry ID (e.g. "1775595534460-0").
    """
    if article.published_at.tzinfo is None:
        # Treat naive datetimes as UTC; scrapers must provide timezone-aware datetimes.
        raise ValueError(f"published_at must be timezone-aware: {article.canonical_url}")

    payload: dict[str, str] = {
        "canonical_url": article.canonical_url,
        "content_hash":  compute_hash(article.canonical_url, article.lead),
        "title":         article.title,
        "lead":          article.lead,
        "source_domain": article.source_domain,
        "published_at":  article.published_at.astimezone(timezone.utc).isoformat(),
        "tier":          str(article.tier),
        "share_count":   str(article.share_count),
        "recency_bias":  str(article.recency_bias),
    }

    entry_id: str = await rdb.xadd(stream, payload, maxlen=maxlen, approximate=True)
    return entry_id
