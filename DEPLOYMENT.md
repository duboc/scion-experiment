# `psearch-eval` Deployment Status & Cloud Infrastructure

**Project:** `psearch-eval` / `psearch`  
**GCP Project:** `riojucu-sandbox` (`us-central1`)  
**Lead Evaluator:** `@eval-lead`  
**Engineer:** `@ui-craftsman` (Antigravity harness)  

---

## 1. Cloud Run Deployments

| Service Name | Description | URL | Active Revision | Status | Configuration |
|---|---|---|---|---|---|
| **`psearch-eval-dashboard`** | Control Tower & Impeccable UI Workbench | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) | `psearch-eval-dashboard-00011-tsf` | **ACTIVE (100% Traffic)** | 1 CPU, 512MB RAM, min-instances=1, Secrets: `github-token` |
| **`psearch-serving`** | Target Search Engine Backend (Spanner hybrid + Gemini 4 Argon) | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) | `psearch-serving-00025-5mx` | **ACTIVE (100% Traffic)** | 2 CPU, 2GB RAM, min-instances=1, `gemini-3.8-flash` + `gemini-4-argon` |

---

## 2. Managed Google Cloud Resources

### Cloud Spanner
- **Instance ID:** `psearch-instance` (Enterprise Edition, 100 Processing Units, `regional-us-central1`)
- **Database ID:** `psearch-db`
- **Key Tables:**
  - `products`, `skus`, `regional_inventory`, `product_prices`: Core catalog with composite multilingual indexes and `INTERLEAVE IN PARENT` hierarchy.
  - `search_synonyms`: Low-relevance synonym mapping table (0.3x score weight).
  - `merchandising_rules`: 8-signal scoring modifiers and business rules.
  - `embedding_cache`: Spanner-level vector embedding cache (1024-dim `gemini-embedding-2` `v2`).
  - `factory_agent_status`: Real-time multi-agent factory progress, turn counts, and model calls.
  - `eval_runs`: Canonical record of all search quality evaluation sweeps, NDCG@10, MRR@10, latency percentiles, 14/14 requirement compliance states, and 5-Layer Product Evaluation Pyramid (`L1–L5`, 76 scenarios) results.

### Cloud Firestore (Native Mode)
- **Database:** `(default)` in `us-central1`
- **Collections:**
  - `eval_runs`: Mirrored evaluation sweeps for rapid dashboard queries.
  - `requirements_compliance`: Historical compliance status for all 14 Requirements (`4.1.1` through `4.1.14`, Waves 1–5).
  - `pyramid_layers`: 5-Layer Product Evaluation Methodology Pyramid (`L1`–`L5`, 76 scenarios).
  - `agents`: Real-time agent status documents (10 agents across `psearch` and `psearch-eval`).
  - `tasks`: Factory backlog task state tracking (26 issues).

### Cloud Storage (GCS)
- **Bucket:** `gs://psearch-eval-data-riojucu-sandbox/`
- **Snapshots Path:** `gs://psearch-eval-data-riojucu-sandbox/snapshots/` (`latest_factory_state.json`)
- **Role:** Immutable audit trail and disaster recovery snapshots of factory and evaluation runs.

### Secret Manager
- **Secret ID:** `github-token` (`projects/riojucu-sandbox/secrets/github-token`)
- **Role:** Provides authenticated rate-limit-exempt access to GitHub API for repository backlog and commit sync.

---

## 3. Latest Release Verification Ledger (Issue #26)

- **Commit:** `b10120a` (`branch: eval-dashboard`)
- **Cloud Build ID:** `343a6466-dc2d-4e5b-af80-024233a27773` (`SUCCESS`, image digest `sha256:c63c2fe1f5de7ffff4158d21e55dc2f82e77d31e56dccd94e1b5e87d8424b1dd`)
- **Cloud Run Revision:** `psearch-eval-dashboard-00011-tsf` (`--to-latest`, 100% traffic)
- **Live `/api/eval` Verification (`200 OK`):**
  - `run_id`: `probe-1791941093-29012b` (`data_source: spanner`, `is_sample: false`)
  - `overall_ndcg_at_10`: `0.980` · `overall_mrr_at_10`: `1.000` · `p50_latency_ms`: `35.4ms` · `p95_latency_ms`: `54.1ms`
  - **Requirements Compliance:** `14 / 14 PASS` (Waves 1–5: `#1`–`#10`, `#19`–`#22`)
  - **5-Layer Product Evaluation Pyramid:** `76 / 76 Scenarios PASS` (`100%` threshold across all 5 layers):
    - `L1` (`l1_trajectory_sequence`): `12/12 PASS`
    - `L2` (`l2_policy_guardrails`): `15/15 PASS`
    - `L3` (`l3_adversarial_redteam`): `16/16 PASS`
    - `L4` (`l4_routing_and_parity`): `28/28 PASS`
    - `L5` (`l5_live_vertex_and_telemetry`): `5/5 PASS`

---

## 4. Operational Deployment Commands

```bash
# Build and deploy evaluation dashboard to Cloud Run
gcloud builds submit --config=eval-dashboard/cloudbuild.yaml --project=riojucu-sandbox

# Or deploy via standalone deployment script
bash eval-dashboard/scripts/deploy.sh
```

