"""
ESPN scraper — Tier A example site.

Scrapes the ESPN front page for article headlines and leads.
This is a reference implementation; real extraction logic will
be tuned once Playwright stealth is wired end-to-end.
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from urllib.parse import urljoin

from ..publisher import RawArticle
from .base import BaseSite, SiteConfig

log = logging.getLogger(__name__)

ESPN_CONFIG = SiteConfig(domain="espn.com", tier=1, url="https://www.espn.com")


class ESPNSite(BaseSite):
    async def scrape(self) -> list[RawArticle]:
        articles: list[RawArticle] = []
        page = await self.context.new_page()
        try:
            await page.goto(self.config.url, wait_until="domcontentloaded", timeout=30_000)

            # ESPN article cards — selector subject to change; update as needed.
            cards = await page.query_selector_all("article.contentItem")
            for card in cards[:20]:  # cap at 20 per cycle
                try:
                    headline_el = await card.query_selector("h1, h2, h3")
                    lead_el     = await card.query_selector("p")
                    link_el     = await card.query_selector("a[href]")

                    if not headline_el or not link_el:
                        continue

                    title = (await headline_el.inner_text()).strip()
                    lead  = (await lead_el.inner_text()).strip() if lead_el else title
                    href  = await link_el.get_attribute("href") or ""
                    url   = urljoin(self.config.url, href)

                    articles.append(RawArticle(
                        canonical_url=url,
                        title=title,
                        lead=lead,
                        source_domain=self.config.domain,
                        published_at=datetime.now(timezone.utc),  # ESPN doesn't surface pub date in cards
                        tier=self.config.tier,
                    ))
                except Exception:
                    log.debug("skipping card", exc_info=True)
        except Exception:
            log.warning("espn scrape failed", exc_info=True)
        finally:
            await page.close()

        return articles
