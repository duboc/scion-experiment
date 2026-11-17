# Intelligent product search UI

This directory contains the React 19 single-page application (`psearch-ui`) for the Intelligent Product Search (`psearch`) platform.
The Go serving API also includes an embedded single-file storefront (`defaultStorefrontHTML`) in the [HTTP routes handler (`routes.go`)](../../psearch/serving/internal/api/routes.go).
Both interfaces call the REST endpoints defined in the [OpenAPI 3.0.3 serving contract (`biggy-search-openapi.yaml`, version `2.6.0`)](../../../knowledge-plane/contracts/biggy-search-openapi.yaml).

> [!IMPORTANT]
> Do not commit secrets, API keys, service URLs, or real Google Cloud project IDs to tracked files.
> Use placeholder values such as `PROJECT_ID` and `SERVICE_URL` in configuration examples.

## How the two storefronts work

The search platform provides two user interfaces that maintain feature parity for core search workflows:

1. **React application (`src/application/ui/`)**: A Material UI storefront and administration console with client-side routes for product search (`/`), product details (`/product/:productId`), catalog source ingestion (`/source-ingestion`), merchandising rules (`/manage`), and sponsored ad campaigns (`/ads`).
2. **Embedded storefront (`defaultStorefrontHTML` in `routes.go`)**: A zero-dependency HTML and JavaScript storefront compiled into the Go serving binary with a default dark theme.

When you set the `STATIC_DIR` environment variable on the Go serving binary, the server serves the compiled React bundle from that directory.
When `STATIC_DIR` is empty or the build directory does not exist, `GET /` serves `defaultStorefrontHTML`.
The embedded storefront header displays a repository badge (`data-testid="repo-badge"`) with `Repo: duboc/scion-experiment` and omits Google Cloud project identifiers.
The Go serving binary also embeds the [interactive cloud architecture diagram (`diagram.html`)](../../psearch/serving/internal/api/diagram.html) with `//go:embed` and serves it at `GET /diagram`.

## Prerequisites

Install these tools before you build or run the user interface:

- **Node.js**: Version 20 LTS or later (the container [`Dockerfile`](Dockerfile) builds with `node:24-alpine`).
- **npm**: Version 10 or later.
- **Go**: Version 1.26 or later (required to run the Go serving API and storefront tests).

## Install and run the UI

Follow these steps to run the React application on your workstation against a local Go serving API:

1. Open a terminal in `src/psearch/serving/` and start the Go serving API in memory mode on port `8080`:

   ```bash
   USE_MEMORY_STORE=true go run ./cmd/server
   ```

2. Open a second terminal in `src/application/ui/`.
3. Install the package dependencies with legacy peer dependency resolution:

   ```bash
   npm ci --legacy-peer-deps
   ```

4. Set the serving API URL environment variable:

   ```bash
   export REACT_APP_API_URL="http://localhost:8080"
   ```

5. Start the development server on port `3000`:

   ```bash
   npm start
   ```

6. (Optional) Run the development server on port `3001` with host-header checks disabled for remote or container workspaces:

   ```bash
   npm run start:dev
   ```

### View the embedded storefront and architecture diagram locally

Follow these steps to view `GET /` and `GET /diagram` directly from the Go serving binary:

1. Open a terminal in `src/psearch/serving/`.
2. Start the Go server in memory mode on port `8080`:

   ```bash
   USE_MEMORY_STORE=true go run ./cmd/server
   ```

3. Open `http://localhost:8080/` in your browser to inspect the embedded storefront.
4. Open `http://localhost:8080/diagram` in your browser to inspect the interactive cloud architecture diagram.

### Configure the API proxy and environment variables

During local development, the [development proxy middleware (`src/setupProxy.js`)](src/setupProxy.js) forwards `/search` requests to `REACT_APP_API_URL`.
The [runtime configuration module (`src/config.js`)](src/config.js) reads the following environment variables at build time and startup:

