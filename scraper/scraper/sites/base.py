"""
BaseSite ABC — all site-specific scrapers implement this interface.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass

from playwright.async_api import BrowserContext

from ..publisher import RawArticle


@dataclass
class SiteConfig:
    domain: str    # e.g. "espn.com"
    tier: int      # 1 / 2 / 3
    url: str       # entry point to scrape


class BaseSite(ABC):
    def __init__(self, config: SiteConfig, context: BrowserContext) -> None:
        self.config = config
        self.context = context

    @abstractmethod
    async def scrape(self) -> list[RawArticle]:
        """
        Scrape the site and return a list of articles.
        Return an empty list (not None) if nothing was found or scraping failed.
        """
        ...

    @property
    def domain(self) -> str:
        return self.config.domain

    @property
    def tier(self) -> int:
        return self.config.tier
