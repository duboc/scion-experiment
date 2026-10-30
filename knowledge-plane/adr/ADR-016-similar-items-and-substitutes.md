# ADR-016: Item-to-Item Similarity, In-Stock Substitutes & Co-Visitation (`GET /api/products/:id/similar`)

- **Status:** Proposed (Wave 6 — Production Search Depth)
- **Covered Issue:** GitHub **Issue #44** — "Item-to-Item Vector Similarity, In-Stock Substitutes & Session Co-Visitation Recommendations (`GET /api/products/:id/similar`)"
- **Migrations:** None required (reuses `products.embedding_v2` + `024_products_vector_index.sql` from `ADR-012`, `user_session_events` from `ADR-007`, and `product_prices` from `ADR-008`)

## Context
Search rarely ends on the initial SERP:
1. When a shopper lands on an **out-of-stock** item (sorted last per `ADR-002` or reached via direct link/SKU search), showing dead-end availability loses the sale unless the engine immediately recommends **in-stock substitutes** in the same category and price band.
2. On any product detail view across electronics, apparel, industrial parts, grocery, or pharmacy, shoppers expect **"Similar Products"** (semantic/multimodal embedding neighbors) and **"Frequently Viewed / Bought Together"** (behavioral co-visitation from real sessions).

Because `psearch` already stores 1024-d `gemini-embedding-2` vectors (`embedding_v2`) on `products` and real-time interaction trails in `user_session_events` (`021_user_session_events.sql`), Spanner can serve hybrid semantic + behavioral item-to-item recommendations in a single low-latency service without an external recommendation cluster.

## Decision

### 1. Endpoint Contract (`GET /api/products/:id/similar`)
- **Path param:** `:id` (`product_id` of the seed item; returns `HTTP 404` if seed product does not exist or is invisible in `tenant_id`).
- **Query params:**
  - `mode`: `similar` (default: blended semantic + co-visitation), `substitutes` (same `category` as seed, strictly `in_stock`, scored by semantic similarity + price proximity), or `co_viewed` (ranked primarily by session co-occurrence with vector backfill when sparse).
  - `limit`: default `6`, max `24`.
  - Standard catalog context params: `tenant_id` (default `default`), `locale`, `channel`, `price_table_id` (B2B effective pricing & exclusive assortment from `ADR-008`), `cep` (attaches fastest `DeliveryPromise` from `ADR-005`), `only_in_stock` (default `true` when `mode=substitutes`, `false` otherwise).
- **Response (`SimilarProductsResponse`):**
  ```json
  {
    "seed_product_id": "prod-001",
    "mode": "substitutes",
    "results": [ /* []SearchResult with similarity_score, co_view_count, substitute_reason */ ],
    "total_found": 6,
    "latency_ms": 8.4,
    "backend_mode": "spanner",
    "vector_index_used": true
  }
  ```

### 2. Hybrid Semantic + Co-Visitation Scoring (`internal/services/recommendations.go`)
1. **Seed Lookup:** Fetch the seed product's `(category, brand, effective_price, embedding_v2, title)` for `tenant_id`.
   - If `embedding_v2` is `NULL` on the seed row (e.g., recently ingested SKU before backfill), fall back cleanly to lexical token overlap (`SEARCH` on normalized seed `title` + `category`) so recommendations still return high-relevance peers!
2. **Vector kNN Leg (Spanner ANN / Exact Fallback):**
   - Query nearest neighbors (`product_id != @seed_id`) passing `catalogFilterSQL` (and `LOWER(category) = LOWER(@seed_category)` + in-stock predicate when `mode=substitutes`), using `APPROX_COSINE_DISTANCE` (`ADR-012`) with automatic exact `COSINE_DISTANCE` fallback.
3. **Session Co-Visitation Leg (`user_session_events`):**
   - Query co-occurring products interacted with in the same sessions as `@seed_id` over the last 30 days:
     ```sql
     SELECT e2.product_id,
            SUM(CASE e2.event_type WHEN 'purchase' THEN 3.0 ELSE 1.0 END) AS co_weight
     FROM user_session_events e1
     JOIN user_session_events e2
       ON e1.tenant_id = e2.tenant_id AND e1.session_id = e2.session_id
     WHERE e1.tenant_id = @tenant_id
       AND e1.product_id = @seed_id
       AND e2.product_id != @seed_id
     GROUP BY e2.product_id
     ORDER BY co_weight DESC
     LIMIT 50
     ```
4. **Blended Score in Go:**
   - Run the vector leg and co-visitation leg concurrently via `errgroup`.
   - Combine normalized vector similarity $\text{sim} \in [0, 1]$ and normalized co-visitation $\text{covis} \in [0, 1]$:
     - `similar`: $0.75 \cdot \text{sim} + 0.25 \cdot \text{covis}$
     - `substitutes`: $0.70 \cdot \text{sim} + 0.20 \cdot \text{price\_proximity} + 0.10 \cdot \text{covis}$ (where $\text{price\_proximity} = \max(0, 1 - |p - p_{\text{seed}}| / \max(p_{\text{seed}}, 1))$, strictly in-stock & same category)
     - `co_viewed`: $0.70 \cdot \text{covis} + 0.30 \cdot \text{sim}$
   - Enrich top `limit` items with `DeliveryPromises` when `cep` is provided and contract `PriceInfo` when `price_table_id` is provided.

### 3. Dual-Engine Parity, Storefront UI & `cmd/sqlcheck`
- Implement full memory-mode parity (`s.client == nil`) over `seedProducts` + in-memory `SessionService` events so `go test -race ./...` tests all 3 modes (`similar`, `substitutes`, `co_viewed`), `404` handling, B2B `price_table_id` filtering, and `NULL`-embedding lexical fallback offline.
- Storefront (`src/application/ui/src/components/ProductDetails.js` + `App.js` + `defaultStorefrontHTML`):
  - Product detail modal displays tabbed **"Similar Products"** and **"Frequently Viewed Together"** carousels, clicking any recommendation navigates/opens that item.
  - Out-of-stock cards on the SERP render a **"View In-Stock Alternatives"** quick action calling `GET /api/products/:id/similar?mode=substitutes&limit=4`.
- `cmd/sqlcheck` verifies both the neighbor SQL and `user_session_events` self-join query against `psearch-db`.

## Consequences
- Converts out-of-stock dead ends into recoverable sessions and adds rich item-to-item discovery across every industry vertical with zero extra infrastructure beyond Cloud Spanner.
