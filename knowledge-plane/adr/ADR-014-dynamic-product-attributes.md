# ADR-014: Schema-Free Dynamic Product Attributes & Vertical-Agnostic Facets

- **Status:** Proposed (Wave 6 — Production Search Depth)
- **Covered Issue:** GitHub **Issue #42** — "Dynamic Schema-Free Product Attributes (`product_attributes` Interleaved Table + `attributes JSON`) & Auto-Discovered Facets for Any Industry"
- **Migrations:** `src/iac/migrations/025_product_attributes.sql`, `src/iac/migrations/025b_seed_product_attributes.dml.sql`

## Context
Currently `psearch` only models fixed catalog columns (`brand`, `category`, `price`) plus apparel-oriented SKU columns (`color`, `size` from `ADR-009`). An industry-agnostic search engine must serve radically different catalogs on the same deployment without DDL changes per vertical:
- **Consumer Electronics:** `ram` (`16GB`, `32GB`), `storage` (`512GB`, `1TB`), `screen_size` (`14"`, `16"`)
- **B2B / Industrial Parts:** `voltage` (`110V`, `220V`, `Bivolt`), `material` (`Stainless Steel`, `Brass`), `ip_rating` (`IP65`, `IP68`)
- **Grocery / CPG:** `dietary` (`Vegan`, `Gluten-Free`, `Zero Lactose`), `packaging` (`Refill`, `Glass`)
- **Health & Pharmacy:** `form` (`Comprimido`, `Cápsula`, `Gel`), `sun_protection` (`FPS 30`, `FPS 50`, `FPS 70`)
- **Footwear / Apparel:** `drop` (`8mm`, `10mm`), `pronation` (`Neutral`, `Stability`)

## Decision

### 1. Hybrid JSON + Interleaved Attribute Index Schema (`025_product_attributes.sql`)
1. Add `attributes JSON` on `products` for zero-join document payload projection on returned search cards:
   ```sql
   ALTER TABLE products ADD COLUMN attributes JSON;
   ```
2. Create an interleaved key-value table `product_attributes` co-located in the parent `products` split for fast `EXISTS` filtering and `GROUP BY` facet aggregation:
   ```sql
   CREATE TABLE product_attributes (
     product_id STRING(1024) NOT NULL,
     attr_name STRING(64) NOT NULL,
     attr_value STRING(256) NOT NULL,
     attr_value_lower STRING(256) AS (LOWER(attr_value)) STORED,
     tenant_id STRING(64) NOT NULL DEFAULT ('default'),
   ) PRIMARY KEY (product_id, attr_name, attr_value),
     INTERLEAVE IN PARENT products ON DELETE CASCADE;

   CREATE INDEX product_attributes_by_tenant_name_val
   ON product_attributes(tenant_id, attr_name, attr_value_lower);
   ```
   - Co-location via `INTERLEAVE IN PARENT products` makes `EXISTS (SELECT 1 FROM product_attributes pa WHERE pa.product_id = products.product_id ...)` a local split read, while `product_attributes_by_tenant_name_val` supports selective attribute lookups and tenant attribute discovery.
3. Seed `025b_seed_product_attributes.dml.sql` (and `DefaultSeedProducts()` in `catalog_seed.go`) with realistic multi-industry attributes across seed computers (`ram`, `storage`, `processor`), footwear (`surface`, `cushioning`), appliances (`voltage`, `energy_rating`), and pharmacy/dermocosmetics (`fps`, `skin_type`, `form`).

### 2. Dynamic Attribute Filtering (`attrs`) in `/api/search`
1. **Request Contract:**
   - `POST /api/search` accepts `"attrs": { "ram": ["16GB", "32GB"], "voltage": ["220V"] }` (`map[string][]string`).
   - `GET /api/search` accepts query parameters with prefix `attr.` (e.g., `?q=notebook&attr.ram=16GB,32GB&attr.storage=1TB`).
   - Validate attribute keys (`^[a-zA-Z0-9_-]{1,64}$`, max 10 attribute keys per query, max 20 values per key, each value <= 256 chars); return `HTTP 400` on invalid input. Never interpolate `attr_name` into SQL—bind both `@attr_k_name` and `@attr_k_vals`!
2. **Disjunctive Semantics (`ADR-013` Consistent):**
   - Values within the same `attr_name` combine with `OR` (`pa.attr_name = @attr_0_name AND pa.attr_value_lower IN UNNEST(@attr_0_vals)`).
   - Different `attr_name` keys combine with `AND` (one correlated `EXISTS` per active attribute key).

### 3. Auto-Discovered Dynamic Facets (`facets.attributes`)
1. Extend `models.Facets` with `Attributes []models.DynamicFacet` (`{ Name string, Label string, Buckets []FacetBucket }`).
2. In `Facets()` (`facets.go`), run an additional concurrent `errgroup` branch querying `product_attributes`:
   ```sql
   SELECT attr_name, attr_value, COUNT(DISTINCT product_id) AS count
   FROM product_attributes
   WHERE product_id IN UNNEST(@ids)
   GROUP BY attr_name, attr_value
   ORDER BY attr_name, count DESC, attr_value
   ```
   - Rank discovered `attr_name` groups by coverage across the candidate set (so searching *"notebook"* automatically surfaces `ram`, `storage`, `processor` facets, whereas searching *"protetor solar"* automatically surfaces `fps` and `skin_type` facets!), returning the top `MAX_DYNAMIC_FACET_GROUPS` (default `6`, max `10` values per group).
   - Apply `ADR-013` exclude-own-dimension counting when `attrs[attr_name]` is actively selected.
3. Project `attributes` (`map[string]any`) on each `SearchResult` so product cards and detail drawers can render key specs.
4. Provide admin/ingestion upsert endpoint `PUT /api/products/:id/attributes` (guarded by `c.authorizeAdmin(ctx)`) that atomically writes `products.attributes` JSON and replaces child `product_attributes` rows in a single Spanner `ReadWriteTransaction` (and memory store).

### 4. Storefront UI & `cmd/sqlcheck`
- Storefront (`src/application/ui/` + `defaultStorefrontHTML`) dynamically renders collapsible facet sections for every `facets.attributes` group returned by `/api/search` (humanizing `snake_case` keys like `skin_type` → `Skin Type`) and shows top 2–3 attribute spec pills on product cards.
- `cmd/sqlcheck` verifies `product_attributes` existence, `attr.*` `EXISTS` predicates, and dynamic facet aggregation SQL.

## Consequences
- `psearch` becomes genuinely vertical-agnostic: onboarding an automotive, electronics, industrial, grocery, or healthcare catalog requires zero schema or UI code changes to get faceted spec navigation.
