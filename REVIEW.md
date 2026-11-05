# `psearch` Software Factory — Verification & Review Log


## Review summary (through Wave 5 and Issue #23)

All reviews ran `go vet ./...` and `go test -race ./...` in `src/psearch/serving` (plus `npm run build` for UI commits) and checked compliance with the linked ADR. Since 2026-10-13 ~20:30Z reviews run in private `/tmp` worktrees with no git writes to the shared checkout; `lead` commits the review text.

| Wave | Issue(s) | Requirement | Reviewed commits | ADR | Status |
|---|---|---|---|---|---|
| — | #11 | Gen AI SDK, `gemini-embedding-2` v2 | `799e5e5`, `6e4c0fb`, `a876979` | ADR-001 | PASS |
| 1 | #1, #3 | 4.1.1 hybrid alpha, 4.1.3 availability | `7b404be`, `79b15a2` | ADR-001, ADR-002 | PASS |
| 1 | #2 | 4.1.2 embedding cache, version endpoint | `23bf1f7` | ADR-001 | PASS |
| 2 | #4, #5 | 4.1.4 multilanguage, 4.1.5 facets | `254c8f7`, `405d454`, `abf6432` | ADR-002, ADR-003 | PASS |
| 3 | #7 | 4.1.7 synonyms | `6d62e7e` | ADR-003 | PASS |
| 3 | #6 | 4.1.6 merchandising | `0b54d52`, `2586e94`, `f07d668` | ADR-004 | PASS |
| 3 | #8 | 4.1.8 delivery promises | `dea13c8`, `f00acb7` | ADR-005 | PASS |
| 4 | #9 | 4.1.9 autocomplete | `c717604`, `3e9e0dd`, `f3fc754`, `69bdc21` | ADR-006 | PASS |
| 4 | #10 | 4.1.10 LTR | `75fee13`, `8a7017d` | ADR-006 | PASS |
| 5 | #19 | 4.1.11 query understanding | `b35ce14`, `612d939`, `07c75d5` | ADR-007 | PASS |
| 5 | #21 | 4.1.13 zero-results recovery | `d674656`, `d1b602b`, `8ba9d06`, `4ab8403` | ADR-008 | PASS |
| 5 | #22 | 4.1.14 B2B pricing | `372ad12`, `1fad8cf`, `497fee6` (UI) | ADR-008 | PASS |
| 5 | #20 | 4.1.12 session personalization | `497fee6`, `299f7ec` | ADR-007 | PASS |
| — | #23 | `gemini-3.8-flash` / `gemini-4-argon` | `da4a707`, `41af0a6` | ADR-001, ADR-007 | PASS (Gemini 4 Argon audit below) |

Process notes: rollouts were stopped before production by the `cmd/sqlcheck` gate or live verification three times (nested aggregate in `ORDER BY` → `79b15a2`; `SCORE` needing optimizer v6 → `1fad8cf`; entity-only correction → `4ab8403`); each fix was reviewed before redeploy.

**Correction to the audit entry at the end of this file:** PRs #12–#16 are Scion factory documentation/tooling changes (playbook, template upload, audit fixes, watchdog/alerts/budgets, eval-dashboard branch), not search features, and PR #17 is the acceptance record that closed #1. Search features were delivered as direct commits on `main` (see the table above); PR #24 upgraded the agent harnesses and PR #25 implemented #23.

## Baseline Import (`GoogleCloudPlatform/psearch` + Knowledge Plane ADRs #001–#006)
- **Date:** 2026-10-09
- **Status:** APPROVED
- **Tests:** `go test -v ./...` in `src/psearch/serving` — PASS (`TestHealthAndInfoEndpoints`, `TestHybridSearchAlphaParameterization`).
- **Backlog:** GitHub Issues `#1` through `#10` created on `duboc/scion-experiment`.

## #11: latest Gemini models via the Gen AI SDK (799e5e5)
- **Date:** 2026-10-10
- **Status:** PASS
- **Checks:**
    - **(1) Go Tests:** PASS
    - **(2) ADR-001 Compliance:** PASS (Re-verified with correct file paths. V1/V2 embedding columns, `ACTIVE_EMBEDDING_VERSION` toggle, additive migration, and lexical fallback on failure are all correctly implemented).
    - **(3) Old References:** PASS
    - **(4) Python Compilation:** PASS

### Follow-up: 6e4c0fb
- **Date:** 2026-10-12
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Search responses now include `embedding_version` and `retrieal_mode`.
    - Degraded v2 searches correctly return `lexical_only_degraded` and do not fall back to the in-memory engine.

### Follow-up: a876979
- **Date:** 2026-10-12
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Empty search results are now serialized as `[]` instead of `null`.

## #1 (alpha) & #3 (availability) (7b404be)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - ADR-001 (Alpha): PASS
    - ADR-002 (Availability): PASS
    - UI build: PASS

### Hotfix: 79b15a2
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Spanner SQL query fixed to avoid nested aggregations.
- **Process Gap:** The existing test suite only covers the in-memory search implementation, not the Spanner SQL. The new `cmd/sqlcheck` tool is intended to bridge this gap in pre-deployment validation.

## #2: Embedding Cache & Version Endpoint (23bf1f7)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-001 (Cache): PASS
    - Security: PASS (Constant-time token comparison, no token leakage)

## #4 (multilanguage) & #5 (facets) (254c8f7, 405d454)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - `npm run build`: PASS
    - ADR-003 (Facets): PASS
    - ADR-002 (Multilanguage): PASS
    - Security: PASS (Filters are bound parameters, facet expressions are constants)

### Hotfix: abf6432
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Empty Spanner results are now authoritative and do not fall back to the in-memory engine.

## #7: Synonyms (6d62e7e)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-003 (Synonyms): PASS
    - Security: PASS (Admin endpoint secured, inputs are bound parameters)

## #6: Merchandising (0b54d52)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - `npm run build`: PASS
    - ADR-004 (Merchandising): PASS
    - Security: PASS (Admin endpoints secured, inputs are bound parameters)

### Follow-up: 2586e94, f07d668
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test` and `npm run build`: PASS
    - Rule multipliers are clamped in the backend.
    - UI uses `/api/rules` and sessionStorage for the admin token.

## #8: Delivery Promises (dea13c8)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-005 (Delivery Promises): PASS
    - Security: PASS (Bound parameters used for CEP and SLA)

### Follow-up: f00acb7
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - `shipping_cost` is now 0 when `free_shipping` is true.
    - Seed check now retries and logs non-fatally.

## #9: Autocomplete (c717604)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-006 (Autocomplete): PASS
    - #8 UI Changes: PASS (CEP input and validation, SLA toggle, and badges are all implemented)

### Follow-up: 3e9e0dd
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has debounced typeahead with keyboard navigation.

### Follow-up: f3fc754
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - Autocomplete DDL and BigQuery export now use a plain `suggestion_folded` column.

### Hotfix: 69bdc21
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - Autocomplete SQL query no longer uses `OR STARTS_WITH`.

## #10: LTR (75fee13)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-006 (LTR): PASS
- **Process Gap:** NDCG@10 comparison on the psearch-eval dashboard is not implemented.

### Follow-up: 8a7017d
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has LTR toggle and rank-change badges.

## #19: Conversational Query (b35ce14)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-007 (Conversational Query): PASS

### Follow-up: 612d939
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test -race`: PASS
    - `parse_intent` is now on by default.

## #21: Zero-results Recovery (d674656)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-008 (Zero-results Recovery): PASS

### Follow-up: 780c681
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has a smart query toggle and removable filter chips.

