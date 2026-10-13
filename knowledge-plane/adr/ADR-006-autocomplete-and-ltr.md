# ADR-006: Analytics N-Gram Autocomplete & Two-Stage Learning to Rank (LTR)

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #9** (`[Req 4.1.9]`), **Issue #10** (`[Req 4.1.10]`)
- **Migrations:** `src/iac/migrations/018_autocomplete.sql`, `018b_seed_autocomplete.dml.sql`
- **Implementation:** `src/psearch/serving/internal/services/autocomplete.go`, `ltr.go`, `src/iac/bigquery_scripts/autocomplete_export_to_spanner.sql`, `ltr_train.sql`

## Context
1. **Issue #9 (Req 4.1.9 — Autocomplete):** Needs prefix + substring suggestions backed by search analytics and Spanner `TOKENIZE_NGRAMS`.
2. **Issue #10 (Req 4.1.10 — Learning to Rank):** Needs 2-stage retrieval (Spanner Top-100) + fast in-memory ML re-ranking (`<2ms` in Go).

## Decision & `psearch` Implementation
1. **Autocomplete (`GET /api/autocomplete?q=...`, `autocomplete.go`):**
   - **Spanner DDL Gotcha:** Spanner rejects `REGEXP_REPLACE` inside `TOKENIZE_NGRAMS(...)` and rejects `SEARCH_NGRAMS(...) OR STARTS_WITH(...)` in the same `WHERE` clause.
   - Schema (`018_autocomplete.sql`):
     ```sql
     CREATE TABLE autocomplete_suggestions (
       tenant_id STRING(64) NOT NULL,
       locale STRING(16) NOT NULL,
       suggestion_text STRING(256) NOT NULL,
       suggestion_folded STRING(256),
       search_count INT64,
       ctr_score FLOAT64,
       updated_at TIMESTAMP OPTIONS (allow_commit_timestamp = true),
       suggestion_ngrams TOKENLIST AS (TOKENIZE_NGRAMS(suggestion_folded, ngram_size_min=>2, ngram_size_max=>4)) HIDDEN
     ) PRIMARY KEY (tenant_id, locale, suggestion_text);
     CREATE SEARCH INDEX autocomplete_ngram_idx ON autocomplete_suggestions(suggestion_ngrams);
     ```
   - For prefixes of $\ge 2$ runes, `WHERE` uses `SEARCH_NGRAMS(suggestion_ngrams, @q)` alone and `ORDER BY IF(STARTS_WITH(suggestion_folded, @q), 0, 1), ctr_score DESC, search_count DESC, suggestion_text`. For 1-rune prefixes (below `ngram_size_min=2`), `WHERE` uses `STARTS_WITH(suggestion_folded, @q)`.
2. **Two-Stage LTR (`?ltr=true`, `ltr.go`):**
   - Stage 1 fetches up to Top-100 candidates (`facetCandidates = 100`) from Spanner Hybrid Search.
   - Stage 2 (`services.Rerank`) evaluates an in-memory linear + boosted regression-stump model (`LTRModel`, configurable via `LTR_MODEL_JSON` or `DefaultLTRModel()`) over 12 normalized `[0, 1]` features (`rrf_score`, `vector_score`, `text_score`, 8 boost signals from `CandidateSignals`, and `sla_hours` from `DeliveryPromises`) in `<100µs`, preserving availability, pin, and bury precedence.
   - Exposes `ltr`, `ltr_model`, `ltr_latency_us` on `SearchResponse` and `score.{ltr_score, stage1_rank, stage2_rank}` on each `SearchResult`.
