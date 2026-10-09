# ADR-006: Analytics N-Gram Autocomplete & Two-Stage Learning to Rank (LTR)

## Context (GitHub Issues #9 & #10)
1. **Issue #9 (Req 4.1.9 — Autocomplete):** Needs prefix + substring suggestions backed by BigQuery search analytics and Spanner `TOKENIZE_NGRAMS`.
2. **Issue #10 (Req 4.1.10 — Learning to Rank):** Needs 2-stage retrieval (Spanner Top-100) + fast in-memory ML re-ranking (~2ms in Go).

## Decision
1. **Autocomplete (`GET /api/autocomplete?q=...`):**
   - Table `autocomplete_suggestions` with `suggestion_ngrams TOKENLIST AS (TOKENIZE_NGRAMS(suggestion_text, ngram_size_min=>2, ngram_size_max=>4)) HIDDEN` and `SEARCH INDEX autocomplete_ngram_idx`.
2. **Two-Stage LTR (`?ltr=true`):**
   - Stage 1 fetches Top-100 candidates from Spanner Hybrid Search; Stage 2 runs an in-memory Go GBDT/linear feature scorer over 12 normalized features (`rrf_score`, `vector_score`, `text_score`, 8 boost signals, and `sla_hours`) in `<2ms`.
