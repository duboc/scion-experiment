# ADR-013: Disjunctive Multi-Select Faceting ("Exclude-Own-Dimension" Counts)

- **Status:** Proposed (Wave 6 — Production Search Depth)
- **Covered Issue:** GitHub **Issue #41** — "Disjunctive Multi-Select Faceting (`OR` Within Dimension, `AND` Across Dimensions & Exclude-Own-Dimension Counts)"
- **Migrations:** None required (operates over existing `products`, `product_prices`, and interleaved `skus` indexes)

## Context
In `ADR-003` (Issue #5) and Issue #34, `psearch` introduced server-side facet counts (`categories`, `brands`, `price_ranges`, `availability`, and `colors`/`sizes` from `ADR-009`) computed over the filtered result set, with single-value filter fields (`category`, `brand`, `color`, `size`).

In production search across every industry, conjunctive single-value facets break multi-brand and multi-option comparison:
1. Once a shopper selects `Brand = Samsung`, `catalogFilterSQL` filters candidates to `Samsung` only, causing `Facets()` in `internal/services/facets.go` to return `count = 0` (disappearing) for `LG`, `Sony`, and `Apple`.
2. Shoppers cannot select `Brand IN ('Samsung', 'LG') AND Category = 'TVs' AND Size IN ('55"', '65"')`.

Industry-standard search engines implement **disjunctive faceting**:
- **Within** the same facet dimension, multiple selected values combine with **`OR`** (`LOWER(brand) IN UNNEST(@brands)`).
- **Across** different facet dimensions, active filters combine with **`AND`**.
- **Facet counts for dimension $D$** are computed over candidates matching the query and **all active filters EXCEPT dimension $D$** ("exclude-own-dimension"), so sibling options in $D$ retain accurate non-zero counts while other dimensions narrow dynamically.

## Decision

### 1. Backward-Compatible Multi-Select Request Contract (`models.go` & `handlers.go`)
1. Extend `SearchRequest` and `SearchFilters` with slice fields while preserving 100% backward compatibility with existing singular fields:
   - `Categories []string` (`json:"categories,omitempty"`, `GET` supports repeated `?category=A&category=B`, comma-separated `?categories=A,B`, or singular `?category=A`).
   - `Brands []string` (`json:"brands,omitempty"`, `?brands=Nike,Asics` or repeated `?brand=...`).
   - `Colors []string` (`json:"colors,omitempty"`, `?colors=azul,preto` or repeated `?color=...`).
   - `Sizes []string` (`json:"sizes,omitempty"`, `?sizes=40,42` or repeated `?size=...`).
   - `PriceRanges []string` (`json:"price_ranges,omitempty"`, e.g. `["0-100", "100-500"]` mapped to bucket predicates alongside explicit `min_price`/`max_price`).
2. Normalize and deduplicate values (trimmed, case-folded for SQL `UNNEST(@brands_lower)`), capping each dimension slice at 20 values (`HTTP 400` if exceeded or any item > 128 chars). Singular `req.Brand` merges seamlessly into `filters.Brands`.

### 2. Disjunctive Filter Predicates in SQL & Memory (`spanner_service.go`)
1. Factor `catalogFilterSQL` so each dimension predicate is modular (`filterDimension` enum: `dimCategory`, `dimBrand`, `dimPrice`, `dimAvailability`, `dimColor`, `dimSize`):
   - Base non-facet filters (`is_visible`, `tenant_id`, `locale`, `channel`, `cep`/`max_sla_hours`, B2B exclusive assortment) always apply.
   - Dimension predicates use bound array parameters:
     - `LOWER(category) IN UNNEST(@categories)`
     - `LOWER(brand) IN UNNEST(@brands)`
     - `EXISTS (SELECT 1 FROM skus s WHERE s.product_id = products.product_id AND IFNULL(s.stock, 0) > 0 AND LOWER(s.color) IN UNNEST(@colors))`
     - `EXISTS (SELECT 1 FROM skus s WHERE s.product_id = products.product_id AND IFNULL(s.stock, 0) > 0 AND LOWER(s.size) IN UNNEST(@sizes))`
     - Disjunctive `price_ranges` buckets (`0-100`, `100-500`, `500-1500`, `1500+`) evaluated over buyer `effectivePriceSQL("products")`.

### 3. Exclude-Own-Dimension Facet Aggregation (`facets.go`)
1. **Zero Active Facet Dimensions (Fast Path):** When no facet dimension filter is active, keep the existing fast path—compute all 6 facet groups concurrently (`errgroup`) over the Stage-1 base candidate IDs (zero overhead vs. today).
2. **One or More Active Facet Dimensions:**
   - Retrieve base candidate rows (query + base non-facet filters: tenant, locale, channel, CEP, explicit min/max price, up to top `FACET_CANDIDATE_LIMIT = 500`) carrying `(product_id, category, brand, effective_price, in_stock, variant_colors, variant_sizes)`—or evaluate per-dimension `GROUP BY` excluding dimension $D$'s predicate in `errgroup`.
   - For each dimension $D \in \{\text{category}, \text{brand}, \text{price\_range}, \text{availability}, \text{color}, \text{size}\}$, count products that satisfy all active filters **where $F \neq D$**.
   - Ensure any currently selected facet value in $D$ with `0` matches is still returned in `facets.<D>` with `selected: true, count: 0` (optional `selected` boolean on `FacetBucket`) so the UI never traps a user with an un-deselectable active checkbox.
3. **Dual-Engine Parity:** Mirror identical disjunctive filtering and exclude-own-dimension counting in `memoryHybridSearchExpanded` and `memoryFacets` so offline `go test -race ./...` exercises multi-brand + category + color combinations deterministically.

### 4. Storefront UI (`src/application/ui/` & `defaultStorefrontHTML`)
- Upgrade sidebar facets from single-select toggles to multi-select checkboxes with active filter chips (`Brand: Nike ×`, `Brand: Asics ×`, `Clear all`) and live sibling counts that stay visible when comparing brands, categories, colors, sizes, or price buckets.

## Consequences
- Shoppers in any retail or B2B vertical can compare multiple brands, categories, sizes, or price tiers simultaneously without losing sibling facet counts.
- Zero breaking changes for existing single-parameter callers (`?brand=Nike` continues to work unchanged).
