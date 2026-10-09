# ADR-004: Relevance & Merchandising Rules Engine + 8 Boost Signals

## Context (GitHub Issue #6 — Req 4.1.6)
Upstream `ruleService.js` stores merchandising rules in browser `localStorage` with zero backend integration.

## Decision
1. **8 Boost Signals on `products`:**
   - Columns: `clicks INT64`, `orders INT64`, `discount_pct FLOAT64`, `popularity_score FLOAT64`, `margin_pct FLOAT64`, `ctr FLOAT64`, `conversion_rate FLOAT64`, `recency_score FLOAT64`.
2. **`merchandising_rules` Table & `/api/rules` CRUD:**
   - Persist rules in Spanner (`rule_id`, `name`, `query_pattern`, `target_brand`, `target_category`, `boost_multiplier`, `pin_product_ids`, `bury_product_ids`, `active`).
   - Combine `rrf_score + boost_signal_score + merchandising_rule_delta` in the SQL ranking expression and connect `src/application/ui/src/services/ruleService.js` to `/api/rules`.
