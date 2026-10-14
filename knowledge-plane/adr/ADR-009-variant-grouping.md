# ADR-009: Variant Grouping / Collapse (Products → SKUs)

- **Status:** Accepted (Issue #32)
- **Covered Issue:** GitHub **Issue #32** — "Variant Grouping / Collapse" (agrupamento de SKUs por pai com agregação de cores/tamanhos)
- **Migrations:** `src/iac/migrations/022_sku_variants.sql`, seed `022b_seed_sku_variants.dml.sql`

## Context
A product sold in many colors/sizes, if indexed as one item per variant, monopolizes the top results or forces rigid denormalization at ingestion. PSearch already models `products` (parent) → `skus` (child, `INTERLEAVE IN PARENT products`, migration 017), but SKUs carry no variant attributes and results expose no variant information.

## Decision
1. **Schema:** add variant columns to the interleaved `skus` table — `color STRING(64)`, `size STRING(32)`, `stock INT64`, `is_available BOOL` — and seed variants for the catalog (e.g. footwear in several colors × sizes).
2. **One card per parent:** retrieval stays at the `products` level (one row per `product_id`), so variants never crowd out other products.
3. **Child-level filtering via `EXISTS`:** new request filters `color` and `size` keep a product only if a child SKU matches and is in stock: `(@color = '' OR EXISTS (SELECT 1 FROM skus s WHERE s.product_id = products.product_id AND LOWER(s.color) = @color AND IFNULL(s.is_available, TRUE) AND IFNULL(s.stock, 0) > 0))`, applied inside both RRF legs (same for `size`).
4. **Aggregation in the same SQL query:** the hybrid statement returns, per parent, an array of its variants via a split-local correlated subquery (`ARRAY(SELECT AS STRUCT s.sku_id, s.color, s.size, s.stock, … FROM skus s WHERE s.product_id = a.product_id …)`), restricted to the requested color/size when filters are present.
5. **API:** each result carries `variants` (`sku_id`, `color`, `size`, `stock`, `available`), `variant_count`, `available_colors`, `available_sizes`; facets add `colors` and `sizes` over in-stock variants of the candidates.
6. **Parity:** the in-memory engine mirrors variants, filters and aggregation (`DefaultVariants`).

## Consequences
- Interleaving keeps variant lookups and the `EXISTS` filters on the same split as the parent row.
- Conversational color extraction (#19) stays soft (text) by default; the explicit `color` filter is hard.
