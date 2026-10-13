# ADR-008: Zero-Results Recovery (Spell Check & Relaxation) & B2B Contract Pricing

## Context (GitHub Issues #21 — Req 4.1.13, #22 — Req 4.1.14)
1. **#21:** Typos ("sansung", "geladera") or over-constrained multi-term queries give zero lexical hits; no "Did you mean" and no operator relaxation.
2. **#22:** A single list price per product; no contract price tables, exclusive assortments, or price-based sorting per buyer.

## Decision
1. **Zero-results recovery (#21):**
   - `SpellCheckService`: candidate generation from the `autocomplete_ngram_idx` (`SEARCH_NGRAMS`, migration 018; must be the only predicate in `WHERE`) plus the in-memory catalog vocabulary; ranked by Damerau-Levenshtein distance (≤ 2, ≤ 1 for short tokens) then popularity (`search_count`).
   - Recovery runs **only when the lexical leg has zero matches**: (1) high-confidence correction → re-run with the corrected query (`did_you_mean`, `auto_corrected=true`); (2) otherwise, for multi-term queries, relax the `SEARCH()` expression from implicit AND to `t1 OR t2 …` (`query_relaxed=true`) before falling back to vector-only.
2. **B2B contract pricing (#22):**
   - `product_prices (product_id, price_table_id, tenant_id, price, currency_code, min_quantity, is_exclusive) PRIMARY KEY (product_id, price_table_id), INTERLEAVE IN PARENT products ON DELETE CASCADE` (migration 019 + 019b seed).
   - `price_table_id` request param: `effective_price = COALESCE(pp.price, p.price)` via a co-located lookup; exclusive rows (`is_exclusive`) are visible only to buyers on that table; `min_price`/`max_price` filter and the price facet use the effective price; `sort_by = relevance | price_asc | price_desc` (availability-last still dominant).
   - The response `priceInfo.numericPrice` reflects the effective price; the base price is kept as `priceInfo.originalPrice`/`base_price` for display.

## Consequences
- Recovery adds at most one extra Spanner query per zero-result search.
- Pricing joins are split-local (interleaved), so no cross-split network cost.
