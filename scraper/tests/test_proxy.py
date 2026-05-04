"""
TDD tests for proxy tier management — TRD §3.2

Tests are written first; proxy.py is verified against them.
"""

import pytest
from unittest.mock import AsyncMock


# ---------------------------------------------------------------------------
# Helpers: build a fake async Redis client
# ---------------------------------------------------------------------------

def _make_rdb(get_val=None, incr_val=1):
    """Return an async Redis mock with configurable get/incr behavior."""
    rdb = AsyncMock()
    rdb.get = AsyncMock(return_value=get_val)
    rdb.incr = AsyncMock(return_value=incr_val)
    rdb.set = AsyncMock()
    rdb.delete = AsyncMock()
    return rdb


# ---------------------------------------------------------------------------
# get_tier
# ---------------------------------------------------------------------------

class TestGetTier:
    @pytest.mark.asyncio
    async def test_returns_default_when_no_key(self):
        from scraper.proxy import get_tier
        rdb = _make_rdb(get_val=None)
        assert await get_tier(rdb, "espn.com") == 2

    @pytest.mark.asyncio
    async def test_returns_default_1_when_specified(self):
        from scraper.proxy import get_tier
        rdb = _make_rdb(get_val=None)
        assert await get_tier(rdb, "espn.com", default=1) == 1

    @pytest.mark.asyncio
    async def test_returns_stored_tier(self):
        from scraper.proxy import get_tier
        rdb = _make_rdb(get_val=b"1")
        assert await get_tier(rdb, "espn.com") == 1

    @pytest.mark.asyncio
    async def test_returns_tier2_when_stored(self):
        from scraper.proxy import get_tier
        rdb = _make_rdb(get_val=b"2")
        assert await get_tier(rdb, "espn.com") == 2

    @pytest.mark.asyncio
    async def test_reads_correct_redis_key(self):
        from scraper.proxy import get_tier
        rdb = _make_rdb(get_val=None)
        await get_tier(rdb, "nba.com")
        rdb.get.assert_awaited_once_with("vela:proxy:tier:nba.com")


# ---------------------------------------------------------------------------
# record_success
# ---------------------------------------------------------------------------

class TestRecordSuccess:
    @pytest.mark.asyncio
    async def test_noop_on_tier1(self):
        from scraper.proxy import record_success
        rdb = _make_rdb()
        result = await record_success(rdb, "espn.com", current_tier=1)
        assert result == 1
        rdb.incr.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_noop_on_tier3(self):
        from scraper.proxy import record_success
        rdb = _make_rdb()
        result = await record_success(rdb, "espn.com", current_tier=3)
        assert result == 3
        rdb.incr.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_increments_counter_on_tier2(self):
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=5)
        await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        rdb.incr.assert_awaited_once_with("vela:proxy:successes:espn.com")

    @pytest.mark.asyncio
    async def test_no_demotion_below_threshold(self):
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=49)
        result = await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        assert result == 2
        rdb.set.assert_not_awaited()
        rdb.delete.assert_not_awaited()

    @pytest.mark.asyncio
    async def test_demotion_at_threshold(self):
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=50)
        result = await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        assert result == 1

    @pytest.mark.asyncio
    async def test_demotion_clears_counter(self):
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=50)
        await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        rdb.delete.assert_awaited_once_with("vela:proxy:successes:espn.com")

    @pytest.mark.asyncio
    async def test_demotion_sets_tier1_in_redis(self):
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=50)
        await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        rdb.set.assert_awaited_once_with("vela:proxy:tier:espn.com", 1)

    @pytest.mark.asyncio
    async def test_demotion_at_above_threshold(self):
        # Should also demote if counter somehow exceeds threshold (e.g., race).
        from scraper.proxy import record_success
        rdb = _make_rdb(incr_val=99)
        result = await record_success(rdb, "espn.com", current_tier=2, threshold=50)
        assert result == 1


# ---------------------------------------------------------------------------
# record_failure
# ---------------------------------------------------------------------------

class TestRecordFailure:
    @pytest.mark.asyncio
    async def test_deletes_success_counter(self):
        from scraper.proxy import record_failure
        rdb = _make_rdb()
        await record_failure(rdb, "espn.com")
        rdb.delete.assert_awaited_once_with("vela:proxy:successes:espn.com")

    @pytest.mark.asyncio
    async def test_does_not_change_tier(self):
        from scraper.proxy import record_failure
        rdb = _make_rdb()
        await record_failure(rdb, "espn.com")
        rdb.set.assert_not_awaited()
