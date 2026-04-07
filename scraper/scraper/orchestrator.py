"""
Tier orchestrator with backpressure — TRD §3.2

Backpressure rule:
  - When the Redis stream length exceeds backpressure_high (5,000): pause Tier C (3).
  - Resume Tier C when stream length drops below backpressure_low (2,000).
  - Tier A (1) and Tier B (2) always run regardless of stream depth.

The orchestrator runs scrape cycles in a loop; each cycle dispatches all
eligible sites and publishes their articles to the Redis stream.
"""

from __future__ import annotations

import asyncio
import logging

from redis.asyncio import Redis

from . import proxy as proxy_mod
from .config import Settings
from .publisher import RawArticle, publish
from .sites.base import BaseSite

log = logging.getLogger(__name__)


class TierOrchestrator:
    def __init__(
        self,
        rdb: Redis,
        sites: list[BaseSite],
        settings: Settings,
    ) -> None:
        self._rdb = rdb
        self._sites = sites
        self._settings = settings
        self._tier_c_paused = False

    async def _check_backpressure(self) -> None:
        length: int = await self._rdb.xlen(self._settings.stream_name)
        if length > self._settings.backpressure_high:
            if not self._tier_c_paused:
                log.warning("backpressure: pausing Tier C (stream=%d)", length)
            self._tier_c_paused = True
        elif length < self._settings.backpressure_low:
            if self._tier_c_paused:
                log.info("backpressure: resuming Tier C (stream=%d)", length)
            self._tier_c_paused = False

    def _eligible_tiers(self) -> set[int]:
        tiers = {1, 2}
        if not self._tier_c_paused:
            tiers.add(3)
        return tiers

    async def _dispatch(self, site: BaseSite) -> None:
        domain = site.domain
        tier = site.tier

        try:
            articles: list[RawArticle] = await site.scrape()
        except Exception:
            log.exception("scrape failed: %s", domain)
            await proxy_mod.record_failure(self._rdb, domain)
            return

        published = 0
        for article in articles:
            try:
                await publish(
                    self._rdb,
                    article,
                    stream=self._settings.stream_name,
                    maxlen=self._settings.stream_maxlen,
                )
                published += 1
            except Exception:
                log.exception("publish failed: %s", article.canonical_url)

        if published > 0:
            await proxy_mod.record_success(
                self._rdb,
                domain,
                tier,
                threshold=self._settings.proxy_demotion_threshold,
            )

        log.debug("dispatched %s: %d/%d published", domain, published, len(articles))

    async def run_cycle(self) -> None:
        """Run one full scrape cycle across all eligible sites."""
        await self._check_backpressure()
        eligible = self._eligible_tiers()

        tasks = [
            self._dispatch(site)
            for site in self._sites
            if site.tier in eligible
        ]
        await asyncio.gather(*tasks, return_exceptions=True)

    async def run_forever(self, cycle_interval_secs: float = 60.0) -> None:
        """Run cycles indefinitely, sleeping *cycle_interval_secs* between them."""
        log.info("orchestrator starting (interval=%.0fs)", cycle_interval_secs)
        while True:
            try:
                await self.run_cycle()
            except Exception:
                log.exception("cycle error")
            await asyncio.sleep(cycle_interval_secs)
