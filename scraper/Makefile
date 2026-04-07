.PHONY: install run test lint

install:
	uv sync
	uv run playwright install chromium

run:
	uv run scraper

test:
	uv run pytest tests/ -v

lint:
	uv run ruff check scraper/ tests/
