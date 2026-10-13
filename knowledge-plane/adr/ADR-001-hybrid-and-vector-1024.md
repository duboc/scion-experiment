# ADR-001: Hybrid Search Dynamic Alpha & 1024-dim Blue/Green Vector Search with Cache

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #1** (`[Req 4.1.1]`), **Issue #2** (`[Req 4.1.2]`), **Issue #11** (Gen AI SDK upgrade), **Issue #23** (Latest Gemini models: `gemini-3.8-flash` & `gemini-4-argon`)
- **Migrations:** `src/iac/migrations/011_add_embedding_v2_gemini_1024.sql`, `src/iac/migrations/013_embedding_cache.sql`
- **Implementation:** `src/psearch/serving/internal/services/spanner_service.go`, `embedding_service.go`, `embedding_cache.go`, `internal/config/config.go`, `internal/api/handlers.go`, `src/psearch/gen_ai/`, `tooling/enrichment/`

## Context
1. **Issue #1 (Req 4.1.1 — Hybrid Search):** `HybridSearch` in `spanner_service.go` received `alpha float64` (`0.0` to `1.0`), but the initial SQL query used unweighted RRF `SUM(1 / (60 + rank))` without applying `@alpha`.
2. **Issue #2 (Req 4.1.2 — Vector Search):** Base `schema.sql` used `embedding VECTOR(768)` without Blue/Green index switching or an embedding cache.

## Decision & `psearch` Implementation
1. **Dynamic RRF Weight `@alpha` (`buildHybridSQLWith` in `spanner_service.go`):**
   - Both the Spanner SQL (`fts` + `ann` CTEs) and the in-memory engine (`memoryHybridSearchExpanded`) weight the vector leg by `@alpha` and the text leg by `(1.0 - @alpha)`, clamped to `[0.0, 1.0]`.
   - The `GROUP BY product_id` aggregation is wrapped in an outer subquery so `ORDER BY` references plain column aliases (Spanner rejects nested aggregates like `ORDER BY IF(IFNULL(ANY_VALUE(is_available), ...))`).
   - Deterministic tie-breaking: `ORDER BY unavailable_rank, pin_rank, buried, final_score DESC, vector_score DESC, text_score DESC, product_id`.
   - `/api/search` exposes `alpha` via query param (`?alpha=0.7`) and POST JSON body, returning `effective_alpha`, `retrieval_mode` (`hybrid_v2`, `hybrid_v1`, `lexical_only_degraded`, `memory_fallback`), and per-item `score.{hybrid, vector, text, alpha, boost, rule_multiplier, final_score}`.
2. **1024-dim Blue/Green Vector Column & Two-Tier `embedding_cache`:**
   - `products` retains `embedding VECTOR(768)` (`v1`, deterministic 768-d hash) and adds `embedding_v2 ARRAY<FLOAT32>(vector_length=>1024)` (`v2`, `gemini-embedding-2` via `google.golang.org/genai` on `GENAI_LOCATION=global`).
   - Runtime version is controlled via `ACTIVE_EMBEDDING_VERSION` (`v1`|`v2`) and per-instance `GET/POST /api/config/embedding-version` (POST guarded by `X-Admin-Token`).
   - If `v2` query embedding fails at runtime, `planVectorQuery` degrades gracefully to `lexical_only_degraded` (Spanner FTS only, never masking the error with in-memory 768-d vectors).
   - `embedding_cache (content_sha256 STRING(64), model_id STRING(128), dimensions INT64, embedding ARRAY<FLOAT32>, created_at TIMESTAMP) PRIMARY KEY (content_sha256, model_id)` backs an in-memory LRU + Spanner read-through/async-write cache (`EmbeddingCache`), exposed in `GET /api/info` (`embedding_cache`) and `SearchResponse.cache_hit`.
3. **Latest Model Baseline Across `psearch` (Issue #23):**
   - All text/flash model defaults across `src/psearch/serving` (`IntentLLMModel`), `src/psearch/gen_ai` (`enrichment_service.py`, `marketing_service.py`, `conversational_search_service.py`, `client_utils.py`), and `tooling/enrichment` (`gemini_client.py`) use **`gemini-3.8-flash`** on `GENAI_LOCATION=global`.
   - All Pro-tier model defaults (`ProLLMModel` in Go serving, `sql_fixer.py` / `sql_fix_service.py` in Python `gen_ai`) use **`gemini-4-argon`** (`GEMINI_PRO_MODEL`, released 2026-10-13 on Vertex AI `global`), with `NormalizeModelID` / `normalize_model_id` mapping `"gemini-4-pro"` and `"gemini-4.0-pro"` → `"gemini-4-argon"`.
   - `GET /api/info` exposes `embedding_model`, `text_model`, `pro_model`, and `genai_location` for live verification.
