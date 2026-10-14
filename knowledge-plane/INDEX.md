# `psearch` Knowledge Plane — Master Index

Welcome to the canonical **Knowledge Plane** for the `psearch` Software Factory on [`duboc/scion-experiment`](https://github.com/duboc/scion-experiment).
All agents (`lead`, `backend`, `frontend`, `reviewer`, `deployer`) MUST consult this index and the linked ADRs before planning, implementing, reviewing, or deploying any GitHub Issue (`#1`–`#11`, `#19`–`#23`, `#28`).

---

## 1. Architecture Decision Records (ADRs by Backlog Wave)

| ADR | GitHub Issues | Wave & Status | `psearch` Functional Requirements |
| :--- | :--- | :--- | :--- |
| [`adr/ADR-001-hybrid-and-vector-1024.md`](adr/ADR-001-hybrid-and-vector-1024.md) | **#1**, **#2**, **#11**, **#23** | Wave 1 / 5 — ✅ Live (`00025-5mx`) | **4.1.1 Busca Híbrida** (`@alpha` RRF), **4.1.2 Busca Vetorial** (`gemini-embedding-2` 1024-d `embedding_v2`, Blue/Green `v1`/`v2`, `embedding_cache`), & **#23 Latest Models** (`gemini-3.8-flash` text/flash, `gemini-4-argon` Pro with `gemini-4-pro` alias normalization) |
| [`adr/ADR-002-catalog-locale-avail.md`](adr/ADR-002-catalog-locale-avail.md) | **#3**, **#4** | Wave 1/2 — ✅ Live (`00025-5mx`) | **4.1.3 Disponibilidade** (`is_available`, `stock`, `is_visible`, `channels`, OOS sorted last) & **4.1.4 Catálogo Multilanguage** (`tenant_id`, `locale`, accent-folded `TOKENLIST` in `products_text_idx`) |
| [`adr/ADR-003-facets-and-synonyms.md`](adr/ADR-003-facets-and-synonyms.md) | **#5**, **#7** | Wave 2/3 — ✅ Live (`00025-5mx`) | **4.1.5 Facets Server-Side** (Concurrent Go `errgroup` + Spanner `GROUP BY` over Top-100) & **4.1.7 Low-Relevance Synonyms** (`search_synonyms` table + `0.3 * SCORE()` expansion with exact-first rank) |
| [`adr/ADR-004-merchandising-boosts.md`](adr/ADR-004-merchandising-boosts.md) | **#6** | Wave 3 — ✅ Live (`00025-5mx`) | **4.1.6 Relevance & Merchandising Rules** (`merchandising_rules` in Spanner, `/api/rules` CRUD, pins/buries, and 8 normalized boost signals in SQL score) |
| [`adr/ADR-005-delivery-promises.md`](adr/ADR-005-delivery-promises.md) | **#8** | Wave 3 — ✅ Live (`00025-5mx`) | **4.1.8 Delivery Promises** (`products` → `skus` → `regional_inventory` via Spanner `INTERLEAVE IN PARENT` + CEP/SLA filter and fastest promise badge) |
| [`adr/ADR-006-autocomplete-and-ltr.md`](adr/ADR-006-autocomplete-and-ltr.md) | **#9**, **#10** | Wave 4 — ✅ Live (`00025-5mx`) | **4.1.9 Autocomplete** (`autocomplete_suggestions` + `SEARCH_NGRAMS`) & **4.1.10 Learning to Rank** (Stage-1 Spanner Top-100 + Stage-2 `<100µs` Go in-memory 12-feature re-ranker) |
| [`adr/ADR-007-conversational-query-and-session.md`](adr/ADR-007-conversational-query-and-session.md) | **#19**, **#20**, **#23** | Wave 5 — ✅ Live (`00025-5mx`) | **4.1.11 Conversational Query Understanding** (Parser-first + `gemini-3.8-flash` fallback, `query_intent_cache`, extracted `min_price`/`max_price`/`brand`/`color` filters) & **4.1.12 Session-Aware Personalization** (Real-time session vector centroid re-ranking) |
| [`adr/ADR-008-zero-results-and-b2b-pricing.md`](adr/ADR-008-zero-results-and-b2b-pricing.md) | **#21**, **#22** | Wave 5 — ✅ Live (`00025-5mx`) | **4.1.13 Zero-Results Recovery & Spell Check** (`SEARCH_NGRAMS` Did You Mean + `AND`→`OR` relaxation) & **4.1.14 B2B / Contract Pricing & Assortment** (`product_prices` via `INTERLEAVE IN PARENT products`, effective price, `sort_by`) |
| [`adr/ADR-009-variant-grouping.md`](adr/ADR-009-variant-grouping.md) | **#32** | Variant grouping — ✅ Accepted (live since `00027-dmv`) | **Variant Grouping / Collapse** (one card per parent `product_id`; `skus` variant columns; `EXISTS` color/size filters on the interleaved child; `ARRAY` aggregation of variants in the hybrid SQL; color/size facets) |
| [`adr/ADR-010-visual-multimodal-search.md`](adr/ADR-010-visual-multimodal-search.md) | **#33** | Visual & multimodal search — ✅ Live (`00028-klr`) | **Visual & Multimodal Search** (photo query via `gemini-embedding-2` into the existing `embedding_v2` space; fused photo+text vector, text drives FTS; vector-only SQL for photo-only; inline base64 only, no URL fetch; `multimodal_v2` mode) |

---

## 2. `psearch` Codebase Map & Engineering Invariants

### Backend Serving (`src/psearch/serving/`)
- **Entrypoints:**
  - `cmd/server/main.go` — HTTP server (`PORT=8080`).
  - `cmd/sqlcheck/main.go` — Pre-deploy read-only Spanner SQL verification tool (`go run ./cmd/sqlcheck`). Every new Spanner query/table MUST add a check here.
  - `cmd/backfill-embeddings/main.go` — Populates `embedding_v2` (`1024-d` via `gemini-embedding-2`).
- **API & Routing (`internal/api/`):**
  - `handlers.go` — `Controller` wiring (`Search`, `Autocomplete`, `ListSynonyms`/`UpsertSynonym`, `ListRules`/`CreateRule`/`UpdateRule`/`DeleteRule`, `RecordSessionEvent`/`ListSessionEvents`, `GetEmbeddingVersion`/`SetEmbeddingVersion`, `Info`, `HealthCheck`).
  - `routes.go` — Gin router + `defaultStorefrontHTML` (embedded single-page storefront served at `GET /` when `STATIC_DIR` is not mounted).
- **Models & Services (`internal/models/`, `internal/services/`):**
  - `models/models.go` — `SearchRequest`, `SearchFilters`, `SearchResponse`, `SearchResult`, `BoostSignals`, `DeliveryPromise`, `Facets`, `ParsedQuery`.
  - `services/spanner_service.go` — `HybridSearchDetailed`, `buildHybridSQLWith`, `catalogFilterSQL`, `memoryHybridSearchExpanded`.
  - `services/embedding_service.go` & `embedding_cache.go` — Vertex AI Gen AI SDK client + LRU/Spanner cache (`013_embedding_cache.sql`).
  - `services/textfold.go` — `FoldAccents` and `NormalizeQuery`.
  - `services/facets.go` — Concurrent `errgroup` facet aggregation (`categories`, `brands`, `price_ranges`, `availability`).
  - `services/synonyms.go` — `SynonymService` (`015_search_synonyms.sql`).
  - `services/merchandising.go` — `BoostSignalScore` + `RuleService` (`016_merchandising.sql`).
  - `services/delivery.go` — `NormalizeCEP`, `DeliveryPromises`, `DefaultInventory` (`017_delivery_interleaved.sql`).
  - `services/autocomplete.go` — `AutocompleteService` (`018_autocomplete.sql`).
  - `services/ltr.go` — `LTRModel`, `Rerank`, `CandidateSignals`.
  - `services/query_understanding.go` — `QueryUnderstandingService` (`020_query_intent_cache.sql`).
  - `services/spellcheck.go` — `SpellCheckService`, `RelaxedQuery`, `LexicalHits` (#21).
  - `services/session.go` — `SessionService`, `Centroid`, `ProductVectors` (`021_user_session_events.sql`, #20).
  - `services/pricing.go` — effective price, exclusive assortment, `ValidatePricing` (`019_b2b_product_prices.sql`, #22).

### Frontend UI Surfaces (Keep Both in Sync)
1. **React Application (`src/application/ui/`):**
   - `src/App.js`, `src/components/SearchInput.js`, `src/components/Filters.js`, `src/components/RuleManager.js`, `src/services/ruleService.js`, `src/components/SessionPanel.js`, `src/services/sessionService.js`, `src/components/ProductDetails.js`.
   - Build gate: `cd src/application/ui && npm run build`.
2. **Embedded Storefront (`src/psearch/serving/internal/api/routes.go`):**
   - `defaultStorefrontHTML` serves `GET /` directly on `psearch-serving` Cloud Run revisions (`<SERVICE_URL>/`). Any new search control or badge added for an issue should be wired in both the React UI and `defaultStorefrontHTML`.

### Mandatory Invariants for All Changes
1. **Dual-Engine Parity (Spanner + Memory):** Every feature in `src/psearch/serving` MUST work both against Cloud Spanner (`s.client != nil`) and in the deterministic in-memory fallback (`s.client == nil`) so `go test -race ./...` passes offline in `<2s`.
2. **Authoritative Spanner Empty Results:** When Spanner returns `0` rows without error (`err == nil`), return `[]` with `retrieval_mode=hybrid_v2` — never fall through to `memoryHybridSearchExpanded` on empty filter matches.
3. **Security:** All SQL queries MUST use bound `@params` (only package-level constant column/token identifiers may be interpolated). All write endpoints (`POST`/`PUT`/`DELETE` for config, rules, synonyms) MUST call `c.authorizeAdmin(ctx)`.

---

## 3. Live State, Product Docs & Evaluation

- **Active revision:** `psearch-serving-00025-5mx` (Issue #23; source `41af0a6`), rollback `00024-ltv`. Models: `gemini-3.8-flash` (text), `gemini-4-argon` (pro), `gemini-embedding-2` 1024-d, Vertex AI `global`. All 14 requirements (4.1.1–4.1.14) live.
- **Product & engineering docs:** [`../README.md`](../README.md) · [`../PRODUCT.md`](../PRODUCT.md) · [`../DESIGN.md`](../DESIGN.md) · [`../DEPLOYMENT.md`](../DEPLOYMENT.md) · [`../REVIEW.md`](../REVIEW.md).
- **Verification & evaluation:** [`../docs/scion/verification-and-eval.md`](../docs/scion/verification-and-eval.md) — control-plane verification and the 5-layer (L1–L5) product evaluation pyramid.
- **Migrations:** `src/iac/migrations/011`–`021` (see DEPLOYMENT.md §3).

---

## 4. API Contracts & Operational Playbooks

- [`contracts/biggy-search-openapi.yaml`](contracts/biggy-search-openapi.yaml) — Canonical REST contract for `psearch-serving` (`/api/search`, `/api/autocomplete`, `/api/rules`, `/api/synonyms`, `/api/session/events`, `/api/config/embedding-version`, `/api/info`, `/api/health`).
- [`playbooks/SPANNER-DDL-MIGRATION-GUIDE.md`](playbooks/SPANNER-DDL-MIGRATION-GUIDE.md) — Online Cloud Spanner DDL/DML rules, `cmd/sqlcheck` pre-deploy gate, and Spanner SQL gotchas in the sandbox project (`psearch-instance` / `psearch-db`).
- [`playbooks/GITHUB-ISSUE-FACTORY-WORKFLOW.md`](playbooks/GITHUB-ISSUE-FACTORY-WORKFLOW.md) — Execution protocol for `lead`, `backend`, `frontend`, `reviewer`, and `deployer`.
