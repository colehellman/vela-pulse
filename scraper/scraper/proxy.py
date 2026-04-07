"""
Proxy tier management and demotion logic — TRD §3.2

Demotion rule: when a domain has 50 consecutive successes on Tier 2 (Residential),
it is demoted back to Tier 1 (Home IP). The consecutive-success count is tracked in
Redis so it survives scraper restarts.

Redis key layout:
  vela:proxy:successes:{domain}  →  integer (INCR on success, DEL on failure)
  vela:proxy:tier:{domain}       →  "1" or "2" (explicit override; absent = default)
"""

from redis.asyncio import Redis

_SUCCESSES = "vela:proxy:successes"
_TIER      = "vela:proxy:tier"


async def get_tier(rdb: Redis, domain: str, default: int = 2) -> int:
    """Return the current proxy tier for *domain* (1 = Home, 2 = Residential)."""
    val = await rdb.get(f"{_TIER}:{domain}")
    return int(val) if val is not None else default


async def record_success(rdb: Redis, domain: str, current_tier: int, threshold: int = 50) -> int:
    """
    Increment the consecutive-success counter for *domain* on Tier 2.
    If the counter reaches *threshold*, demote the domain to Tier 1 and
    return the new tier. Returns *current_tier* unchanged for all other cases.
    """
    if current_tier != 2:
        return current_tier

    key = f"{_SUCCESSES}:{domain}"
    count = await rdb.incr(key)
    if count >= threshold:
        await rdb.delete(key)
        await rdb.set(f"{_TIER}:{domain}", 1)
        return 1
    return 2


async def record_failure(rdb: Redis, domain: str) -> None:
    """Reset the consecutive-success counter on any failure."""
    await rdb.delete(f"{_SUCCESSES}:{domain}")
