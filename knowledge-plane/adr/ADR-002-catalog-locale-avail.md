# ADR-002: Availability Filtering/Sorting & Multilanguage Multi-Tenant Catalog

## Context (GitHub Issues #3 & #4)
1. **Issue #3 (Req 4.1.3 — Availability):** Upstream `spanner_service.go` hardcodes `Availability: "IN_STOCK"` and relies on React client-side filtering.
2. **Issue #4 (Req 4.1.4 — Multilanguage Catalog):** Upstream `products` table has no `tenant_id` or `locale` columns.

## Decision
1. **Availability Columns & Ordering (Issue #3):**
   - Use columns `is_available BOOL`, `stock INT64`, `is_visible BOOL`, `channels ARRAY<STRING(64)>` on `products`.
   - Always filter `COALESCE(is_visible, TRUE) = TRUE`, optionally filter `@channel IN UNNEST(channels)` and `@only_available`, and sort final results by `ORDER BY COALESCE(is_available, FALSE) DESC, rrf_score DESC` so out-of-stock products appear at the bottom.
2. **Multi-Tenant & Multilanguage (Issue #4):**
   - Add `tenant_id STRING(64)` (default `"default"`) and `locale STRING(16)` (`"pt-BR"`, `"en-US"`, `"es-MX"`) to `products` and `/api/search` (`?tenant_id=default&locale=pt-BR`).