| Variable | Default value | Purpose |
|---|---|---|
| `REACT_APP_API_URL` | `http://localhost:8080` | Base URL for search, autocomplete, facets, recommendations, merchandising rules, ad campaigns, shopper sessions, and analytics |
| `REACT_APP_GEN_AI_URL` | `http://localhost:8080` | Base URL for generative AI product enrichment, marketing copy, image enhancement, and SQL transformation endpoints |
| `REACT_APP_INGESTION_SOURCE_API_URL` | `http://localhost:8082` | Base URL for catalog source ingestion endpoints |
| `REACT_APP_PROJECT_ID` | `null` | Google Cloud project ID placeholder (`PROJECT_ID`) used by generative AI schema transformation requests |

> [!NOTE]
> The `start:dev` script in [`package.json`](package.json) also sets `REACT_APP_MOCK_DATA=true` and `PORT=3001`, and `src/` contains legacy mock fixtures (`src/mockData.js` and `src/mockAiEnhancements.js`).
> Current search components in [`src/App.js`](src/App.js) send requests to `REACT_APP_API_URL`, so run the Go server with `USE_MEMORY_STORE=true` when you test locally without Google Cloud.

## Build for production

Follow these steps to build the production bundle or container image:

1. Run the production build from `src/application/ui/`:

   ```bash
   npm run build
   ```

   The build writes static assets to `src/application/ui/build/`.

2. (Optional) Build the multi-stage NGINX container image (`node:24-alpine` build stage and `nginx:alpine` serving stage on port `8080`) from `src/application/ui/`:

   ```bash
   docker build \
     --build-arg REACT_APP_API_URL="SERVICE_URL" \
     --build-arg REACT_APP_GEN_AI_URL="SERVICE_URL" \
     --build-arg REACT_APP_INGESTION_SOURCE_API_URL="SERVICE_URL" \
     --build-arg REACT_APP_PROJECT_ID="PROJECT_ID" \
     -t psearch-ui:latest .
   ```

> [!NOTE]
> Do not commit the `src/application/ui/build/` directory to Git.
> Continuous integration and the [container `Dockerfile`](Dockerfile) generate the production bundle during image builds.

## Project structure

| Path | Description |
|---|---|
| [`package.json`](package.json) | Dependencies, npm scripts (`start`, `build`, `test`, `eject`, `start:dev`), and browser targets |
| [`Dockerfile`](Dockerfile) | Multi-stage `node:24-alpine` build and `nginx:alpine` serving image definition on port `8080` |
| [`src/App.js`](src/App.js) | Main storefront view, client-side router, search request orchestration, product grid, pagination, and analytics drawer |
| [`src/config.js`](src/config.js) | Environment variable loader for `REACT_APP_API_URL`, `REACT_APP_GEN_AI_URL`, `REACT_APP_INGESTION_SOURCE_API_URL`, and `REACT_APP_PROJECT_ID` |
| [`src/setupProxy.js`](src/setupProxy.js) | Development proxy middleware that forwards `/search` requests to `REACT_APP_API_URL` |
| [`src/products.json`](src/products.json) | JSON Schema Draft-07 destination product schema used by `src/services/genAiService.js` during ingestion mapping |
| [`src/components/SearchInput.js`](src/components/SearchInput.js) | Search bar, photo upload (`image_base64`), autocomplete dropdown, `alpha` slider, channel, CEP, SLA, LTR, smart query, price table, sort, page size, and experiment controls |
| [`src/components/Filters.js`](src/components/Filters.js) | Disjunctive multi-select checkbox facets, price range filters, availability filters, and dynamic attribute facet groups |
| [`src/components/AIFilterSuggestion.js`](src/components/AIFilterSuggestion.js) | Suggested filter chips derived from active search results |
| [`src/components/ProductDetails.js`](src/components/ProductDetails.js) | Product detail view, purchase attribution trigger, recommendation carousels, and generative AI content, marketing, and image enhancement dialogs |
| [`src/components/SourceIngestion.js`](src/components/SourceIngestion.js) | Catalog file and database ingestion workflow with AI-generated BigQuery SQL transformations and [`SqlErrorFix.js`](src/components/SqlErrorFix.js) |
| [`src/components/RuleManager.js`](src/components/RuleManager.js) | Merchandising rule editor for query-scoped or global boost, bury, and pin actions |
| [`src/components/AdCampaignManager.js`](src/components/AdCampaignManager.js) | Sponsored product campaign manager and CPC bid configuration dialog |
| [`src/components/SessionPanel.js`](src/components/SessionPanel.js) | Real-time shopper session panel showing click history, demo personas, and category or brand affinities |
| [`src/services/`](src/services/) | HTTP client modules for ad campaigns (`adService.js`), generative AI (`genAiService.js`), merchandising rules (`ruleService.js`), sessions (`sessionService.js`), and source ingestion (`sourceIngestionService.js`) |
| [`../../psearch/serving/internal/api/routes.go`](../../psearch/serving/internal/api/routes.go) | Go HTTP router, embedded `defaultStorefrontHTML` storefront (`GET /`), and `GET /diagram` route |
| [`../../psearch/serving/internal/api/diagram.html`](../../psearch/serving/internal/api/diagram.html) | Self-contained interactive cloud architecture diagram embedded with `//go:embed diagram.html` |
| [`../../psearch/serving/internal/api/storefront_test.go`](../../psearch/serving/internal/api/storefront_test.go) | Automated Go tests that verify `defaultStorefrontHTML`, `GET /diagram`, and identifier redaction rules |

