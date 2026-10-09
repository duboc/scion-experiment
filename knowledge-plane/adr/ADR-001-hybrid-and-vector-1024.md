# ADR-001: Hybrid Search Dynamic Alpha & 1024-dim Blue/Green Vector Search with Cache

## Context (GitHub Issues #1 & #2)
1. **Issue #1 (Req 4.1.1 — Hybrid Search):** `HybridSearch` in `src/psearch/serving/internal/services/spanner_service.go` receives `alpha float64` (`0.0` to `1.0`), but the SQL query uses unweighted RRF `SUM(1 / (60 + rank))` without applying `@alpha`.
2. **Issue #2 (Req 4.1.2 — Vector Search):** Upstream `schema.sql` uses `embedding VECTOR(768)` without Blue/Green index switching or an embedding cache.

## Decision
1. **Dynamic RRF Weight `@alpha` (Issue #1):**
   - In `HybridSearch`, compute weighted RRF in SQL (and in the fallback/unit-test scorer):
     ```sql
     SELECT
       product_id,
       ANY_VALUE(title) AS title,
       ANY_VALUE(product_data) AS product_data,
       SUM(weight / (60.0 + rank)) AS rrf_score,
       MAX(IF(source = 'vector', 1.0 / (60.0 + rank), 0.0)) AS vector_score,
       MAX(IF(source = 'text', 1.0 / (60.0 + rank), 0.0)) AS text_score
     FROM (
       SELECT 'vector' AS source, @alpha AS weight, rank, product_id, title, product_data FROM ann
       UNION ALL
       SELECT 'text' AS source, (1.0 - @alpha) AS weight, rank, product_id, title, product_data FROM fts
     )
     GROUP BY product_id
     ORDER BY rrf_score DESC
     LIMIT @limit
     ```
   - Expose `alpha` via both `POST /api/search` (JSON field `alpha`) and `GET /api/search?q=...&alpha=0.7`, and return `effective_alpha` in `SearchResponse`.
2. **1024-dim Blue/Green & `embedding_cache` (Issue #2):**
   - Support `embedding_v1` and `embedding_v2` columns (`VECTOR_LENGTH=>1024`) with runtime config toggle `ACTIVE_EMBEDDING_VERSION` (`v1` or `v2`) and `GET/POST /api/config/embedding-version`.
   - Create `embedding_cache (content_sha256 STRING(64), model_id STRING(128), embedding ARRAY<FLOAT64>, updated_at TIMESTAMP) PRIMARY KEY (content_sha256, model_id)` to cache embeddings by SHA-256 of normalized query/product text.
