# ADR-003: Concurrent Server-Side Facets & Weighted Low-Relevance Synonyms

## Context (GitHub Issues #5 & #7)
1. **Issue #5 (Req 4.1.5 — Server-Side Facets):** Upstream UI computes facets in browser memory over top-300 results.
2. **Issue #7 (Req 4.1.7 — Low-Relevance Synonyms):** Upstream text search has no synonym expansion or reduced synonym weighting.

## Decision
1. **Concurrent Server-Side Facets (Issue #5):**
   - Run concurrent Go goroutines (`golang.org/x/sync/errgroup`) querying Spanner `GROUP BY` aggregations for `categories`, `brands`, `price_ranges` (`0-100`, `100-500`, `500-1500`, `1500+`), and `availability`, returning `facets` in `SearchResponse`.
2. **Low-Relevance Synonyms (Issue #7):**
   - Create table `search_synonyms (tenant_id STRING(64), locale STRING(16), term STRING(128), synonyms ARRAY<STRING(128)>, weight FLOAT64) PRIMARY KEY (tenant_id, locale, term)`.
   - Expand query tokens in Go and multiply synonym text scores by `weight` (default `0.3`), ensuring exact matches (`1.0x`) always rank above synonym matches (`0.3x`).
