# ADR-003: Concurrent Server-Side Facets & Weighted Low-Relevance Synonyms

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #5** (`[Req 4.1.5]`), **Issue #7** (`[Req 4.1.7]`)
- **Migrations:** `src/iac/migrations/015_search_synonyms.sql`, `015b_seed_synonyms.dml.sql`
- **Implementation:** `src/psearch/serving/internal/services/facets.go`, `synonyms.go`, `spanner_service.go`

## Context
1. **Issue #5 (Req 4.1.5 — Server-Side Facets):** Facets were previously computed only in browser memory over returned items.
2. **Issue #7 (Req 4.1.7 — Low-Relevance Synonyms):** Text search had no tenant/locale synonym expansion or reduced synonym weighting.

## Decision & `psearch` Implementation
1. **Concurrent Server-Side Facets (`Facets` in `facets.go`):**
   - `Controller.Search` retrieves up to `facetCandidates = 100` candidate rows from `HybridSearchDetailed` and passes their `product_id` list (`UNNEST(@ids)`) to `SpannerService.Facets`.
   - Four `GROUP BY` queries (`categories`, `brands`, `price_ranges` in `0-100` / `100-500` / `500-1500` / `1500+`, and `availability` in `IN_STOCK` / `OUT_OF_STOCK`) run concurrently via `golang.org/x/sync/errgroup`.
   - Response includes `facets`, `facets_latency_ms`, and `candidates`.
2. **Low-Relevance Synonyms (`SynonymService` in `synonyms.go`):**
   - Table `search_synonyms (tenant_id STRING(64), locale STRING(16), term STRING(128), synonyms ARRAY<STRING(128)>, weight FLOAT64, updated_at TIMESTAMP) PRIMARY KEY (tenant_id, locale, term)` with TTL caching and `GET/POST /api/synonyms` (`POST` guarded by `X-Admin-Token`).
   - In `fts`, exact query matches always outrank synonym-only matches via `ORDER BY IF(SCORE(title_tokens, @query_text) + SCORE(description_tokens, @query_text) > 0, 0, 1)` followed by `(SCORE(exact) + @syn_weight * SCORE(syn)) DESC` (`default weight = 0.3`).
   - `SearchResponse` reports `expanded_terms` (rendered as an "Also matching" badge in the UI).
