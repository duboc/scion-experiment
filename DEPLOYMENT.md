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
