# ADR-002: Availability Filtering/Sorting & Multilanguage Multi-Tenant Catalog

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #3** (`[Req 4.1.3]`), **Issue #4** (`[Req 4.1.4]`)
- **Migrations:** `src/iac/migrations/012_availability_visibility_channels.sql`, `012b_backfill_visibility_channels.dml.sql`, `014_tenant_locale.sql`, `014b_accent_folded_tokens.sql`, `014c_backfill_tenant_locale.dml.sql`
- **Implementation:** `src/psearch/serving/internal/services/spanner_service.go`, `textfold.go`, `internal/models/models.go`

## Context
1. **Issue #3 (Req 4.1.3 — Availability):** Original `spanner_service.go` hardcoded `Availability: "IN_STOCK"` and relied on client-side filtering.
2. **Issue #4 (Req 4.1.4 — Multilanguage Catalog):** `products` had no `tenant_id`, `locale`, or accent-insensitive full-text search tokens (so `q=tenis` missed `"Tênis de Corrida Nimbus Cloud"` at `alpha=0`).

## Decision & `psearch` Implementation
1. **Availability, Visibility & Channels (`catalogFilterSQL` in `spanner_service.go`):**
   - Columns on `products`: `is_available BOOL`, `stock INT64`, `is_visible BOOL`, `channels ARRAY<STRING(64)>`.
   - `catalogFilterSQL` is applied inside BOTH `ann` and `fts` CTE legs:
     - `COALESCE(is_visible, TRUE)` (hidden products are never returned)
     - `(@channel = '' OR @channel IN UNNEST(IFNULL(channels, ['web', 'mobile', 'store'])))`
     - `(NOT @only_available OR (IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0))`
   - Unavailable products (`is_available = FALSE` or `stock <= 0`, such as `prod-004` and `prod-012`) always sort after available products via `ORDER BY IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 0, 1)`.
2. **Multi-Tenant, Locale & Accent-Folded Full-Text Search:**
   - Columns on `products`: `tenant_id STRING(64)` (defaults to `"default"`), `locale STRING(16)` (`"pt-BR"`, `"en-US"`, `"es-MX"`), plus `category STRING(256)` and `brand STRING(256)`.
   - Generated columns `title_tokens_folded` and `description_tokens_folded` use `TOKENIZE_FULLTEXT(REGEXP_REPLACE(NORMALIZE(col, NFD), r'\p{M}', '')) HIDDEN` and are indexed in `products_text_idx`. When `TEXT_FOLDING=true`, `SpannerService` folds query accents (`FoldAccents` in `textfold.go`) and searches the `*_tokens_folded` columns.
   - **Authoritative Empty Results:** When a Spanner query succeeds with 0 matching rows (e.g., `tenant_id=acme`), `HybridSearchDetailed` returns `[]` with `retrieval_mode=hybrid_v2` rather than falling back to the in-memory seed catalog.