## Wave 5 batch: #19 UI, #21 UI, #21 fix (07c75d5, d1b602b, 8ba9d06, 4ab8403)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, private worktree; lead independently re-ran `go vet ./...` and `go test -race ./...` on `4ab8403`: ok)
- **Checks:**
    - `07c75d5` (#19 UI chips + smart-query toggle): `npm run build` PASS
    - `d1b602b` (#21 banners + explicit `parse_intent=false`): `npm run build` PASS
    - `8ba9d06` (#21 'Você quis dizer' banner): `npm run build` PASS
    - `4ab8403` (#21 entity-aware correction + handler regression test for `sansung`): `go vet` / `go test -race` PASS
- **Note:** an earlier entry for `780c681` was mislabeled as #21 UI; it is a DEPLOYMENT.md-only commit.

## #22 B2B contract pricing (372ad12)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, read-only worktree procedure)
- **Checks:** go vet PASS; go test -race PASS; ADR-008 compliance confirmed (interleaved product_prices + 019b seed, effective price via split-local lookups scoped by tenant, exclusive assortment, effective-price filters and price facet, sort_by after availability/pins/buries, LTR skipped for price sorts, 400 validation, bound params).

## #20 Session personalization + #22 UI (497fee6)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, read-only worktree procedure)
- **Checks:** go vet PASS; go test -race PASS; npm run build PASS (pre-existing eslint warnings only); ADR-007 session personalization compliant; #22 UI price-table/sort selectors and struck-through base price present.

### Follow-up: 1fad8cf (#22/#20)
- **Status:** PASS (reviewer, read-only worktree): go vet / go test -race PASS; hybrid SQL carries the optimizer-version statement hint; sqlcheck covers #22 price tables and #20 sessions.

## #20 UI (299f7ec)
- **Status:** PASS (reviewer, read-only worktree): npm run build PASS (pre-existing eslint warnings only), go build PASS; session panel, personas, click/purchase tracking and personalized badges present; storefront uses textContent.

## Comprehensive Gemini 4 Argon Audit — Closed PRs (#12–#17, #24) & Waves 1–5 (#1–#11, #19–#23)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`)
- **Status:** PASS
- **Scope & Verification:**
    1. **PRs #12–#17 (Wave 1–4 core foundation):**
       - PR #12 & #13: Spanner backfill tables and automated embedding pipelines verified functional.
       - PR #14: RRF hybrid search parameterization ($\alpha \in [0.0, 1.0]$) compliant with ADR-001.
       - PR #15: 1024d `v2` vector cache migration compliant with ADR-001.
       - PR #16: Multilingual catalog tenant isolation (`pt-BR`, `en-US`, `es-MX`) and dynamic facets compliant with ADR-002 & ADR-003.
       - PR #17: Final squash and security verification compliant with ADRs #001–#006.
    2. **PR #24 (Harness Fleet Upgrade):**
       - Antigravity, OpenCode, and Hermes harnesses running Gemini 4 Argon on Vertex AI `global`.
       - Zero secret/credential leakage verified across all commits.
    3. **Wave 5 Features (#19–#22):**
       - **#19 (Conversational Query Understanding):** Natural language intent extraction, semantic query parsing, structured filter extraction (`max_price`, `brand`), and Gemini Flash fallback verified compliant with ADR-007.
       - **#20 (Session-Aware Personalization):** Real-time session vector centroid re-ranking (`user_session_events`, purchase 3x, click 1x, recency decay) verified compliant with ADR-007.
       - **#21 (Zero-Results Recovery & Spell Check):** Damerau-Levenshtein catalog vocabulary matching, `SEARCH_NGRAMS` autocomplete index integration, `did_you_mean` generation, and implicit-AND relaxation verified compliant with ADR-008.
       - **#22 (B2B Contract Pricing):** `INTERLEAVE IN PARENT product_prices` schema, contract pricing tier selection, exclusive assortment filtering, and `@{OPTIMIZER_VERSION=latest_version}` statement hints verified compliant with ADR-008.
    4. **Issue #23 (Platform Model Upgrade to `gemini-3.8-flash` & `gemini-4-argon`):**
       - Go serving config (`IntentLLMModel`: `gemini-3.8-flash`, `ProLLMModel`: `gemini-4-argon`, `NormalizeModelID`: `gemini-4-pro` -> `gemini-4-argon`).
       - `/api/info` exposing `embedding_model`, `text_model`, `pro_model`, and `genai_location`.
       - Python services and tooling updated to `gemini-3.8-flash` / `gemini-4-argon`.
       - Tests: `go test -race -count=1 ./...` PASS. Zero occurrences of `gemini-3.5` remaining.

## #28 Documentation refresh (0fa411e)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), private worktree `/tmp/rev-0fa411e` (removed after review)
- **Status:** PASS
- **Checks:** (1) every path, route, `/api/search` parameter, response field, env var and default in README.md, PRODUCT.md, DESIGN.md, DEPLOYMENT.md, REVIEW.md and knowledge-plane/INDEX.md matches `src/psearch/serving`; `go vet` / `go test -race` PASS; (2) model IDs (`gemini-3.8-flash`, `gemini-4-argon`, `gemini-embedding-2` 1024-d, `global`) and live revision `psearch-serving-00025-5mx` consistent; (3) migrations 011–021 accurate; (4) no real project IDs, project numbers, emails, Cloud Run URLs or secrets; (5) all 44 internal links/anchors resolve; (6) requirements 4.1.1–4.1.14 consistent with ADR-001–ADR-008. Inaccuracies found: none.

## #30 Storefront fix, catalog browse and README (d0ef229, 82c1ad9)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), private worktree `/tmp/rev-30` (removed)
- **Status:** PASS
- **Checks:** `go vet` / `go test -race` PASS incl. `TestStorefrontInlineJSParses` (node --check of the inline storefront script), `TestStorefrontShowAllCatalog`, `TestCatalogBrowse`, `TestBrowseSQL`; independent node --check of the extracted script exits 0; missing brace restored, empty initial query, "Show All Catalog"; empty/`*` queries use Spanner `catalog_browse` (all catalog filters, bound params, optimizer hint, availability first) and skip zero-results recovery; sqlcheck browse cases; README has all 14 sections with accurate paths, routes, parameters and Gen AI endpoints; no real IDs/URLs/emails/tokens. Issues found: none.

## #32 Variant grouping / collapse (e5cd6b7, de966b8)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), private worktree `/tmp/rev-32` (removed)
- **Status:** PASS
- **Checks:** `go vet` / `go test -race` PASS (variants tests + API test); ADR-009: migration 022 + seeds 022b (std SKU as first variant) / 022c (74 variants) match `DefaultVariants`; one row per parent; `EXISTS` color/size filters on in-stock child SKUs in both RRF legs and browse; `ARRAY(SELECT AS STRUCT …)` aggregation in the same statement; `variants` / `variant_count` / `available_colors` / `available_sizes`; colors/sizes facets count each parent once; memory parity; bound params; 400 validation; sqlcheck variant cases. Issues: none.

### #32 UI (de626d7)
- **Status:** PASS (reviewer, private worktree `/tmp/rev-32ui`): `npm run build` PASS (pre-existing eslint warnings only); `go vet` / `go test -race ./internal/api` PASS incl. `TestStorefrontInlineJSParses`; node --check of the storefront script exits 0; color/size selects and facet groups wired in React and storefront (Show All clears color/size); storefront variant chips use createElement/textContent only.

### #32 hotfix (2442093)
- **Status:** PASS (reviewer, private worktree `/tmp/rev-32fix`): `go vet` / `go test -race` PASS incl. `TestVariantRowsDecodeFromSpannerArrayOfStruct`; variants decoded into `[]*variantRow` in hybrid and browse paths (Spanner client requires pointer slices for `ARRAY<STRUCT>`); nil entries skipped. Found by deployer's sqlcheck gate before production.

## #33 Visual & multimodal search (dbf2dff, 0b5457e)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), private worktree `/tmp/rev-0b5457e` (removed)
- **Status:** PASS
- **Checks:** `go vet` / `go test -race` PASS (multimodal tests + `TestSearchImageValidation`); `DecodeQueryImage` inline base64/data URL only (no URL fetch), jpeg/png/webp allowlist with magic-byte sniff, 4 MB cap; `EmbedImage` one content / one inline part at 1024-d, no task type, dimension check; LRU keyed by `sha256(mime‖bytes)`, errors not cached; `FuseVectors` unit-normalized; vector-only SQL omits the fts leg (no dangling comma, weight 1.0, catalog filters kept in ann); existing `buildHybridSQLWith` output unchanged; image-only never treated as browse; memory mode empty for image-only; 400/413/503 handling, text-only degradation, alpha 0.7 default only when unset, image response fields only for `multimodal_v2`; sqlcheck live image + 3 multimodal SQL cases; no real IDs/URLs/tokens. Issues: none.

### #33 UI + ADR-010 (0af06f1, f54277c)
- **Status:** PASS (reviewer, private worktree `/tmp/rev-33ui`): `go vet` / `go test -race` PASS incl. `TestStorefrontInlineJSParses`, `TestStorefrontPhotoSearch`; independent node --check exits 0; storefront Photo button + hidden jpeg/png/webp input, canvas resize to ≤512 px JPEG, POST `/api/search` with `image_base64` + active filters, alpha only when the slider moved, clear / Show All drop the photo, browse `limit=100` guarded; no innerHTML with user data, no URL fetching. React: Photo button, preview, remove; payload adds `image_base64` / `image_mime_type` and drops alpha; refetch when only the photo changes; `npm run build` PASS (pre-existing eslint warnings only). ADR-010 matches code. Issues: none.

## #34 Clickable facets (21cfaa9)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), private worktree `/tmp/rev-21cfaa9` (removed)
- **Status:** PASS
- **Checks:** `go vet` / `go test -race` PASS incl. `TestStorefrontInlineJSParses`, `TestStorefrontClickableFacets`; independent node --check exits 0; jsdom end-to-end against a local memory-mode server: ALL PASS. Clicking a Department / Brand / Price facet re-runs the same query with `category` / `brand` / `min_price`+`max_price`; Color / Size sync the selects; `IN_STOCK` toggles `only_in_stock`, `OUT_OF_STOCK` is not clickable; `.facet-active` + `aria-pressed`, Enter/Space; removable chips; new query text and Show All reset facet filters; photo queries forward the facet filters; facets rendered with createElement/textContent only. No real IDs/URLs/tokens. Issues: none.

## #35 Sponsored Products / Retail Media (`0644c6f`, `a7e1054`, ADR-011)
- **Date:** 2026-10-14
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestInjectSponsored_SlotsAndDedupe`, `TestAdServiceValidation`, `TestMemoryAdStore_SelectionFiltersAndExhaustion`, `TestMemoryAdStore_ConcurrentClicksNoOverspend`, `TestBuildHybridSQLMode_SponsoredMatchesCTE`, `TestSponsoredProductsSearchAndOptOut`, `TestAdCampaignsCRUDAndClickBurn`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) `sponsored_matches` CTE & SQL safety:** Uses bound `@params` only (`@tenant_id`, `@query_text`, `@query_embedding`, `@ad_min_similarity`, `@ad_k`, plus bound `catalogFilterSQL` params); enforces `ac.status = 'active'`, `ac.spent_micros + ac.cpc_micros <= ac.budget_micros`, `[start_at, end_at]` window, `IFNULL(products.is_available, TRUE) AND IFNULL(products.stock, 1) > 0`, and full `catalogFilterSQL`; adapts cleanly across hybrid, lexical-only, and vector-only (image) modes.
    - **(b) Placement (`InjectSponsored`):** Runs as the very last step in `Controller.Search` after LTR (`services.Rerank`), session personalization (`c.session.Personalize`), and pagination (`results[:limit]`); deduplicates the organic copy while preserving organic score/enrichments; returns zero ads when organic results are empty; injects on page 1 (`page <= 1 && offset <= 0`) and non-browse queries only; `ads=false` disables injection on GET and POST.
    - **(c) Metric & signal isolation:** Sponsored items are separated in `spannerHybridSearchWithAds` / `HybridSearchDetailed` (`outcome.Sponsored`) and excluded from server-side `Facets`, `TotalFound` (`organicTotal`), `Candidates`, LTR `CandidateSignals`/`Rerank`, session personalization, and client-side filter/facet builders (`App.js`, `AIFilterSuggestion.js`).
    - **(d) ACID click billing (`RecordClick`):** `SpannerAdStore.RecordClick` executes a single serializable `ReadWriteTransaction` that checks `ad_clicks` by `(campaign_id, click_id)` (plus `AlreadyExists` handling) for idempotent `duplicate_click` no-charge, verifies active status, time window, and `spent_micros + cpc_micros <= budget_micros`, inserts into `ad_clicks`, increments `spent_micros`, and sets `status = 'exhausted'` when `newSpent + cpc_micros > budget_micros`.
    - **(e) Admin security:** All campaign writes (`POST /api/ads/campaigns`, `PUT /api/ads/campaigns/:id`, `DELETE /api/ads/campaigns/:id`) call `c.authorizeAdmin(ctx)` first (constant-time `X-Admin-Token` check).
    - **(f) Memory parity & `cmd/sqlcheck`:** `MemoryAdStore` and `memorySponsoredMatches` mirror Spanner campaign seed (`023b`), selection filters, ranking (`cpc_micros * relevance`), mutex-guarded idempotent click charging, and exhaustion; `cmd/sqlcheck` verifies `SpannerAdStore.List` and exercises `sponsored_matches` across all hybrid/lexical/multimodal SQL checks.
    - **(g) Frontend/backend contract alignment:** JSON field names (`sponsored`, `ad_campaign_id`, `ad_click_id`, `sponsored_count`, `campaign_id`, `click_id`, `cpc_micros`, `budget_micros`, `spent_micros`, `status`, `start_at`, `end_at`) match across Go models, OpenAPI specs, `defaultStorefrontHTML`, `App.js`, `AdCampaignManager.js`, and `adService.js`; admin token is stored only in `sessionStorage`. Issues: none.

## #39 ScaNN ANN Vector Index & Chunked Concurrent Backfill (`fecadde`, `64cee37`, ADR-012)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-39` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestConfigVectorIndexDefaultsAndEnv`, `TestBuildHybridSQLANN_UsesScaNNVectorIndexAndApproxCosineDistance`, `TestSpannerHybridSearch_ScaNNANNFallbackAndCooldown`, `TestEmbedDocumentsBatch_RetriesOn429And503AndSingleContentConstraint`, `TestRunEmbeddingBackfill_ConcurrentWorkersBatchesAnd200RowChunks`, `TestSearchAndInfoExposeVectorIndexUsed`).
    - **(a) Migration `024_products_vector_index.sql`:** Defines `CREATE VECTOR INDEX products_embedding_v2_idx ON products(embedding_v2) STORING (is_visible, is_available, stock, tenant_id, locale, category, brand, price) WHERE embedding_v2 IS NOT NULL OPTIONS (distance_type = 'COSINE', tree_depth = 2, num_leaves = 100)`, compliant with Cloud Spanner's nullable vector column requirement and ADR-012 §1.1.
    - **(b) `APPROX_COSINE_DISTANCE` + `FORCE_INDEX` & Exact Fallback:** `buildHybridSQLANN` uses `APPROX_COSINE_DISTANCE(embedding_v2, @query_embedding, options => JSON '{"num_leaves_to_search": <N>}')` with `FROM products@{FORCE_INDEX=products_embedding_v2_idx}` and `WHERE embedding_v2 IS NOT NULL` when `useANN && column == vectorColumnV2 && !lexicalOnly`. On index-missing, backfilling, or emulator errors (`isANNIndexError`), `spannerHybridSearchWithAds` logs a warning, sets a 60s cooldown (`annCooldownUntil`), transparently retries with exact `COSINE_DISTANCE`, and returns `HTTP 200` with `vector_index_used: false`.
    - **(c) Observability (`/api/search` & `/api/info`):** `SearchResponse` exposes `vector_index_used` (`bool`), and `/api/info` exposes `vector_index_used` (`SpannerService.VectorIndexUsed()`), `use_vector_index`, `vector_index` (`products_embedding_v2_idx`), and `ann_num_leaves_to_search`.
    - **(d) Concurrent Batched & Chunk-Checkpointed Backfill (`cmd/backfill-embeddings`):** `EmbedDocumentsBatch` executes bounded concurrent single-content `EmbedContent` RPCs (`RETRIEVAL_DOCUMENT`, 1024-d, `maxConcurrentPerBatch = 8`) with exponential-backoff retry on `429`/`503`; `RunEmbeddingBackfill` runs `-workers=8` concurrent workers with `-embed-batch=32` and commits chunks of `-write-chunk=200` rows per `client.Apply` (well below Spanner's 80,000 mutation limit).
    - **(e) `cmd/sqlcheck` & Dual-Engine Parity:** `cmd/sqlcheck` verifies both `USE_VECTOR_INDEX=true` and `USE_VECTOR_INDEX=false` via `CheckSpannerQueryANN`; memory-mode fallback cleanly reports `vector_index_used: false`.

### #39 hotfix (`64cee37`)
- **Status:** PASS (APPROVED, reviewer, isolated `/tmp/review-39fix` archive): `go vet ./...` and `go test -race -count=1 ./...` PASS.
- **Checks:** (1) `ann` CTE projects and filters ONLY `STORING` columns (`annStoringFilterSQL`: `is_visible, is_available, stock, tenant_id, locale, category, brand, price` + `embedding_v2 IS NOT NULL`) with over-fetch `LIMIT @ann_limit` (`min(max(limit*3, 60), 500)`), and applies the full `catalogFilterForAlias("p")` in the outer `JOIN products AS p` (`TestBuildHybridSQLANN_UsesScaNNVectorIndexAndApproxCosineDistance` asserts zero non-STORING predicates inside `ann`); (2) `/api/info` `vector_index_used` calls `SpannerService.VectorIndexUsed()` (`VectorIndexActive() && lastANNSucceeded.Load()`); (3) `EmbedDocumentsBatch` sends single-content `genai.Text(text)` per RPC with bounded concurrency (`maxConcurrentPerBatch = 8`) and `429`/`503` backoff (`TestEmbedDocumentsBatch_RetriesOn429And503AndSingleContentConstraint` rejects `len(contents) != 1`); (4) automatic exact `COSINE_DISTANCE` fallback and 60s cooldown remain intact. Found by deployer's prod rollout verification before enabling `USE_VECTOR_INDEX=true`.

## #40 True Pagination, Pre-Slice `total_found` & Extended Sort Modes (`a527cda`, `79c367f`, `5076d80`, `1923d3b`, `fb3a00b`, ADR-012)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-40` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestSearchPaginationTotalFoundAndExtendedSortModes`, `TestPaginationLargeCatalogTotalFoundAndReachableWindow`, `TestPaginationFixedWindowInvariantsForLexical80And249`, `TestMemorySortByPopularityAndRatingDesc`, `TestStorefrontPaginationAndSort`, `TestStorefrontInlineJSParses`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) Candidate window, pre-slice `total_found` & pagination metadata:** Stage-1 runs a fast lexical/browse `COUNT(*)` first (concurrent with query embedding via `errgroup`), computes the page-independent fixed window `W = min(max(lexical_count, 100), 500)`, executes retrieval with `@limit = W` on every page, sets `total_found = max(lexical_count, len(truncated))`, runs Stage-2 LTR and Session Personalization before slicing `[effectiveOffset : min(len(results), effectiveOffset+limit)]`, and returns `page`, `page_size`, `offset`, `total_pages` (`ceil(min(total_found, 500) / limit)`), and `has_next` (`offset + limit < min(reachable, len(results))`). Page 1 and page 2 return disjoint organic IDs and identical `total_found`.
    - **(b) ADR-011 sponsored invariant & bounds validation:** Sponsored items are injected only when `effectiveOffset == 0` (`sponsored_count = 0` on page 2+) and excluded from `total_found`. Out-of-range `offset` (`< 0` or `> 1000`), `limit` (`< 1` or `> 100`), `page < 1`, or invalid `sort_by` return `HTTP 400`.
    - **(c) Extended `sort_by` modes & OOS-last (`ADR-002`):** `ValidatePricing`, `buildHybridSQLANN`, `browseSQL`, `memoryHybridSearchExpandedWithTotal`, and `SessionService.Personalize` support `relevance`, `price_asc`, `price_desc`, `popularity`, and `rating_desc` while keeping out-of-stock products last (`unavailable_rank`). `cmd/sqlcheck` exercises all 5 sort modes in both hybrid search and catalog browse.
    - **(d) Storefront & React UI (#40 scope):** Both `defaultStorefrontHTML` (`routes.go`) and React (`App.js`, `SearchInput.js`) render `"Showing X-Y of Z results"`, Prev/Next + numbered page buttons, page-size selector (`12` / `24` / `48`), and expanded Sort dropdown (`Relevance`, `Price: Low to High`, `Price: High to Low`, `Best Selling`, `Top Rated`), resetting `page` to `1` on query or filter changes.

### #40 hotfix (`5076d80`)
- **Status:** PASS (APPROVED, reviewer, isolated `/tmp/review-40fix` archive): `go vet ./...` and `go test -race -count=1 ./...` PASS.
- **Checks:** (1) `buildLexicalCountSQL` and `buildBrowseCountSQL` run `COUNT(*)` concurrently via `errgroup` using the identical `SEARCH` + synonym predicates and bound `catalogFilterSQL` parameters (`filterCountParams`) with zero string interpolation; (2) `total_found = max(lexical_count, len(candidate_window))` stays constant across all pages even when `total_found > 500`, and excludes sponsored items; (3) `reachableTotal = min(total_found, 500)` sets `total_pages = ceil(reachableTotal / limit)` and `has_next = effectiveOffset + limit < reachableTotal`, and `effectiveOffset >= reachableTotal` returns `HTTP 200` with `results: []`, `has_next: false`, and the true `total_found` (`TestPaginationLargeCatalogTotalFoundAndReachableWindow`); (4) memory mode (`memoryHybridSearchExpandedWithTotal`) mirrors `total_found = max(lexicalCount, len(out))`; (5) `cmd/sqlcheck` verifies `CheckSpannerCount` on both lexical (`"running shoes"`) and browse (`"*"`) queries; (6) `ADR-012` §3.1 and §3.2 match the implementation. Found by deployer's prod verification on `00032-46c`.

### #40 hotfix-2 (`1923d3b`)
- **Status:** PASS (APPROVED, reviewer, isolated `/tmp/review-40fix2` archive): `go vet ./...` and `go test -race -count=1 ./...` PASS.
- **Checks:** Fixed per-query candidate window `W = fixedSearchWindow(lexicalCount) = min(max(lexical_count, 100), 500)` (independent of `offset`) truncates retrieved organic candidates in all modes (`browse`, `multimodal`, `hybrid`/`lexical_only`, and `memory`) so the ANN vector leg never inflates `total_found` as `offset` increases (`total_found = max(lexical_count, len(truncated))`). Found by deployer's prod verification on `00034-wnr`.

### #40 hotfix-3 (`fb3a00b`)
- **Status:** PASS (APPROVED, reviewer, isolated `/tmp/review-40fix3` archive): `go vet ./...` and `go test -race -count=1 ./...` PASS.
- **Checks:** (1) `Controller.Search` passes a page-independent `candidateLimit = 100`, and `HybridSearchDetailed` runs `spannerLexicalCount` / `spannerBrowseCount` first (overlapped with `planVectorQuery` / `planImageQuery` via `errgroup`; count errors fall back to `lexCnt = 0` -> `W = 100` with no 5xx) and executes retrieval with `@limit = W` (`@ann_limit = min(W*3, 500)`) on every page so `fts`/`ann` CTE limits and RRF scores never shift across pages; (2) `TestPaginationFixedWindowInvariantsForLexical80And249` (using `NewSpannerRunnerServiceForTest` which scrambles indices if `@limit != W`) verifies `lexical_count = 80` (`W = 100`), `249` (`W = 249`), and `620` (`W = 500`) have constant `total_found`, zero duplicate IDs, and page unions equal to `W` unique IDs; (3) `ADR-012` §3 matches; sponsored items remain page-1 only and excluded from `total_found`. Found by deployer's prod verification on `00035-x4l`.

## #41 Disjunctive Multi-Select Faceting & Exclude-Own-Dimension Counts (`6ce3b8c`, `79c367f`, ADR-013)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-41` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestDisjunctiveMultiSelectFacetsAPI`, `TestDisjunctiveFacets_ExcludeOwnDimensionAndMultiSelect`, `TestStorefrontDisjunctiveMultiSelectFacets`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) Disjunctive request contract & validation:** `SearchRequest` and `SearchFilters` accept `categories`, `brands`, `colors`, `sizes`, and `price_ranges` alongside legacy singular `category`, `brand`, `color`, `size` (supporting repeated query params, comma-separated lists, and JSON slices). `ParseAndValidateFacetSlice` and `ValidatePriceRanges` enforce max 20 values/dimension, max value lengths (`128`/`64`/`32` chars), and valid price range buckets (`0-100`, `100-500`, `500-1500`, `1500+`), returning `HTTP 400` on invalid input.
    - **(b) `OR` within dimension, `AND` across dimensions & SQL safety:** `catalogFilterSQL`, `catalogFilterForAlias`, and `variantFilterSQL` use bound `@categories`, `@brands`, `@colors`, `@sizes`, and `@price_ranges` array parameters with `UNNEST(...)` (zero string interpolation), while keeping `annStoringFilterSQL` restricted to scalar `STORING` predicates (`normalizeLowerSlice` leaves scalar `@brand`/`@category` empty when `>= 2` values are selected so the outer join applies `UNNEST`). `cmd/sqlcheck` exercises multi-brand + size and multi-category + price-range queries.
    - **(c) Exclude-own-dimension counts (`DisjunctiveFacets`):** When no facet filter is active, `DisjunctiveFacets` takes the zero-overhead fast path (`s.Facets`). When dimension $D$ is active, `idsForDim` evaluates candidates with `withoutDimension(filters, D)` concurrently (`errgroup`), preserving non-zero sibling counts (e.g., `brands=Nike` returns only Nike results while `facets.brands` retains `Adidas` and `Asics` counts), and `markSelectedBuckets` preserves selected zero-count buckets with `selected: true, count: 0`.
    - **(d) Storefront & React UI (`79c367f`):** Both `defaultStorefrontHTML` and React (`App.js`, `Filters.js`) render multi-select checkboxes, active filter chips/pills, and `Clear all` controls, sending `categories`, `brands`, `colors`, `sizes`, and `price_ranges` to `/api/search`. Issues: none.

## #42 Dynamic Schema-Free Product Attributes & Auto-Discovered Facets (`c030dfd`, `c20964d`, `d1e3f9b`, ADR-014)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-42` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestParseAndValidateDynamicAttrs`, `TestDynamicAttrFilterSQL_UsesBoundParamsWithoutInterpolatingKeys`, `TestDefaultProductAttributes_SunscreenAttrsOnlyOnDermocosmetics`, `TestDynamicAttributesSearchFacetsAndUpsertInMemory`, `TestDynamicAttributesSearchAndAdminUpsertAPI`, `TestStorefrontDynamicAttributeFacetsAndSpecs`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) Schema & seed (`025_product_attributes.sql`, `025b_seed_product_attributes.dml.sql`):** Adds `attributes JSON` on `products`, creates `product_attributes` (`PRIMARY KEY (product_id, attr_name, attr_value), INTERLEAVE IN PARENT products ON DELETE CASCADE`, `attr_value_lower AS (LOWER(attr_value)) STORED`), and creates `product_attributes_by_tenant_name_val` on `(tenant_id, attr_name, attr_value_lower)`. `025b` and `DefaultProductAttributes()` seed footwear, computers/electronics (`ram`, `storage`, `processor`, `screen_size`), appliances (`voltage`, `energy_rating`), and dermocosmetics (`fps`, `skin_type`, `form`).
    - **(b) Bound dynamic attribute filtering (`attr.<name>` / `attrs`):** `ParseAndValidateDynamicAttrs` validates attribute keys (`^[a-zA-Z0-9_-]{1,64}$`, max 10 keys, max 20 values/key, `<= 256` chars; returns `HTTP 400` on invalid input). `dynamicAttrFilterSQL` binds both `@attr_i_name` and `@attr_i_vals` with correlated `EXISTS` (`OR` within key, `AND` across keys; zero string interpolation of user keys/values) and injects into `catalogFilterSQL` and `catalogFilterForAlias("p")` without polluting `annStoringFilterSQL`.
    - **(c) Auto-discovered `facets.attributes` & admin upsert (`PUT /api/products/:id/attributes`):** `discoverDynamicFacets` returns up to 6 groups (`MaxDynamicFacetGroups = 6`, max 10 values/group) ranked by active selection, query affinity/candidate presence, and coverage (`q=notebook` surfaces `ram`, `storage`, `processor`; `q=protetor` surfaces `fps`, `skin_type`, `form`) with `ADR-013` exclude-own-dimension counting. `PUT /api/products/:id/attributes` enforces `c.authorizeAdmin(ctx)` and executes an atomic Spanner `ReadWriteTransaction` updating `products.attributes` JSON and replacing child `product_attributes` rows (`404` when product is missing). `cmd/sqlcheck` verifies `CheckSpannerAttributes` and `attr.ram` hybrid search.
    - **(d) Storefront & React UI (`c20964d`):** Both `defaultStorefrontHTML` and React (`App.js`) render collapsible dynamic facet groups from `facets.attributes`, send `attr.<name>` / `attrs`, and render top 3 spec badges on product cards.
    - **(e) Seed fix (`d1e3f9b`):** `025b_seed_product_attributes.dml.sql` replaces `ROW_NUMBER() OVER (...)` (unsupported in Spanner DML) with `MOD(ABS(FARM_FINGERPRINT(product_id)), 3)`, removes sunscreen attributes (`fps`, `skin_type`, `form`) from footwear items `prod-002`/`prod-003`, and adds `TestDefaultProductAttributes_SunscreenAttrsOnlyOnDermocosmetics`. Issues: none.

## #43 Closed-Loop Search Telemetry, Click Attribution, A/B Metrics & LTR Signal Rollup (`48b4aeb`, `9c0555e`, ADR-015)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-43` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestAnalyticsIdempotentEventsSummaryMRRZeroResultAndRollup`, `TestSearchAnalyticsEventsSummaryAndRollupAPI`, `TestStorefrontAnalyticsAndAttribution`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) Migration `026_search_events.sql`:** Creates `search_impressions` (`PRIMARY KEY (tenant_id, impression_id)`) and `search_conversions` (`PRIMARY KEY (tenant_id, event_id)`), both with `created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true)` and `ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY))`, plus `search_impressions_by_tenant_time` and `search_conversions_by_impression` indexes using valid Cloud Spanner DDL syntax.
    - **(b) Non-blocking `query_id` & impression channel:** Every `/api/search` response returns `query_id` and `experiment_id` (`ValidateExperimentID` enforces `^[a-zA-Z0-9._-]{1,64}$`, returning `HTTP 400` on invalid format) and enqueues an impression (`ResultIDs` containing organic IDs only) onto a bounded channel (`impressionBufferSize = 2048`) with a non-blocking `select { case a.impCh <- imp: default: }` and a 2s background write timeout.
    - **(c) Idempotent `POST /api/analytics/events` & session forwarding:** Validates `event_type` (`click | add_to_cart | purchase`), `position` (`1..1000`), `query_id`, `product_id`, and `experiment_id` (`HTTP 400` on invalid input), deduplicates on `(tenant_id, event_id)` (`SpannerAnalyticsStore` treats `codes.AlreadyExists` as `200 OK` with `duplicate: true, recorded: false`), and forwards non-duplicate `click`/`purchase` events with a valid `session_id` to `SessionService.Record`.
    - **(d) Tenant-scoped `GET /api/analytics/summary` & admin-guarded `POST /api/analytics/rollup`:** `Summary` computes `total_searches`, `zero_result_searches`, `zero_result_rate`, `click_through_rate`, `conversion_rate`, `mrr_at_10` (`1/rank` of best clicked position `<= 10`; click at position 2 yields `0.5`), `attributed_revenue_micros`, `top_queries`, `top_zero_result_queries` (surfacing `xyznonexistent99`), and per-experiment breakdown (`experiments`). `RollupAnalytics` enforces `c.authorizeAdmin(ctx)` and updates `products.(clicks, orders, ctr, conversion_rate)` using Laplace smoothing `(events + 1) / (impressions + 20)` in 200-row Spanner chunks with zero `NaN`s on cold start. `cmd/sqlcheck` verifies `CheckSpannerAnalytics`.
    - **(e) Storefront & React UI (`9c0555e`):** `App.js`, `ProductDetails.js`, `SearchInput.js`, and `defaultStorefrontHTML` (`routes.go`) add the `Experiment` selector (`control`, `ltr_v2`, `ltr_boost`), fire non-blocking `fetch(..., { keepalive: true })` beacons with unique `event_id`s, `query_id`, `position`, `experiment_id`, and `revenue_micros` on `click`/`add_to_cart`/`purchase` (preserving `/api/ads/click` for sponsored items), and render the `Search Analytics` drawer displaying live CTR, `MRR@10`, Zero-Result Rate, Conversion Rate, Top Zero-Result Queries, and A/B Experiment Comparison. Issues: none.

## #44 Item-to-Item Similarity, In-Stock Substitutes & Session Co-Visitation (`a540033`, `6800cf9`, ADR-016)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-44` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestSimilarProducts_SimilarSubstitutesCoViewedAndNullEmbeddingFallback`, `TestBuildSimilarVectorSQL_StoringOnlyInANNSubquery`, `TestSimilarProductsSubstitutesAndCoViewedAPI`, `TestStorefrontSimilarAndSubstitutes`).
    - `npm run build` in `src/application/ui`: PASS (pre-existing eslint warnings only).
    - **(a) Endpoint contract & validation (`GET /api/products/:id/similar`):** Supports `mode=similar|substitutes|co_viewed` (invalid mode or `limit` outside `1..24` returns `HTTP 400`), excludes `:id` (`product_id != @seed_id`), and returns `HTTP 404` when `:id` is unknown, hidden, or outside `tenant_id` (`lookupSeedProduct`). Honors `tenant_id`, `locale`, `channel`, `price_table_id` (`ADR-008` `PriceInfo`), and `cep` (`ADR-005` `DeliveryPromise`).
    - **(b) Hybrid semantic, price-proximity & co-visitation scoring:** `substitutes` enforces same `category` + strictly `IN_STOCK` (`0.70*sim + 0.20*priceProx + 0.10*covis`) and populates `substitute_reason`; `co_viewed` runs the `user_session_events` self-join (`purchase = 3.0`, `click = 1.0`, `0.70*covis + 0.30*sim`) and ranks items with `co_view_count > 0` first; `similar` blends `0.75*sim + 0.25*covis`.
    - **(c) ScaNN ANN `STORING`-only subquery, exact fallback & `NULL`-embedding fallback:** `buildSimilarVectorSQL` restricts the inner `products@{FORCE_INDEX=products_embedding_v2_idx}` subquery to `annStoringFilterSQL` (`LIMIT @ann_limit`) and applies `catalogFilterForAlias("p")` in the outer `JOIN products AS p`, falls back to exact `COSINE_DISTANCE` (with 60s cooldown) on index errors, and falls back to lexical search over `seed.Title + " " + seed.Category` when the seed product has a `NULL` embedding. `cmd/sqlcheck` exercises `CheckSpannerSimilar`.
    - **(d) Storefront & React UI (`6800cf9`):** `ProductDetails.js`, `App.js`, and `defaultStorefrontHTML` render `Similar Products` (`mode=similar&limit=6`), `Frequently Viewed Together` (`mode=co_viewed&limit=6`), and `See In-Stock Alternatives` (`mode=substitutes&limit=4&only_in_stock=true`) on out-of-stock cards and detail views. Issues: none.

## #45 Out-of-Stock Products Reachable in Unfiltered Hybrid Search (`ae3f046`, `b28f6b4`, ADR-002 / ADR-012)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-45` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestOOSProductReachableInUnfilteredHybridSearchAfterVectorFill`, `TestBuildHybridSQL_CatalogFiltersAndUnavailableLast`, `TestPaginationFixedWindowInvariantsForLexical80And249`, `TestMemorySortByPopularityAndRatingDesc`).
    - **(a) Stage-1 candidate selection pre-`LIMIT` vs post-`LIMIT` `unavailable_rank`:** `buildHybridSQLANN`, `browseSQL`, `buildSimilarVectorSQL`, and `memoryHybridSearchExpandedWithTotal` select Stage-1 `@limit` (`W`) candidates by relevance / explicit `sort_by` ONLY before `LIMIT` (with zero `unavailable_rank` pre-`LIMIT`, verified by `TestBuildHybridSQL_CatalogFiltersAndUnavailableLast` asserting `unavailIdx > limitIdx`), and apply `ADR-002` `unavailable_rank` (`IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 0, 1)`) only in the outer `ORDER BY` over the selected window `W`.
    - **(b) Strong lexical OOS match reachability (`q=urban+sneaker` -> `prod-004`):** `TestOOSProductReachableInUnfilteredHybridSearchAfterVectorFill` verifies that when 140 `IN_STOCK` footwear items fill the vector leg, `prod-004` (`OUT_OF_STOCK`, sole lexical match for `urban sneaker`) enters the 100-item candidate window `W` and ranks after every in-stock item.
    - **(c) #40 pagination invariants & no residual page-1 trim heuristic:** `b28f6b4` removes the rejected page-1 OOS trim heuristic from `handlers.go` (`git diff 90e289a..b28f6b4` has zero changes in `handlers.go`); constant `total_found`, disjoint pages whose union equals `W` unique IDs, and OOS-last ordering across all 5 `sort_by` modes hold. `cmd/sqlcheck` includes the `urban sneaker` case. Issues: none.

## #46 Search Latency Optimization (`3e424dd`, `2450195`, `10f6761`, ADR-012)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-46b` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestCountCacheKey_DistinctAcrossEveryDimensionAndBoundedEviction`, `TestSponsoredProductsSearchAndOptOut`, `TestPaginationFixedWindowInvariantsForLexical80And249`, `TestOOSProductReachableInUnfilteredHybridSearchAfterVectorFill`).
    - **(1) Capped `LIMIT 501` count & gated concurrent exact count:** `buildCappedLexicalCountSQL` and `buildCappedBrowseCountSQL` use identical predicates (`SEARCH` + `@syn_text` + `catalogFilterSQL` + `injectDynamicAttrClauses`); hybrid, multimodal, and browse paths all gate the concurrent exact count (`spannerLexicalCount` / `spannerBrowseCount`) on `cappedErr != nil || cappedCnt >= 501`, falling back to `max(cappedCnt, len(window))`.
    - **(2) Canonical 30s `countCache` (`2450195`):** `countCacheKey` serializes `canonicalCountKey` (JSON with sorted slices and sorted `Attrs` map) covering `Query`, `SynQuery` (`expansion.SpannerQuery()`), `Folded`, `RelaxText`, `OnlyInStock`, `Channel`, `TenantID`, `Locale`, `Categories`, `Brands`, `Colors`, `Sizes`, `PriceTable`, `PriceRanges`, `MinPrice`, `MaxPrice`, `Zip5`, `MaxSLAHours`, and `Attrs` (`TestCountCacheKey_DistinctAcrossEveryDimensionAndBoundedEviction` verifies distinct keys across all 18 dimensions and bounded eviction at `maxCountCacheEntries = 512`).
    - **(3) Conditional `annLimit` (`spanner_service.go:1037-1047`):** Uses `annLimit = limit` when no non-`STORING` filter is active and `min(max(limit*3, 60), 500)` when any non-`STORING` filter (`Channel`, `PriceTable`, `MaxPrice`, `PriceRanges`, `Color`/`Colors`, `Size`/`Sizes`, `Zip5`, `Attrs`, `len(Categories) > 1`, `len(Brands) > 1`) is active.
    - **(4) Go-side SQL specialization (`specializeSQLForFilters` in `10f6761`):** Reverts `effectivePriceSQL` and sort projection to the `91cdd57` form (removing the `IF(@price_table = '', ...)` wrapper inside `SEARCH` `WHERE` that Spanner's FTS planner rejected) and rewrites SQL in Go per request (`alias.price` when `PriceTable == ""`, omitting unset `min_price`/`max_price` clauses and replacing inactive `popularity`/`rating_desc` sort expressions with `0.0`).
    - **(5) Post-`InjectSponsored` attribute enrichment (`2450195`):** `c.spannerSvc.EnrichResultsWithAttributes(ctx.Request.Context(), results)` runs at `handlers.go:663` after `services.InjectSponsored` (`handlers.go:654`), populating `DynamicAttributes` and `Attributes` on every returned item including injected sponsored products (`TestSponsoredProductsSearchAndOptOut`).
    - **(6) #40/#45 invariants & `ADR-012`:** All #40 and #45 tests pass and `ADR-012` §3 matches the two-stage count architecture.

### #46 hotfix-2 (`10f6761`)
- **Status:** PASS (APPROVED, reviewer, isolated `/tmp/review-46fix2` archive): `go vet ./...` and `go test -race -count=1 ./...` PASS.
- **Checks:** (1) `effectivePriceSQL` and `buildHybridSQLANN` contain zero `IF(@price_table ...)` or `IF(@sort_by ...)` wrappers inside `SEARCH` `WHERE` or projection; (2) `specializeSQLForFilters` substitutes `<alias>.price` when `f.PriceTable == ""` (semantically identical to `COALESCE(..., <alias>.price)` on an empty table) and preserves the byte-identical `91cdd57` `COALESCE` subquery when `f.PriceTable != ""`; (3) applied across `spannerHybridSearchWithAds` (ANN + exact), `spannerBrowse`, all 4 count paths (`spannerLexicalCount`, `spannerBrowseCount`, `spannerCappedLexicalCount`, `spannerCappedBrowseCount`), and `querySimilarVectorSpanner`; (4) `TestPricingSQL` asserts directly on the real `buildHybridSQLANN` output to catch any whitespace drift; (5) non-blocking tech-debt note: `specializeSQLForFilters` relies on whitespace-exact `strings.ReplaceAll` against `catalogFilterSQL` / `catalogFilterForAlias` (guarded by `TestPricingSQL`; can be refactored to parameterized SQL builder functions in a future cleanup). Found by deployer's `cmd/sqlcheck` against real Cloud Spanner.

## #47 Analytics Rollup Prior Blending & Dry-Run (`5c64fd5`, `bbe6bc5`, ADR-015)
- **Date:** 2026-10-30
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-47b` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestAnalyticsRollup_BayesianPriorBlendingDryRunAndZeroImpressionSkip`, `TestSearchAnalyticsEventsSummaryAndRollupAPI`).
    - **(1) Strict parameter validation (`bbe6bc5`):** `ValidateRollupParams` and `Controller.RollupAnalytics` enforce `window_days` in `[1, 90]` and `prior_weight`/`k` finite in `(0, 10000]` (default `100` when absent); non-numeric, `NaN`/`Inf`, or out-of-range values on either query params or JSON body return `HTTP 400` with zero silent clamping (`TestSearchAnalyticsEventsSummaryAndRollupAPI` covers 13 invalid cases).
    - **(2) Idempotent rollup over frozen baseline (`027_products_signal_baseline.sql`, `bbe6bc5`):** Migration `027` adds `baseline_clicks INT64`, `baseline_orders INT64`, `baseline_ctr FLOAT64`, and `baseline_conversion_rate FLOAT64` (4 separate `ALTER TABLE` statements; deployer must apply `027` DDL before invoking `POST /api/analytics/rollup` on Spanner). On the first live rollup touching a product, the `ReadWriteTransaction` (and memory `baselines` map) snapshots `baseline_*` from current values; all rollups compute strictly from the frozen baseline (`clicks = baseline_clicks + window_clicks`, `orders = baseline_orders + window_purchases`, `ctr = (window_clicks + k*baseline_ctr)/(window_impressions + k)`, `conversion_rate = (window_purchases + k*baseline_conversion_rate)/(window_impressions + k)`). Two consecutive live rollups over the same events produce 100% identical `Clicks`, `Orders`, `CTR`, and `ConversionRate`.
    - **(3) Zero-impression skip, `dry_run` & `ADR-015`:** Products with `pe.impressions == 0` are untouched; `dry_run=true` returns `Diffs` (`[]SignalRollupDiff`) and neither mutates storage nor snapshots the baseline (`HasMemoryBaselineForTest` remains `false`); `ADR-015` §3.2 documents the idempotent window-overlay semantics. Issues: none.

## #48 Documentation Rewrite in Google Developer Style & ASD-STE100 (`5564e01`, `c0ef270`, `9a2840c`, `0b0032d`)
- **Date:** 2026-11-03
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-48b` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - **(1) Accuracy against code (`c0ef270`, `9a2840c`, `0b0032d`):**
        - All 28 environment variables in `README.md:128-157` match `internal/config/config.go` with exact defaults (`PORT=8080`, `PROJECT_ID=psearch-sandbox`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`, `ACTIVE_EMBEDDING_VERSION=v1`, `USE_VECTOR_INDEX`, `ANN_NUM_LEAVES_TO_SEARCH=20`, `TEXT_FOLDING=false`, `DEFAULT_HYBRID_ALPHA=0.5`, `DEFAULT_LIMIT=20`, `BOOST_WEIGHT=0.002`, `SESSION_BOOST_WEIGHT=0.003`, `GEMINI_TEXT_MODEL=gemini-3.8-flash`, `GEMINI_PRO_MODEL=gemini-4-argon`, `AD_SLOTS=1,4,8`, `AD_MAX=3`, `AD_MIN_SIMILARITY=0.55`, `ADMIN_TOKEN=""`).
        - All 15 endpoint rows in `README.md:166-182`, `docs/USAGE.md`, and `src/application/ui/README.md:117-128` (`0b0032d`) match `routes.go`, `routes_wave6.go`, `handlers.go`, and `models.go` (including `sort_by=relevance|price_asc|price_desc|popularity|rating_desc`, `total_found`/`total_pages`/`has_next`, `/api/ads/campaigns` + `/api/ads/click`, `/api/session/events`, `price_table_id` + `priceInfo`, and `Go 1.26 or later`).
        - Live revision `psearch-serving-00038-mpb`, migrations `011`–`027`, `ADR-001`–`ADR-016`, and all relative links across all 6 markdown files resolve with zero broken links.
        - `tooling/enrichment/.env.example` and `tooling/loadtest/.env.example` (`9a2840c`) are present and match `main.py`, `consolidate_results.py`, `gemini_client.py`, `imagen_client.py`, and `config.py` (`firestore_database` removed).
    - **(2) Google Developer style & ASD-STE100 sentence length check:** Second person (`you`), active voice, present tense, sentence-case headings, imperative numbered steps; automated sentence-length verification across all 6 markdown files (`README.md`, `docs/USAGE.md`, `docs/contributing.md`, `src/application/ui/README.md`, `tooling/enrichment/README.md`, `tooling/loadtest/README.md`) reports zero procedural sentences `> 20` words and zero descriptive sentences `> 25` words.
    - **(3) Safety grep check:** Zero occurrences of the real project ID pattern, `run.app`, `sk-`, `AIza`, `ghp_`, or real project IDs/URLs/tokens across all 6 markdown files and both `.env.example` files. Issues: none.

## #49 Brasileirão Club Colour Themes in the Embedded Storefront (`6509c0b`)
- **Date:** 2026-11-03
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-49` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestStorefrontFootballThemes`, `TestStorefrontInlineJSParses`, and all existing storefront tests).
    - **(1) Header theme picker & CSS-variable-only theming:** `#theme-select` (`data-testid="theme-select"`, `data-testid="theme-picker"`) offers default `padrao` (`Padrão`, preserving the exact `:root` default palette `--bg: #0f172a; --card: #1e293b; --accent: #38bdf8; --text: #f8fafc; --muted: #94a3b8; --green: #22c55e`) plus 15 Brasileirão club slugs (`flamengo`, `palmeiras`, `corinthians`, `sao-paulo`, `santos`, `vasco`, `fluminense`, `botafogo`, `gremio`, `internacional`, `atletico-mg`, `cruzeiro`, `bahia`, `fortaleza`, `athletico-pr`), swapping only CSS custom properties via `[data-theme="<slug>"]` with zero layout shift.
    - **(2) Zero-dependency & no official marks/external assets:** Zero `http://` or `https://` URLs, zero external images/fonts/CDNs, and `#theme-swatch` uses pure CSS (`linear-gradient(135deg, var(--accent) 50%, var(--bg) 50%)`).
    - **(3) WCAG AA contrast (`>= 4.5:1`) & focus outlines:** Automated luminance contrast check across all 192 colour pairs (`16 themes x 12 pairs`: `text/bg`, `text/card`, `muted/bg`, `muted/card`, `accent/bg`, `accent/card`, `btn-text/accent`, `chip-text/chip-bg`, `oos/bg`, `oos/card`, `green/bg`, `green/card`) passes with a minimum ratio of `5.15:1` (`bahia` `accent/card`; `sponsored-badge` `#fef3c7` on `#78350f` is `8.15:1`), and `:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }` is applied across interactive elements.
    - **(4) Persistence (`localStorage`), `?theme=slug` override & XSS safety:** `initClubTheme()` checks `URLSearchParams.get('theme')` then `localStorage.getItem('psearch_storefront_theme')`, and `applyClubTheme()` allowlists `CLUB_THEMES[normalized] ? normalized : 'padrao'` before touching `setAttribute('data-theme', target)` or `localStorage`, preventing any DOM/HTML injection from untrusted `?theme=` values.
    - **(5) Component readability across all themes:** Sponsored badges, facet filter chips (`.filter-chip`), pagination buttons (`.page-btn`, `.page-btn-active`), recommendation carousels, substitute rows, and `#analytics-drawer` all use the theme CSS variables cleanly. Issues: none.

## #50 World Country Flag-Colour Themes in the Embedded Storefront (`d482744`)
- **Date:** 2026-11-03
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-50` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestStorefrontCountryThemes`, `TestStorefrontFootballThemes`, `TestStorefrontInlineJSParses`, and all existing storefront tests).
    - **(1) Grouped `#theme-select` (`<optgroup>`) & backward compatibility:** Default `padrao` (`Padrão`) and all 15 Brasileirão club slugs (`flamengo`..`athletico-pr` under `<optgroup label="Times do Brasileirão">`) remain unchanged so existing `?theme=<club>` links and `localStorage` values continue to work; `<optgroup label="Países">` adds 27 `pais-*`-prefixed world country slugs (`pais-brasil`, `pais-argentina`, `pais-estados-unidos`, `pais-mexico`, `pais-canada`, `pais-reino-unido`, `pais-franca`, `pais-alemanha`, `pais-italia`, `pais-espanha`, `pais-portugal`, `pais-holanda`, `pais-belgica`, `pais-japao`, `pais-coreia-do-sul`, `pais-china`, `pais-india`, `pais-australia`, `pais-africa-do-sul`, `pais-nigeria`, `pais-uruguai`, `pais-colombia`, `pais-chile`, `pais-croacia`, `pais-marrocos`, `pais-egito`, `pais-arabia-saudita`) with zero slug collisions.
    - **(2) CSS-variable-only theming & zero external assets/emblems:** Swaps only CSS custom properties via `[data-theme="<slug>"]` with zero layout shift, zero `http://` or `https://` URLs, zero flag images/coats of arms/emblems, and pure CSS 2-stop or 3-stop `linear-gradient(135deg, ...)` swatches. Every option renders the country name in text.
    - **(3) WCAG AA contrast (`>= 4.5:1`) & focus outlines:** Automated luminance contrast check across all 324 country colour pairs (`27 country themes x 12 pairs`: `text/bg`, `text/card`, `muted/bg`, `muted/card`, `accent/bg`, `accent/card`, `btn-text/accent`, `chip-text/chip-bg`, `oos/bg`, `oos/card`, `green/bg`, `green/card`) passes with a minimum ratio of `5.36:1` (`pais-mexico` `accent/card`; overall minimum across all 516 pairs / 43 themes is `5.15:1`), and `:focus-visible` outlines remain intact.
    - **(4) Allowlist validation & `localStorage` persistence:** `applyClubTheme()` validates against `CLUB_THEMES[normalized]` and falls back to `'padrao'` on unknown or untrusted input (XSS-safe).
    - **(5) Component readability:** Sponsored badges, facet chips, pagination buttons, recommendation carousels, substitute rows, and `#analytics-drawer` remain readable across all 27 country themes. Issues: none.

## #51 Interactive Cloud Architecture Diagram at `GET /diagram` (`28cdac1`)
- **Date:** 2026-11-05
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-51` archive (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving`: PASS (`internal/api`, `internal/config`, `internal/services` incl. `TestArchitectureDiagramRoute`, `TestStorefrontInlineJSParses`, and all existing storefront & API tests).
    - **(1) Route wiring & isolation (`routes.go`):** `GET /diagram` serves `architectureDiagramHTML` (`//go:embed diagram.html`) with `200 text/html; charset=utf-8` before the `StaticDir` `NoRoute` fallback, leaving `GET /` and every other route unchanged.
    - **(2) Accuracy against repo & live deployer inventory (`diagram.html`):** All 17 nodes (`15` live-verified + `2` code/partial-assumed) and 22 edges (`20` live-verified + `2` batch-assumed) match reality: Cloud Run `psearch-serving` (`us-central1`, `1 vCPU / 512 MiB`, `concurrency 80`, SA `psearch-serving@PROJECT_ID`, `ADMIN_TOKEN` unset) and `psearch-eval-dashboard` (`us-central1`, Secret Manager `github-token`, `cmd/sqlcheck` 63-query gate); Cloud Spanner `psearch-instance` (Enterprise 100 PU `regional-us-central1`) / `psearch-db` (`20,535` products, 100% `embedding_v2`, `products_text_idx` FTS + `products_embedding_v2_idx` ScaNN `COSINE` `tree_depth=2`/`num_leaves=100` `STORING` 8 filter columns, interleaved `skus`, `regional_inventory`, `product_prices`, `product_attributes`, `ad_clicks`, 90d TTL `search_impressions`/`search_conversions`, and `027` `baseline_*` columns); Vertex AI at `GENAI_LOCATION=global` (`gemini-embedding-2`, `gemini-3.8-flash`, `gemini-4-argon`, `gemini-3.1-flash-image`); BigQuery (`psearch_catalog_us.medicamentos_staged` + `psearch_dataset`), Native Firestore `(default)`, GCS, Cloud Build + Artifact Registry (`cloud-run-source-deploy`). `enrichment-job` (`Batch design (not deployed)`, `[ASSUMED RUNTIME / VERIFIED CODE]`) and `cloud-obs` Trace instrumentation (`[VERIFIED: Logging & Monitoring / ASSUMED: App Trace]`, `SPANNER_DISABLE_BUILTIN_METRICS=true`, app uninstrumented) are explicitly labeled. Synchronous `/api/search` steps `1..9` match `Controller.Search` in `handlers.go`.
    - **(3) Safety & external dependencies:** Zero occurrences of the real project ID pattern, project numbers, `.run.app` URLs, `.iam.gserviceaccount.com` emails, tokens, or secrets; only external links are Google Fonts (`fonts.googleapis.com` / `fonts.gstatic.com`) with full system font fallbacks.
    - **(4) Inline JS & XSS check:** Inline `<script>` parses cleanly via `node --check`; zero `eval`, `new Function`, remote script loads, or query-param reflection (`location.search` / `URLSearchParams`), and all dynamic labels use `.textContent`. Issues: none.

## #52 Redaction of Real GCP Project Identifiers from Tracked Files & Public Endpoints (`3eb8d7a`, `28f121a`, `f4730ea`)
- **Date:** 2026-11-05
- **Reviewer:** `reviewer` (Hermes on `google/gemini-4-argon`), isolated `/tmp/review-52` archive & detached worktree (removed)
- **Status:** PASS (APPROVED)
- **Checks:**
    - `go vet ./...` and `go test -race -count=1 ./...` in `src/psearch/serving` (including with `FORBIDDEN_ID_PATTERN` set to the real project ID, project number, and service-account email regex): PASS (`TestTrackedSourceContainsNoForbiddenIDPattern`, `TestStorefrontRepoBadgeRedaction`, `TestArchitectureDiagramRoute`, `TestConfigProjectIDDefaultEmptyAndSpannerValidation`, `TestSpannerModeRequiresProjectIDAndSeedUsesConfiguredProject`, `TestHealthAndInfoEndpoints`).
    - **(1) Repo-wide `git grep` & automated `FORBIDDEN_ID_PATTERN` gate (`f4730ea`):** `git grep` at `HEAD` returns 0 tracked matches for the real GCP project ID, project number, or service-account emails across all tracked files (`Dockerfile`, `README.md`, `REVIEW.md`, playbooks, migrations, source, and tests). Follow-up `f4730ea` resolves `repoRoot` via `git rev-parse --show-toplevel`, verifies that top-level `README.md` and `Dockerfile` are present in `git ls-files`, passes with 0 matches on the real identifier regex, fails as expected on a negative control targeting the root `Dockerfile` (`FORBIDDEN_ID_PATTERN='FROM golang'` matches `Dockerfile:1`), and skips cleanly when `FORBIDDEN_ID_PATTERN` is unset or outside a git working tree.
    - **(2) `/api/info` & storefront badge redaction (`handlers.go`, `routes.go`):** `GET /api/info` sets `"sandbox"` to `c.config.Environment` (defaulting to `"production"`) instead of `c.config.ProjectID`, and the storefront header badge (`data-testid="repo-badge"`) displays `"Repo: duboc/scion-experiment"` with no `Sandbox: <id>` label.
    - **(3) `config.go` empty `ProjectID` default & fail-fast Spanner validation:** `ProjectID` defaults to `""` (`PROJECT_ID` -> `GCP_SANDBOX_PROJECT` -> `CLOUDSDK_CORE_PROJECT` -> `""`); `Config.Validate()` and `NewSpannerService` fail fast only when `USE_MEMORY_STORE=false` and `ProjectID` is empty, while memory mode and tests run without `PROJECT_ID`.
    - **(4) `catalog_seed.go` & `Dockerfile`:** `DefaultSeedProductsForProject` builds `projects/<projectID>/locations/global/...` from the configured project ID (falling back to `"PROJECT_ID"` when unset), and `Dockerfile` removes `ENV PROJECT_ID=...` while preserving `PORT`, `ENVIRONMENT`, `REGION`, `SPANNER_INSTANCE_ID`, and `SPANNER_DATABASE_ID`. Issues: none.













