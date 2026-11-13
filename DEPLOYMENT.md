# PSearch — Deployment Guide & Release Log

**Live:** Cloud Run service `psearch-serving`, revision **`psearch-serving-00044-8r5`** (100% traffic, follows latest): #66 Phase A per-search deadline (10s) and in-flight admission control (8 per instance, 250ms queue wait), deployed from a clean `git archive` of `aa90182` (code `598fc1f`). Rollback target: `psearch-serving-00043-p87`.

Real project IDs, service-account emails and service URLs are intentionally replaced with placeholders in this document:

| Placeholder | Meaning |
|---|---|
| `<PROJECT_ID>` / `<PROJECT_NUMBER>` | GCP sandbox project |
| `<SERVICE_URL>` | `psearch-serving` Cloud Run URL |
| `<EVAL_DASHBOARD_URL>` | `psearch-eval-dashboard` Cloud Run URL |
| `<RUNTIME_SA>` | dedicated runtime service account `psearch-serving@<PROJECT_ID>.iam.gserviceaccount.com` (roles: `roles/aiplatform.user` on the project, `roles/spanner.databaseUser` on `psearch-db`) |
| `<DEPLOYER_SA>` | deployer agent service account (holds `roles/iam.serviceAccountUser` on `<RUNTIME_SA>`) |
| `<COMPUTE_DEFAULT_SA>` | default compute service account (used only before Issue #11 moved to `<RUNTIME_SA>`) |

## 1. Environment

| Item | Value |
|---|---|
| Region | `us-central1` (Cloud Run); Vertex AI calls use location `global` |
| Cloud Spanner | instance `psearch-instance` (Enterprise, 100 PU), database `psearch-db` |
| BigQuery | dataset `psearch_dataset` (no tables yet; query logging is a follow-up) |
| Container | root `Dockerfile` (multi-stage `golang:1.26-alpine` → `alpine:3.20`, binary `/app/psearch-server`, port 8080) |

## 2. Runtime configuration (live revision)

| Variable | Value | Notes |
|---|---|---|
| `PROJECT_ID` / `GCP_SANDBOX_PROJECT` / `CLOUDSDK_CORE_PROJECT` | `<PROJECT_ID>` | `ProjectID` falls back through these (Issue #23, `41af0a6`) |
| `SPANNER_INSTANCE_ID` / `SPANNER_DATABASE_ID` | `psearch-instance` / `psearch-db` | |
| `ACTIVE_EMBEDDING_VERSION` | `v2` | 1024-d `embedding_v2` |
| `GENAI_LOCATION` | `global` | |
| `EMBEDDING_MODEL_NAME` / `EMBEDDING_DIMENSION` | `gemini-embedding-2` / `1024` | |
| `TEXT_FOLDING` | `true` | accent-folded tokenlists (migration 014b) |
| `SPANNER_DISABLE_BUILTIN_METRICS` | `true` | silences client-metrics export (runtime SA has no monitoring role) |
| `GEMINI_TEXT_MODEL` / `GEMINI_PRO_MODEL` | defaults `gemini-3.8-flash` / `gemini-4-argon` | reported by `/api/info` as `text_model` / `pro_model` |
| `INTENT_LLM_FALLBACK` | unset (`false`) | parser-only query understanding |
| `ADMIN_TOKEN` | unset | admin writes (rules, synonyms, embedding-version switch) return 403 |
| `BOOST_WEIGHT`, `SESSION_BOOST_WEIGHT`, `LTR_MODEL_JSON` | defaults (`0.002`, `0.003`, baseline model) | |

Service account: `<RUNTIME_SA>`. Full variable reference: [DESIGN.md §16](DESIGN.md#16-configuration-environment).

## 3. Spanner schema migrations

The base schema (`products` with 768-d `embedding`, `title_tokens`, `description_tokens`, search index `products_text_idx`, plus `eval_runs` and `factory_agent_status`) predates the numbered migrations and is defined in `src/iac/modules/spanner`. Numbered migrations live in `src/iac/migrations/` and are applied online with `gcloud spanner databases ddl update psearch-db --instance=psearch-instance --ddl-file=<file>` (DDL) or `gcloud spanner databases execute-sql` (DML, comment lines stripped). See [knowledge-plane/playbooks/SPANNER-DDL-MIGRATION-GUIDE.md](knowledge-plane/playbooks/SPANNER-DDL-MIGRATION-GUIDE.md).

| Migration | Kind | Issue | Change |
|---|---|---|---|
| `011_add_embedding_v2_gemini_1024.sql` | DDL | #11 / #2 | `products.embedding_v2 ARRAY<FLOAT32>(vector_length=>1024)` (backfilled by `cmd/backfill-embeddings`) |
| `012_availability_visibility_channels.sql` | DDL | #3 | `is_visible`, `channels` |
| `012b_backfill_visibility_channels.dml.sql` | DML | #3 | visibility + channels for the seed catalog |
| `013_embedding_cache.sql` | DDL | #2 | `embedding_cache` |
| `014_tenant_locale.sql` | DDL | #4 | `tenant_id`, `locale` |
| `014b_accent_folded_tokens.sql` | DDL | #4 | `title_tokens_folded`, `description_tokens_folded` + `ALTER SEARCH INDEX products_text_idx` (applied in two steps) |
| `014c_backfill_tenant_locale.dml.sql` | DML | #4 | tenant/locale from `product_data` |
| `015_search_synonyms.sql` / `015b_seed_synonyms.dml.sql` | DDL / DML | #7 | `search_synonyms` + 8 seed rows |
| `016_merchandising.sql` / `016b_backfill_boost_signals.dml.sql` | DDL / DML | #6 | 8 signal columns + `merchandising_rules`; signal backfill |
| `017_delivery_interleaved.sql` | DDL | #8 | `skus` (interleaved in `products`), `regional_inventory` (interleaved in `skus`) |
| `017b_seed_skus.dml.sql` / `017c_seed_regional_inventory.dml.sql` | DML | #8 | 15 SKUs, 57 inventory rows (5 CD regions) |
| `018_autocomplete.sql` / `018b_seed_autocomplete.dml.sql` | DDL / DML | #9 | `autocomplete_suggestions` + `autocomplete_ngram_idx`; 48 bootstrap suggestions |
| `019_b2b_product_prices.sql` / `019b_seed_product_prices.dml.sql` | DDL / DML | #22 | `product_prices` (interleaved in `products`); 45 contract prices |
| `020_query_intent_cache.sql` | DDL | #19 | `query_intent_cache` |
| `021_user_session_events.sql` | DDL | #20 | `user_session_events` |
| `023_ad_campaigns.sql` / `023b_seed_ad_campaigns.dml.sql` | DDL / DML | #35 | `ad_campaigns` + `ad_campaigns_by_tenant_product` (STORING) + `ad_clicks` (interleaved in `ad_campaigns`, ON DELETE CASCADE), applied as 3 separate statements; 5 seed campaigns (tenant `default`) |
| `024_products_vector_index.sql` | DDL | #39 | `CREATE VECTOR INDEX products_embedding_v2_idx ON products(embedding_v2) STORING (is_visible, is_available, stock, tenant_id, locale, category, brand, price) WHERE embedding_v2 IS NOT NULL`, COSINE, `tree_depth=2`, `num_leaves=100`. Online LRO `_auto_op_8e2272d28739ab64` took about 2.5 min. **Rebuilt** 2026-10-30 after the full backfill (DROP + same DDL, LRO `_auto_op_812837e126700655`, 3 min) |
| `025_product_attributes.sql` / `025b_seed_product_attributes.dml.sql` | DDL / DML | #42 | `products.attributes JSON`; `product_attributes` (interleaved in `products`, ON DELETE CASCADE, generated `attr_value_lower`) + `product_attributes_by_tenant_name_val`, applied as 3 statements. Seed from **`d1e3f9b`** (the `6b0aee9` version used `ROW_NUMBER()`, which Spanner rejects): 307 rows, 100 products (15 `prod-*` + 85 sunscreens with fps/skin_type/form) |
| `026_search_events.sql` | DDL | #43 | `search_impressions` (PK `tenant_id, impression_id`) + `search_impressions_by_tenant_time` (STORING); `search_conversions` (PK `tenant_id, event_id`) + `search_conversions_by_impression` (STORING); both tables `ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY))`. Applied as 4 statements |
| `027_products_signal_baseline.sql` | DDL | #47 | `products.baseline_clicks INT64`, `baseline_orders INT64`, `baseline_ctr FLOAT64`, `baseline_conversion_rate FLOAT64` (nullable; snapshotted lazily by the first live rollup; reads `COALESCE` to live values). Applied as 4 statements before the deploy |

## 4. Release procedure

1. Code reviewed (PASS in [REVIEW.md](REVIEW.md)), reviewed in a private worktree — never in the shared checkout.
2. Stage a clean snapshot: `git archive <sha> | tar -x -C /tmp/deploy-<sha>` (never deploy from `/workspace`).
3. Apply additive DDL/DML first (split multi-statement DDL so failures are attributable).
4. **Standing gate (lead, 2026-10-30):** `go run ./cmd/sqlcheck` from the archive with the production env against **real Spanner**. Every case must pass before any deploy (63 checks at `00038-mpb`). Unit tests run in memory mode and cannot catch Spanner planner rejections.
5. `gcloud run deploy psearch-serving --source /tmp/deploy-<sha> --region us-central1 --service-account <RUNTIME_SA> …`, then `gcloud run services update-traffic psearch-serving --to-latest` (`deploy` does not accept `--to-latest`; after any `--to-revisions` rollback the next rollout must restore `--to-latest`).
6. Verify live endpoints and logs; on failure `update-traffic --to-revisions=<previous>=100`.
7. Record the result below and post evidence on the GitHub issue.

## 5. Revision history

| Revision | Source | Issue(s) | Outcome |
|---|---|---|---|
| `00001-dj8` | baseline | — | initial psearch serving (v1, compute SA) |
| `00002-gq9` | `799e5e5` | #11 | Gen AI SDK + v1 config, live |
| `00003-p7p` | `799e5e5` | #11 | v2 flip failed (IAM 403) → rolled back |
| `00004-d2t` | `799e5e5` | #11 | v1 rollback |
| `00005-vq7` | `1e96a34` | #11 | `retrieval_mode` field |
| `00006-caw` | probe | #11 | zero-traffic v2 IAM probe (403) |
| `00007-sr9` | template reset | #11 | zero-traffic, template back to v1 |
| `00008-ms8` | `b4229a0` | #11 | v1 on dedicated runtime SA |
| `00009-9dc` | `b4229a0` | #11 | **v2 live** on runtime SA |
| `00010-w7g` | `8894c6d` | #1, #3 | Spanner nested-aggregate error → rolled back |
| `00011-p6t` | `79b15a2` | #1, #3 | live |
| `00012-65q` | `23bf1f7` | #2 | embedding cache + version endpoint |
| `00013-5vm` | `14add09` | #4, #5 | tenant/locale, accent folding, facets |
| `00014-pc5` | `abf6432` | #4 | empty Spanner results authoritative |
| `00015-dzf` | `6d62e7e` | #7 | synonyms |
| `00016-jtj` | `0b54d52` | #6 | merchandising rules + boost signals |
| `00017-9wc` | `cbc384c` | #8 | delivery promises |
| `00018-rc5` | `69bdc21` | #9, #10 | autocomplete + LTR |
| `00019-dsr` | `5d5c136` | #10 | LTR UI |
| `00020-552` | `7a958ef` | #19 | query understanding (opt-in) |
| `00021-x6b` | `612d939` | #21, #19 | "sansung" correction not applied → rolled back |
| `00022-nll` | `4ab8403` | #21, #19 | spell recovery (entity fix), intent default-on, UIs |
| `00023-z7g` | `1fad8cf` | #22, #20 | B2B pricing (optimizer hint), session personalization |
| `00024-ltv` | `299f7ec` | #20 | session UI |
| `00025-5mx` | `41af0a6` | #23 | `gemini-3.8-flash` / `gemini-4-argon`, project env fallbacks |
| `00026-7qm` | `2915faf` | #30 | storefront script fix (unclosed autocomplete block), catalog browse (`q=` / `q=*`), "Show All Catalog" |
| `00027-dmv` | `8367cb0` | #32 | variant grouping (color/size over interleaved SKUs, variant facets) + `[]*variantRow` decode fix + UI selects |
| `00028-klr` | `2b9a75e` | #33 | visual & multimodal search (image → `embedding_v2`, text-fused, image cache) + photo-search UI |
| `00029-7ss` | `4e8951b` | #34 | clickable facets (storefront only: facet clicks re-run the search, active-filter chips) |
| **`00030-pqq`** | `5e6bf63` | #35 | sponsored products (ADR-011): `sponsored_matches` CTE, fixed ad slots, idempotent CPC click billing with real-time budget burn, campaign CRUD, Sponsored badge + campaign manager UI |
| **`00031-n6v`** | `350f63b` | #39 | ScaNN ANN code path (ADR-012) + chunked concurrent backfill CLI. Index built, but the ANN query is rejected, so every request uses the exact fallback (`vector_index_used=false`) |
| **`00032-46c`** | `2b3d49b` | #40 (+ #41 UI) | pagination (`page`/`offset`/`limit`, `has_next`, `total_pages`), `popularity`/`rating_desc` sorts, pager UI. ⚠️ `total_found` equals the candidate window, so it is wrong past page 8 (see #40 entry) |
| **`00033-4n8`** | `f32ecf3` | #39 fix | ANN CTE limited to STORING-column predicates with over-fetch; `vector_index_used` reflects real query execution; single-content embedding calls. **ANN live** |
| **`00034-wnr`** | `952cf20` | #40 fix, #41 (+ #42 UI) | true lexical/browse COUNT for `total_found`, reachable window ≤ 500; disjunctive multi-select facets (ADR-013). ✅ #41. ⚠️ #40 only partly fixed |
| **`00035-x4l`** | `6b0aee9` | #42, #44, #40 fix-2 | schema-free product attributes + auto-discovered facets + admin upsert (ADR-014); similar / substitutes / co-viewed (ADR-016); fixed per-query window W. ✅ #42, #44. ⚠️ #40: pages overlap when W > 100 |
| `00036-nf2` | `90e289a` | #43, #40 fix-3 | ❌ **never served**: container failed to start (`Application exec likely failed`, no app output; build normal; same code starts locally in 4 s). Cloud Run kept 00035 at 100% |
| **`00037-g74`** | `739c62e` | #43, #40 fix-3, #45 | search telemetry (`query_id`, impressions/conversions, A/B summary, rollup) (ADR-015); page-independent candidate set W; relevance-ranked window with OOS-last applied after it. ✅ #40, #43, #45 |
| **`00038-mpb`** | `8b28300` | #46, #47 | capped critical-path count + concurrent exact count, Go-side SQL specialization (no `IF` inside the SEARCH `WHERE`), scoped enrichment, ANN over-fetch only when needed; rollup over a frozen baseline (027) with strict params / dry-run. **p50 400/185/152 ms.** ✅ #46, #47 |
| **`00039-x8j`** | `ff5f8eb` | #49 (+ #48 docs) | storefront theme picker (Padrão + 15 Brasileirão clubs), `?theme=` param, localStorage persistence; serving code otherwise identical to 00038 |
| **`00040-r2j`** | `3de4d36` | #50 | theme picker gains a "Países" optgroup (27 `pais-*` flag-colour themes) next to "Times do Brasileirão" (15); storefront only |
| **`00041-x6r`** | `8a25c90` | #51 | embedded `diagram.html` + `GET /diagram` route (8-view stepped Google Cloud architecture diagram); API unchanged |
| **`00042-mbh`** | `4f6d4a7` | #52 | real project identifiers removed from tracked files, the storefront badge and `/api/info`; config fails fast without `PROJECT_ID` in Spanner mode; Dockerfile no longer sets `PROJECT_ID` |
| **`00043-p87`** | `014b6df` | #53 (+ #54 docs, #52 test guard) | storefront theme picker, `?theme=` handling and theme CSS removed (#49/#50 reverted in the UI); default dark storefront |
| **`00044-8r5`** | `aa90182` (code `598fc1f`) | #66 Phase A | per-search context deadline (`SEARCH_TIMEOUT` 10s, below the 15s `WriteTimeout`; no in-memory fallback when it fires), per-instance admission control (`SEARCH_MAX_INFLIGHT` 8, `SEARCH_QUEUE_WAIT` 250ms), `/api/info.search_admission`; all three env vars left unset (code defaults). **current** |

---

## 6. Release log (newest last)

## Issue #11: Gemini model upgrade rollout (2026-10-12)

Source: `main` @ `73a9017` (code from `799e5e5`, review passed in `73a9017`). Runtime SA: `<COMPUTE_DEFAULT_SA>`.

| Step | Result |
|---|---|
| 1. DDL migration `011_add_embedding_v2_gemini_1024.sql` | ✅ `embedding_v2 ARRAY<FLOAT32>(vector_length=>1024)` added (additive; v1 `embedding ARRAY<FLOAT64>(768)` untouched) |
| 2a. Backfill `-dry-run` | ✅ embedded 15 products with gemini-embedding-2 (1024 dims), nothing written |
| 2b. Backfill (real) | ✅ wrote embedding_v2 for 15 products; Spanner check: total=15, v2 non-null=15, len min/max=1024/1024 |
| 3. Deploy, `ACTIVE_EMBEDDING_VERSION=v1` → rev `psearch-serving-00002-gq9` | ✅ health 200 `healthy`; info `active_embedding_version=v1`, `embedding_dimension=768`, `product_count=15`; `search?q=running+shoes&alpha=0.5` → 15 results, top `prod-001` (text and vector non-zero) |
| 4. Flip `ACTIVE_EMBEDDING_VERSION=v2` → rev `psearch-serving-00003-p7p` | ❌ info showed `v2`/`1024`, but every query embedding failed with **403 PERMISSION_DENIED** (`aiplatform.endpoints.predict` on `publishers/google/models/gemini-embedding-2`, reason `IAM_PERMISSION_DENIED`); log line `WARN: query embedding failed (...); lexical-only search` |
| Rollback `ACTIVE_EMBEDDING_VERSION=v1` → rev `psearch-serving-00004-d2t` | ✅ health 200; info `v1`/`768`; `running shoes` → 15 hybrid results. **Serving now: v1, 100% traffic** |

Env added on the service: `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`, `ACTIVE_EMBEDDING_VERSION=v1`.

### Findings / blockers
- **IAM blocker:** the compute SA lacks Vertex AI predict permission (needs `roles/aiplatform.user` on `<PROJECT_ID>`). The backfill worked because it ran as `agent-deployer`. The Cloud Resource Manager API is disabled, so the project policy can't be read with gcloud.
- **False-positive check:** at v2, `q=tenis` still returned non-zero `score.vector`. The lexical-only Spanner query returns 0 rows for `tenis`, so `HybridSearch` falls back to `memoryHybridSearch`, which uses the 768-d deterministic embedding. "Non-zero score.vector" therefore doesn't prove Gemini is serving. Better criteria: no `lexical-only` / `PERMISSION_DENIED` logs, plus a v2-only signal (e.g. a per-response `embedding_version` or `retrieval_mode` field).
- **To retry v2:** grant `roles/aiplatform.user` to `<COMPUTE_DEFAULT_SA>` (or use a dedicated runtime SA), then `gcloud run services update psearch-serving --project=<PROJECT_ID> --region=us-central1 --update-env-vars=ACTIVE_EMBEDDING_VERSION=v2`.

## Issue #11 follow-up: redeploy at v1 with `retrieval_mode` (2026-10-12)

Source: `main` @ `1e96a34` (fix `6e4c0fb`, review passed in `1e96a34`). Revision **`psearch-serving-00005-vq7`**, 100% traffic, `ACTIVE_EMBEDDING_VERSION=v1`. Env kept: `GCP_PROJECT_ID`, `PROJECT_ID`, `SPANNER_INSTANCE(_ID)`, `SPANNER_DATABASE(_ID)`, `BIGQUERY_DATASET`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`.

| Check | Result |
|---|---|
| `GET /api/health` | 200 `{"status":"healthy","backend_mode":"spanner"}` |
| `GET /api/info` | 200 `active_embedding_version=v1`, `embedding_dimension=768`, `product_count=15` |
| `GET /api/search?q=tenis` | 200 `total_found=15`, `embedding_version=v1`, `retrieval_mode=hybrid_v1`; top `prod-002` "Tênis de Corrida Nimbus Cloud" (text 0, vector 0.0164) |
| `GET /api/search?q=running+shoes&alpha=0.5` | 200 `total_found=15`, `embedding_version=v1`, `retrieval_mode=hybrid_v1`; top `prod-001` "UltraBoost Pro Running Shoes" (text and vector 0.0164) |

v2 **not** flipped. It is waiting on the `roles/aiplatform.user` grant for `<COMPUTE_DEFAULT_SA>`. v2 acceptance: `retrieval_mode=hybrid_v2` (not `lexical_only_degraded` / `memory_fallback`) and no `lexical` / `PERMISSION_DENIED` / 403 in the logs.

## Issue #11: zero-traffic v2 IAM probe and template reset (2026-10-12)

**Probe:** revision `psearch-serving-00006-caw`, same image as `00005-vq7`, `ACTIVE_EMBEDDING_VERSION=v2`, `--no-traffic --tag=v2probe`.

| Check (tag URL `https://v2probe---<SERVICE_URL>`) | Result |
|---|---|
| `GET /api/info` | 200 `active_embedding_version=v2`, `embedding_dimension=1024` |
| `GET /api/search?q=running+shoes` | 200 `retrieval_mode=lexical_only_degraded`, `effective_alpha=0`, `total_found=1` (prod-001, vector 0) |
| `GET /api/search?q=tenis` | 200 `retrieval_mode=lexical_only_degraded`, `total_found=0`, `results=null` (the 6e4c0fb fix means memory fallback no longer masks it) |
| Revision logs (14:03:06Z, ×2) | `WARN: query embedding failed (embed RETRIEVAL_QUERY (gemini-embedding-2 @ global): Error 403, Message: Permission 'aiplatform.endpoints.predict' denied on resource '//aiplatform.googleapis.com/projects/<PROJECT_ID>/locations/global/publishers/google/models/gemini-embedding-2' ... PERMISSION_DENIED` |

Conclusion: `roles/aiplatform.user` is **still missing** for `<COMPUTE_DEFAULT_SA>`. Tag `v2probe` removed afterwards.

**Template reset:** revision `psearch-serving-00007-sr9` (Ready, 0% traffic, `--no-traffic`) sets the service template back to `ACTIVE_EMBEDDING_VERSION=v1`, so the next deploy can't ship v2 by accident. Template env: `GCP_PROJECT_ID`, `PROJECT_ID`, `SPANNER_INSTANCE(_ID)`, `SPANNER_DATABASE(_ID)`, `BIGQUERY_DATASET`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`, `ACTIVE_EMBEDDING_VERSION=v1`.

**Traffic:** pinned at 100% to `psearch-serving-00005-vq7` (v1, `retrieval_mode=hybrid_v1`). It no longer follows the latest revision, so the next rollout must use `--to-latest` or `gcloud run services update-traffic`.

## Issue #11: v2 LIVE on dedicated runtime SA (2026-10-12)

**Runtime SA (new):** `<RUNTIME_SA>`, with `roles/aiplatform.user` (project) and `roles/spanner.databaseUser` (`psearch-db`), created by the PO. It replaces `<COMPUTE_DEFAULT_SA>`. No IAM change was made to the compute SA. Earlier attempts to enable the CRM API (`serviceusage.services.enable` denied) and to grant the compute SA (CRM `SERVICE_DISABLED`) both failed and changed nothing; the plan was superseded.

Source: `main` @ `b4229a0` (includes `6e4c0fb` retrieval_mode and `a876979` empty results `[]`).

| Step | Revision | Result |
|---|---|---|
| Deploy v1 on new SA, `update-traffic --to-latest` (unpins from `00005-vq7`) | `psearch-serving-00008-ms8` | ✅ health 200; info v1/768; `tenis` and `running shoes` both `retrieval_mode=hybrid_v1`, `total_found=15`; no Spanner errors or warnings |
| Flip `ACTIVE_EMBEDDING_VERSION=v2` | **`psearch-serving-00009-9dc`** | ✅ passed on the first attempt (no IAM propagation retries needed) |

v2 verification (`00009-9dc`, 100% traffic, follows latest):
- `GET /api/health`: 200 `healthy`, `backend_mode=spanner`
- `GET /api/info`: 200 `active_embedding_version=v2`, `embedding_dimension=1024`, `product_count=15`
- `GET /api/search?q=tenis`: `retrieval_mode=hybrid_v2`, `embedding_version=v2`, `total_found=15`; top `prod-002` "Tênis de Corrida Nimbus Cloud" (vector 0.0164)
- `GET /api/search?q=running+shoes&alpha=0.5`: `retrieval_mode=hybrid_v2`, `total_found=15`; top `prod-001` "UltraBoost Pro Running Shoes" (text 0.0164, vector 0.0156)
- Logs: `embedding: gemini-embedding-2 generated 1024-d RETRIEVAL_QUERY vector in ~257ms` for each query; **0** lines matching `403` / `lexical` / `PERMISSION_DENIED` / `WARN` / severity ≥ ERROR.

Service template = serving revision: SA `psearch-serving@`, `ACTIVE_EMBEDDING_VERSION=v2`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`, plus project/Spanner/BQ vars. No tags.

**Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00008-ms8=100` (v1, new SA), or `--update-env-vars=ACTIVE_EMBEDDING_VERSION=v1`.

Observation (not a blocker): with only 15 products, the v2 ANN leg ranks every row, so low-relevance items appear lower in the list (e.g. `tenis` #3 is a TV). A `min_score`/cosine cutoff for the vector leg would be worth considering.

## Wave 1 (#1 alpha / #3 availability, visibility, channels): rollout 2026-10-13. Deploy ROLLED BACK

Source: `main` @ `8894c6d` (code `7b404be`, review `8894c6d`). Runtime SA `psearch-serving@`, `ACTIVE_EMBEDDING_VERSION=v2`.

| Step | Result |
|---|---|
| 1. DDL `012_availability_visibility_channels.sql` | ✅ `is_visible BOOL`, `channels ARRAY<STRING(64)>` added (additive) |
| 2. DML `012b_backfill_visibility_channels.dml.sql` | ✅ "Statement modified 15 rows". Check: `null_channels=0`, `null_visible=0`, `visible=15/15`, 0 mismatches vs `product_data.channels`. Products without `store`: prod-002, 004, 005, 009, 012, 015 |
| Old rev `00009-9dc` after the DDL | ✅ still `hybrid_v2` (tenis, running shoes, 15 results each); no warnings |
| 3. Deploy → `psearch-serving-00010-w7g` (100%) | ❌ **every query `retrieval_mode=memory_fallback`** |
| Rollback `update-traffic --to-revisions=psearch-serving-00009-9dc=100` | ✅ `hybrid_v2` restored. **Serving now: `00009-9dc`, 100% (pinned)** |

**Root cause** (`00010-w7g` logs):
```
WARN: Spanner HybridSearch fallback triggered: spanner: code = "InvalidArgument",
desc = "Aggregations of aggregations are not allowed [at 44:46]
  ORDER BY IF(IFNULL(ANY_VALUE(is_available), TRUE) AND IFNULL(...
```
In `buildHybridSQL`, the `ORDER BY` resolves `is_available`/`stock` to the SELECT aliases, which are already `ANY_VALUE(...)`, so the expression nests aggregates. Fix: order by the aliases directly, e.g. `ORDER BY IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 0, 1), rrf_score DESC`, or wrap the query in a subquery. Unit tests only exercise memory mode, so they didn't catch this. A test against the Spanner emulator, or a SQL dry-run, would.

Behaviour observed under `memory_fallback` on `00010-w7g` (Go logic only, not the Spanner path): `effective_alpha` echoed 0 / 1 / 0.5 and clamped 1.7→1; alpha=0 vs alpha=1 orderings differed; `q=shoes` put prod-004/prod-012 (OUT_OF_STOCK) last; `only_in_stock=true` → 13 results, both excluded; `channel=store` → 9 results, prod-002 absent.

**Other finding (pre-existing, not a regression):** the runtime SA `psearch-serving@` lacks `monitoring.timeSeries.create`, so the Spanner client logs `PermissionDenied ... Need monitoring metric writer permission` (also seen on `00009-9dc`). Fix with `roles/monitoring.metricWriter` on the SA, or turn client-side metrics off with `SPANNER_DISABLE_BUILTIN_METRICS=true`.

**Template note:** the service template points at `00010-w7g`'s image (same env and SA as `00009-9dc`) while traffic is pinned to `00009-9dc` (image `psearch-serving@sha256:814672c0…`). The next fixed deploy needs `--to-latest` / `update-traffic`.

## Wave 1 (#1 / #3) fix deployed: LIVE 2026-10-13

Source: `main` @ `79b15a2` (RRF aggregate wrapped in a subquery; `ORDER BY` uses plain columns; adds `cmd/sqlcheck`). SA `psearch-serving@`. Env as before plus **`SPANNER_DISABLE_BUILTIN_METRICS=true`** (silences the client-metrics PermissionDenied noise; no new role granted).

**Pre-deploy `go run ./cmd/sqlcheck`** (real Spanner SQL, read-only) passed all 6 cases with exit 0:
```
OK q="running shoes" alpha=0.0                       rows=15 mode=hybrid_v2
OK q="running shoes" alpha=1.0                       rows=15 mode=hybrid_v2
OK q="shoes" alpha=0.5                               rows=15 mode=hybrid_v2
OK q="shoes" alpha=0.5 OnlyInStock=true              rows=13 mode=hybrid_v2
OK q="shoes" alpha=0.5 Channel=store                 rows=9  mode=hybrid_v2
OK q="tenis" alpha=0.5 OnlyInStock=true Channel=mobile rows=12 mode=hybrid_v2
```

Deploy → **`psearch-serving-00011-p6t`**, `--to-latest`, 100% (follows latest; template = serving). Rollback target: `psearch-serving-00009-9dc`.

| Check | Result |
|---|---|
| `/api/health` | 200 `healthy`, `backend_mode=spanner` |
| `/api/info` | 200 v2 / 1024, `product_count=15` |
| `running shoes` alpha=0 vs alpha=1 | ✅ different orderings. alpha=0: prod-001, 008, 007, 014, 003… alpha=1: prod-002, 003, 001, 007, 014…; `effective_alpha` echoed 0 / 1 / 0.5 |
| alpha=1.7 | ✅ clamped, `effective_alpha=1` |
| `q=shoes` | ✅ 15 results, prod-004 and prod-012 (OUT_OF_STOCK) last |
| `q=shoes&only_in_stock=true` | ✅ 13 results, prod-004/012 excluded |
| `q=shoes&channel=store` | ✅ 9 results, prod-002 absent (also prod-004/005/009/012/015) |
| `q=tenis&channel=store` | ✅ 9 results, prod-002 absent |
| `retrieval_mode` | ✅ `hybrid_v2` on all 11 queries |
| Logs (`00011-p6t`) | ✅ no `WARN` / `fallback` / `InvalidArgument` / `PermissionDenied` / `lexical` / 403 / ERROR; metrics noise gone |

Observation (not a blocker): at alpha=0 only text matches score (`final_score>0`). The remaining rows have `final_score=0` and keep an arbitrary tie order (e.g. prod-008 before prod-007 despite a lower vector RRF). A tie-break on `vector_score`, or dropping zero-score rows at alpha=0, would make this deterministic.

## #2 two-tier embedding cache, embedding-version endpoint, deterministic ranking: LIVE 2026-10-13

Source: **clean `git archive` of `23bf1f7fe3509555903eb2bcba5e1a569c088b44`** in `/tmp/deploy-23bf1f7` (PO rule; not `/workspace`). SA `psearch-serving@`. Env unchanged (`ACTIVE_EMBEDDING_VERSION=v2`, `SPANNER_DISABLE_BUILTIN_METRICS=true`, …). **`ADMIN_TOKEN` unset**, so the POST endpoint is disabled.

| Step | Result |
|---|---|
| DDL `013_embedding_cache.sql` | ✅ `embedding_cache` table created (PK `content_sha256, model_id`) |
| `cmd/sqlcheck` (from archive, v2 env) | ✅ 6/6 OK, `mode=hybrid_v2`, exit 0 (rows 15/15/15/13/9/12). `embedding_cache` count stayed 0 afterwards (baseline) |
| Deploy → **`psearch-serving-00012-65q`** | ✅ 100%, then `update-traffic --to-latest` (follows latest). Note: `gcloud run deploy` doesn't accept `--to-latest`, so it has to be done with `update-traffic`. Rollback target: `00011-p6t` |

Verification on `00012-65q`:
- `/api/health` 200. `/api/info`: `vector_dimensions=1024`, `embedding_dimension=1024`, `embedding_cache={enabled:true, max_entries:1000, …}`
- `GET /api/config/embedding-version` → 200 `{"active_embedding_version":"v2","available":["v1","v2"],"scope":"instance","vector_dimensions":1024}`
- `POST /api/config/embedding-version` → **403 `{"error":"forbidden"}`** (no token and a bogus token); GET afterwards is still v2
- Cache, fresh query `mochila impermeavel para trilha`: 1st `cache_hit` absent, **253.6 ms**; 2nd `cache_hit=true`, **18.4 ms**; 3rd `cache_hit=true`, 33.2 ms. `notebook gamer` repeat: `cache_hit=true`, 13.6 ms. Stats: `misses=2, memory_hits=5, entries=2`
- Spanner tier: `embedding_cache` has **2 rows** (`gemini-embedding-2`, dims 1024, `ARRAY_LENGTH=1024`) written about 20 s after the misses. The async write landed under request-based CPU, and the `psearch-serving@` databaseUser role covers it
- Ranking: `running shoes` alpha=0 gives an identical order over 3 calls (zero-score tail now tie-broken); alpha=0 vs 1 still differ; Wave 1 regressions OK (`shoes` OOS last, `only_in_stock` 13, `channel=store` 9 without prod-002)
- `retrieval_mode=hybrid_v2` on all queries. Logs show no `WARN`, `embedding cache store`, `PermissionDenied`, `fallback`, `lexical`, `InvalidArgument` or ERROR (no cold-start read timeout either)

## #4 tenant/locale/category/brand filters + accent folding, #5 server-side facets: LIVE 2026-10-13

Source: **clean `git archive` of `14add095420dc2aa0629788d0486aaef696fb833`** (`254c8f7` + UI `405d454`; in the image only `internal/api/routes.go` storefront JS differs from `254c8f7`). SA `psearch-serving@`. Env as before plus **`TEXT_FOLDING=true`**.

| Step | Result |
|---|---|
| DDL `014_tenant_locale.sql` | ✅ `tenant_id STRING(64)`, `locale STRING(16)` |
| DML `014c` | ✅ 15 rows modified; `tenant_id IS NULL OR locale IS NULL` = 0; `default/en-US` 10, `default/pt-BR` 5 |
| DDL `014b` part 3a (two `ADD COLUMN` generated `TOKENLIST`s with `REGEXP_REPLACE(NORMALIZE(x, NFD), r'\p{M}', '')`) | ✅ accepted by Spanner |
| DDL `014b` part 3b (`ALTER SEARCH INDEX products_text_idx ADD COLUMN …` ×2) | ✅ index is now `(title_tokens, description_tokens, title_tokens_folded, description_tokens_folded)`. SQL check: `SEARCH(title_tokens_folded,'tenis')` → prod-002 "Tênis…", plain `title_tokens` → 0 rows |
| `cmd/sqlcheck` (archive, v2, `TEXT_FOLDING=true`) | ✅ 9/9 + facets OK, `hybrid_v2`, exit 0 (incl. `tenis` pt-BR → 5, Computers+Acer → 1, tenant `acme` → 0) |
| Deploy → **`psearch-serving-00013-5vm`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00012-65q` |

Verification on `00013-5vm`:
- `q=tenis&alpha=0` (text only) → **prod-002 "Tênis de Corrida Nimbus Cloud" first** (accent folding)
- `q=tenis&locale=pt-BR` → 5 results, all pt-BR (002, 009, 006, 013, 011)
- `q=shoes&tenant_id=acme` → 0 results
- `q=notebook&category=Computers&brand=Acer` → prod-006 only
- `facets`: 4 groups (`categories`, `brands`, `price_ranges`, `availability`) with counts, plus `facets_latency_ms` (≈6–47 ms); facets follow the active filters
- `q=shoes&limit=3` → 3 results, `candidates=15`
- Regressions: alpha=0 deterministic, alpha 0≠1, 1.7→1; `only_in_stock` 13; `channel=store` 9 without prod-002; OOS last; cache miss 857 ms → hit 16.7 ms (`spanner_hits=1` across instances); `/api/info` v2/1024; GET config v2, POST 403
- `retrieval_mode=hybrid_v2` on all non-empty queries. Logs show no WARN, fallback, InvalidArgument, PermissionDenied, lexical, embedding cache store or ERROR

**Finding (labeling bug, not a rollback trigger):** legitimate zero-result Spanner queries (`tenant_id=acme`; `category=Computers&brand=Nike`) report `retrieval_mode=memory_fallback`. `HybridSearch` only returns the Spanner outcome when `len(res) > 0`; on a successful empty result it falls through to `memoryHybridSearch`. There's no SQL error (logs clean, sqlcheck shows the same case as `hybrid_v2` rows=0). Risks: it breaks the "memory_fallback = SQL error" signal, and if the Spanner catalog diverges from the seed set, memory mode could return products Spanner correctly filtered out. Suggested fix: return the Spanner result (with `hybrid_v2`/`hybrid_v1`) whenever `err == nil`, even when empty.

## #4 follow-up: empty Spanner results are authoritative: LIVE 2026-10-13

Source: clean `git archive` of **`abf6432393b0dfe340396ae0e07875f330edddd2`** (only change in the image: `spanner_service.go`, where `case len(res) > 0` becomes `default`). Env unchanged (`TEXT_FOLDING=true`). No DDL.
`cmd/sqlcheck`: 9/9 + facets OK, `hybrid_v2`, exit 0. Deploy → **`psearch-serving-00014-pc5`**, then `update-traffic --to-latest` (100%, follows latest). Rollback target: `00013-5vm`.

| Check | Result |
|---|---|
| `q=shoes&tenant_id=acme` | ✅ 0 results, `results=[]`, **`retrieval_mode=hybrid_v2`** (was `memory_fallback` on 00013) |
| `q=shoes&category=Computers&brand=Nike` | ✅ 0 results, **`hybrid_v2`** |
| `q=tenis&alpha=0` | ✅ prod-002 first |
| `q=tenis&locale=pt-BR` | ✅ 5 results, all pt-BR |
| `category=Computers&brand=Acer` | ✅ prod-006 |
| facets | ✅ 4 groups on every response; `facets_latency_ms` 5.7–15 ms (absent on empty results) |
| `only_in_stock` / `channel=store` | ✅ 13 / 9 (prod-002 absent) |
| Logs | ✅ one expected cold-start `WARN: embedding cache store read failed (treated as miss): DeadlineExceeded … getting session` (200 ms read budget on a new instance; agreed non-blocking); nothing else |

## #7 tenant/locale synonym expansion: LIVE 2026-10-13

Source: clean `git archive` of **`6d62e7e257d1bf86c48cfa8c884f5cbd4b96b9c0`**. Env unchanged (v2, `TEXT_FOLDING=true`, no `ADMIN_TOKEN`).

| Step | Result |
|---|---|
| DDL `015_search_synonyms.sql` | ✅ `search_synonyms` (PK `tenant_id, locale, term`) |
| DML `015b_seed_synonyms.dml.sql` | ✅ "Statement modified 8 rows": pt-BR fone/geladeira/notebook/tenis, en-US fridge/laptop/sneakers/tv, weight 0.3 |
| `cmd/sqlcheck` | ✅ `search_synonyms rows=8 expand(geladeira,pt-BR)=[fridge refrigerador refrigerator]`; 11 search cases + facets OK (`geladeira` a=0 → 5, `notebook gamer` → 15), `hybrid_v2`, exit 0 |
| Deploy → **`psearch-serving-00015-dzf`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00014-pc5` |

Verification on `00015-dzf`:
- `q=geladeira&locale=pt-BR&alpha=0` → **prod-011 "Geladeira…" first**, `expanded_terms=[fridge, refrigerador, refrigerator]`
- prod-012 "Smart French Door Refrigerator" is **en-US**, so `locale=pt-BR` correctly excludes it. Without a locale (`q=geladeira&alpha=0`) it **appears with text score 0.0161 > 0 via the synonym, ordered last (OUT_OF_STOCK)**. Exact-first holds: prod-011 is at text rank 1
- `q=laptop&locale=en-US` → `expanded_terms=[computer, notebook]`; prod-005 "…Laptop" first, prod-007 also text>0
- `GET /api/synonyms?locale=pt-BR` → `count=4` (fone, geladeira, notebook, tenis). `POST /api/synonyms` → **403** (no or bogus token); table still 8 rows
- Regressions: `tenis&alpha=0` → prod-002 first; `tenant_id=acme` → 0 results, `hybrid_v2`; pt-BR 5; Acer → prod-006; store 9; in-stock 13; facets 4 groups; cache miss 315 ms → hit 15 ms
- Logs: no WARN, fallback, InvalidArgument, PermissionDenied, lexical or ERROR

Notes: (a) with no `locale`, expansion uses synonyms from every locale of the tenant (e.g. `tenis` gets the pt-BR set), which may be intended; (b) an expansion can repeat the query phrase itself (`fone de ouvido` → `[fone de ouvido, headphones]`), which is harmless.

## #6 merchandising rules + 8 boost signals: LIVE 2026-10-13

Source: clean `git archive` of **`0b54d5204b3d4539f66323a39806949a6b261ed7`** (#6 plus the #7 synonym-chip UI). Env unchanged (v2, `TEXT_FOLDING=true`); `BOOST_WEIGHT` not set (default 0.002); **`ADMIN_TOKEN` unset**.

| Step | Result |
|---|---|
| DDL `016` part a: 8 `ADD COLUMN` (clicks, orders, discount_pct, popularity_score, margin_pct, ctr, conversion_rate, recency_score) | ✅ |
| DDL `016` part b: `CREATE TABLE merchandising_rules` (PK `tenant_id, rule_id`) | ✅ (the split matches the original file exactly) |
| DML `016b` | ✅ "Statement modified 15 rows"; `clicks IS NULL` = 0, all 8 signals non-null on 15/15; `merchandising_rules` = 0 rows |
| `cmd/sqlcheck` | ✅ synonyms 8, `merchandising_rules rows=0`, 12 search cases (incl. in-memory pin/bury/boost rule params) + facets OK, `hybrid_v2`, exit 0 |
| Deploy → **`psearch-serving-00016-jtj`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00015-dzf` |

Verification on `00016-jtj`:
- `score.boost` on **15/15 results** for `running shoes` and `shoes`: range 0.000387–0.001064 (> 0, ≤ 0.002); `rule_multiplier=1`; `final_score = hybrid + boost` (exact). Boost reorders near-ties, e.g. `shoes`: prod-008 (boost 0.001064) is above prod-002
- `GET /api/rules` → 200 `{"count":0,"rules":[]}`. `POST /api/rules`, `PUT /api/rules/x`, `DELETE /api/rules/x` → **403**; table still 0 rows
- Regressions: `tenis&alpha=0` → prod-002 first; `geladeira&locale=pt-BR` → prod-011 first with `[fridge, refrigerador, refrigerator]`; without a locale, prod-012 last (OOS); `acme` → 0 results, `hybrid_v2`; Acer → prod-006; store 9; in-stock 13; facets 4 groups; OOS (004, 012) last everywhere; cache miss 248 ms → hit 18.5 ms
- Logs: no WARN, fallback, InvalidArgument, PermissionDenied, lexical or ERROR. **The cold-start `embedding cache store read failed` WARN is gone** (new 1500 ms first-read budget)
- Rule effects (pin/bury/boost) aren't exercised in prod without an admin token, which is expected; they're covered by unit tests and the sqlcheck rule-params case. The multiplier clamp hardening (`2586e94`) ships with the next deploy.

## #8 CEP/SLA delivery promises (interleaved skus / regional_inventory): LIVE 2026-10-13

Source: clean `git archive` of **`cbc384c879f39f2c29b4f994902476cc2df26e2a`** (#8 `dea13c8` + clamp `2586e94` + React UI `f07d668`). Env unchanged; `ADMIN_TOKEN` unset.

| Step | Result |
|---|---|
| DDL `017` part a: `skus` `INTERLEAVE IN PARENT products ON DELETE CASCADE` | ✅ |
| DDL `017` part b: `regional_inventory` `INTERLEAVE IN PARENT skus ON DELETE CASCADE` | ✅ (the split matches the original file exactly) |
| DML `017b` → `017c` | ✅ "modified 15 rows", "modified 57 rows". **skus=15, regional_inventory=57**. Per region: SP 12 (SLA 24–48h), SUL 12 (72–96h), RJ 11, MG 11, NE 11. Sellers: seller-psearch 10, 5 marketplace sellers |
| `cmd/sqlcheck` | ✅ 14 search cases + 2 delivery cases (`shoes` → 8, `notebook` → 9) + "delivery promises for 01310: 3 products" + facets OK, exit 0 |
| Deploy → **`psearch-serving-00017-9wc`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00016-jtj` |

Verification on `00017-9wc`:
- `q=shoes&cep=01310-100` → 12 deliverable products (excluded: prod-004 and prod-012, OOS everywhere, and prod-008, which has no SP inventory row). Every result has `deliveryPromise.region_code=SP`, `cd_id=cd-sp-cajamar`. Label is **"Chega amanhã - CD São Paulo"** (24h, seller-psearch) or **"Chega em 2 dias - CD São Paulo"** (48h, marketplace sellers: nike, keychron, sony, acer)
- `+max_sla_hours=24` → 8 results, **all `sla_hours=24`**
- `cep=123` → **HTTP 400** `invalid cep "123": expected 8 digits (e.g. 01310-100)`
- `cep=90000-000` → region **SUL**, `cd-pr-curitiba`, labels "Chega em até 3 dias - CD Sul" (72h) / "Chega em até 4 dias - CD Sul" (96h), different costs. Same 12-product set as SP (both regions stock the same SKUs); `cep=20040-020` (RJ) gives a different set of 9
- No `cep` → 0 results carry `deliveryPromise`
- Regressions: `tenis&alpha=0` → prod-002 first; geladeira pt-BR → prod-011 first with synonyms; `score.boost` in (0, 0.002] on all results; OOS last; `acme` → 0 results, `hybrid_v2`; Acer → prod-006; facets 4 groups; cache miss 369 ms → hit 25 ms; `/api/rules` GET 200 / POST 403
- Logs: one non-blocking startup line, `WARN: Spanner seed check: DeadlineExceeded … getting session`, 21 s after instance start. `SeedIfEmpty` runs in a background goroutine with a 20 s budget, and under request-based CPU the idle post-deploy instance was throttled. It only checks whether `products` is empty (it has 15 rows); the backend stayed `spanner`, and every query was `hybrid_v2`. First occurrence on any revision. Otherwise no WARN, fallback, InvalidArgument, PermissionDenied, lexical or ERROR

Notes: (a) `shipping_cost` is still reported when `free_shipping=true` (e.g. prod-007: free, cost 15). The UI should show "Frete grátis" or 0 in that case; (b) prod-008 has inventory only in MG/NE/RJ, as seeded; confirm that's intended.

## #9 autocomplete (n-gram typeahead) + #10 two-stage LTR: LIVE 2026-10-13

Source: clean `git archive` of **`69bdc217a4a4d7db7864b2a03865810466257fdb`** (#9 `c717604` + fixes `f3fc754`/`69bdc21`, UI `3e9e0dd`, #8 follow-up `f00acb7`, #10 `75fee13`). Env unchanged; `LTR_MODEL_JSON` unset (baseline model); `ADMIN_TOKEN` unset.

| Step | Result |
|---|---|
| 1st attempt, original 018 (`c717604`) | ❌ Spanner rejected the generated column: `Invalid use of tokenize_ngrams: regexp_replace is not supported as a tokenization argument`. Nothing created |
| Throwaway probe (approved): `autocomplete_ddl_probe` + search index with the `f3fc754` definitions | ✅ accepted, then `DROP SEARCH INDEX` + `DROP TABLE`; 0 probe objects remain |
| DDL `018` part a: `autocomplete_suggestions` (plain `suggestion_folded`, `suggestion_ngrams = TOKENIZE_NGRAMS(suggestion_folded, 2..4)`) | ✅ |
| DDL `018` part b: `CREATE SEARCH INDEX autocomplete_ngram_idx` | ✅ |
| DML `018b` | ✅ 48 rows (en-US 30, pt-BR 18); all `suggestion_folded` values pre-verified (0 mismatches) |
| sqlcheck on `f3fc754` | ❌ `SEARCH_NGRAMS is not supported in this query` for `gel` / `tên`. Cause: `SEARCH_NGRAMS(...) OR STARTS_WITH(suggestion_folded, ...)` in WHERE. Fixed in `69bdc21` (SEARCH_NGRAMS alone in WHERE, STARTS_WITH only in ORDER BY) |
| sqlcheck on `69bdc21` | ✅ all OK, exit 0: autocomplete `gel` → 2, `tên` → 2, `n` → 3; `ltr candidate signals: 3 products`; search/delivery/synonyms/rules/facets OK |
| Deploy → **`psearch-serving-00018-rc5`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00017-9wc` |

BigQuery (read-only): `psearch_dataset` exists (us-central1) but has **0 tables**, so `search_query_logs` doesn't exist and the export script wasn't run. Autocomplete serves the 48 bootstrap suggestions until query logging exists.

Verification on `00018-rc5`:
- `/api/autocomplete?q=gel&locale=pt-BR` → **"geladeira"** first, then "geladeira inverse frost free inox 540l"; 85 ms, then **`cached=true` in 0.04 ms**
- `q=tên` → **"tênis de corrida nimbus cloud", "tenis"** (accent-insensitive); 115 ms → cached 0.04 ms
- `q=n` (1-char prefix path) → notebook, notebook gamer…, nike; `q=lap&locale=en-US` → laptop, probook…laptop
- 101-char `q` → **400** `q too long (max 100)`; 100 chars → 200
- #10: `/api/search?q=shoes&ltr=true&limit=5` → `ltr=true`, `ltr_model=baseline-2026-10`, **`ltr_latency_us` 48 / 21**; every result has `stage1_rank`/`stage2_rank`/`ltr_score`; it re-ranks (prod-015 moves from stage1 6 → stage2 5, replacing prod-007). Without `ltr`, no ltr fields at all
- #8 still OK: `cep=01310-100` → 12, all SP, labels "Chega amanhã / Chega em 2 dias - CD São Paulo"; `max_sla_hours=24` → 8; SUL 12; `cep=123` → 400. Free-shipping items now report `shipping_cost=0` (follow-up `f00acb7`)
- Regressions: tenis → prod-002 first; geladeira synonyms; boost (0, 0.002]; OOS last; acme → 0 results, `hybrid_v2`; Acer → prod-006; facets 4; cache miss → hit; info v2/1024; POST `/api/rules`, `/api/synonyms`, `/api/config/embedding-version` → 403
- Logs: fully clean (no WARN, fallback, InvalidArgument, PermissionDenied, lexical, SEARCH_NGRAMS or ERROR; no seed-check timeout this time)

## #10 UI (LTR toggle in storefront): LIVE 2026-10-13

Source: clean `git archive` of **`5d5c13619c2ee8fb5bee972bba5452d94759ab67`** (adds `8a7017d`; the only image change since `69bdc21` is `internal/api/routes.go` storefront HTML/JS). No DDL; env unchanged.
`cmd/sqlcheck`: 22 OK, 0 FAIL, exit 0. Deploy → **`psearch-serving-00019-dsr`**, then `update-traffic --to-latest` (100%, follows latest). Rollback target: `00018-rc5`.

- Storefront `GET /` → 200; HTML contains `<input type="checkbox" id="ltr" /> Re-rank (LTR)` and the `ltr-badge` element plus the JS that appends `&ltr=true` and renders "LTR: <model> (<µs>)"
- `/api/search?q=shoes&ltr=true&limit=5` unchanged from 00018: `ltr=true`, `ltr_model=baseline-2026-10`, `ltr_latency_us=36`; identical stage1/stage2 ranks and `ltr_score`s (prod-015 6→5). Without `ltr`: no ltr fields
- Regressions: tenis → prod-002 first; geladeira pt-BR → prod-011 first; acme → 0 results, `hybrid_v2`; cep SP → 12 promises; boost OK; OOS last; facets 4; autocomplete `gel` → geladeira; health 200
- Logs: clean

## Wave 5 #19 conversational query understanding (parser-only): LIVE 2026-10-13

Source: clean `git archive` of **`7a958efb372ea4e1fb3ad2203b32e355953c20bd`** (#19 `b35ce14`, review `7a958ef`). Env unchanged; **`INTENT_LLM_FALLBACK` unset (false, parser only)**; `ADMIN_TOKEN` unset.

| Step | Result |
|---|---|
| DDL `020_query_intent_cache.sql` | ✅ `query_intent_cache` (PK `query_hash`) |
| `cmd/sqlcheck` | ✅ 25 OK, 0 FAIL, exit 0. New: `tenis de corrida` with `MaxPrice:200` → 5 rows; `notebook` with `MinPrice:1000 MaxPrice:1700` → 6 rows; `catalog vocabulary entries=15` |
| Deploy → **`psearch-serving-00020-552`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00019-dsr` |

Verification on `00020-552`:
- `q=tenis+de+corrida+ate+200+reais&parse_intent=true` → `parsed_query={original_query, clean_query:"tenis de corrida", extracted_filters:{max_price:200}, intent_source:"parser"}`; 5 results, all `numericPrice` ≤ 200 (169.9, 189.9, 149.5, 119, 95)
- `q=running+shoes+adidas+under+200&parse_intent=true` → `extracted_filters={brand:"Adidas", max_price:200}`, `clean_query:"running shoes"` → **prod-001 only**
- `…&parse_intent=true&max_price=2000` → explicit param overrides: 15 results up to 1650. Note: `parsed_query.extracted_filters` still reports the parsed `max_price:200`, not the applied value
- `min_price=500&max_price=100` → **400** `min_price/max_price must be >= 0 and min_price <= max_price`
- Without `parse_intent` → no `parsed_query`
- `query_intent_cache` stays at 0 rows (parser intents are cached in-process only; the persistent store is for LLM results, and the fallback is off)
- Regressions: tenis → prod-002 first; geladeira pt-BR → prod-011; acme → 0 results, `hybrid_v2`; cep SP → 12 promises; `ltr=true` → baseline-2026-10; boost OK; OOS last; facets 4; autocomplete `gel` → geladeira; storefront 200; health 200
- Logs: clean

Process note: during this rollout lead's uncommitted edits in `/workspace` were reverted by someone (not deployer: only read-only git plus `git archive` into `/tmp`). The image came from the archive of `7a958ef`, so it wasn't affected.

## Wave 5 #21 spell recovery + #19 intent default-on (`612d939`): deployed then ROLLED BACK 2026-10-13

Source: clean `git archive` of **`612d9396b1acf4bddfcf3ee207369fb4a2e363a8`** (#21 `d674656` + #19 default-on). No DDL. `cmd/sqlcheck`: 27 OK, 0 FAIL (new: `spell 'sansung' -> "samsung"`, `relaxed OR query`).
Deploy → `psearch-serving-00021-x6b` (100%), verified, then **rolled back**: `update-traffic --to-revisions=psearch-serving-00020-552=100`. **Serving now: `00020-552` (pinned)**; the service template points at the 00021 image, so the next rollout needs `update-traffic --to-latest`.

| Check on `00021-x6b` | Result |
|---|---|
| #21 `q=sansung&alpha=0` → `did_you_mean=samsung`, `auto_corrected=true`, prod-010 first | ❌ `did_you_mean=samsung` only; **no `auto_corrected`**; top = prod-008/015/001 |
| #21 `q=geladera` | ✅ `did_you_mean=geladeira`, `auto_corrected=true`, prod-011 first |
| #21 `q=samsung` untouched | ✅ no correction (parser extracts `brand:Samsung` → prod-010 only) |
| #19 default-on `q=tenis+de+corrida+ate+200+reais` (no `parse_intent`) | ✅ `parsed_query.extracted_filters.max_price=200`, 5 results, max price 189.9 |
| #19 `parse_intent=false` | ✅ no `parsed_query` (15 results) |
| #19 `q=geladeira` | ✅ no `parsed_query` |

**Root cause of the #21 failure:** recovery only auto-corrects when the corrected query has lexical hits (`services.LexicalHits(alt.Results) > 0`). "samsung" appears only in `products.brand`; prod-010's title/description ("Galaxy S25 Ultra 5G 512GB Smartphone" / "Flagship Android phone…") don't contain it, and the search index covers only title/description tokens. Read-only checks: `SEARCH(title_tokens_folded|description_tokens_folded, 'samsung')` → 0 rows; `q=samsung&parse_intent=false&alpha=0` → no text hits, prod-010 not on top. Fix options: (a) after spell-correcting, re-run the intent parser on the corrected query, so `samsung` → brand filter → prod-010, and accept the correction when that yields results; (b) index brand/category text (e.g. a `brand_tokens` TOKENLIST added to `products_text_idx` and to the fts WHERE), which needs DDL.

## Wave 5 #21 spell recovery (entity fix) + #19 default-on + UI: LIVE 2026-10-13

Source: clean `git archive` of **`4ab84038d7445eb846a1eb685ebbfb577512eb52`** (`612d939` + `b306c09` test + **`4ab8403` handlers.go entity re-parse** + UI `07c75d5`/`d1b602b`/`8ba9d06`). No DDL; env unchanged.
Pre-check: `b306c09` alone contained only a unit test (the handlers.go change had been lost to a working-tree revert), so the rollout was held until `4ab8403`.
`cmd/sqlcheck`: 27 OK, 0 FAIL (`spell 'sansung' -> "samsung"`, `relaxed OR query rows=15`). Deploy → **`psearch-serving-00022-nll`**, then `update-traffic --to-latest` (**unpinned** from 00020; 100%, follows latest). Rollback target: `00020-552`.

| Check on `00022-nll` | Result |
|---|---|
| #21 `q=sansung&alpha=0` | ✅ `did_you_mean=samsung`, **`auto_corrected=true`**, **prod-010 first** (brand filter from re-parse; same without alpha) |
| #21 `q=geladera` | ✅ `did_you_mean=geladeira`, `auto_corrected=true`, prod-011 first |
| #21 `q=samsung` | ✅ untouched (no did_you_mean); parser `brand:Samsung` → prod-010 |
| #19 default-on `q=tenis+de+corrida+ate+200+reais` (no param) | ✅ `parsed_query.extracted_filters.max_price=200`; 5 results, max 189.9 |
| #19 `parse_intent=false` | ✅ no `parsed_query` |
| #19 `q=geladeira` | ✅ no `parsed_query` |
| Storefront `/` | ✅ 200; contains the "Você quis dizer" banner, did_you_mean/relaxed handling, the `parse_intent` toggle, filter chips, and the LTR checkbox |
| Regressions | ✅ tenis → prod-002 first; geladeira pt-BR → prod-011; acme → 0 results, `hybrid_v2`; cep SP → 12 promises; LTR baseline; adidas<200 → prod-001; autocomplete gel; bad price → 400; boost OK; facets 4; health 200 |
| Logs | ✅ clean |

Observation: when a correction resolves to an entity (sansung → brand Samsung), the response carries `did_you_mean`/`auto_corrected` but no `parsed_query`, so the UI shows no brand chip for the applied filter. Consider surfacing it.

## Wave 5 #22 B2B contract pricing + #20 session personalization: LIVE 2026-10-13

Source: clean `git archive` of **`1fad8cfc8b220d01960cb2384d79a3527d510e8b`** (#22 `372ad12` + #20 `497fee6` + fix `1fad8cf`). Env unchanged.

| Step | Result |
|---|---|
| DDL `019_b2b_product_prices.sql`: `product_prices` `INTERLEAVE IN PARENT products ON DELETE CASCADE` | ✅ (applied during the first #22 attempt) |
| DML `019b` | ✅ 45 rows: b2b-corporate 15 (×0.90), b2b-gold 15 (×0.82 Computers/Appliances, ×0.92 otherwise), vip-club 15 |
| sqlcheck on `372ad12` (1st attempt) | ❌ 17/27 FAIL: `SCORE is only supported in queries with query optimizer version 6 or above` (correlated `product_prices` subqueries in the fts CTE). The listed #22 sqlcheck cases were also missing. Not deployed; prod unaffected. Fix proven in a private copy: statement hint `@{OPTIMIZER_VERSION=latest_version}` |
| DDL `021_user_session_events.sql` | ✅ |
| sqlcheck on `1fad8cf` (hint + real #22/#20 cases) | ✅ 32 OK, 0 FAIL: laptop b2b-corporate price_asc → 15; shoes vip-club price_desc ≤300 → 7; macbook b2b-gold ≤1200 → 12; `user_session_events readable`; facets with b2b-gold |
| Deploy → **`psearch-serving-00023-z7g`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00022-nll` |

#22 verification:
- `q=laptop&price_table_id=b2b-corporate&sort_by=price_asc` → every `priceInfo.numericPrice` = **0.90 × basePrice** (e.g. prod-015 119 → 107.1), `priceTableId=b2b-corporate`, **ascending among available**, OOS (prod-004, prod-012) last
- Facets use contract prices: `notebook gamer` Acer with b2b-gold → prod-006 **1650 → 1353**, price_ranges `500-1500`; without a table → 1650, `1500+`
- `q=macbook&price_table_id=b2b-gold&max_price=1200` → includes **prod-007 at 1147.18**
- No `price_table_id` → prices unchanged (basePrice = numericPrice, no priceTableId)
- `sort_by=cheap` → **400** `invalid sort_by "cheap" (relevance | price_asc | price_desc)`

#20 verification:
- `POST /api/session/events` `{sess-qa-runner1, prod-003, purchase}` → **201**; `{sess-qa-laptop1, prod-005, purchase}` → 201; invalid `session_id` → **400** (`8-128 chars [A-Za-z0-9_-]`); invalid `event_type` → 400. Rows persisted in `user_session_events`
- `GET /api/session/events?session_id=sess-qa-runner1` → count 1 (prod-003 purchase)
- `q=shoes&session_id=sess-qa-runner1` → `personalized=true`, `session_boost` on all results (prod-003 0.003 max); order 001, 003, **002**, 008, …
- `q=shoes&session_id=sess-qa-laptop1` → `personalized=true`; order 001, **005**, 008, 007, … (prod-005 moves from rank 11 to 2), clearly different from the runner session
- No session, or a session with no events → unchanged baseline (001, 003, 008, 002, …), no `session_boost`
- OOS prod-004/prod-012 last in every variant

Regressions: sansung → samsung auto-corrected, prod-010; geladera → geladeira; default intent max_price 200; tenis → prod-002 first; cep SP → 12 promises; LTR baseline; acme → 0 results with `hybrid_v2`; autocomplete gel; storefront/health 200. Logs clean (no SCORE/optimizer, WARN, fallback, PermissionDenied or ERROR).
Follow-up: UI commit `299f7ec` (#20 session panel) landed after `1fad8cf` and ships as a separate small deploy.

## #20 UI follow-up (session panel / storefront session field): LIVE 2026-10-13

Source: clean `git archive` of **`299f7ec5ca66b9a83b16e3063757eeef26afecca`** (review `d6a2ab7`); the only image change since `1fad8cf` is `internal/api/routes.go` (React files aren't in the image). No DDL; env unchanged. sqlcheck 32 OK / 0 FAIL. Deploy → **`psearch-serving-00024-ltv`**, then `update-traffic --to-latest` (100%, follows latest). Rollback target: `00023-z7g`.
- Storefront `/` → 200 with the session field (`session_id`), personalized badges, and click/purchase tracking to `/api/session/events`; the LTR toggle, "Você quis dizer" banner and price_table selector are still present
- Unchanged API behavior: runner session personalized (001, 003, 002, …) vs baseline (001, 003, 008, …); b2b-corporate asc → prod-015 at 107.1; sansung → samsung → prod-010; cep SP → 12; bad session_id → 400; health 200. Logs clean

## Wave 5 #23 Platform model upgrade (`gemini-3.8-flash` & `gemini-4-argon`): LIVE 2026-10-14

Source: clean `git archive` of **`41af0a6`** (PR #25). No DDL; env configured via `GCP_SANDBOX_PROJECT` and `CLOUDSDK_CORE_PROJECT`. sqlcheck 32 OK / 0 FAIL. Deploy → **`psearch-serving-00025-5mx`**, then `update-traffic --to-latest` (100%, follows latest). Rollback target: `00024-ltv`.
- `/api/info` → 200 with `text_model: "gemini-3.8-flash"`, `pro_model: "gemini-4-argon"`, `embedding_model: "gemini-embedding-2"`, `genai_location: "global"`, `active_embedding_version: "v2"`, `embedding_dimension: 1024`
- Conversational query understanding (#19): `q=tenis de corrida ate 200 reais` → `clean_query: "tenis de corrida"`, `extracted_filters.max_price: 200`, `intent_source: "parser"`
- Zero-results recovery & spell check (#21): `q=sansung` → `did_you_mean: "samsung"`, `auto_corrected: true`
- B2B contract pricing (#22): `q=laptop&price_table_id=b2b-corporate&sort_by=price_asc` → prod-015 contract price `107.1` (10% discount on 119 basePrice)
- Session personalization (#20): session boost active, personas and badges working
- Regressions pass; logs clean (no ERROR or WARN).

## Issue #30 storefront fix + catalog browse: LIVE 2026-10-14

Source: clean `git archive` of **`2915faf`** (fix `d0ef229`, README `82c1ad9`, review record). The image change since `41af0a6` covers `routes.go` (storefront), `handlers.go` and `spanner_service.go` (catalog browse). No DDL; env unchanged; runtime SA `<RUNTIME_SA>`.
`cmd/sqlcheck`: **35 OK / 0 FAIL**, including the new catalog-browse cases: no filter → 15, `locale=pt-BR` → 5, `b2b-gold` + `price_asc` → 15. Deploy → **`psearch-serving-00026-7qm`**, then `update-traffic --to-latest` (100%, follows latest). Rollback target: `00025-5mx`.

Verification on `00026-7qm` (`<SERVICE_URL>`):
- `GET /` → 200. The single inline `<script>` (21.8 KB), extracted from the live HTML, **passes `node --check`** (Node v24). The page contains the **"Show All Catalog"** button (`showAllCatalog()`)
- `GET /api/search?q=&limit=100` and `q=*` → **all 15 products** (15 unique), **`retrieval_mode=catalog_browse`** (not `memory_fallback`), 4 facet groups, OOS (prod-012, prod-004) **last**
- `q=*&locale=pt-BR` → **5** products, all pt-BR
- `q=*&price_table_id=b2b-gold&sort_by=price_asc` → 15, ascending contract prices among available (109.48, 137.54, …), OOS last
- Regressions: tenis → prod-002 first; sansung → samsung (auto-corrected, prod-010); cep SP → 12 promises; b2b-corporate asc → prod-015 at 107.1; session `sess-qa-runner1` personalized; LTR baseline; acme → 0 results, `hybrid_v2`; intent "ate 200 reais" → 5; autocomplete `gel` → geladeira; health 200
- Logs: clean (no WARN, fallback, InvalidArgument, PermissionDenied, SCORE or ERROR)

## Issue #32 variant grouping (color/size): LIVE 2026-10-14

Source: clean `git archive` of **`8367cb0`** (backend `e5cd6b7`, sqlcheck cases `de966b8`, UI `de626d7`, decode fix `2442093`, review records). Env unchanged; runtime SA `<RUNTIME_SA>`.

| Step | Result |
|---|---|
| DDL `022_sku_variants.sql` (4 `ADD COLUMN` on `skus`: `color`, `size`, `stock`, `is_available`) | ✅ |
| DML `022b` (the 10 `-std` SKUs become the first variant) → `022c` (74 variant SKUs) | ✅ "modified 10 rows", "modified 74 rows". **skus = 89** (15 + 74), **color NOT NULL = 84**, size NOT NULL = 72, available = 58; prod-001…004 have 18 variants each, 005…010 have 2, 011…015 have 1; `regional_inventory` unchanged (57) |
| sqlcheck on `a65176d` (1st attempt) | ❌ 14 OK / 25 FAIL on every search/browse case: `failed to decode column 11 … type *[]services.variantRow cannot be used for decoding ARRAY[STRUCT]`. The Spanner Go client requires a slice of struct pointers. Not deployed; prod unaffected. Fix proven in a private copy (`[]*variantRow`) → 39/39 |
| sqlcheck on `8367cb0` (fix `2442093` + `spanner.NewRow` decoding test) | ✅ **39 OK / 0 FAIL**: `running shoes` azul/42 → 3; `laptop` prata → 3; browse preto/40 → 3; variant facets colors=[azul 2, branco 2, preto 2, grafite 1, prata 1], sizes=6 |
| Deploy → **`psearch-serving-00027-dmv`**, then `update-traffic --to-latest` | ✅ 100%, follows latest. Rollback target: `00026-7qm` |

Verification on `00027-dmv` (`<SERVICE_URL>`):
- `q=running+shoes&color=azul&size=42` → **3 results, all Footwear** (prod-001/002/003); each result's `variants` restricted to **azul/42**, available; `variant_count=1`, `available_colors=[azul]`, `available_sizes=[42]`
- Unfiltered: **prod-001 `variant_count=18`**, `available_colors=[azul, branco, preto]`, `available_sizes=[38…43]`
- Facets include **`colors`** and **`sizes`** (azul/branco/preto ×3; sizes 38–43 ×3)
- `color=roxo` → **0 results** (`hybrid_v2`)
- 40-char `size` → **400** `color must be <= 64 chars and size <= 32 chars`
- `q=*` browse → 15 products, 10 carrying variants (84 in total), colors/sizes facets; browse `color=preto&size=40` → prod-001/002/003 with preto/40 only
- Storefront `GET /` → 200; the inline script (25.0 KB) **passes `node --check`**; HTML has `<select id="color">` and `<select id="size">`, "Show All Catalog", the LTR toggle and the session field
- Regressions: #30 browse pt-BR → 5; tenis → prod-002 first; sansung → samsung → prod-010; cep SP → 12 promises; b2b-corporate asc → prod-015 at 107.1; session personalized; LTR baseline; acme → 0 results, `hybrid_v2`; autocomplete `gel`
- Logs: clean (no decode errors, WARN, fallback, InvalidArgument, SCORE or ERROR)

## Issue #33 Visual & Multimodal Search: LIVE 2026-10-14

Source: clean `git archive` of **`2b9a75e`** (code = `0af06f1`: backend `dbf2dff`, sqlcheck cases `0b5457e`, UI `0af06f1`; ADR-010 + review records). **No DDL**: photo queries search the existing `products.embedding_v2` (gemini-embedding-2, 1024-d). Env unchanged (`ACTIVE_EMBEDDING_VERSION=v2`); runtime SA `<RUNTIME_SA>`; Vertex AI location `global` in `<PROJECT_ID>`.

Pre-check (feasibility probe, read-only): `gemini-embedding-2` embeds JPEG images at `output_dimensionality=1024` in the same space as text (shoe photo → top-4 catalog matches all footwear; fridge photo → both refrigerators first). The `embedContent` API accepts **one content per call**; image + text parts in one Content give a single fused vector.

`cmd/sqlcheck` with `SQLCHECK_IMAGE=<probe shoe.jpg>`: **43 OK / 0 FAIL**, including `OK image embedding dims=1024` and 3× multimodal:
- image only → top `[prod-002, prod-003, prod-001, prod-007]` (footwear first)
- image + "mas em verde" → top `[prod-002, prod-003, prod-001, prod-015]`
- image + "running shoes" + `color=azul` (variants) → `[prod-001, prod-002, prod-003]`

Deploy → **`psearch-serving-00028-klr`**, then `update-traffic --to-latest` (100%, follows latest). **Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00027-dmv=100`.

| Check (`POST <SERVICE_URL>/api/search`, JSON with `image_base64`) | Result |
|---|---|
| a) shoe photo only | ✅ `retrieval_mode=multimodal_v2`, `effective_alpha=1`, `image_query=true`, `image_weight=1`; top prod-002 / prod-003 / prod-001 (all Footwear); 976 ms (cold) |
| b) shoe photo + `query="mas em verde"` | ✅ `multimodal_v2`, **`image_weight=0.6`**, **`effective_alpha=0.7`**; footwear top 3; 334 ms |
| c) fridge photo only | ✅ prod-011 "Geladeira Inverse…" **#1**. prod-012 (Smart French Door Refrigerator) has the **highest vector score** (0.01639) but is OUT_OF_STOCK, so the out-of-stock-last rule places it at #14 |
| d) repeat a) | ✅ **`image_cache_hit=true`**, 230 ms (vs 976 ms) |
| e) `image_mime_type=image/gif` | ✅ **400** `image_mime_type must be image/jpeg, image/png or image/webp` |
| f) `GET /api/search?q=running+shoes` / `q=` | ✅ `hybrid_v2` (no `image_query`) / `catalog_browse` (15), unchanged |
| g) storefront `GET /` | ✅ 200, contains `id="image-btn"`; the inline script (28.1 KB) passes `node --check` |
| Regressions | ✅ variants azul/42 → 3; sansung → samsung → prod-010; cep SP → 12; b2b-corporate asc → prod-015; session personalized; acme → 0 results, `hybrid_v2` |
| Logs | ✅ clean (no WARN, fallback, InvalidArgument, PermissionDenied, decode or ERROR) |

## Issue #34 clickable facets (storefront): LIVE 2026-10-14

Source: clean `git archive` of **`4e8951b`** (code = `21cfaa9`; image change = `internal/api/routes.go` storefront + its test). **No DDL, no SQL change.** Env unchanged; runtime SA `<RUNTIME_SA>`.
`cmd/sqlcheck` (`SQLCHECK_IMAGE` = probe shoe photo): **43 OK / 0 FAIL**. Deploy → **`psearch-serving-00029-7ss`**, then `update-traffic --to-latest` (100%, follows latest). **Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00028-klr=100`.

| Check (`<SERVICE_URL>`) | Result |
|---|---|
| a) `GET /` | ✅ 200; contains `id="active-filters"` and `function toggleFacet`; the inline script (33.7 KB) passes `node --check` |
| b) jsdom browser-level e2e against the live service (saved live HTML + live API) | ✅ **RESULT: ALL PASS**. Steps: category click (15 → 4 results, chip "Departamento: Footwear"); + brand stacks (`brand=Adidas`); brand toggled off; price 100–500; IN_STOCK; chip × removes the first filter; new query keeps `only_in_stock`; color azul; Show All → `q=&limit=100` |
| c) `GET /api/search?q=shoes&category=Footwear` | ✅ 4 results, Footwear only (prod-001/003/002/004) |
| c) `…&min_price=100&max_price=500` | ✅ 3 results (189.9, 149.5, 169.9), all in range |
| Regressions | ✅ `q=running shoes` → `hybrid_v2`; `q=` → `catalog_browse` (15); photo POST → `multimodal_v2` (footwear top 3); sansung → prod-010; cep SP → 12; azul/42 → 3 |
| Logs | ✅ clean |

## Issue #35 Sponsored Products (ADR-011): LIVE 2026-10-14

Source: clean `git archive` of **`5e6bf63`** in `/tmp/deploy-5e6bf63` (code = backend `0644c6f` + UI `a7e1054`; `5e6bf63` adds REVIEW.md only). Runtime SA `<RUNTIME_SA>`. **Env unchanged** (13 vars, same as `00029-7ss`). `AD_SLOTS` / `AD_MAX` / `AD_MIN_SIMILARITY` are not set, so the code defaults apply: `1,4,8` / `3` / `0.55`.

| Step | Result |
|---|---|
| DDL `023_ad_campaigns.sql` (online, 3 statements: `ad_campaigns` → `ad_campaigns_by_tenant_product` → `ad_clicks` INTERLEAVE IN PARENT `ad_campaigns` ON DELETE CASCADE) | ✅ each statement `done`; checked with `ddl describe` |
| DML `023b_seed_ad_campaigns.dml.sql` | ✅ 5 rows modified (nike-pegasus/prod-003, asics-nimbus/prod-002, apple-macbook/prod-007, acer-predator/prod-006, bose-qc/prod-008), all `active`, `spent_micros=0` |
| `cmd/sqlcheck` (archive, prod env) | ✅ **44 OK / 0 FAIL**, exit 0. Includes `OK ad_campaigns rows=5` (`SpannerAdStore.List`); all hybrid cases (`hybrid_v2`) run the `sponsored_matches` CTE; multimodal + facets OK |
| Deploy → **`psearch-serving-00030-pqq`**, then `update-traffic --to-latest` | ✅ 100%, follows latest |

**Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00029-7ss=100`. The new tables are additive, so `00029-7ss` ignores them.

| Check (`<SERVICE_URL>`) | Result |
|---|---|
| `GET /api/health` / `GET /api/info` | ✅ 200 `healthy`, `spanner`; info: v2, `gemini-embedding-2`, 1024-d, 20535 products |
| `GET /api/search?q=running+shoes&alpha=0.5` | ✅ `hybrid_v2`, 20 results, **`sponsored_count=2`**: slot 1 `prod-003` (`camp-nike-pegasus`) and slot 4 `prod-002` (`camp-asics-nimbus`), each with `sponsored=true` and an `ad_click_id` (`clk-…`) |
| `…&ads=false` | ✅ `sponsored_count=0`, 0 flagged results, 20 total |
| `POST /api/ads/click` `{camp-nike-pegasus, clk-6c0725ddebdec5d8}` | ✅ 200 `charged=true`, `cost_micros=400000`, `spent_micros=400000`, `remaining_micros=4600000` |
| Replay of the same `click_id` | ✅ 200 `charged=false`, `cost_micros=0`, `reason=duplicate_click`, spend unchanged |
| `GET /api/ads/campaigns` | ✅ `camp-nike-pegasus` `spent_micros` 0 → **400000**; other 4 still 0. `ad_clicks` has exactly 1 row (400000) |
| `GET /` storefront | ✅ 200. jsdom render against the live API (`q=running shoes`): 20 cards, **2 `.sponsored-badge` "Sponsored · Patrocinado"**. Clicking a sponsored card fires the `/api/ads/click` beacon with `{campaign_id, click_id}` (stubbed in the test, so no extra charge) |
| Logs (`00030-pqq`, severity ≥ ERROR) | ✅ none |

Note: the smoke test left 1 billed click on `camp-nike-pegasus` (400000 micros, about 0.40 in currency units). This is intentional evidence and was not reverted.

## Issue #39 ScaNN ANN vector index (ADR-012): DEPLOYED 2026-10-30, ANN NOT ACTIVE

Source: clean `git archive` of **`350f63b`** in `/tmp/deploy-39` (code `fecadde`; `350f63b` adds REVIEW.md only). The shared tree held uncommitted #40/#41 work, and none of it was deployed. Runtime SA `<RUNTIME_SA>`. **Env unchanged.** `USE_VECTOR_INDEX` is not set, so the code default applies (`true` on v2). `/api/info` reports `use_vector_index=true` and `ann_num_leaves_to_search=20`.

| Step | Result |
|---|---|
| Pre-check (`ddl describe`) | ✅ `embedding_v2 ARRAY<FLOAT32>(vector_length=>1024)` present; all STORING columns exist; no vector index yet. 2515 of 20535 rows have `embedding_v2` |
| DDL `024_products_vector_index.sql` (`--async`) | ✅ LRO `_auto_op_8e2272d28739ab64` started 16:59:39Z and finished **17:02:11Z** at `progressPercent=100` |
| Deploy → **`psearch-serving-00031-n6v`** | ✅ 100%, follows latest (`latestRevision: True`) |
| `cmd/sqlcheck` (archive, prod env) | ✅ 46 OK / 0 FAIL, exit 0. ⚠️ All hybrid cases ran on the exact fallback (same `InvalidArgument` WARN as production) |
| `backfill-embeddings -dry-run -limit=50` | ❌ exits 1 with the default `-embed-batch=32`: `the embedContent API for this model only supports one content at a time` (gemini-embedding-2). ✅ With `-embed-batch=1` it embedded 50/50 at 48.2 docs/s (8 workers, `written=0`) |

| Check (`<SERVICE_URL>`) | Result |
|---|---|
| `GET /api/health` | ✅ 200 `healthy`, `spanner` |
| `GET /api/info` | ✅ 200. Fields: v2, 1024-d, 20535 products, `use_vector_index=true`, `vector_index=products_embedding_v2_idx`, `vector_index_used=true`. ⚠️ This value reflects config/cooldown state, not actual query execution, so it disagrees with `/api/search` |
| `GET /api/search?q=running+shoes&alpha=0.5` | ✅ 200, `hybrid_v2`, 20 results, `sponsored_count=2`. ❌ **`vector_index_used=false`** on every request, including requests made after the LRO finished (`trail running shoes`, `wireless headphones` also `false`) |
| Logs (`00031-n6v`) | ⚠️ `WARN: ScaNN vector index unavailable (spanner: code = "InvalidArgument", desc = "The use of function APPROX_COSINE_DISTANCE is not supported in this query with forced index products_embedding_v2_idx. Try restructuring the query to remove complex filters before the ORDER BY clause ...")`. First seen 17:03:15Z, after the index finished building. It repeats after each 60 s cooldown |

**Root cause (found by probing Spanner directly with a constant 1024-d query vector):** the index works. The query shape is the problem.

| ANN probe (`FORCE_INDEX=products_embedding_v2_idx`, `ORDER BY APPROX_COSINE_DISTANCE`) | Result |
|---|---|
| Only `embedding_v2 IS NOT NULL` | ✅ rows returned |
| + filters on STORING columns (`is_visible`, `tenant_id`, `price`), including `COALESCE`/`IFNULL` and `(@p = '' OR col = @p)` forms | ✅ |
| `ORDER BY distance` alias, wrapped in `ARRAY(SELECT AS STRUCT ...)` | ✅ |
| + `'web' IN UNNEST(IFNULL(channels, ...))` (`channels` is **not** a STORING column) | ❌ same `InvalidArgument` |

`catalogFilterSQL`, used inside the `ann` CTE, filters on `channels` and on effective-price, assortment, variant and delivery predicates, which rely on non-stored columns or subqueries. Spanner does not accept these before an ANN `ORDER BY`. Possible fixes: (a) keep only STORING-column predicates in the ANN CTE, over-fetch candidates, and apply the remaining `catalogFilterSQL` in the outer query, or (b) add the needed columns (for example `channels`) to `STORING`, which means rebuilding the index. The subquery-based filters still need (a). Also, `/api/info.vector_index_used` should reflect what queries actually do.

**Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00030-pqq=100`. The vector index is additive, and `00030-pqq` does not use it. Search results are unaffected, because the exact fallback matches the pre-#39 behaviour.

**Status:** #39 stays open until `vector_index_used=true` shows up in `/api/search`.

## Issue #40 Pagination and sorts (ADR-012 part 2): DEPLOYED 2026-10-30, `total_found` DEFECT

Source: clean `git archive` of **`2b3d49b`** in `/tmp/deploy-40` (code `a527cda` + UI `79c367f`, which also contains the #41 multi-select UI; `2b3d49b` adds REVIEW.md only). Backend's uncommitted #39-fix edits in the shared tree were not deployed. **No DDL. Env unchanged.**

| Step | Result |
|---|---|
| `cmd/sqlcheck` (archive, prod env) | ✅ 50 OK / 0 FAIL, exit 0 |
| Deploy → **`psearch-serving-00032-46c`** | ✅ 100%, follows latest |

| Check (`<SERVICE_URL>`) | Result |
|---|---|
| `GET /api/health` | ✅ 200 |
| `q=protetor&limit=12&page=1` / `page=2` / `page=3` | ✅ 12 organic results each; pages 1∩2 and 2∩3 have **0 shared IDs**; `total_found=100` on all three (> 24); `has_next=true`; `total_pages=9`; page 2 `sponsored_count=0` |
| `sort_by=popularity` / `rating_desc` (`protetor`, `shoes`, `running shoes`, `fone`, `limit=100`) | ✅ 200; order differs from `relevance`; **OOS items last** (`running shoes` OOS at index 99; `shoes` at 98–99) |
| `sort_by=bogus` / `offset=2000` / `page=0` / `limit=0` | ✅ 400 with clear messages (`invalid sort_by ...`, `offset must be between 0 and 1000`, `page must be >= 1`, `limit must be between 1 and 100`) |
| Single-select facets (no #41 regression) | ✅ `category=Footwear` 4 results, Footwear only; `brand=Nike` 1 Nike; `min_price=100&max_price=500` 3 results (149.50, 189.90, 169.90). `brands=Nike&brands=Adidas` returns 20 results unfiltered (backend #41 not live yet, as expected) |
| Regression `q=running+shoes&alpha=0.5` | ✅ `hybrid_v2`, 20 results, `sponsored_count=2` |
| Storefront `/` (jsdom against live API, `protetor`) | ✅ "Showing 1-24 of 100 results"; pager `Prev(disabled) 1 2 3 4 5 Next`; Next → "Showing 25-48 of 100 results"; sort select offers `relevance, price_asc, price_desc, popularity, rating_desc` |

**❌ Defect: `total_found` and `has_next` are only correct while `offset + limit <= 100`.** `handlers.go` fetches `candidateLimit = max(100, offset+limit)` (capped at `maxSearchWindow = 500`). The hybrid path returns `TotalFound: len(res)`, and because the vector leg always fills `@limit`, this equals `candidateLimit`:

| `q=protetor&limit=12` | `total_found` | results | `has_next` | `total_pages` |
|---|---|---|---|---|
| page 1 / 4 / 8 | 100 | 12 | true | 9 |
| page 9 | **108** | 12 | **false** | 9 |
| page 10 / 20 / 40 | **120 / 240 / 480** | 12 | **false** | 10 / 20 / 40 |
| offset 492 / 504 / 900 | 500 | 8 / **0** / **0** | false | 42 / 43 / 76 |

The count changes from page to page; `has_next=false` while 40 more pages are reachable; offsets 500–1000 pass validation (≤ 1000) but return empty pages. For comparison, only 80 rows match `protetor` lexically; the rest of the hybrid results come from the vector leg. The storefront pager looks correct on the first pages only because they stay inside the 100-row window.

**Rollback:** `gcloud run services update-traffic psearch-serving --project=<PROJECT_ID> --region=us-central1 --to-revisions=psearch-serving-00031-n6v=100` (no schema changes).

**Status:** #40 commented, **not closed**; lead decides.

## Issue #39 hotfix: ScaNN ANN LIVE, full backfill, index rebuilt (2026-10-30)

Source: clean `git archive` of **`f32ecf3`** in `/tmp/deploy-39fix` (fix `64cee37` on top of #40 `2b3d49b`). **No new DDL file. Env unchanged.**

| Step | Result |
|---|---|
| `cmd/sqlcheck` (archive, prod env) | ✅ 50 OK / 0 FAIL; **no** `ScaNN vector index unavailable` WARN (it appeared before the fix) |
| Deploy → **`psearch-serving-00033-4n8`** | ✅ 100%, follows latest |
| ANN on the existing (2.5k-vector) index | ✅ `/api/search` `vector_index_used=true` for `running shoes`, `protetor solar`, `wireless headphones`; `/api/info` `vector_index_used=true`; revision logs show no `InvalidArgument`/ScaNN WARN |
| `backfill-embeddings -dry-run -limit=50` (**default flags**) | ✅ 50/50, 2 batches, 63 docs/s, `written=0` |
| `backfill-embeddings` (full, default flags: 8 workers, embed-batch 32, write-chunk 200) | ✅ **embedded=18020 written=18020**, 564 batches, 91 chunks, about 2.3 min (129 docs/s), 0 errors. Coverage **20535 / 20535** (was 2515) |
| Recall probe before the rebuild (6 product vectors, ANN `num_leaves_to_search=20` vs exact `COSINE_DISTANCE`, top 20) | mean **recall@20 = 0.975**, top-1 6/6 |
| `DROP VECTOR INDEX products_embedding_v2_idx` → recreate with the same DDL as `024` (STORING unchanged) | ✅ drop 17:34:55–17:35:04Z; create LRO `_auto_op_812837e126700655` 17:35:05 → **17:38:07Z**, 100% |
| During the rebuild | ✅ `/api/search` 200 `hybrid_v2`, 20 results, `vector_index_used=false`; `/api/info` also `false` (it now reflects real query execution) |
| Recall probe after the rebuild | ✅ mean **recall@20 = 0.992** (5 of 6 probes at 1.00, one at 0.95), top-1 6/6 |
| ANN after the rebuild | ✅ `vector_index_used=true` on all 3 queries (2 rounds) and in `/api/info`; warm latency 0.5–2.2 s |

Relevance sanity check (`ads=false`, top 5):

| Query | `alpha=0` (lexical) | `alpha=0.5` (hybrid, ANN) | `alpha=1` (ANN only) |
|---|---|---|---|
| running shoes | UltraBoost Pro, then lexical noise (only 1 lexical match) | UltraBoost Pro, **Tênis de Corrida Nimbus Cloud**, **Air Zoom Pegasus Trail Runner**, … | same footwear top 3 |
| protetor solar | ADCOS / L'Oréal *PROTETOR SOLAR* … | Nivea, Sundown, L'Oréal, Actine *PROTETOR SOLAR* … | **PHOTOPROT FPS 100**, **FILT. SOLAR FPS 70**, NIVEA FPS 50 (sunscreens without the word "protetor") |
| wireless headphones | QuietComfort Ultra, then noise | QuietComfort Ultra, **FONE DE OUVIDO**, **Fone de Ouvido Sem Fio WH-1000XM5**, … | same |

Note: `alpha=0` responses also report `vector_index_used=true` (the vector leg still runs, with weight 0). This is cosmetic.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00032-46c=100`. The rebuilt index and the backfilled embeddings are additive.

**Status:** #39 **closed**.

## #40 hotfix + #41 Disjunctive multi-select facets (ADR-013): DEPLOYED 2026-10-30

Source: clean `git archive` of **`952cf20`** in `/tmp/deploy-952` (#40 fix `5076d80`, #41 backend `6ce3b8c`, UI `79c367f` + #42 UI `c20964d`, and the #39 fix). **No DDL. Env unchanged.**

| Step | Result |
|---|---|
| `cmd/sqlcheck` (archive, prod env) | ✅ 54 OK / 0 FAIL (new: `count q="running shoes" total_found=1`, `count q="*" total_found=20535`) |
| Deploy → **`psearch-serving-00034-wnr`** | ✅ 100%, follows latest; `vector_index_used=true` |

### #41: all checks pass ✅

| Check (`<SERVICE_URL>`, `ads=false` unless noted) | Result |
|---|---|
| `q=shoes&brands=Nike` | ✅ only `prod-003` (Nike). `facets.brands` still lists siblings with counts > 0 (Adidas 1, Asics 1, Apple 1, Bose 1, Vichy 2, …): each dimension's facet ignores its own filter. With ads on, `prod-003` appears as the **sponsored** result (active campaign) and is removed from the organic list |
| `q=shoes&brands=Nike,Asics&sizes=42` (also as repeated `brands=` params) | ✅ `prod-003` (Nike) + `prod-002` (Asics). Brand facet narrows to footwear (Adidas/Asics/Nike, 1 each), because the size filter applies to the brand facet |
| `q=shoes&sizes=42` | ✅ 3 results (Adidas, Nike, Asics) |
| Legacy `q=shoes&brand=Nike` | ✅ unchanged: 1 result, Nike |
| 21 values in `brands` / 20 values | ✅ 400 `too many brands values (max 20)` / 200 |
| Storefront (jsdom against the live API, `shoes`) | ✅ 37 checkboxes; ticking Nike, then Asics sends `brands=Nike%2CAsics`; pills `Brand: Nike ×`, `Brand: Asics ×`, `Clear all`; "Showing 1-2 of 2 results"; both stay checked; Adidas sibling still offered. **Clear all** → request without `brands`, no facet checked (only the separate "Smart query" toggle stays on), no filter pills, "Showing 1-21 of 100 results". **0 JS errors.** #42 attribute facets are not rendered (`facets.attributes` is absent; backend not live yet) and cause no error |

### #40 hotfix: partly fixed ❌, #40 stays open

| Query (`limit=12`) | Lexical count | Result |
|---|---|---|
| `q=po` | 249 | ✅ `total_found=249` on pages 1, 2, 9 and 20; `total_pages=21` |
| `q=` (browse) | 20535 | ✅ `total_found=20535` on pages 1 and 2 |
| `q=running+shoes&alpha=0.5` (lead's check) | 1 | ✅ page 1 `total_found=100`, `has_next=true`, 10 organic + 2 sponsored; page 2 has 12 results, disjoint from page 1, `sponsored_count=0`; page 3 has 12 results |
| `q=protetor` | 80 | ❌ pages 1–8 `total_found=100`; **page 9 → 108, `has_next=false`**; page 10 → 120; offset 480 → 492; offset 492 → 500 (8 results); **offsets 504 and 900 → `total_found=100`, empty** |
| `q=po` past the true count | 249 | ❌ page 21 → 252, page 22 → 264, still 12 results each |

**Cause** (`handlers.go`): `organicTotal = max(outcome.TotalFound, len(results))`. `candidateLimit` still grows as `offset + limit` (up to 500), and the ANN leg always fills it, so `len(results)` beats the lexical count whenever that count is below the window. For `protetor` the total therefore grows with each page and drops back to 100 once offset ≥ 500 empties the window. **Possible fix:** fix the window per query (for example `window = min(max(lexical_count, 100), 500)`, independent of offset), fetch `candidateLimit = window`, report `total_found = window`, and return an empty page with that same total when offset ≥ window.

Other notes: page 1 shows "Showing 1-21" because the 3 sponsored products occupy slots and are removed from the organic list. No organic result is skipped: comparing with `ads=false` ranks 0–44, only the 3 sponsored items are absent from pages 1+2 organic, and they are shown in sponsored slots. This is cosmetic.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00033-4n8=100` (no schema changes).

**Status:** #41 **closed**; #40 **open** (comment posted).

## #42 Product attributes + #44 Similar products + #40 fix-2: DEPLOYED 2026-10-30

Source: clean `git archive` of **`6b0aee9`** in `/tmp/deploy-6b0` (#42 `c030dfd`, #44 `a540033` + UI `6800cf9`, #40 fix-2 `1923d3b`). Backend's uncommitted #43 work in the shared tree was not deployed. **Env unchanged.**

| Step | Result |
|---|---|
| DDL `025_product_attributes.sql` (3 statements: `ALTER products ADD attributes JSON` → `CREATE TABLE product_attributes … INTERLEAVE IN PARENT products ON DELETE CASCADE` → `CREATE INDEX product_attributes_by_tenant_name_val`) | ✅ each `done`; checked with `ddl describe` |
| DML `025b` from `6b0aee9` (read-only dry run of its SELECT) | ❌ `UNIMPLEMENTED: Unsupported built-in function: ROW_NUMBER`. **Not applied.** It also gave sunscreen attributes to the shoes prod-002/prod-003. Reported to lead → fixed in `d1e3f9b` |
| DML `025b` from **`d1e3f9b`** (`MOD(ABS(FARM_FINGERPRINT(product_id)),3)`) | ✅ dry run: 307 rows, 0 shoe/sunscreen rows → applied: **307 rows modified**. fps/skin_type/form 85 each; prod-002/003 have only surface/cushioning/drop |
| `cmd/sqlcheck` (archive, prod env) | ✅ **57 OK / 0 FAIL** (new: `similar/substitutes rows=3`) |
| Deploy → **`psearch-serving-00035-x4l`** | ✅ 100%, follows latest |

### #42 Product attributes: all checks pass ✅

| Check | Result |
|---|---|
| `q=notebook` `facets.attributes` | ✅ `ram` (16GB 2, 32GB 2), `storage` (1TB 2, 512GB 2), `processor`, `screen_size`, `connectivity`… Results carry `dynamic_attributes` (prod-007 Apple M3/16GB/512GB/15") |
| `q=protetor` | ✅ `fps` (FPS 50: 25, FPS 30: 19, FPS 70: 13), `skin_type` (Pele Oleosa / Pele Mista / Todos os Tipos), `form` (Gel-Creme / Loção / Fluido) |
| `q=tenis` | ✅ **no fps facet**; `surface` (Road 2, Track 1, Trail 1), `cushioning`, `drop`. prod-003 attributes = Responsive / 9mm / Trail |
| `q=notebook&attr.ram=16GB,32GB` | ✅ 4 results (prod-007 16GB, prod-005 32GB, prod-006 32GB, prod-010 16GB) |
| `q=notebook&attr.ram=16GB` | ✅ 2 results (prod-007, prod-010); RAM facet still shows **32GB (2)** with 16GB `selected: true`; other facets narrow |
| `attr.bad!key=1` | ✅ 400 `invalid attribute key "bad!key" (must match ^[a-zA-Z0-9_-]{1,64}$)` |
| `PUT /api/products/prod-005/attributes` without a token / wrong `X-Admin-Token` / wrong Bearer | ✅ 403 `forbidden` ×3; Spanner row unchanged (ram = 32GB) |

### #44 Similar products: all checks pass ✅

| Check | Result |
|---|---|
| `GET /api/products/prod-004/similar?mode=substitutes&limit=4` (prod-004 OOS sneaker) | ✅ 200: prod-003, prod-002, prod-001, all **Footwear**, all **IN_STOCK**, seed excluded |
| `prod-012/similar?mode=substitutes` (OOS refrigerator) | ✅ prod-011, prod-013 (Appliances, IN_STOCK) |
| `mode=co_viewed` for prod-005 before → after posting session events (`click` prod-005, then `click` prod-013, session `sess-deploy44-1793385256`) | ✅ before: prod-013 absent, no `co_view_count`. After: **prod-013 ranked first with `co_view_count=1`**, similarity 0.86. (The reverse, prod-013 → prod-005, is not boosted; co-visitation is directional) |
| Unknown id `nope-999` | ✅ 404 `product not found` |
| `mode=bogus` | ✅ 400 `invalid mode "bogus" (want similar \| substitutes \| co_viewed)` |

Note: the smoke test left 2 click events in session `sess-deploy44-1793385256`. This is intentional evidence and was not reverted.

### Storefront (jsdom against the live API), 0 JS errors ✅

| Flow | Result |
|---|---|
| `notebook` | RAM facet with 16GB/32GB checkboxes; **7 spec badges** (`Processor: Intel Core i9 · Ram: 32GB · Screen Size: 16"` …). Ticking 16GB sends `attr.ram=16GB` → "Showing 1-1 of 2 results" (prod-007 occupies the sponsored slot); 32GB sibling still offered |
| `protetor` / `tenis` | FPS 50 + Pele Oleosa facets / Road surface facet, **no FPS** on tenis |
| `urban sneaker` (locale `en-US`) | OOS card shows **See In-Stock Alternatives** → calls `prod-004/similar?mode=substitutes&limit=4&only_in_stock=true` → panel lists Air Zoom Pegasus and UltraBoost Pro (IN_STOCK, Footwear); button changes to "Hide In-Stock Alternatives" |
| Product details | Similar Products + Frequently Viewed Together carousels load (`mode=similar` / `mode=co_viewed`) |

### #40 fix-2: totals fixed ✅, pages overlap when W > 100 ❌, #40 stays open

| Query (`limit=12`) | Result |
|---|---|
| `protetor` (W=100) | ✅ `total_found=100` on pages 1/2/8/9/10/20/21/22 and offsets 492/504/1000; page 9 = 4 results, `has_next=false`; pages ≥ 10 empty with the same total; `total_pages=9`. **100 organic IDs across 9 pages, 100 unique** |
| `running shoes` (W=100) | ✅ same pattern (total 100, page 9 = 4, then empty) |
| `po` (W=249) | ✅ `total_found=249` on every page/offset; page 21 = 9 results, `has_next=false`; page 22+ empty. ❌ **249 organic IDs across 21 pages, only 235 unique**: 14 results appear twice (for example `med-6860` at positions 118 and 120, `med-19154` at 129 and 132) and 14 never appear |

**Cause:** `handlers.go` still sets the retrieval `candidateLimit = max(100, offset+limit)`; only the response is truncated to W. Both search legs (`fts`/`ann` CTEs) are cut at that `LIMIT` before RRF, so once offset+limit > 100 the ranking depends on the page. For `po`: `offset=108,limit=12` ≠ `offset=108,limit=24`[:12], and both differ from a single-window slice. Identical requests are deterministic. **Fix:** retrieve with `candidateLimit = W` on every page (compute the lexical count first, or retrieve `maxSearchWindow` and truncate).

### Separate finding (existed before this deploy): OOS products cannot be reached in unfiltered searches

There are 6579 visible OOS products, yet `shoes`, `geladeira` and `urban sneaker` (`limit=100`, `only_in_stock` unset) return **no** OOS items. prod-004 is the **only lexical match** for "urban sneaker" and is still missing. It appears only when the candidate pool is small (for example `locale=en-US` → position 8). Running the `952cf20` and `6b0aee9` builds locally against prod gives the same result, so neither the 00034 nor the 00035 code caused it. Likely cause: unavailable-last ordering is applied before the candidate `LIMIT`. Since the full `embedding_v2` backfill (#39), the ANN leg alone supplies ≥ 100 in-stock candidates, so OOS rows are always cut. On 00032 (2.5k embeddings), OOS items still appeared at positions 98–99. Consequence: the #44 "See In-Stock Alternatives" button rarely appears in normal browsing.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00034-wnr=100`. 025/025b are additive; 00034 ignores them.

**Status:** #42 **closed**, #44 **closed**, #40 **open**.

## #43 Search analytics + #40 fix-3 + #45 OOS reachability: DEPLOYED 2026-10-30

| Step | Result |
|---|---|
| Latency baseline on `00035-x4l` (server `latency_ms`, 12 requests each, pages 1–5) | p50: `po` 1712 ms, `protetor` 1437 ms, `running shoes` 1184 ms |
| DDL `026_search_events.sql` (4 statements) | ✅ each `done`; `ddl describe` shows both tables with `ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY))` and both indexes |
| `cmd/sqlcheck` `90e289a` / `739c62e` | ✅ 58/0 and 59/0 |
| Deploy `90e289a` → `00036-nf2` | ❌ `terminated: Application failed to start … Application exec likely failed`, startup TCP probe failed after about 10 s, no application logs. Cloud Build normal (`CGO_ENABLED=0` static binary, same Dockerfile/base images as 00035); the same build starts locally in 4 s. **No traffic impact**: 00035 kept serving 100% |
| Deploy `739c62e` (90e289a + #45 `ae3f046`/`b28f6b4`) → **`psearch-serving-00037-g74`** | ✅ 100%, follows latest. The 00036 failure was transient |

### #40 fix-3 + #45: walking every page (`limit=12`, `ads=false`)

| Query | `total_found` (all pages + next empty page) | Pages | Last page | IDs / unique | OOS | First OOS position | OOS strictly last |
|---|---|---|---|---|---|---|---|
| `po` (W=249) | {249} | 21 | 9 results, `has_next=false`; page 22 empty | **249 / 249** | 61 | 188 | ✅ |
| `protetor` (W=100) | {100} | 9 | 4, `has_next=false` | 100 / 100 | 18 | 82 | ✅ |
| `urban sneaker` | {100} | 9 | 4, `has_next=false` | 100 / 100 | 26 | 74 (**prod-004 at 74**, first OOS) | ✅ |
| `shoes` | {100} | 9 | 4 | 100 / 100 | 17 | 83 (prod-004 at 83) | ✅ |
| `geladeira` | {100} | 9 | 4 | 100 / 100 | 10 | 90 | ✅ |

With ads on: `po` 249/249 organic unique; `running shoes` 98 organic unique + 2 sponsored on page 1. Storefront: OOS cards appear on deep pages, and "See In-Stock Alternatives" works on them (#44 flow, verified on 00035).

**Latency follow-up** (server `latency_ms` p50, same method as the baseline): `po` **1712 → 3939 ms** (p90 3737 → 6084), `protetor` 1437 → 1934 ms, `running shoes` 1184 → 1334 ms. The `po` increase is larger than the added COUNT alone: every page now retrieves the full W=249 window (previously 100 for pages 1–8). Backend is optimizing this separately.

### #43 Search analytics: all checks pass ✅

| Check | Result |
|---|---|
| `GET /api/search?q=notebook&experiment_id=ltr_v2` | ✅ `query_id=imp-1793387978033049975-df6f961d267496f5`, `experiment_id=ltr_v2` |
| `POST /api/analytics/events` click `dep43-click-1` (prod-005, `position=2`) | ✅ 200 `recorded=true, duplicate=false` |
| purchase `dep43-purchase-1` (`revenue_micros=1299000000`) | ✅ 200 `recorded=true` |
| Replay of the click | ✅ 200 **`duplicate=true, recorded=false`**; summary still shows 1 click |
| `GET /api/analytics/summary?window_hours=24` | ✅ `total_clicks=1`, `total_conversions=1`, `ctr=0.0075` (1/133), `mrr_at_10=0.5`, `attributed_revenue_micros=1299000000`. Experiments: **`ltr_v2`** searches 1, clicks 1, conversions 1, CTR 1.0, **MRR@10 0.5**, revenue 1299000000; `default` 132 searches |
| `q=xyznonexistent99`, then summary | ✅ `top_zero_result_queries=[{xyznonexistent99, 1}]`, `zero_result_searches=1`. Note: the search response itself shows `total_found=100` (the ANN leg fills the window); telemetry classifies by lexical zero |
| Spanner | ✅ `search_impressions` 133 rows (19:12:24 → 19:19:40Z); our impression has `query_norm=notebook`, `ltr_v2`, `hybrid_v2`, `total_found=100`, 10 organic `result_ids`. `search_conversions` holds `dep43-click-1` (click, pos 2) + `dep43-purchase-1` (purchase, 1299000000) |
| `POST /api/analytics/rollup` on prod, no token / wrong `X-Admin-Token` | ✅ 403 `forbidden` ×2; `products` signals unchanged (prod-001 clicks 792, ctr 0.186) |
| Admin rollup | Not run against prod Spanner (lead's decision: it would rewrite `clicks/orders/ctr/conversion_rate` catalog-wide). Local **memory-mode** server (`USE_MEMORY_STORE=true`, nonexistent project/instance, `backend_mode=memory` confirmed before calling): no token 403; `X-Admin-Token` → **200**, `products_updated=14`, `window_days=30`, **no NaN/Inf**. Note that with near-zero impressions the smoothing flattened prod-001 `ctr` 0.186 → 0.0476, which confirms the risk of running it on prod. `ADMIN_TOKEN` stays unset on the service (user decision) |
| Storefront (jsdom against the live API, analytics POSTs intercepted so nothing extra reached prod) | ✅ experiment selector (`control`, `ltr_v2`, `ltr_boost`) → search sends `experiment_id=ltr_v2`. **"+ Clique"** on an organic card → `/api/analytics/events` `{event_type: click, query_id, position: 2, experiment_id: ltr_v2}` + `/api/session/events`. **"+ Cart"** → `add_to_cart` beacon (sponsored cards also fire `/api/ads/click`). Analytics drawer (`window_hours=168`): CTR 0.7%, MRR 0.500, zero-result 0.7%, conversion 0.7%, zero query `xyznonexistent99`, A/B rows default vs ltr_v2. **0 JS errors** |

Note: the smoke test left 1 click + 1 purchase (1,299 currency units) on impression `imp-1793387978033049975-df6f961d267496f5` (`ltr_v2`), plus verification impressions. This is intentional evidence; the rows expire through the 90-day TTL. A leftover local `srv-6b0` process (from the #45 investigation; read-only searches with pre-#43 code, so no analytics writes) was found and stopped.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00035-x4l=100`. 026 is additive.

**Status:** #40, #43, #45 **closed**.

## #46 Latency fix + #47 Idempotent rollup: DEPLOYED 2026-10-30

| Step | Result |
|---|---|
| DDL `027_products_signal_baseline.sql` (4 `ALTER TABLE products ADD COLUMN`) | ✅ each `done`, checked with `ddl describe`. 00037 code still passed sqlcheck 59/0 afterwards (additive) |
| Transient during the DDL window | ⚠️ one HTTP 503 on 00037 at 20:04:47Z (19.4 s, "connection to the instance had an error"), then an AUTOSCALING instance start; no OOM or crash logged; no recurrence |
| Pre-deploy latency on `00037-g74` (n=20, pages 1–5, server `latency_ms`) | p50 `po` 2994 ms, `protetor` 1515 ms, `running shoes` 1269 ms |
| sqlcheck gate on the first GO snapshot **`67fe6b0`** | ❌ **29 OK / 34 FAIL**: all hybrid/lexical searches, ScaNN and perf `po` got `InvalidArgument: SEARCH is not supported in this query … unsupported query shapes`. **Deploy held; 00037 kept serving.** Bisected to `3e424dd` (its parent `91cdd57` passes): `effectivePriceSQL` changed to `IF(@price_table = '', alias.price, COALESCE((SELECT … product_prices …), alias.price))`. That expression sits inside `catalogFilterSQL` in the same `WHERE` as `SEARCH()`. Proof: `67fe6b0` with only `pricing.go` reverted passes 63/0; reverting the `IF(@sort_by…)` sort change alone did not help |
| Fix `10f6761` (Go-side SQL specialization: plain `alias.price` when there is no price table, original `COALESCE` subquery otherwise; min/max predicates omitted when unset; no `IF` in the SEARCH `WHERE`) | ✅ gate 63/0, covering b2b-gold ×2 / b2b-corporate / vip-club price tables (hybrid + browse), 5 min/max price cases, popularity/rating_desc/price sorts. Capped count equals exact for `running shoes` 1, `protetor` 80, `po` 249; browse capped 501 / exact 20535 |
| Gate on the final snapshot **`8b28300`** (= `10f6761` + REVIEW.md) | ✅ **63 OK / 0 FAIL** |
| Deploy → **`psearch-serving-00038-mpb`** | ✅ 100%, follows latest; logs clean (no ERROR, no SEARCH rejection) |

### #46 Latency: target met ✅ (target: within ~10% of the pre-fix-3 baseline)

| Query (n=20, `limit=12`, pages 1–5, server `latency_ms`) | Pre-fix-3 (00035) | 00037 (fix-3) | **00038** | vs pre-fix-3 |
|---|---|---|---|---|
| `po` (W=249) | 1712 ms | 2994 ms (earlier run 3939) | **400 ms** (p90 516) | **−77%** |
| `protetor` | 1437 ms | 1515 ms | **185 ms** (p90 245) | **−87%** |
| `running shoes` | 1184 ms | 1269 ms | **152 ms** (p90 184) | **−87%** |
| 20 never-seen queries (cache-cold) | — | — | **p50 376 ms**, p90 780, max 932 | all `hybrid_v2`, `vector_index_used=true`, full pages |

### Invariants and functional checks ✅

| Check | Result |
|---|---|
| #40/#45 page walk (`ads=false`) | `po` totals {249}, 21 pages, last page 9 results `has_next=false`, next page empty, **249/249 unique**, 59 OOS from position 190, OOS strictly last. `protetor` 100/100 (19 OOS from 81). `urban sneaker` 100/100, **prod-004 at 73** (first OOS). `shoes` 100/100 (prod-004 at 83). `geladeira` 100/100. All totals constant |
| Count-cache keying (filtered vs unfiltered) | `q=shoes` 100 → `sizes=40,42` **7** → `sizes=42` **3** → `cep=01310100` **9** → `cep=69000000` **0** → `q=shoes` again **100** |
| Sponsored slot attributes | `running shoes`: sponsored prod-003 (Trail/Responsive/9mm) and prod-002 (Road/Maximum/8mm) carry `dynamic_attributes` + `attributes`; `notebook`: sponsored prod-006 / prod-007 carry ram/storage/processor/screen_size |
| `price_table_id=b2b-gold` | ✅ contract prices: prod-007 **1147.18** (list 1399.00), prod-005 1229.18 (1499.00), prod-006 1353.00 (1650.00), prod-003 137.54 (149.50), matching `product_prices` exactly. `min_price=100&max_price=170` filters on the contract price |
| `POST /api/analytics/rollup` (no token, wrong `X-Admin-Token`, and `?dry_run=true` without a token) | ✅ 403 `forbidden` ×3. **No live run.** `baseline_ctr IS NOT NULL` = **0** rows (baseline not snapshotted) |

**Rollback:** `update-traffic --to-revisions=psearch-serving-00037-g74=100`. 027 is additive; 00037 ignores the baseline columns.

**Status:** #46 **closed** (latency target met by a wide margin), #47 **closed**.

## #49 Brasileirão club colour themes: DEPLOYED 2026-11-03

Source: clean `git archive` of **`ff5f8eb`** in `/tmp/deploy-ff5` (feature `6509c0b`; `ff5f8eb` adds REVIEW.md; the snapshot also carries the #48 docs commits). Since `8b28300` the only serving changes are `internal/api/routes.go` (embedded storefront) and `storefront_test.go`. **No DDL. Env unchanged.**

| Step | Result |
|---|---|
| sqlcheck gate (real Spanner) | ✅ **63 OK / 0 FAIL**; `go test ./internal/api -run 'Storefront\|Theme'` ok |
| Deploy → **`psearch-serving-00039-x8j`** | ✅ 100%, follows latest |

| Check (`<SERVICE_URL>`, jsdom against the live service for client-side behaviour) | Result |
|---|---|
| `GET /` | ✅ 200; `#theme-select` with **16 options**: `padrao` + flamengo, palmeiras, corinthians, sao-paulo, santos, vasco, fluminense, botafogo, gremio, internacional, atletico-mg, cruzeiro, bahia, fortaleza, athletico-pr; one `[data-theme="…"]` CSS block per club |
| `/?theme=flamengo` / `/?theme=palmeiras` | ✅ `<html data-theme="flamengo">` / `"palmeiras"`; picker synced; stored in `localStorage.psearch_storefront_theme`. `?theme=FLAMENGO` normalizes to `flamengo` |
| `/?theme=<script>alert(1)</script>` and `/?theme=nonexistent-club` | ✅ fall back to Padrão (no `data-theme`, picker `padrao`, nothing stored); payload not reflected in the server HTML; no injected script nodes; 0 errors |
| Persistence | ✅ picker → corinthians stores `corinthians`; reload with that storage → `data-theme=corinthians`, picker `corinthians`. A URL param overrides storage (`?theme=palmeiras` with stored corinthians → palmeiras). Tampered stored value (`<img src=x onerror=…>`) → Padrão, 0 errors |
| Regression suites (0 JS errors in every run) | ✅ pagination "Showing 1-24 of 100", pager, Next → 25-48, 5 sort modes; multi-select facets + pills + Clear all; In-Stock Alternatives panel + Similar / Frequently Viewed carousels; experiment selector → `experiment_id=ltr_v2`, analytics drawer (CTR, MRR 0.500, A/B rows); attribute facets + 7 spec badges, no FPS on `tenis`. With `?theme=flamengo`: `running shoes` → "Showing 1-22 of 100", **2 sponsored badges**, 0 errors |
| `/api/search` p50 (n=20, same method) | ✅ unchanged: `po` **398 ms** (00038: 400), `protetor` **184** (185), `running shoes` **171** (152; p90 184 for both) |

**Rollback:** `update-traffic --to-revisions=psearch-serving-00038-mpb=100` (storefront only, no schema changes).

**Status:** #49 **closed**.

## #50 World country flag-colour themes: DEPLOYED 2026-11-03

Source: clean `git archive` of **`3de4d36`** in `/tmp/deploy-3de` (feature `d482744`; `3de4d36` adds REVIEW.md). Since `ff5f8eb` the only serving changes are `internal/api/routes.go` (embedded storefront) and `storefront_test.go`. **No DDL. Env unchanged.**

| Step | Result |
|---|---|
| sqlcheck gate (real Spanner) | ✅ **63 OK / 0 FAIL**; `go test ./internal/api` ok |
| Deploy → **`psearch-serving-00040-r2j`** | ✅ 100%, follows latest |

| Check (jsdom against the live service for client-side behaviour) | Result |
|---|---|
| `GET /` | ✅ 200; `#theme-select` = `padrao` + optgroup **"Times do Brasileirão" (15)** + optgroup **"Países" (27)**: pais-brasil, argentina, estados-unidos, mexico, canada, reino-unido, franca, alemanha, italia, espanha, portugal, holanda, belgica, japao, coreia-do-sul, china, india, australia, africa-do-sul, nigeria, uruguai, colombia, chile, croacia, marrocos, egito, arabia-saudita. 43 unique options, each with a `[data-theme]` CSS block |
| `/?theme=pais-brasil` / `pais-japao` / `pais-alemanha` / `flamengo` | ✅ `data-theme` applied, picker synced, stored. `?theme=PAIS-JAPAO` normalizes to `pais-japao` |
| All 43 options via the picker | ✅ each sets the matching `data-theme` (`padrao` = none) and storage; 0 mismatches |
| Malicious/unknown: `<script>alert(1)</script>`, `pais-"><img src=x onerror=alert(1)>`, `pais-atlantida`, `pais-` | ✅ all fall back to Padrão; nothing stored; payload not reflected in the server HTML; no injected nodes |
| Persistence | ✅ picker → `pais-portugal` survives a reload; a URL param overrides storage (palmeiras); a tampered stored value → Padrão |
| Regression suites (0 JS errors in every run) | ✅ pagination + 5 sorts; multi-select facets/pills/Clear all; In-Stock Alternatives + Similar/Co-viewed carousels; experiment selector + beacons + analytics drawer; attribute facets + 7 spec badges. With `?theme=pais-japao`: `running shoes` "Showing 1-22 of 100", **2 sponsored badges** |
| `/api/search` p50 | Sequential runs read about 7–13% above this morning's 00039 numbers (00040: 425/195/194 and 429/203/189 vs 398/184/171). **Interleaved A/B** (no-traffic tag `ab39` → 00039, n=30 each, alternating): `po` 449 vs 495 ms, `protetor` 204 vs 192, `running shoes` 200 vs 192, so the differences go both ways (−46/+12/+8 ms) and both revisions had drifted up together. **No code-attributable change** (backend code identical). Tag removed afterwards; traffic 100% on 00040 |

**Rollback:** `update-traffic --to-revisions=psearch-serving-00039-x8j=100` (storefront only).

**Status:** #50 **closed**.

## #51 Architecture diagram at `/diagram`: DEPLOYED 2026-11-05

Source: clean `git archive` of **`8a25c90`** in `/tmp/deploy-8a2` (feature `28cdac1`; `8a25c90` adds REVIEW.md). Serving changes since `3de4d36`: new `internal/api/diagram.html` (embedded), `routes.go` (+8 lines, `/diagram` route), `storefront_test.go`. **No DDL. Env unchanged.** The diagram was built from the deployer's verified, placeholder-only infrastructure inventory.

| Step | Result |
|---|---|
| sqlcheck gate (real Spanner) | ✅ **63 OK / 0 FAIL**; `go test ./internal/api` ok |
| Deploy → **`psearch-serving-00041-x6r`** | ✅ 100%, follows latest |

| Check | Result |
|---|---|
| `GET /diagram` | ✅ 200 `text/html; charset=utf-8`, 61,862 B |
| No identifiers in the served page (`curl` + grep for project ID/number, `*.run.app`, `*@*.iam.gserviceaccount.com`, GitHub/API-key token patterns) | ✅ **0 matches**. Only external resources: `fonts.googleapis.com` / `fonts.gstatic.com` |
| Stepped views (jsdom against the live page; `scrollIntoView` stubbed because jsdom lacks it) | ✅ **8 views**: System Context → Request Entry & Query Understanding (steps 1–3) → Spanner Hybrid RRF (FTS + ScaNN ANN) (4–5) → Personalization/Sponsored/Facets (6–9) → Data, Enrichment & Backfill → Closed-Loop Telemetry & Bayesian Rollup → Operations, Observability & Pre-Deploy Verification → Complete Architecture & Evidence Summary. Next advances through all 8 and wraps to 0; Prev and ArrowRight work; each view highlights a different subset (lit 13–25 nodes / dimmed 77–89) |
| Icons | ✅ 10 Google Cloud product icons defined inline; all 17 cards resolve to a defined icon; 112 icon paths rendered in the SVG (331 nodes, 22 edges, 79 labels) |
| Google Fonts blocked | ✅ jsdom loads no external resources, so this run is the "blocked" case: 0 JS errors, full render. CSS fallbacks: `--body` "Google Sans Text" → Roboto, Helvetica Neue, Arial, sans-serif; `--mono` "Roboto Mono" → ui-monospace, SF Mono, Menlo, monospace |
| JS errors | ✅ **0** (after stubbing jsdom's missing `scrollIntoView` and sending key events to `body`; both were harness artifacts, not page defects) |
| Storefront `/` | ✅ 200. Theme suite (pais-brasil/japao/alemanha, flamengo, malicious fallbacks, persistence) unchanged; pagination, facets and analytics drawer OK; 0 JS errors |
| `/api/search` p50 | Sequential run on 00041: 611/291/213 ms, higher than the 00040 runs of 2026-11-03 (425/195/194). **Interleaved A/B** (no-traffic tag `ab40` → 00040, n=30 each, alternating): `po` **564 vs 608**, `protetor` **275 vs 308**, `running shoes` **189 vs 214** ms: 00041 equal or faster, so **no deploy-attributable change**. Both revisions are slower than on 2026-11-03 (environmental drift; flagged to lead). Tag removed; 100% on 00041 |

Note: at the time, the storefront `/` (not `/diagram`) still showed the sandbox project ID in its existing "Repo / Sandbox" header badge (present since before #49). Reported to lead; fixed in #52 (`00042-mbh`).

**Rollback:** `update-traffic --to-revisions=psearch-serving-00040-r2j=100` (no schema changes).

**Status:** #51 **closed**.

## #52 Remove real project identifiers: DEPLOYED 2026-11-05

Source: clean `git archive` of **`4f6d4a7`** in `/tmp/deploy-4f6` (`3eb8d7a` storefront badge + redaction tests, `28f121a` tracked files + `/api/info` + config fail-fast; `4f6d4a7` adds REVIEW.md). **No DDL.** The Dockerfile no longer sets `ENV PROJECT_ID` (it keeps `PORT`, `ENVIRONMENT`, `REGION`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`). `config.Validate()` now returns `PROJECT_ID is required when USE_MEMORY_STORE is false` instead of falling back to a hardcoded default project.

| Step | Result |
|---|---|
| Pre-step (before the code change) | ✅ `psearch-serving` already sets `PROJECT_ID` (and `GCP_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`) as Cloud Run service env vars, which override the Dockerfile, so no env change was needed. Re-checked right before deploying: all set, correct values. `psearch-eval-dashboard` uses its own image and sets its project explicitly; no Cloud Run Jobs exist |
| Identifier scan of the whole snapshot (all files, root docs included; project ID, project number, service-URL hash, `*@<project>.iam.gserviceaccount.com`) | ✅ **0 files** |
| sqlcheck gate (real Spanner) | ✅ **63 OK / 0 FAIL** |
| `go test ./...` with `PROJECT_ID` / `GCP_SANDBOX_PROJECT` unset | ✅ api, config, services ok |
| `TestTrackedSourceContainsNoForbiddenIDPattern` with `FORBIDDEN_ID_PATTERN` = ID, number, URL hash, SA pattern (snapshot copied to a scratch git repo) | ✅ PASS (299 tracked files). ⚠️ **Coverage gap:** `repoRoot` resolves 4 levels up from `internal/config`, which is `src/`, not the repo root, so the test scans only `src/**`. Negative control: an ID planted in `src/PLANTED.txt` fails the test as expected, but one planted at the repo root goes undetected. The independent whole-snapshot scan above covers that gap for this deploy. Reported for a follow-up fix (5 levels up, or `git rev-parse --show-toplevel`) |
| Deploy → **`psearch-serving-00042-mbh`** | ✅ started normally (fail-fast satisfied by the service env); 100%, follows latest; no WARNING+ logs |

| Check | Result |
|---|---|
| `GET /api/health` | ✅ 200 |
| `GET /api/info` | ✅ `backend_mode=spanner`, `psearch-instance` / `psearch-db`, `product_count=20535`, `repo=duboc/scion-experiment`; the `sandbox` field now carries the environment name (`production`), not the project ID. `vector_index_used` reads `false` on a cold instance until the first ANN search, then `true` (reflects real execution) |
| Identifier grep (ID, number, URL hash, SA emails, `run.app`) on the served `/`, `/?theme=pais-brasil`, `/api/info`, `/diagram`, `/api/health` | ✅ **0 matches** on every page |
| Storefront badge | ✅ `Repo: duboc/scion-experiment` (no sandbox ID) |
| `GET /diagram` | ✅ byte-identical to 00041 (same MD5) |
| Search | ✅ `protetor` and `running shoes`: `hybrid_v2`, 20 results, total 100, `vector_index_used=true`; `running shoes` has 2 sponsored |
| Storefront regressions | ✅ pagination (1-24 → 25-48 of 100), facets/pills, analytics drawer, themes (including malicious fallback); 0 JS errors |
| `/api/search` p50, interleaved A/B vs 00041 (no-traffic tag, n=30 each, alternating) | ✅ in line: `po` 530 vs 477 ms, `protetor` 294 vs 297, `running shoes` 219 vs 234 (differences in both directions). Tag removed; 100% on 00042 |

**Git history:** the removed values are **still present in git history** (earlier commits of the 9 files and the playbooks). Rewriting history (for example with `git filter-repo`, then a force-push) is a **maintainer decision** and was not done.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00041-x6r=100` (00041 still has the Dockerfile `PROJECT_ID`, and the service env sets it too).

**Status:** #52 **closed**.

## #53 Remove the demo theme picker + #54 docs feature matrix: DEPLOYED 2026-11-05

Source: clean `git archive` of **`014b6df`** in `/tmp/deploy-014` (#53 `ed578f3`; #54 docs `fdcfdc3` + `e7b5015`; also `f4730ea`, the #52 forbidden-ID test now scanning from the git toplevel; `014b6df` adds REVIEW.md). Serving changes since `4f6d4a7`: `routes.go` (−199 lines: theme picker, theme CSS and `?theme=` logic), `storefront_test.go`, `config_test.go`. #54 touches only `README.md`, `docs/USAGE.md` and `src/application/ui/README.md`. **No DDL. Env unchanged.**

| Step | Result |
|---|---|
| Identifier scan of the whole snapshot | ✅ 0 files |
| sqlcheck gate (real Spanner) | ✅ **63 OK / 0 FAIL** |
| `go test ./...` (with `PROJECT_ID` unset) | ✅ api, config, services ok |
| Forbidden-ID guard with `FORBIDDEN_ID_PATTERN` (scratch git repo of the snapshot) | ✅ clean tree passes. **Root-level negative control now FAILS as intended** (`forbidden ID pattern matched in PLANTED-ROOT.txt:1`), confirming the #52 coverage gap is fixed by `f4730ea` |
| Deploy → **`psearch-serving-00043-p87`** | ✅ 100%, follows latest |

| Check | Result |
|---|---|
| `GET /` | ✅ 200, 92,942 B (about 20 KB smaller); `#theme-select` absent, 0 `data-theme` CSS blocks, no theme storage key; default dark `:root` (`--bg: #0f172a`, `--accent: #38bdf8`); badge `Repo: duboc/scion-experiment` |
| `?theme=` has no effect (jsdom) | ✅ `/`, `/?theme=flamengo`, `/?theme=pais-japao`, and `/` with a leftover stored `flamengo` from earlier visits: all `data-theme=null`, `--bg #0f172a`, 24 cards, 0 errors |
| Storefront regressions (0 JS errors in every run) | ✅ pagination 1-24 → 25-48 of 100 + 5 sorts; multi-select facets/pills; In-Stock Alternatives + Similar/Co-viewed carousels; experiment selector + analytics drawer; attribute facets + 7 spec badges; **2 sponsored badges** on `running shoes` |
| `/diagram` | ✅ byte-identical to 00041/00042 (same MD5) |
| `/api/info` | ✅ unchanged shape (21 keys): `spanner`, `psearch-instance` / `psearch-db`, 20535 products, `sandbox=production`, `repo=duboc/scion-experiment` |
| Identifier grep (ID, number, URL hash, SA emails, `run.app`) on `/`, `/?theme=flamengo`, `/api/info`, `/diagram` | ✅ **0 matches** each |
| `/api/search` p50, interleaved A/B vs 00042 (no-traffic tag, n=30 each) | ✅ in line: `po` 553 vs 546 ms, `protetor` 304 vs 263, `running shoes` 224 vs 252 (both directions). Tag removed; 100% on 00043 |

**#54:** docs only (feature matrix for #1–#52 in README/USAGE, UI README updated for the removed themes, `/diagram` and the repo badge). No deploy needed beyond riding this image; identifier scan clean.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00042-mbh=100` (would bring the theme picker back).

**Status:** #53 **closed** (deployed), #54 **closed** (docs).

## #66 Phase A, per-search deadline and admission control: DEPLOYED 2026-11-13

Source: clean `git archive` of **`aa90182`** in `/tmp/deploy-66` (code commit `598fc1f`, merged on top of upstream docs commits; review `d2ec8a3`). Serving changes since `014b6df`: `handlers.go`, `config.go`, `spanner_service.go`, `embedding_cache.go`, `query_understanding.go` and tests. **No SQL text changes, no DDL.** Cloud Run settings (concurrency 80, max 100 instances, 1 vCPU / 512 MiB, timeout 300s, startup boost), env vars and Spanner PU (100) were **unchanged**; the new `SEARCH_TIMEOUT` / `SEARCH_MAX_INFLIGHT` / `SEARCH_QUEUE_WAIT` are **not set** (code defaults 10s / 8 / 250ms).

| Step | Result |
|---|---|
| sqlcheck gate (real Spanner, production env) | ✅ **63 OK / 0 FAIL** |
| `go vet ./...`, `go test ./...` | ✅ api, config, services ok |
| Deploy → **`psearch-serving-00044-8r5`** | ✅ 100%, follows latest |
| Service config diff before/after (env, resources, concurrency, timeout, SA, annotations) | ✅ identical; no `SEARCH_*` env vars |

| Check (1 request at a time) | Result |
|---|---|
| `GET /api/health` | ✅ 200 |
| `GET /api/info` → `search_admission` | ✅ `max_inflight 8`, `queue_wait_ms 250`, `timeout_ms 10000`, `in_flight 0`, `rejected_total 0`, `timed_out_total 0` |
| `/api/search` × 6 queries (`running shoes`, `protetor solar`, `po`, `wireless headphones`, `dipirona 500mg`, `touca sanfonada verde`) | ✅ all 200, `retrieval_mode=hybrid_v2` (same as 00043), 20 results each |
| `POST /api/analytics/rollup` without token | ✅ 403 |
| `/api/search` p50 A/B vs 00043 (no-traffic tag, n=7 each) | ✅ no drift: `running shoes` 235 vs 421 ms, `protetor solar` 205 vs 418, `po` 895 vs 903. Tag removed; 100% on 00044 |

**2 rps check** (PO limit; open loop, 120 requests over 60s, 15:17:45–15:18:46Z, 12 rotating queries, `alpha=0.5`):

| Metric | 00044-8r5 (this run) | 00043-p87 (eval run 2026-11-12, 2 rps) |
|---|---|---|
| Status codes | **120 × 200, 0 × 503, 0 × 504** | 120 × 200 |
| Client p50 / p95 / p99 / max | 0.435 / 2.54 / 3.49 / 4.39 s | — |
| Server (Cloud Run log) p50 / p95 / p99 | 0.394 / 2.51 / 3.45 s | 0.23 / 1.28 / 2.2 s |
| Instances | 1 (no cold start) | 1 |
| Admission counters after run | `rejected_total 0`, `timed_out_total 0` | n/a |

The 2 rps tail is wider than in the eval's run. The cause is Spanner-side queueing, not the new code: the hybrid RRF query averaged **0.59 s** (CPU about 0.09–0.14 s per execution) against 0.10–0.21 s in the eval's minute, and Phase A changes no SQL. Phase A does not add capacity; 100 PU still degrades from about 2 rps, as diagnosed on #66 (Phase B). No 5xx and no deadline or admission trips, so **no rollback**.

**Rollback:** `update-traffic --to-revisions=psearch-serving-00043-p87=100`.

**Status:** #66 **open** (PO decides on closing and on Phase B).
