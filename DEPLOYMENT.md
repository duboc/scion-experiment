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
