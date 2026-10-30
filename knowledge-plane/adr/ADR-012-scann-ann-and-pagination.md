# ADR-012: ScaNN ANN Vector Index, Concurrent Embedding Backfill & True Pagination

- **Status:** Proposed (Wave 6 — Production Search Depth)
- **Covered Issues:**
  - GitHub **Issue #39** — "ScaNN ANN Vector Index (`APPROX_COSINE_DISTANCE`) & Concurrent Chunked Embedding Backfill"
  - GitHub **Issue #40** — "True Search Pagination (`page`/`offset`), Accurate `total_found`, and Extended Sort Modes"
- **Migrations:** `src/iac/migrations/024_products_vector_index.sql`

## Context
With the `psearch` catalog scaled from 15 seed items to **20,535+ products** on Cloud Spanner (`psearch-instance`, Enterprise Edition), three production bottlenecks exist in the current retrieval and serving pipeline:

1. **Brute-Force Vector Scan:** `buildHybridSQLMode` in `src/psearch/serving/internal/services/spanner_service.go` computes exact `COSINE_DISTANCE(embedding_v2, @query_embedding)` across every candidate row. `011_add_embedding_v2_gemini_1024.sql` already defines `embedding_v2 ARRAY<FLOAT32>(vector_length=>1024)`, which qualifies for Cloud Spanner's native ScaNN `VECTOR INDEX`.
2. **Sequential & Single-Transaction Backfill Cliff:** `cmd/backfill-embeddings/main.go` embeds products one-by-one and flushes all mutations in a single `client.Apply(ctx, muts)` call at exit. On 20k+ SKUs (especially with secondary `SEARCH INDEX` and `VECTOR INDEX` maintenance mutations), a single `Apply` exceeds Spanner's **80,000 mutations per transaction** limit and takes hours without batch `EmbedContents` calls and bounded worker concurrency.
3. **No-Op Pagination & Truncated `total_found`:** `Controller.Search` (`internal/api/handlers.go`) parses `page` and `offset` only to gate page-1 sponsored ads (`isFirstPage`), then always slices `results[:limit]` from index `0` and sets `total_found = len(results)`. Shoppers cannot navigate to page 2+, and API callers never know how many total products matched.

## Decision

### 1. ScaNN ANN Vector Index (`024_products_vector_index.sql`, Issue #39)
1. **Online DDL:**
   ```sql
   CREATE VECTOR INDEX products_embedding_v2_idx
   ON products(embedding_v2)
   STORING (is_visible, is_available, stock, tenant_id, locale, category, brand, price)
   WHERE embedding_v2 IS NOT NULL
   OPTIONS (distance_type = 'COSINE', tree_depth = 2, num_leaves = 100);
   ```
   - `WHERE embedding_v2 IS NOT NULL` is mandatory on nullable vector columns in Cloud Spanner.
   - `tree_depth = 2, num_leaves = 100` fits catalogs from ~1k to 250k rows cleanly on a 100-PU Enterprise instance.
2. **ANN Query Path with Automatic Exact Fallback (`spanner_service.go`):**
   - Add config flag `USE_VECTOR_INDEX` (default `true` when `ACTIVE_EMBEDDING_VERSION=v2`, configurable via env `USE_VECTOR_INDEX` and configurable `ANN_NUM_LEAVES_TO_SEARCH`, default `20`).
   - When enabled, the vector leg CTE queries:
     ```sql
     SELECT product_id,
            APPROX_COSINE_DISTANCE(embedding_v2, @query_embedding,
              options => JSON '{"num_leaves_to_search": 20}') AS distance
     FROM products@{FORCE_INDEX=products_embedding_v2_idx}
     WHERE embedding_v2 IS NOT NULL
       AND ... -- catalogFilterSQL predicates supported alongside ANN
     ORDER BY distance ASC
     LIMIT @vec_k
     ```
   - **Resilience / Emulator Guard:** If Spanner returns an error indicating `products_embedding_v2_idx` is still backfilling, not found, or `APPROX_COSINE_DISTANCE` is unsupported (e.g., local Spanner Emulator), `SpannerService` logs a warning, marks ANN unavailable for a cool-down window (e.g., 60s), and seamlessly executes the exact `COSINE_DISTANCE` SQL so search availability is never interrupted.
   - Expose `vector_index_used: bool` on `SearchResponse` (and `/api/info`).