## UI feature map

Both the React application and `defaultStorefrontHTML` implement these core search capabilities:

| Feature area | UI elements | Backend API contract |
|---|---|---|
| **Hybrid, multimodal, and conversational search** | Search bar, photo query upload (`image_base64`, `image_mime_type`), debounced typeahead suggestions, `alpha` slider (`0.0` textual to `1.0` vector), `Smart query` (`parse_intent`) extracted filter chips, spell-check banners (`auto_corrected`, `did_you_mean`, `query_relaxed`), and the `ANN: ScaNN` / `ANN: exact` badge (`vector_index_used`) | `POST /api/search`, `POST /search`, `GET /api/search`, `GET /api/autocomplete` |
| **Repository header badge (#52)** | Header badge (`data-testid="repo-badge"`) displaying `Repo: duboc/scion-experiment` with no Google Cloud project ID | `GET /` |
| **Cloud architecture diagram (#51)** | Self-contained SVG and HTML presentation (`diagram.html`) covering system context, `/api/search` steps `1–9`, data ingestion, telemetry, and operations | `GET /diagram` |
| **Pagination and sort modes (#40)** | `Showing X-Y of Z results` counter, `Previous` / `Next` and numbered page buttons, page size selector (`12`, `24`, `48`), and five sort modes (`relevance`, `price_asc`, `price_desc`, `popularity`, `rating_desc`). Resets `page` to `1` when the query or filters change | Request: `page`, `offset`, `limit`, `sort_by`; Response: `total_found`, `page`, `page_size`, `offset`, `total_pages`, `has_next` |
| **Multi-select facets and filter pills (#41)** | Disjunctive checkbox groups for `brands`, `categories`, `colors`, `sizes`, `availability`, and `price_ranges`, plus removable active filter pills and a `Clear all` button | `brands[]`, `categories[]`, `colors[]`, `sizes[]`, `price_ranges[]`, `only_in_stock`, `facets` |
| **Dynamic attribute facets and spec badges (#42)** | Collapsible facet groups rendered automatically from `facets.attributes`, and up to three `key: value` specification badges on each product card (`dynamic_attributes`) | `attrs` map on `POST /api/search`, `attr.<name>` on `GET /api/search`, `PUT /api/products/:id/attributes`, `facets.attributes` |
| **Recommendations and in-stock alternatives (#44)** | **Similar Products** (`mode=similar&limit=6`) and **Frequently Viewed Together** (`mode=co_viewed&limit=6`) carousels in product details, plus **See In-Stock Alternatives** (`mode=substitutes&limit=4`) on out-of-stock cards | `GET /api/products/:id/similar?mode=similar\|co_viewed\|substitutes` |
| **Search telemetry and A/B experiments (#43)** | Experiment selector (`control`, `ltr_v2`, `ltr_boost`), click/cart/purchase attribution beacons with `query_id` and 1-based `position`, and the **Search Analytics** drawer showing CTR, `MRR@10`, Top Zero-Result Queries, and A/B comparison | `POST /api/analytics/events`, `GET /api/analytics/summary`, `POST /api/analytics/rollup` |
| **Sponsored products and retail media** | `Patrocinado` / `Sponsored` badge on promoted product cards (`sponsored`, `ad_campaign_id`, `ad_click_id`) and the **Sponsored Ads** management view (`/ads`) | `GET /api/ads/campaigns`, `POST /api/ads/campaigns`, `PUT /api/ads/campaigns/:id`, `DELETE /api/ads/campaigns/:id`, `POST /api/ads/click` |
| **Merchandising rules** | **Manage Rules** view (`/manage`) for creating query-scoped or global boost, bury, and pin rules | `GET /api/rules`, `POST /api/rules`, `PUT /api/rules/:id`, `DELETE /api/rules/:id` |
| **Session personalization** | **Session Panel** showing live shopper click events, demo personas, and in-session category or brand affinities (`personalized`, `session_boost`) | `POST /api/session/events`, `GET /api/session/events?session_id=SESSION_ID` |
| **B2B contract pricing and delivery SLA** | Price table selector (`b2b-corporate`, `b2b-gold`, `vip-club`) displaying contract prices and strike-through base prices, plus CEP postal code input and `Full 24h` (`max_sla_hours: 24`) delivery filtering | Request: `price_table_id`, `cep`, `max_sla_hours`; Response: `price_table_id`, `priceInfo`, `deliveryPromise` |

## Run storefront tests

Update `defaultStorefrontHTML` in the [HTTP routes handler (`routes.go`)](../../psearch/serving/internal/api/routes.go) whenever you change a storefront feature.
Add matching assertions in the [storefront test suite (`storefront_test.go`)](../../psearch/serving/internal/api/storefront_test.go) for every new storefront element.

1. Run the React production build from `src/application/ui/` to verify that all components compile:

   ```bash
   npm run build
   ```

2. Run the Go storefront, diagram, and catalog browse tests from `src/psearch/serving/`:

   ```bash
   go test -v ./internal/api -run "TestStorefront|TestCatalogBrowse|TestArchitectureDiagramRoute"
   ```

## Troubleshooting

### `npm install` or `npm ci` fails with peer dependency errors

`react-scripts` `5.0.1` requires legacy peer dependency resolution with React 19 and Material UI.
Run `npm ci --legacy-peer-deps` instead of `npm ci`.

### Search requests fail with network or CORS errors

1. Confirm that the Go serving API is running at the URL in `REACT_APP_API_URL` (default `http://localhost:8080`).
2. Restart `npm start` after you change any `REACT_APP_*` environment variable.
3. Start the Go server with `USE_MEMORY_STORE=true go run ./cmd/server` if you want to test the UI locally with the in-memory product catalog.

### Changes in `src/application/ui/` do not appear at `GET /` on the Go server

The Go server returns `defaultStorefrontHTML` unless you compile the React bundle and set `STATIC_DIR` to `src/application/ui/build`.
Update both `src/application/ui/` and `defaultStorefrontHTML` in `src/psearch/serving/internal/api/routes.go` to keep both storefronts synchronized.
