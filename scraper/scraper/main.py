"""
Vela Pulse scraper entrypoint.

Usage:
    uv run scraper
    # or
    python -m scraper.main
"""

from __future__ import annotations

import asyncio
import logging
import signal

import redis.asyncio as aioredis

from .browser import BrowserPool
from .config import settings
from .orchestrator import TierOrchestrator
from .sites.espn import ESPN_CONFIG, ESPNSite


def _configure_logging() -> None:
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )


async def _run() -> None:
    _configure_logging()
    log = logging.getLogger(__name__)

    rdb = aioredis.from_url(settings.redis_url, decode_responses=True)

    async with BrowserPool() as pool:
        # Tier 1 context — home IP (or first configured Tier 1 proxy).
        t1_proxy = settings.tier1_proxy_list[0] if settings.tier1_proxy_list else None
        ctx_t1 = await pool.new_context(proxy_url=t1_proxy)

        # Tier 2 context — residential proxy (first configured URL, if any).
        # Each site gets the context matching its tier at startup. If a domain is
        # demoted from Tier 2 → Tier 1 at runtime (tracked in Redis), the proxy
        # change takes effect on the next scraper restart.
        t2_proxy = settings.tier2_proxy_list[0] if settings.tier2_proxy_list else None
        ctx_t2 = await pool.new_context(proxy_url=t2_proxy)

        sites = [
            ESPNSite(config=ESPN_CONFIG, context=ctx_t1),
            # Add more sites here as Phase 2 matures.
            # Tier 2 sites use ctx_t2:
            #   SomeSite(config=SOME_CONFIG, context=ctx_t2),
        ]

        orchestrator = TierOrchestrator(rdb=rdb, sites=sites, settings=settings)

        loop = asyncio.get_running_loop()
        stop_event = asyncio.Event()

        def _handle_signal() -> None:
            log.info("shutdown signal received")
            stop_event.set()

        for sig in (signal.SIGINT, signal.SIGTERM):
            loop.add_signal_handler(sig, _handle_signal)

        cycle_task = asyncio.create_task(orchestrator.run_forever(cycle_interval_secs=60.0))
        await stop_event.wait()

        cycle_task.cancel()
        try:
            await cycle_task
        except asyncio.CancelledError:
            pass

        await ctx_t1.close()
        await ctx_t2.close()

    await rdb.aclose()
    log.info("scraper stopped")


def main() -> None:
    asyncio.run(_run())


if __name__ == "__main__":
    main()
