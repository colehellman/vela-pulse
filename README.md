# Vela Pulse

Hybrid-cloud sports news platform: scraper to Redis Streams, Go gateway to Postgres, iOS client on top.

## Repo layout

- `gateway/`: Go API, scoring, feed ingestion, auth, and snapshot pagination
- `scraper/`: Python scraper built around Playwright and Redis publishing
- `infra/`: Docker Compose, SQL migrations, and Terraform provisioning
- `ios/`: SwiftUI client source

## Prerequisites

- Go 1.25+
- Python 3.12+ with `uv`
- Docker
- `goose` available at `$(go env GOPATH)/bin/goose`
- Playwright Chromium installed for the scraper

## Local setup

```bash
cp .env.example .env.local
make dev-up
make migrate
```

Install language-specific dependencies:

```bash
cd gateway && go mod download
cd scraper && uv sync && uv run playwright install chromium
```

## Common commands

```bash
make gateway-run
make scraper-run
make lint
make test
```

## More detail

`CLAUDE.md` contains the fuller architecture, environment, and deployment notes.
