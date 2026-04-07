# Vela Pulse

Hybrid-cloud sports news intelligence platform. Homelab scraper → Redis Streams → OCI gateway → iOS app.

## Architecture

```
[HP EliteDesk homelab]  →  [Redis Streams]  →  [OCI VPC gateway]  →  [iPhone]
  Python scraper               vela:articles      Go API + Postgres
  Playwright + stealth                            pg_partman weekly
```

## Prerequisites

- Go 1.22+
- Python 3.12+ with `uv`
- Docker (for local Postgres + Redis)
- `goose` CLI: already at `$(go env GOPATH)/bin/goose`
- Playwright browsers: `cd scraper && uv run playwright install chromium`

## Install dependencies

```bash
# Gateway
cd gateway && go mod download

# Scraper
cd scraper && uv sync && uv run playwright install chromium
```

## Run tests

```bash
# All (from repo root)
make test

# Gateway only
cd gateway && go test ./... -race

# Scraper only
cd scraper && uv run pytest tests/ -v
```

## Start local dev stack

```bash
# 1. Start Postgres + Redis
make dev-up

# 2. Apply migrations (runs goose)
make migrate

# 3. Start gateway (on :8080)
make gateway-run

# 4. Start scraper (optional — requires homelab proxy config)
make scraper-run
```

## Verify the stack

```bash
curl http://localhost:8080/health          # → {"status":"ok"}
curl http://localhost:8080/v1/feed         # → {articles:[], snapshot_id:...}

# Publish a test article manually:
docker exec vela-redis redis-cli XADD vela:articles '*' \
  canonical_url "https://espn.com/test" \
  content_hash "$(echo -n 'test' | sha256sum | cut -d' ' -f1)" \
  title "Test Article" lead "test lead" source_domain "espn.com" \
  published_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" tier "1" \
  share_count "0" recency_bias "1.0"

sleep 3 && curl http://localhost:8080/v1/feed   # article appears
```

## Environment variables

Copy `.env.example` to `.env.local`. Required:

| Variable | Description |
|---|---|
| `DATABASE_URL` | Postgres DSN |
| `REDIS_URL` | Redis URL |
| `JWT_SECRET` | HS256 signing secret (32+ bytes) |
| `APPLE_CLIENT_ID` | App bundle ID (e.g. `com.vela.pulse`) — only needed for SIWA |

## Key design decisions

**Dedup**: `article_content_hashes` shadow table provides cross-partition uniqueness since PostgreSQL `UNIQUE` constraints don't span partition boundaries. The consumer uses `INSERT ... ON CONFLICT DO NOTHING` and checks `RowsAffected`.

**Pulse Score**: `min(100, S_weight*log10(R+1) + P_bias/(age+30)^1.5)`. Computed at write time, stored in `articles.pulse_score`. `share_count` comes from the stream; default 0.

**normalize_lead**: Must be bit-for-bit identical between Python scraper and Go gateway. Tests in both suites encode the same spec examples — both must pass before merging changes to normalization.

**Snapshot pagination**: `snapshot_id` cached in Redis with 5-min TTL. HTTP 410 on expiry signals client to re-fetch Page 1.

**partman maintenance**: The gateway runs `partman.run_maintenance()` every hour at startup. Without this, pg_partman won't create future weekly partitions and INSERTs will fail.

## OCI deployment

```bash
cd infra/terraform
terraform init
terraform apply   # provisions VCN, subnet, compute instance
# Then: SSH in, docker compose up -d, run migrations
```

## Backup (TrueNAS)

Daily `pg_dump` runs via cron on the OCI instance:
```bash
crontab -e
# 0 2 * * * /opt/vela/scripts/backup.sh
```
See `infra/scripts/backup.sh`. RPO: 24h, RTO: 2h.
