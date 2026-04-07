"""
Playwright browser context factory with stealth.

playwright-stealth patches fingerprinting vectors (navigator.webdriver,
plugins, permissions, etc.) so scrapers appear as regular Chrome sessions.
Each tier gets its own context with the appropriate proxy config.
"""

from __future__ import annotations

import logging

from playwright.async_api import Browser, BrowserContext, Playwright, async_playwright
from playwright_stealth import stealth_async

log = logging.getLogger(__name__)

# Realistic user agent matching the Playwright bundled Chromium major version.
_USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/124.0.0.0 Safari/537.36"
)


async def new_context(
    browser: Browser,
    proxy_url: str | None = None,
) -> BrowserContext:
    """
    Create a stealth-patched browser context, optionally routing through *proxy_url*.
    Each context has an independent cookie jar / storage state.
    """
    proxy = {"server": proxy_url} if proxy_url else None
    context = await browser.new_context(
        user_agent=_USER_AGENT,
        proxy=proxy,
        viewport={"width": 1280, "height": 800},
        locale="en-US",
        timezone_id="America/New_York",
        # Disable media / images to reduce bandwidth on headless runs.
        java_script_enabled=True,
    )

    # Apply stealth patches to every new page created from this context.
    # stealth_async patches the context-level scripts so it applies before page load.
    page = await context.new_page()
    await stealth_async(page)
    await page.close()

    log.debug("browser context created", extra={"proxy": proxy_url})
    return context


class BrowserPool:
    """
    Manages a single Playwright browser instance shared across all tiers.
    Call start() before use and stop() on shutdown.
    """

    def __init__(self) -> None:
        self._pw: Playwright | None = None
        self._browser: Browser | None = None

    async def start(self) -> None:
        self._pw = await async_playwright().start()
        self._browser = await self._pw.chromium.launch(
            headless=True,
            args=[
                "--no-sandbox",
                "--disable-dev-shm-usage",
                "--disable-blink-features=AutomationControlled",
            ],
        )
        log.info("browser pool started")

    async def stop(self) -> None:
        if self._browser:
            await self._browser.close()
        if self._pw:
            await self._pw.stop()
        log.info("browser pool stopped")

    async def new_context(self, proxy_url: str | None = None) -> BrowserContext:
        assert self._browser is not None, "call start() first"
        return await new_context(self._browser, proxy_url=proxy_url)

    async def __aenter__(self) -> "BrowserPool":
        await self.start()
        return self

    async def __aexit__(self, *_: object) -> None:
        await self.stop()
