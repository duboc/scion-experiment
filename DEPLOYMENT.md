# Cloud Run & Sandbox Deployment Status (`psearch`)

- **GCP Project**: `riojucu-sandbox` (`249096448072`)
- **Region**: `us-central1`
- **Service Account**: `agent-deployer@riojucu-sandbox.iam.gserviceaccount.com`
- **Cloud Spanner**: `psearch-instance` (`ENTERPRISE`, `100 PU`) / Database: `psearch-db`
- **BigQuery Dataset**: `riojucu-sandbox:psearch_dataset`
- **Live `psearch-serving` URL**: https://psearch-serving-mhjw244bgq-uc.a.run.app
- **Live `psearch-eval-dashboard` URL**: https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app
- **Alternate `psearch-serving` URL**: https://psearch-serving-249096448072.us-central1.run.app (same service)

## Issue #11: Gemini model upgrade rollout (2026-10-12)

Source: `main` @ `73a9017` (code from `799e5e5`, review passed in `73a9017`). Runtime SA: `249096448072-compute@developer.gserviceaccount.com`.

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
- **IAM blocker:** the compute SA lacks Vertex AI predict permission (needs `roles/aiplatform.user` on `riojucu-sandbox`). The backfill worked because it ran as `agent-deployer`. The Cloud Resource Manager API is disabled, so the project policy can't be read with gcloud.
- **False-positive check:** at v2, `q=tenis` still returned non-zero `score.vector`. The lexical-only Spanner query returns 0 rows for `tenis`, so `HybridSearch` falls back to `memoryHybridSearch`, which uses the 768-d deterministic embedding. "Non-zero score.vector" therefore doesn't prove Gemini is serving. Better criteria: no `lexical-only` / `PERMISSION_DENIED` logs, plus a v2-only signal (e.g. a per-response `embedding_version` or `retrieval_mode` field).
- **To retry v2:** grant `roles/aiplatform.user` to `249096448072-compute@developer.gserviceaccount.com` (or use a dedicated runtime SA), then `gcloud run services update psearch-serving --project=riojucu-sandbox --region=us-central1 --update-env-vars=ACTIVE_EMBEDDING_VERSION=v2`.

## Issue #11 follow-up: redeploy at v1 with `retrieval_mode` (2026-10-12)

Source: `main` @ `1e96a34` (fix `6e4c0fb`, review passed in `1e96a34`). Revision **`psearch-serving-00005-vq7`**, 100% traffic, `ACTIVE_EMBEDDING_VERSION=v1`. Env kept: `GCP_PROJECT_ID`, `PROJECT_ID`, `SPANNER_INSTANCE(_ID)`, `SPANNER_DATABASE(_ID)`, `BIGQUERY_DATASET`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`.

| Check | Result |
|---|---|
| `GET /api/health` | 200 `{"status":"healthy","backend_mode":"spanner"}` |
| `GET /api/info` | 200 `active_embedding_version=v1`, `embedding_dimension=768`, `product_count=15` |
| `GET /api/search?q=tenis` | 200 `total_found=15`, `embedding_version=v1`, `retrieval_mode=hybrid_v1`; top `prod-002` "Tênis de Corrida Nimbus Cloud" (text 0, vector 0.0164) |
| `GET /api/search?q=running+shoes&alpha=0.5` | 200 `total_found=15`, `embedding_version=v1`, `retrieval_mode=hybrid_v1`; top `prod-001` "UltraBoost Pro Running Shoes" (text and vector 0.0164) |

v2 **not** flipped. It is waiting on the `roles/aiplatform.user` grant for `249096448072-compute@developer.gserviceaccount.com`. v2 acceptance: `retrieval_mode=hybrid_v2` (not `lexical_only_degraded` / `memory_fallback`) and no `lexical` / `PERMISSION_DENIED` / 403 in the logs.

## Issue #11: zero-traffic v2 IAM probe and template reset (2026-10-12)

**Probe:** revision `psearch-serving-00006-caw`, same image as `00005-vq7`, `ACTIVE_EMBEDDING_VERSION=v2`, `--no-traffic --tag=v2probe`.

| Check (tag URL `https://v2probe---psearch-serving-mhjw244bgq-uc.a.run.app`) | Result |
|---|---|
| `GET /api/info` | 200 `active_embedding_version=v2`, `embedding_dimension=1024` |
| `GET /api/search?q=running+shoes` | 200 `retrieval_mode=lexical_only_degraded`, `effective_alpha=0`, `total_found=1` (prod-001, vector 0) |
| `GET /api/search?q=tenis` | 200 `retrieval_mode=lexical_only_degraded`, `total_found=0`, `results=null` (the 6e4c0fb fix means memory fallback no longer masks it) |
| Revision logs (14:03:06Z, ×2) | `WARN: query embedding failed (embed RETRIEVAL_QUERY (gemini-embedding-2 @ global): Error 403, Message: Permission 'aiplatform.endpoints.predict' denied on resource '//aiplatform.googleapis.com/projects/riojucu-sandbox/locations/global/publishers/google/models/gemini-embedding-2' ... PERMISSION_DENIED` |

Conclusion: `roles/aiplatform.user` is **still missing** for `249096448072-compute@developer.gserviceaccount.com`. Tag `v2probe` removed afterwards.

**Template reset:** revision `psearch-serving-00007-sr9` (Ready, 0% traffic, `--no-traffic`) sets the service template back to `ACTIVE_EMBEDDING_VERSION=v1`, so the next deploy can't ship v2 by accident. Template env: `GCP_PROJECT_ID`, `PROJECT_ID`, `SPANNER_INSTANCE(_ID)`, `SPANNER_DATABASE(_ID)`, `BIGQUERY_DATASET`, `GENAI_LOCATION=global`, `EMBEDDING_MODEL_NAME=gemini-embedding-2`, `EMBEDDING_DIMENSION=1024`, `ACTIVE_EMBEDDING_VERSION=v1`.

**Traffic:** pinned at 100% to `psearch-serving-00005-vq7` (v1, `retrieval_mode=hybrid_v1`). It no longer follows the latest revision, so the next rollout must use `--to-latest` or `gcloud run services update-traffic`.

## Issue #11: v2 LIVE on dedicated runtime SA (2026-10-12)

**Runtime SA (new):** `psearch-serving@riojucu-sandbox.iam.gserviceaccount.com`, with `roles/aiplatform.user` (project) and `roles/spanner.databaseUser` (`psearch-db`), created by the PO. It replaces `249096448072-compute@developer.gserviceaccount.com`. No IAM change was made to the compute SA. Earlier attempts to enable the CRM API (`serviceusage.services.enable` denied) and to grant the compute SA (CRM `SERVICE_DISABLED`) both failed and changed nothing; the plan was superseded.

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

**Rollback:** `gcloud run services update-traffic psearch-serving --project=riojucu-sandbox --region=us-central1 --to-revisions=psearch-serving-00008-ms8=100` (v1, new SA), or `--update-env-vars=ACTIVE_EMBEDDING_VERSION=v1`.

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
