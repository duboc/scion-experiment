# `psearch-eval` Deployment Status & Cloud Infrastructure

**Project:** `psearch-eval` / `psearch`  
**GCP Project:** `riojucu-sandbox` (`us-central1`)  
**Lead Evaluator:** `@eval-lead`  
**Engineer:** `@ui-craftsman` (Antigravity harness)  

---

## 1. Cloud Run Deployments

| Service Name | Description | URL | Status | Configuration |
|---|---|---|---|---|
| **`psearch-eval-dashboard`** | Control Tower & Impeccable UI Workbench | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) | **ACTIVE (Live)** | 1 CPU, 512MB RAM, min-instances=1, Secrets: `github-token` |
| **`psearch-serving`** | Target Search Engine Backend (Spanner hybrid) | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) | **ACTIVE (Live)** | 2 CPU, 2GB RAM, min-instances=1 |

---

## 2. Managed Google Cloud Resources

### Cloud Spanner
- **Instance ID:** `psearch-instance` (Enterprise Edition, 100 Processing Units, `regional-us-central1`)
- **Database ID:** `psearch-db`
- **Key Tables:**
  - `products`, `skus`, `regional_inventory`: Core catalog with composite multilingual indexes and `INTERLEAVE IN PARENT` hierarchy.
  - `search_synonyms`: Low-relevance synonym mapping table (0.3x score weight).
  - `merchandising_rules`: 8-signal scoring modifiers and business rules.
  - `embedding_cache`: Spanner-level vector embedding cache (1024-dim ScaNN).
  - `factory_agent_status`: Real-time multi-agent factory progress, turn counts, and model calls.
  - `eval_runs`: Canonical record of all search quality evaluation sweeps, NDCG@10, MRR@10, latency percentiles, and requirement compliance states.

### Cloud Firestore (Native Mode)
- **Database:** `(default)` in `us-central1`
- **Collections:**
  - `eval_runs`: Mirrored evaluation sweeps for rapid dashboard queries.
  - `requirements_compliance`: Historical compliance status for Requirements 4.1.1 through 4.1.10.
  - `agents`: Real-time agent status documents.
  - `tasks`: Factory backlog task state tracking.

### Cloud Storage (GCS)
- **Bucket:** `gs://psearch-eval-data-riojucu-sandbox/`
- **Snapshots Path:** `gs://psearch-eval-data-riojucu-sandbox/snapshots/`
- **Role:** Immutable audit trail and disaster recovery snapshots of factory and evaluation runs.

### Secret Manager
- **Secret ID:** `github-token` (`projects/riojucu-sandbox/secrets/github-token`)
- **Role:** Provides authenticated rate-limit-exempt access to GitHub API for repository backlog and commit sync.

---

## 3. Operational Deployment Commands

```bash
# Build and deploy evaluation dashboard to Cloud Run
gcloud builds submit --config=eval-dashboard/cloudbuild.yaml --project=riojucu-sandbox

# Or deploy via standalone deployment script
bash eval-dashboard/scripts/deploy.sh
```
