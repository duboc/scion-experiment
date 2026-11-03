# Intelligent Product Search UI

This directory contains the React single-page application (`psearch-ui`) for Intelligent Product Search (`psearch`).
The Go serving API also provides an embedded single-file storefront (`defaultStorefrontHTML`) in [`src/psearch/serving/internal/api/routes.go`](../../psearch/serving/internal/api/routes.go).
Both interfaces use the same OpenAPI v2.6 search, merchandising, recommendation, and telemetry endpoints.

> [!IMPORTANT]
> Do not write secrets, API keys, or real Google Cloud project IDs in tracked files.
> Use placeholder values such as `<PROJECT_ID>` in configuration examples.

## How the two storefronts work

The search platform provides two user interfaces that stay in feature parity:

1. **React application (`src/application/ui/`)**: Full-featured Material UI storefront and merchandising console for local development and container deployments.
2. **Embedded storefront (`defaultStorefrontHTML` in `routes.go`)**: Zero-dependency HTML and JavaScript storefront built into the Go serving binary.

When you set `STATIC_DIR` on the Go serving binary, the server serves the compiled React bundle from that path.
When `STATIC_DIR` is empty or the build directory does not exist, `GET /` serves `defaultStorefrontHTML`.

## Prerequisites

Install these tools before you build or run the user interface:

- **Node.js**: Version 20 LTS or later.
- **npm**: Version 10 or later.
- **Go**: Version 1.26 or later (required for the serving API and storefront tests).

## Install and run the UI

Follow these steps to run the React application on your workstation:

1. Open a terminal in `src/application/ui/`.
2. Install the package dependencies:

   ```bash
   npm ci --legacy-peer-deps
   ```

3. Set the serving API URL environment variable:

   ```bash
   export REACT_APP_API_URL="http://localhost:8080"
   ```

4. Start the development server on port `3000`:

   ```bash
   npm start
   ```

5. (Optional) Start the UI on port `3001` with local mock catalog data instead of a live backend:

   ```bash
   npm run start:dev
   ```

### Configure the API proxy and environment variables

During local development, [`src/setupProxy.js`](src/setupProxy.js) forwards `/search` requests to `REACT_APP_API_URL`.
[`src/config.js`](src/config.js) reads these environment variables at build time and runtime:

| Variable | Default value | Purpose |
|---|---|---|
| `REACT_APP_API_URL` | `http://localhost:8080` | Base URL for search, facets, recommendations, rules, campaigns, sessions, and analytics |
| `REACT_APP_GEN_AI_URL` | `http://localhost:8080` | Base URL for generative AI product enrichment endpoints |
| `REACT_APP_INGESTION_SOURCE_API_URL` | `http://localhost:8082` | Base URL for catalog ingestion endpoints |
| `REACT_APP_PROJECT_ID` | `null` | Google Cloud project ID placeholder (`<PROJECT_ID>`) |
| `REACT_APP_MOCK_DATA` | `false` | Enables local mock data from `src/products.json` when set to `true` |

## Build for production

Run this command from `src/application/ui/` to create an optimized production bundle:

```bash
npm run build
```

The build writes static assets to `src/application/ui/build/`.

> [!NOTE]
> Do not commit the `src/application/ui/build/` directory to Git.
> Continuous integration and the [`Dockerfile`](Dockerfile) generate the production bundle during image builds.

To build the production NGINX container image, run this command from `src/application/ui/`:

```bash
docker build \
  --build-arg REACT_APP_API_URL="<SERVICE_URL>" \
  --build-arg REACT_APP_PROJECT_ID="<PROJECT_ID>" \
  -t psearch-ui:latest .
```

## Project structure

| Path | Description |
|---|---|
| [`package.json`](package.json) | Dependencies, npm scripts (`start`, `build`, `test`, `start:dev`), and browser targets |
| [`Dockerfile`](Dockerfile) | Multi-stage Node.js build and NGINX serving image definition |
| [`src/App.js`](src/App.js) | Main storefront view, search request orchestration, product grid, pagination, and analytics drawer |
| [`src/config.js`](src/config.js) | Environment variable loader for API and service endpoints |
| [`src/setupProxy.js`](src/setupProxy.js) | Development proxy middleware for `/search` requests |
| [`src/components/SearchInput.js`](src/components/SearchInput.js) | Search bar, autocomplete dropdown, price table selector, sort selector, page-size selector, and experiment selector |
| [`src/components/Filters.js`](src/components/Filters.js) | Disjunctive multi-select checkbox facets, price range filters, and dynamic attribute facet groups |
| [`src/components/ProductDetails.js`](src/components/ProductDetails.js) | Product detail view, purchase trigger, and similar or co-viewed recommendation carousels |
| [`src/components/RuleManager.js`](src/components/RuleManager.js) | Merchandising rule editor for boost, bury, and pin actions |
| [`src/components/AdCampaignManager.js`](src/components/AdCampaignManager.js) | Sponsored product campaign manager and bid configuration dialog |
| [`src/components/SessionPanel.js`](src/components/SessionPanel.js) | Real-time shopper session drawer showing click history and category or brand affinities |
| [`src/services/`](src/services/) | HTTP client modules for rules, ad campaigns, sessions, ingestion, and generative AI |
| [`../../psearch/serving/internal/api/routes.go`](../../psearch/serving/internal/api/routes.go) | HTTP router and the embedded `defaultStorefrontHTML` storefront |
| [`../../psearch/serving/internal/api/storefront_test.go`](../../psearch/serving/internal/api/storefront_test.go) | Automated Go tests that verify `defaultStorefrontHTML` markup and API wiring |

