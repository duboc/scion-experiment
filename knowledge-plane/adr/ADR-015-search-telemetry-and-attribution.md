# ADR-015: Closed-Loop Search Telemetry, Click Attribution & Online A/B Metrics

- **Status:** Proposed (Wave 6 — Production Search Depth)
- **Covered Issue:** GitHub **Issue #43** — "Closed-Loop Search Telemetry (`query_id`), Click/Conversion Attribution, A/B Experiment KPIs & LTR Signal Rollup"
- **Migrations:** `src/iac/migrations/026_search_events.sql`

## Context
Today `psearch` consumes engagement features (`clicks`, `orders`, `ctr`, `conversion_rate`) in Stage-1 SQL boosts (`ADR-004`) and Stage-2 Learning-to-Rank (`ADR-006`, `internal/services/ltr.go`), but those columns are populated only by static seed data (`016b_backfill_boost_signals.dml.sql`). Furthermore, search operators have no visibility into:
- **Zero-result queries** (demand gaps requiring synonyms or catalog expansion),
- **Search CTR & Mean Reciprocal Rank (`MRR@10`)** by query or retrieval mode,
- **A/B experiment performance** (`experiment_id`, e.g., `hybrid_only` vs `hybrid_ltr` or `alpha=0.3` vs `alpha=0.7`),
- **Real click & conversion feedback** closing the loop back into product popularity/LTR signals.

## Decision

### 1. Schema (`026_search_events.sql`, Online DDL with TTL)
Create two tenant-scoped tables in Cloud Spanner with automatic row-deletion policies so operational storage stays bounded:
```sql
CREATE TABLE search_impressions (
  impression_id STRING(64) NOT NULL,
  tenant_id STRING(64) NOT NULL,
  query_raw STRING(512),
  query_norm STRING(512),
  experiment_id STRING(64),
  retrieval_mode STRING(32),
  total_found INT64 NOT NULL,
  latency_ms FLOAT64,
  result_ids ARRAY<STRING(1024)>,
  created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (tenant_id, impression_id),
  ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));

CREATE INDEX search_impressions_by_tenant_time
ON search_impressions(tenant_id, created_at DESC)
STORING (query_norm, experiment_id, total_found, latency_ms);

CREATE TABLE search_conversions (
  tenant_id STRING(64) NOT NULL,
  event_id STRING(64) NOT NULL,
  impression_id STRING(64) NOT NULL,
  session_id STRING(64),
  event_type STRING(24) NOT NULL, -- 'click' | 'add_to_cart' | 'purchase'
  product_id STRING(1024) NOT NULL,
  position INT64 NOT NULL,       -- 1-based rank position clicked
  experiment_id STRING(64),
  revenue_micros INT64 NOT NULL DEFAULT (0),
  created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (tenant_id, event_id),
  ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));

CREATE INDEX search_conversions_by_impression
ON search_conversions(tenant_id, impression_id, event_type)
STORING (product_id, position, revenue_micros);
```

### 2. Non-Blocking Impression Tracking & `query_id` on `/api/search`
1. `SearchRequest` accepts optional `experiment_id` (`string`, `<= 64` chars, `[a-zA-Z0-9._-]`).
2. Every `/api/search` call generates a UUID/ULID `query_id` (`impression_id`), returns `query_id` and `experiment_id` on `SearchResponse`, and enqueues an impression record onto an **asynchronous bounded buffered channel** (`AnalyticsService`, buffer e.g. `2048`, batch-flushed or background-written with a 2s timeout) so search tail latency is **never** blocked by telemetry writes.

### 3. Attribution & Signal Rollup Endpoints (`internal/services/analytics.go`)
1. **`POST /api/analytics/events`:**
   - Body: `{ "event_id"?: string, "query_id": string, "session_id"?: string, "event_type": "click"|"add_to_cart"|"purchase", "product_id": string, "position": int, "experiment_id"?: string, "revenue_micros"?: int64, "tenant_id"?: string }`.
   - Idempotent on `(tenant_id, event_id)` (`AlreadyExists` returns `200 OK` with `duplicate: true`).
   - Also forwards `click`/`purchase` to `SessionService` (`ADR-007`) when `session_id` is supplied so callers only need a single event beacon if desired.
2. **Online Product Signal Rollup (`POST /api/analytics/rollup`, guarded by `c.authorizeAdmin(ctx)`):**
   - Aggregates recent `search_impressions` (per-product impression exposures from `result_ids`) and `search_conversions` (`click`, `purchase`) over a configurable window (default `30d`), and updates `products.(clicks, orders, ctr, conversion_rate)` with Laplace-smoothed rates `(clicks + 1) / (impressions + 20)` so Stage-1 boosts (`ADR-004`) and Stage-2 `LTRModel` (`ADR-006`) immediately reflect real shopper behavior without cold-start division-by-zero spikes.
3. **Diagnostics & A/B Summary (`GET /api/analytics/summary?tenant_id=default&window_hours=168`):**
   - Returns:
     - `total_searches`, `zero_result_searches`, `zero_result_rate`, `avg_latency_ms`
     - `click_through_rate`, `conversion_rate`, `mrr_at_10` ($1/\text{rank}$ of first clicked organic item per impression), `attributed_revenue_micros`
     - `top_queries` (query, count, ctr) and `top_zero_result_queries` (query, count — actionable merchandising backlog)
     - `experiments` (`[]ExperimentSummary`: per `experiment_id` searches, CTR, MRR@10, conversion rate, avg latency).

### 4. Dual-Engine Parity, Storefront & `cmd/sqlcheck`
- In-memory `MemoryAnalyticsStore` mirrors impressions, idempotent conversions, KPI math (`CTR`, `MRR@10`, zero-result queries, A/B breakdown), and signal rollup for fast deterministic `go test -race ./...` coverage.
- Storefront (`src/application/ui/` + `defaultStorefrontHTML`) attaches `query_id` and `position` click beacons to organic cards, adds an `Experiment Variant` selector (`control` vs `ltr_boost`), and adds a **Search Analytics** tab/drawer displaying live CTR, MRR@10, Zero-Result Queries, and A/B comparison.
- `cmd/sqlcheck` validates `search_impressions` and `search_conversions` read queries.

## Consequences
- Closes the feedback loop between user behavior and `LTRModel` / merchandising signals while keeping `/api/search` p99 latency unaffected via async impression buffering.
- Gives merchandisers and search engineers built-in zero-result diagnostics and A/B experiment measurement in any industry.