### 2. Concurrent, Batched & Chunk-Checkpointed `cmd/backfill-embeddings` (Issue #39)
1. Extend `EmbeddingService` (`embedding_service.go`) with `EmbedDocumentsBatch(ctx, texts []string) ([][]float32, error)` using the Gen AI SDK batch `EmbedContent` API (up to e.g. 32–64 documents per RPC) with exponential-backoff retry on `429` / `503`.
2. Upgrade `cmd/backfill-embeddings/main.go` with CLI flags:
   - `-workers` (default `8`), `-embed-batch` (default `32`), `-write-chunk` (default `200` rows per `client.Apply`, well below Spanner's 80k mutation ceiling even with secondary indexes), `-limit` (default `0` = all matching rows), `-force`, and `-dry-run`.
   - Stream candidate `product_id` rows (`WHERE embedding_v2 IS NULL` unless `-force`), commit each chunk of `-write-chunk` rows immediately as workers finish (so interrupted runs resume with zero wasted work), and log progress (`embedded X/Y rows, rate=Z docs/s`).
   - Unit-test chunking, batching, and retry behavior with a fake batch embedder + fake writer.

### 3. True Pagination, Pre-Slice `total_found` & Extended `sort_by` (Issue #40)
1. **Candidate Window & True Total Count:**
   - Stage-1 hybrid retrieval returns up to `candidate_window` organic items (default `max(100, offset + limit)` capped at `MAX_SEARCH_WINDOW = 500`, plus a lightweight count or exact count of matched candidates). Specifically:
     - `total_found` MUST report the **total matching organic count** (via a fast count over catalog-filtered lexical/hybrid matches or full candidate count, never `len(page_results)`).
     - Compute `effective_offset`: if `req.Offset > 0`, use `req.Offset`; else if `req.Page > 1`, use `(req.Page - 1) * limit`; else `0`. Reject negative `offset` or `offset > 1000` with `HTTP 400`.
     - Run Stage-2 LTR (`ADR-006`) and Session Personalization (`ADR-007`) over the candidate window **before** slicing `[effective_offset : min(len(results), effective_offset+limit)]` so cross-page ordering is deterministic and consistent.
     - Keep `ADR-011` invariant: sponsored products are injected **only** when `effective_offset == 0` (`page == 1`), and `total_found` excludes sponsored items.
2. **Pagination Metadata on `SearchResponse`:**
   - Add `page` (`int`, 1-based), `page_size` (`int`, effective `limit`), `offset` (`int`), `total_pages` (`int`), and `has_next` (`bool`) alongside `total_found`.
3. **Industry-Standard `sort_by` Modes:**
   - Extend `ValidatePricing` / sort validation (`pricing.go` + `spanner_service.go` + memory mode) to accept:
     - `relevance` (default RRF + boosts + optional LTR/session)
     - `price_asc` (effective buyer price ascending, OOS still last per `ADR-002`)
     - `price_desc` (effective buyer price descending, OOS still last)
     - `popularity` (`orders` / `sales_volume` / `clicks` descending, ties by relevance)
     - `rating_desc` (`avg_rating` descending with review count tie-break, OOS still last)
4. **Storefront Parity (`src/application/ui/` & `defaultStorefrontHTML`):**
   - Display `"Showing X–Y of Z results"`, previous/next + numbered page controls, page-size selector (`12` / `24` / `48`), and expanded Sort dropdown (`Relevance`, `Price: Low to High`, `Price: High to Low`, `Best Selling`, `Top Rated`). Changing filters or query resets `page` to `1`.

### 4. Verification (`cmd/sqlcheck` & Tests)
- `cmd/sqlcheck` verifies both the ANN query path (`USE_VECTOR_INDEX=true`) and exact fallback (`USE_VECTOR_INDEX=false`), plus all 5 `sort_by` modes and non-zero pagination offsets.

## Consequences
- Vector latency remains sub-20ms as the catalog scales from 20k to 100k+ SKUs while maintaining 100% uptime if an index is rebuilding or absent.
- `cmd/backfill-embeddings` becomes safe for production catalogs of any size without hitting Spanner's transaction mutation limit.
- Clients and storefronts get honest match counts and working multi-page navigation.