## UI feature map

Both the React application and `defaultStorefrontHTML` implement these capabilities:

| Feature area | UI elements | Backend API contract |
|---|---|---|
| **Hybrid search and vector index badge** | Search bar, typeahead suggestions (`GET /api/autocomplete`), and the `ScaNN ANN` / `Exact KNN` badge (`vector_index_used`) | `POST /api/search`, `GET /api/search`, `GET /api/autocomplete` |
| **Pagination and sort modes (#40)** | `Showing X–Y of Z` counter, `Prev` / `Next` and numbered page buttons, page size selector (`12`, `24`, `48`), and five sort modes (`relevance`, `price_asc`, `price_desc`, `popularity`, `rating_desc`). Resets `page` to `1` when the query or filters change | Request: `page`, `offset`, `limit`, `sort_by`; Response: `total_found`, `page`, `page_size`, `offset`, `total_pages`, `has_next` |
| **Multi-select facets and filter pills (#41)** | Disjunctive checkbox groups for `brands`, `categories`, `colors`, `sizes`, and `price_ranges`, plus removable active filter pills and a `Clear all` button | `brands[]`, `categories[]`, `colors[]`, `sizes[]`, `price_ranges[]`, `facets` |
| **Dynamic attribute facets and spec badges (#42)** | Collapsible facet groups rendered automatically from `facets.attributes`, and up to three `key: value` specification badges on each product card (`dynamic_attributes`) | `attrs` map on `POST /api/search`, `attr.<name>` on `GET /api/search`, `PUT /api/products/:id/attributes`, `facets.attributes` |
| **Recommendations and in-stock alternatives (#44)** | **Similar Products** (`mode=similar&limit=6`) and **Frequently Viewed Together** (`mode=co_viewed&limit=6`) carousels in product details, plus **See In-Stock Alternatives** (`mode=substitutes&limit=4`) on out-of-stock cards | `GET /api/products/:id/similar?mode=similar\|co_viewed\|substitutes` |
| **Search telemetry and A/B experiments (#43)** | Experiment selector (`control`, `ltr_v2`, `ltr_boost`), click/cart/purchase attribution beacons with `query_id` and 1-based `position`, and the **Search Analytics** drawer showing CTR, `MRR@10`, Top Zero-Result Queries, and A/B comparison | `POST /api/analytics/events`, `GET /api/analytics/summary`, `POST /api/analytics/rollup` |
| **Sponsored products and retail media** | `Sponsored` badge on promoted product cards (`sponsored`, `ad_campaign_id`, `ad_click_id`) and the **Ad Campaigns** management dialog | `GET /api/ads/campaigns`, `POST /api/ads/campaigns`, `PUT /api/ads/campaigns/:id`, `DELETE /api/ads/campaigns/:id`, `POST /api/ads/click` |
| **Merchandising rules** | **Search Rules** dialog for creating query-scoped or global boost, bury, and pin rules | `GET /api/rules`, `POST /api/rules`, `PUT /api/rules/:id`, `DELETE /api/rules/:id` |
| **Session personalization** | **Session Panel** showing live shopper click events, demo personas, and in-session affinities | `POST /api/session/events`, `GET /api/session/events?session_id=<id>` |
| **B2B contract pricing** | Price table selector (`b2b-corporate`, `b2b-gold`, `vip-club`) displaying contract prices and strike-through list prices | Request: `price_table_id`; Response: `price_table_id` and `priceInfo` (`price`, `originalPrice`, `basePrice`, `priceTableId`) |

## Run storefront tests

Update `defaultStorefrontHTML` in [`routes.go`](../../psearch/serving/internal/api/routes.go) whenever you change a UI feature.
Add matching assertions in [`storefront_test.go`](../../psearch/serving/internal/api/storefront_test.go) for every new storefront element.

1. Run the React production build from `src/application/ui/`:

   ```bash
   npm run build
   ```

2. Run the Go storefront and catalog browse tests from `src/psearch/serving/`:

   ```bash
   go test -v ./internal/api -run "TestStorefront|TestCatalogBrowse"
   ```

## Troubleshooting

### `npm install` or `npm ci` fails with peer dependency errors

`react-scripts` 5.0.1 requires legacy peer dependency resolution with React 19 and Material UI.
Run `npm ci --legacy-peer-deps` instead of `npm ci`.

### Search requests fail with network or CORS errors

1. Confirm that the Go serving API is running on the URL in `REACT_APP_API_URL` (default `http://localhost:8080`).
2. Restart `npm start` after you change any `REACT_APP_*` environment variable.
3. Run `npm run start:dev` if you want to test the UI with local mock data only.

### Changes in `src/application/ui/` do not appear at `GET /` on the Go server

The Go server returns `defaultStorefrontHTML` unless you set `STATIC_DIR` to `src/application/ui/build`.
Update both `src/application/ui/` and `defaultStorefrontHTML` in `src/psearch/serving/internal/api/routes.go` to keep both storefronts synchronized.
