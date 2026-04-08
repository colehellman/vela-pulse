# Technical Requirements Document: Vela Pulse (v7.3)

## 1. Project Vision

Vela Pulse is a high-velocity sports news intelligence platform. It utilizes a Hybrid-Cloud Architecture to balance heavy data extraction in a homelab with secure, scalable delivery via a cloud VPC, providing a unified "Super-Feed" of global and private sources.

## 2. Core Behavioral Logic & Ranking

### 2.1 The Pulse Score Formula

```
Score = min(100, (S_weight × log10(R_count + 1)) + P_bias / (T_now − T_pub + 30)^1.5)
```

- **Stability Floor:** 30s.
- **Temporal Pinning:** Rank stability for the top 10 items within 120s windows; client-side re-sorting freeze to prevent UI "flicker."

### 2.2 Namespace & Promotion

- **Origin Definitions:** `global` (Vetted), `private` (User-added), `hybrid` (Private promoted by Global).
- **Conflict Resolution:** If a `content_hash` collision occurs between namespaces, Global metadata wins, but the UI retains the original private source attribution.

## 3. Data Pipeline & Ingestion

### 3.1 Canonical Normalization & Hashing

To ensure 100% hash consistency between Python (Homelab) and Go (VPC), the `content_hash` is generated as:

```
SHA-256(canonical_url + normalize_lead(text))
```

**`normalize_lead(text)` Specs:**

1. Lowercase entire string.
2. Remove HTML tags.
3. Remove non-alphanumeric/non-space characters.
4. Collapse all whitespace to single spaces and `strip()`.
5. Take the first 300 characters.

### 3.2 Tiered Scraping & Backpressure

- **Infrastructure:** HP EliteDesk 800 G3 (Playwright + Stealth).
- **Backpressure:** Pause Tier C (low priority) when Redis Stream > 5,000; resume at 2,000.
- **Proxy Demotion:** Domain returns from Tier 2 (Residential) to Tier 1 (Home) after 50 consecutive successes, tracked in Redis.

## 4. The Gateway (OCI VPC)

### 4.1 Data & Storage

- **Database:** PostgreSQL with weekly partitioning via `pg_partman`.
- **Schema Logic:**
  - `user_id UUID REFERENCES users(id) NULL` (NULL = Global).
  - `UNIQUE INDEX` on `content_hash`.
- **API:** Go (Golang) using Two-Pass Merge:
  - **Pass 1:** Global Cache (Redis, Top 200).
  - **Pass 2:** User-specific articles (Last 24h) fetched and merged in-memory (< 50ms).

### 4.2 Snapshot Pagination

- **`snapshot_id`:** Generated as `Unix_Milliseconds_Rand4`.
- **TTL:** 5-minute expiry in Redis. Forces fresh pull on Page 1 if Page 2+ exceeds TTL.

## 5. Mobile & Operations

### 5.1 Client (iPhone 17)

- **Storage:** SwiftData (Flat Schema). No complex `@Relationship` tags in the high-frequency feed to maintain 60fps.
- **Sync:** Pull-based with `Base64(published_at | id)` cursor.

### 5.2 Resilience

- **Kill Switches:** Redis Pub/Sub for immediate logic toggles (Scoring, AI, Ingest).
- **Backup:** Daily automated Postgres dump to TrueNAS (Home). RPO: 24h, RTO: 2h.
- **Eventual Consistency:** Feed is consistent within ~90s of scrape completion.

## 6. Implementation Roadmap

1. **Phase 1 (Foundation):** OCI Setup + Postgres (Nullable `user_id` Schema) + `pg_partman`.
2. **Phase 2 (Ingestion):** Playwright + `normalize_lead` function + Redis Streams.
3. **Phase 3 (The Engine):** Write-time Scoring + Two-Pass Merge + Redis Snapshotting.
4. **Phase 4 (The Delivery):** Go API + SIWA + Cloudflare.
5. **Phase 5 (Mobile):** SwiftUI + SwiftData.
