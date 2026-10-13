# ADR-008: Zero-Results Recovery (Spell Check & Relaxation) & B2B Contract Pricing

- **Status:** Planned (Wave 5)
- **Covered Issues:** GitHub **Issue #21** (`[Req 4.1.13]`), **Issue #22** (`[Req 4.1.14]`)
- **Planned Migrations:** `src/iac/migrations/019_b2b_product_prices.sql` (or next numbered DDL + seed `.dml.sql`)

## Context (GitHub Issues #21 — Req 4.1.13, #22 — Req 4.1.14)
1. **#21 (Zero-Results Recovery & Spell Check):** Typos (`"sansung"`, `"geladera"`) or over-constrained multi-term queries give zero lexical hits in `SEARCH()`; there is no `"Did you mean"` suggestion and no boolean operator relaxation (`AND` → `OR`).
2. **#22 (B2B / Contract Pricing & Assortment):** `products` has a single list price (`price FLOAT64`); there is no support for customer contract price tables (`price_table_id`), exclusive assortments, or price-based sorting per buyer.

## Decision & `psearch` Implementation
1. **Zero-Results Recovery (#21 — `internal/services/spellcheck.go`):**
   - `SpellCheckService`: candidate generation from `autocomplete_ngram_idx` (`SEARCH_NGRAMS(suggestion_ngrams, @token)` on `autocomplete_suggestions` from migration `018_autocomplete.sql`; per ADR-006, `SEARCH_NGRAMS` must not be `OR`-ed with non-indexed predicates in `WHERE`) plus the in-memory catalog vocabulary (`DefaultSuggestions`); ranked by Damerau-Levenshtein distance ($\le 2$, $\le 1$ for short tokens) then popularity (`search_count`).
   - Recovery runs **only when the lexical leg has zero matches**:
     1. High-confidence typo correction → re-run with the corrected query (`did_you_mean`, `auto_corrected=true`).
     2. Otherwise, for multi-term queries, relax the `SEARCH()` expression from implicit `AND` to `t1 OR t2 …` (`query_relaxed=true`) before falling back to vector-only.
2. **B2B Contract Pricing (#22 — `internal/services/pricing.go`):**
   - Interleaved Spanner table (co-located inside `products` splits; `product_id` must be the leading primary key column):
     ```sql
     CREATE TABLE product_prices (
       product_id STRING(1024) NOT NULL,
       price_table_id STRING(64) NOT NULL,
       tenant_id STRING(64) NOT NULL,
       price FLOAT64 NOT NULL,
       currency_code STRING(8),
       min_quantity INT64,
       is_exclusive BOOL
     ) PRIMARY KEY (product_id, price_table_id),
       INTERLEAVE IN PARENT products ON DELETE CASCADE;
     ```
   - `price_table_id` request param: `effective_price = COALESCE(pp.price, p.price)` via a co-located lookup; exclusive rows (`is_exclusive`) are visible only to buyers on that table; `min_price`/`max_price` filter and the `facets.price_ranges` aggregation use the effective price; `sort_by = relevance | price_asc | price_desc` (unavailable-last still dominant).
   - The response `priceInfo.numericPrice` reflects the effective price; the base price is kept in `priceInfo.originalPrice`/`base_price` for display.

## Consequences
- Recovery adds at most one extra Spanner query per zero-result search.
- Pricing joins are split-local (`INTERLEAVE IN PARENT products`), incurring zero cross-split network cost.
