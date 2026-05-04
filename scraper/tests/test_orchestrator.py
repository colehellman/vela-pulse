"""
TDD tests for TierOrchestrator — TRD §3.2

Backpressure and tier dispatch logic.
Tests are written against the interface; implementation must satisfy them.
"""

import pytest
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch

from scraper.config import Settings
from scraper.publisher import RawArticle


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _settings(**overrides) -> Settings:
    base = dict(
        redis_url="redis://localhost:6379",
        stream_name="vela:articles",
        stream_maxlen=10_000,
        backpressure_high=5_000,
        backpressure_low=2_000,
        proxy_demotion_threshold=50,
    )
    base.update(overrides)
    return Settings(**base)


def _make_rdb(stream_len: int = 0) -> AsyncMock:
    rdb = AsyncMock()
    rdb.xlen = AsyncMock(return_value=stream_len)
    rdb.xadd = AsyncMock(return_value="1234567890-0")
    rdb.incr = AsyncMock(return_value=1)
    rdb.get = AsyncMock(return_value=None)
    rdb.set = AsyncMock()
    rdb.delete = AsyncMock()
    return rdb


_DEFAULT_SENTINEL = object()

def _make_site(tier: int, domain: str, articles=_DEFAULT_SENTINEL) -> MagicMock:
    site = MagicMock()
    site.tier = tier
    site.domain = domain
    # Use sentinel so callers can explicitly pass articles=[] for empty-list tests.
    default_articles = [
        RawArticle(
            canonical_url=f"https://{domain}/1",
            title="Test",
            lead="Test lead",
            source_domain=domain,
            published_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
            tier=tier,
        )
    ]
    site.scrape = AsyncMock(return_value=default_articles if articles is _DEFAULT_SENTINEL else articles)
    return site


# ---------------------------------------------------------------------------
# Backpressure
# ---------------------------------------------------------------------------

class TestBackpressure:
    @pytest.mark.asyncio
    async def test_all_tiers_run_when_stream_is_empty(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=0)
        site_a = _make_site(1, "espn.com")
        site_b = _make_site(2, "nba.com")
        site_c = _make_site(3, "bleacherreport.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site_a, site_b, site_c], settings=_settings())
        await orc.run_cycle()

        site_a.scrape.assert_awaited_once()
        site_b.scrape.assert_awaited_once()
        site_c.scrape.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_tier_c_paused_when_stream_exceeds_high(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=5_001)
        site_c = _make_site(3, "bleacherreport.com")
        site_a = _make_site(1, "espn.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site_a, site_c], settings=_settings())
        await orc.run_cycle()

        site_a.scrape.assert_awaited_once()
        site_c.scrape.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_tier_c_not_paused_exactly_at_high(self):
        # TRD: "pause when Redis Stream > 5,000" — strictly greater-than, so at 5,000 Tier C still runs.
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=5_000)
        site_c = _make_site(3, "bleacherreport.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site_c], settings=_settings())
        await orc.run_cycle()

        site_c.scrape.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_tier_c_paused_at_5001(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=5_001)
        site_c = _make_site(3, "bleacherreport.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site_c], settings=_settings())
        await orc.run_cycle()

        site_c.scrape.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_tier_c_resumes_below_low(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=5_001)
        site_c = _make_site(3, "bleacherreport.com")
        orc = TierOrchestrator(rdb=rdb, sites=[site_c], settings=_settings())

        # First cycle: pause
        await orc.run_cycle()
        site_c.scrape.assert_not_awaited()

        # Stream drops below low threshold
        rdb.xlen = AsyncMock(return_value=1_999)
        site_c.scrape.reset_mock()

        # Second cycle: resume
        await orc.run_cycle()
        site_c.scrape.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_tier_c_stays_paused_between_high_and_low(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=5_001)
        site_c = _make_site(3, "bleacherreport.com")
        orc = TierOrchestrator(rdb=rdb, sites=[site_c], settings=_settings())

        # First cycle: pause
        await orc.run_cycle()
        site_c.scrape.assert_not_awaited()

        # Stream is between high and low — tier C should remain paused
        rdb.xlen = AsyncMock(return_value=3_000)
        site_c.scrape.reset_mock()

        await orc.run_cycle()
        site_c.scrape.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_tiers_a_and_b_never_paused(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb(stream_len=9_999)
        site_a = _make_site(1, "espn.com")
        site_b = _make_site(2, "nba.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site_a, site_b], settings=_settings())
        await orc.run_cycle()

        site_a.scrape.assert_awaited_once()
        site_b.scrape.assert_awaited_once()


# ---------------------------------------------------------------------------
# Dispatch: publish and proxy tracking
# ---------------------------------------------------------------------------

class TestDispatch:
    @pytest.mark.asyncio
    async def test_articles_published_to_stream(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb()
        site = _make_site(1, "espn.com")
        orc = TierOrchestrator(rdb=rdb, sites=[site], settings=_settings())
        await orc.run_cycle()

        rdb.xadd.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_scrape_failure_does_not_crash_cycle(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb()
        bad_site = _make_site(1, "espn.com")
        bad_site.scrape = AsyncMock(side_effect=RuntimeError("network error"))
        good_site = _make_site(2, "nba.com")

        orc = TierOrchestrator(rdb=rdb, sites=[bad_site, good_site], settings=_settings())
        # Should not raise
        await orc.run_cycle()
        good_site.scrape.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_empty_article_list_does_not_call_xadd(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb()
        site = _make_site(1, "espn.com", articles=[])
        orc = TierOrchestrator(rdb=rdb, sites=[site], settings=_settings())
        await orc.run_cycle()

        rdb.xadd.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_scrape_failure_records_proxy_failure(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb()
        site = _make_site(1, "espn.com")
        site.scrape = AsyncMock(side_effect=RuntimeError("timeout"))

        orc = TierOrchestrator(rdb=rdb, sites=[site], settings=_settings())

        with patch("scraper.orchestrator.proxy_mod.record_failure", new_callable=AsyncMock) as mock_fail:
            await orc.run_cycle()
            mock_fail.assert_awaited_once_with(rdb, "espn.com")

    @pytest.mark.asyncio
    async def test_successful_publish_records_proxy_success(self):
        from scraper.orchestrator import TierOrchestrator

        rdb = _make_rdb()
        site = _make_site(2, "nba.com")

        orc = TierOrchestrator(rdb=rdb, sites=[site], settings=_settings())

        with patch("scraper.orchestrator.proxy_mod.record_success", new_callable=AsyncMock) as mock_ok:
            await orc.run_cycle()
            mock_ok.assert_awaited_once_with(rdb, "nba.com", 2, threshold=50)
