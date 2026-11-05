# Use the PSearch storefront and API

This guide shows you how to search products, filter by facets, fetch recommendations, view the architecture diagram, and manage rules and telemetry in PSearch.
This document follows the Google Developer documentation style and the ASD-STE100 Simplified Technical English standard.

> [!IMPORTANT]
> Replace `SERVICE_URL`, `PROJECT_ID`, and `ADMIN_TOKEN` with your environment values.
> Do not commit real service URLs or secret tokens to version control.

## Overview

PSearch exposes all search, faceting, recommendation, merchandising, retail media, and analytics features through the `psearch-serving` REST API.
See [`../DEPLOYMENT.md`](../DEPLOYMENT.md) for the current Cloud Run revision, and see [`../README.md`](../README.md) for the complete feature matrix across Issues `#1`–`#54`.

You can interact with PSearch through the embedded storefront (`GET /`), the interactive architecture diagram (`GET /diagram`), the React UI (`src/application/ui`), or direct HTTP requests.

## Before you begin

1. Start `psearch-serving` locally (`USE_MEMORY_STORE=true go run ./cmd/server`) or set your Cloud Run URL:

   ```bash
   export SERVICE_URL="http://localhost:8080"
   ```

2. Verify that the service is healthy:

   ```bash
   curl -s "$SERVICE_URL/api/health" | jq .
   ```

3. Inspect the active configuration, embedding model, and ScaNN vector index state (`sandbox` reports the environment name; `#52`):

   ```bash
   curl -s "$SERVICE_URL/api/info" | jq .
   ```

## Open the storefront and architecture diagram

1. Open `$SERVICE_URL/` in your web browser to use the zero-dependency embedded storefront.
2. Type a query in the search bar, or click **Show All Catalog** (`q=*`).
3. Select brands, categories, colors, sizes, price buckets, or dynamic attributes in the left sidebar.
4. Select a sort mode, page size, B2B price table, or postal code in the toolbar.
5. Click **See In-Stock Alternatives** on any out-of-stock card to load in-stock substitutes.
6. Open `$SERVICE_URL/diagram` in your browser to inspect the interactive cloud architecture diagram (`#51`).

For the full React administration UI, see [`../src/application/ui/README.md`](../src/application/ui/README.md).

## Run hybrid search and pagination queries

Use `GET /api/search` or `POST /api/search` to query the catalog.
PSearch computes a fixed candidate window `W = min(max(capped_count, 100), 500)` from a capped `LIMIT 501` count (`#40`, `#46`).
Stage-1 selects candidates by relevance and places out-of-stock items last over `W` (`#45`).

1. Run a hybrid vector and full-text search with `alpha=0.5`:

   ```bash
   curl -s "$SERVICE_URL/api/search?q=running+shoes&alpha=0.5&limit=12&page=1" | jq .
   ```

2. Navigate to page 2 and sort by best-selling products (`popularity`) or customer rating (`rating_desc`):

   ```bash
   curl -s "$SERVICE_URL/api/search?q=shoes&limit=12&page=2&sort_by=popularity" | jq .
   ```

3. Run a conversational query in Portuguese (the parser extracts `max_price=200` automatically):

   ```bash
   curl -s "$SERVICE_URL/api/search?q=tenis+de+corrida+ate+200+reais" | jq '.parsed_query, .results[0]'
   ```

4. Enable Stage-2 Learning to Rank (`ltr=true`) and session personalization (`session_id`):

   ```bash
   curl -s "$SERVICE_URL/api/search?q=shoes&ltr=true&session_id=sess-demo-0001" | jq .
   ```

## Filter with disjunctive and dynamic facets

Within a single facet dimension, multiple values combine with `OR`.
Across different facet dimensions, filters combine with `AND`, and each dimension reports exclude-own-dimension sibling counts ([ADR-013](../knowledge-plane/adr/ADR-013-disjunctive-facets.md)).

1. Compare multiple brands (`Nike` or `Asics`) for in-stock size `42`:

   ```bash
   curl -s "$SERVICE_URL/api/search?q=shoes&brands=Nike,Asics&sizes=42" | jq '.facets.brands, .results[].id'
   ```

2. Filter by auto-discovered dynamic product attributes (`attr.<name>` on `GET`, or `attrs` on `POST`; [ADR-014](../knowledge-plane/adr/ADR-014-dynamic-product-attributes.md)):

   ```bash
   curl -s "$SERVICE_URL/api/search?q=notebook&attr.ram=16GB,32GB&attr.storage=1TB" | jq '.facets.attributes, .results[].id'
   ```

3. Apply B2B contract pricing (`price_table_id`) and regional CEP delivery SLA filtering (`cep` and `max_sla_hours`):

   ```bash
   curl -s "$SERVICE_URL/api/search?q=laptop&price_table_id=b2b-gold&cep=01310-100&max_sla_hours=24&sort_by=price_asc" | jq .
   ```

## Search with a query image

Send a base64-encoded JPEG, PNG, or WebP image in a `POST /api/search` request ([ADR-010](../knowledge-plane/adr/ADR-010-visual-multimodal-search.md)).

```bash
curl -s -X POST "$SERVICE_URL/api/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "em azul",
    "image_base64": "BASE64_ENCODED_IMAGE",
    "image_mime_type": "image/jpeg",
    "image_weight": 0.6,
    "limit": 12
  }' | jq .
```

