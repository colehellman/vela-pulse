from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env.local", env_file_encoding="utf-8", extra="ignore")

    redis_url: str = "redis://localhost:6379/0"
    stream_name: str = "vela:articles"
    stream_maxlen: int = 10_000  # approximate MAXLEN cap on the Redis stream

    # Backpressure thresholds (Tier C pause/resume)
    backpressure_high: int = 5_000
    backpressure_low: int = 2_000

    # Proxy tiers — comma-separated URLs
    proxy_tier1_urls: str = ""  # Home IP (SOCKS5 / direct)
    proxy_tier2_urls: str = ""  # Residential proxies

    # Proxy demotion: domain moves from Tier 2 → Tier 1 after N consecutive successes
    proxy_demotion_threshold: int = 50

    log_level: str = "INFO"

    @property
    def tier1_proxy_list(self) -> list[str]:
        return [u.strip() for u in self.proxy_tier1_urls.split(",") if u.strip()]

    @property
    def tier2_proxy_list(self) -> list[str]:
        return [u.strip() for u in self.proxy_tier2_urls.split(",") if u.strip()]


settings = Settings()
