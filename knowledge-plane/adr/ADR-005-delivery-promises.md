# ADR-005: Delivery Promises by CEP/ZIP via Spanner Interleaved Tables

## Context (GitHub Issue #8 — Req 4.1.8)
E-commerce search requires sub-10ms filtering and SLA calculation by buyer postal code (`cep` / `zip_code`) across distribution centers (CDs) and sellers.

## Decision
- Use Cloud Spanner physical co-location (`INTERLEAVE IN PARENT`):
  - `skus (product_id, sku_id, seller_id) PRIMARY KEY (product_id, sku_id), INTERLEAVE IN PARENT products ON DELETE CASCADE`
  - `regional_inventory (product_id, sku_id, region_code, zip_prefix_start, zip_prefix_end, cd_id, stock, sla_hours, free_shipping) PRIMARY KEY (product_id, sku_id, region_code), INTERLEAVE IN PARENT skus ON DELETE CASCADE`
- Support `/api/search?cep=01310-100&max_sla_hours=24` using a co-located `EXISTS` join and return `delivery_promise` on each `SearchResult`.