## Fetch similar products and in-stock substitutes

Call `GET /api/products/:id/similar` with `mode=similar`, `mode=substitutes`, or `mode=co_viewed` ([ADR-016](../knowledge-plane/adr/ADR-016-similar-items-and-substitutes.md)).

1. Fetch in-stock substitutes for an out-of-stock product (`prod-004`):

   ```bash
   curl -s "$SERVICE_URL/api/products/prod-004/similar?mode=substitutes&limit=4" | jq .
   ```

2. Fetch session co-viewed recommendations for `prod-001`:

   ```bash
   curl -s "$SERVICE_URL/api/products/prod-001/similar?mode=co_viewed&limit=6" | jq .
   ```

## Track search telemetry and run idempotent signal rollup

Every `/api/search` response returns a unique `query_id` ([ADR-015](../knowledge-plane/adr/ADR-015-search-telemetry-and-attribution.md)).
The signal rollup (`POST /api/analytics/rollup`) snapshots a frozen baseline on its first live run (`027_products_signal_baseline.sql`, `#47`).
It blends window events with `prior_weight` (`k` in `(0, 10000]`) and skips zero-impression products.

1. Record an idempotent click or purchase event linked to `query_id`:

   ```bash
   curl -s -X POST "$SERVICE_URL/api/analytics/events" \
     -H "Content-Type: application/json" \
     -d '{
       "event_id": "evt-demo-001",
       "query_id": "QUERY_ID_FROM_SEARCH",
       "session_id": "sess-demo-0001",
       "event_type": "click",
       "product_id": "prod-001",
       "position": 1,
       "experiment_id": "ltr_v2"
     }' | jq .
   ```

2. View tenant search KPIs, `mrr_at_10`, top zero-result queries, and A/B experiment metrics:

   ```bash
   curl -s "$SERVICE_URL/api/analytics/summary?tenant_id=default&window_hours=168" | jq .
   ```

3. Preview the signal rollup in dry-run mode (requires `X-Admin-Token` and writes nothing):

   ```bash
   curl -s -X POST "$SERVICE_URL/api/analytics/rollup?dry_run=true" \
     -H "X-Admin-Token: $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"window_days": 30, "prior_weight": 100}' | jq .
   ```

## Manage rules, synonyms, attributes, and ad campaigns

All admin write endpoints require `-H "X-Admin-Token: $ADMIN_TOKEN"`.
If `ADMIN_TOKEN` is unset on the server, admin writes return `HTTP 403 Forbidden`.

1. Create a merchandising rule (`POST /api/rules`):

   ```bash
   curl -s -X POST "$SERVICE_URL/api/rules" \
     -H "X-Admin-Token: $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Pin Pegasus on shoes",
       "query_pattern": "shoes",
       "target_brand": "Nike",
       "boost_multiplier": 1.3,
       "pin_product_ids": ["prod-003"],
       "active": true
     }' | jq .
   ```

2. Upsert dynamic product attributes (`PUT /api/products/:id/attributes`):

   ```bash
   curl -s -X PUT "$SERVICE_URL/api/products/prod-005/attributes" \
     -H "X-Admin-Token: $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"ram": ["32GB", "64GB"], "storage": ["1TB"], "processor": ["Intel Core Ultra 7"]}' | jq .
   ```

3. Create a sponsored product campaign (`POST /api/ads/campaigns`) and record an idempotent ad click (`POST /api/ads/click`):

   ```bash
   curl -s -X POST "$SERVICE_URL/api/ads/campaigns" \
     -H "X-Admin-Token: $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "campaign_id": "camp-demo-001",
       "advertiser": "Adidas",
       "product_id": "prod-001",
       "cpc_micros": 300000,
       "budget_micros": 3000000,
       "status": "active"
     }' | jq .

   curl -s -X POST "$SERVICE_URL/api/ads/click" \
     -H "Content-Type: application/json" \
     -d '{"campaign_id": "camp-demo-001", "click_id": "clk-demo-001"}' | jq .
   ```

## Troubleshoot common API errors

| HTTP status | Common cause | Resolution |
|---|---|---|
| `400 Bad Request` | Invalid `sort_by`, `offset > 1000`, `limit > 100`, invalid CEP, or out-of-range `prior_weight` (`k`) or `window_days`. | Check parameter bounds against [`biggy-search-openapi.yaml`](../knowledge-plane/contracts/biggy-search-openapi.yaml). |
| `403 Forbidden` | Missing or invalid `X-Admin-Token` header, or `ADMIN_TOKEN` is unset on the server. | Set `ADMIN_TOKEN` on the server and pass `-H "X-Admin-Token: $ADMIN_TOKEN"`. |
| `404 Not Found` | Seed `product_id` on `/api/products/:id/similar` or `/attributes` does not exist in `tenant_id`. | Verify that the `product_id` exists and is visible in the requested `tenant_id`. |
| `413 Payload Too Large` | Request body on `POST /api/search` exceeds 8 MB. | Resize the query image to 4 MB or smaller before base64 encoding. |

## What's next

- See the full architecture and scoring formulas in [`../DESIGN.md`](../DESIGN.md).
- Follow the deployment and migration playbook in [`../DEPLOYMENT.md`](../DEPLOYMENT.md).
- Run load tests against the API using [`../tooling/loadtest/README.md`](../tooling/loadtest/README.md).
