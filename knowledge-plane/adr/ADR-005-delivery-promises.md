# ADR-005: Delivery Promises by CEP/ZIP via Spanner Interleaved Tables

- **Status:** Delivered & Live (`psearch-serving-00019-dsr`)
- **Covered Issues:** GitHub **Issue #8** (`[Req 4.1.8]`)
- **Migrations:** `src/iac/migrations/017_delivery_interleaved.sql`, `017b_seed_skus.dml.sql`, `017c_seed_regional_inventory.dml.sql`
- **Implementation:** `src/psearch/serving/internal/services/delivery.go`, `spanner_service.go`, `internal/api/handlers.go`

## Context
E-commerce search requires sub-10ms filtering and SLA calculation by buyer postal code (`cep` / `zip_code`) across distribution centers (CDs) and marketplace sellers.

## Decision & `psearch` Implementation
1. **Spanner Physical Co-Location (`INTERLEAVE IN PARENT`):**
   - `skus (product_id STRING(1024), sku_id STRING(128), seller_id STRING(128), attributes JSON) PRIMARY KEY (product_id, sku_id), INTERLEAVE IN PARENT products ON DELETE CASCADE`
   - `regional_inventory (product_id STRING(1024), sku_id STRING(128), region_code STRING(16), zip_prefix_start STRING(8), zip_prefix_end STRING(8), cd_id STRING(64), stock INT64, sla_hours INT64, shipping_cost FLOAT64, free_shipping BOOL) PRIMARY KEY (product_id, sku_id, region_code), INTERLEAVE IN PARENT skus ON DELETE CASCADE`
2. **Co-Located `EXISTS` Filter & Fastest Promise Resolution:**
   - `NormalizeCEP` validates 8-digit Brazilian CEPs (`01310-100` → 5-digit prefix `01310`, returning HTTP 400 on malformed input).
   - `deliveryFilterSQL` restricts both RRF legs via a co-located `EXISTS (SELECT 1 FROM regional_inventory AS ri WHERE ri.product_id = products.product_id AND @zip5 BETWEEN ri.zip_prefix_start AND ri.zip_prefix_end AND ri.stock > 0 AND (@max_sla = 0 OR ri.sla_hours <= @max_sla))`.
   - `DeliveryPromises` joins `regional_inventory` with `skus` ordered by `sla_hours ASC, shipping_cost ASC` to attach `deliveryPromise` (`cep`, `region_code`, `cd_id`, `seller_id`, `sla_hours`, `free_shipping`, `shipping_cost`, `label` e.g. `"Chega amanhã - CD São Paulo"`), setting `shipping_cost = 0` whenever `free_shipping = true`.
