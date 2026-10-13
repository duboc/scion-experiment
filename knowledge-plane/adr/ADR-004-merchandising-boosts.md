# ADR-004: Relevance & Merchandising Rules Engine + 8 Boost Signals

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #6** (`[Req 4.1.6]`)
- **Migrations:** `src/iac/migrations/016_merchandising.sql`, `016b_backfill_boost_signals.dml.sql`
- **Implementation:** `src/psearch/serving/internal/services/merchandising.go`, `spanner_service.go`, `src/application/ui/src/services/ruleService.js`

## Context
Original `ruleService.js` stored merchandising rules only in browser `localStorage` with no backend persistence or SQL ranking integration.

## Decision & `psearch` Implementation
1. **8 Business Boost Signals on `products` (`boostSignalSQL` in `merchandising.go`):**
   - Columns on `products`: `clicks INT64`, `orders INT64`, `discount_pct FLOAT64`, `popularity_score FLOAT64`, `margin_pct FLOAT64`, `ctr FLOAT64`, `conversion_rate FLOAT64`, `recency_score FLOAT64`.
   - Normalized in `[0, 1]` and scaled by `@boost_weight` (default `0.002`, env `BOOST_WEIGHT`) so business signals break near-ties without overwhelming relevance.
2. **`merchandising_rules` Table & `/api/rules` CRUD:**
   - Table `merchandising_rules (tenant_id STRING(64), rule_id STRING(64), name STRING(256), query_pattern STRING(256), target_brand STRING(128), target_category STRING(128), boost_multiplier FLOAT64, pin_product_ids ARRAY<STRING(1024)>, bury_product_ids ARRAY<STRING(1024)>, active BOOL, updated_at TIMESTAMP) PRIMARY KEY (tenant_id, rule_id)`.
   - Endpoints: `GET /api/rules` (public) and `POST /api/rules`, `PUT /api/rules/:id`, `DELETE /api/rules/:id` (guarded by `X-Admin-Token`, multipliers clamped to `[0.1, 10.0]`).
   - Ranking formula in `buildHybridSQLWith`: `final_score = rrf_score * rule_mult + boost_score`, ordered by `unavailable_rank, pin_rank, buried, final_score DESC`, with `applied_rules` returned on `SearchResponse`.
