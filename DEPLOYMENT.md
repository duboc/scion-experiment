# `psearch-eval` Deployment Status, Release History & Cloud Infrastructure

**Project:** `psearch-eval` (Orphan Branch `eval-dashboard`)  
**GCP Project:** `<GCP_PROJECT_ID>` (`us-central1`)  
**Lead Evaluator:** `@eval-lead` (Antigravity harness · `agy v1.3.4` · `gemini-4-argon`)  
**Release Engineer:** `@eval-deployer` · **UI Engineer:** `@ui-craftsman` · **Relevance Engineer:** `@search-evaluator` · **Factory Analyst:** `@factory-analyst`  

---

## 1. Cloud Run Deployments

| Service Name | Description | Live URL | Active Revisions | Traffic | Configuration |
|---|---|---|---|---|---|
| **`psearch-eval-dashboard`** | Control Tower, 5-Layer Pyramid & Impeccable UI Workbench | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) | `psearch-eval-dashboard-00010-z5g` / `psearch-eval-dashboard-00012-k9v` | **100% LATEST** | 1 CPU, 512MB RAM, `min-instances=1`, Secret: `github-token` |
| **`psearch-serving`** | Target Hybrid Search Engine Backend (Spanner + Gemini 4 Argon) | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) | `psearch-serving-00025-5mx` | **100% LATEST** | 2 CPU, 2GB RAM, `min-instances=1`, `gemini-3.8-flash` + `gemini-4-argon` (`global`) |

---

## 2. Managed Google Cloud Resources

### Cloud Spanner
- **Instance ID:** `psearch-instance` (Enterprise Edition, 100 Processing Units, `regional-us-central1`)
- **Database ID:** `psearch-db`
- **Key Tables:**
  - `products`, `skus`, `regional_inventory`, `product_prices`: Core catalog with composite multilingual indexes (`(tenant_id, locale, product_id)`) and `INTERLEAVE IN PARENT` hierarchy for regional CD inventory and B2B contract price tiers.
  - `search_synonyms`: Low-relevance synonym mapping table ($0.3\times$ score weight).
  - `merchandising_rules`: 8-signal scoring modifiers and campaign boost rules ($\le 2.0\times$ ceiling).
  - `embedding_cache`: Spanner-level vector embedding cache (1024-dim `gemini-embedding-2` `v2`).
  - `factory_agent_status`: Real-time multi-agent factory progress, turn counts, and model calls across all 10 agents (`psearch` and `psearch-eval`).
  - `eval_runs`: Authoritative ledger of all search quality evaluation sweeps, NDCG@10, MRR@10, latency percentiles, 14/14 requirement compliance states, and 5-Layer Product Evaluation Pyramid (`L1–L5`, 76 scenarios) results.

### Cloud Firestore (Native Mode)
- **Database:** `(default)` in `us-central1`
- **Collections:**
  - `eval_runs`: Mirrored evaluation sweeps for rapid dashboard queries.
  - `requirements_compliance`: Compliance status documents for all 14 Requirements (`4.1.1` through `4.1.14`, Waves 1–5).
  - `pyramid_layers`: 5-Layer Product Evaluation Methodology Pyramid (`L1`–`L5`, 76 scenarios).
  - `agents`: Real-time agent status documents (10 agents across `psearch` and `psearch-eval`).
  - `tasks`: Factory backlog task state tracking.

### Cloud Storage (GCS)
- **Bucket:** `gs://psearch-eval-data-<GCP_PROJECT_ID>/`
- **Snapshots Path:** `gs://psearch-eval-data-<GCP_PROJECT_ID>/snapshots/` (`latest_factory_state.json` and timestamped audit archives)
- **Role:** Immutable audit trail and disaster recovery snapshots of factory and evaluation runs.

### Secret Manager
- **Secret ID:** `github-token` (`projects/<GCP_PROJECT_ID>/secrets/github-token`)
- **Role:** Provides authenticated rate-limit-exempt access to the GitHub REST API for repository backlog and commit sync.

---

## 3. Revision & Deployment History (`psearch-eval-dashboard`)

| Revision | Commit (`eval-dashboard`) | Cloud Build ID | Key Capabilities Delivered | Verification Status |
|---|---|---|---|---|
| **`psearch-eval-dashboard-00012-k9v`** | `b10120a` | `343a6466-dc2d-4e5b-af80-024233a27773` | 5-Layer Product Evaluation Pyramid (`L1–L5`, 76/76 scenarios PASS), 14/14 Biggy Search Requirements (`Waves 1–5`), interactive scenario drilldown UI | **LIVE (100% Traffic)** |
| **`psearch-eval-dashboard-00010-z5g`** | `b10120a` | `00010-z5g` rollout | 5-Layer Product Evaluation Pyramid (`L1–L5`, 76 scenarios, 100% threshold), 14 Requirements matrix (`4.1.1`–`4.1.14`), Gemini 4 Argon model telemetry | **VERIFIED** |
| **`psearch-eval-dashboard-00009-d42`** | `ba2371a` | `7704fe52-2438-47a3-9d5b-69bd1e368e13` | Integrated `@factory-analyst` diagnostics, `CRITICAL` / `WARNING` / `HEALTHY` semantic badge tokens, initial PRD/DESIGN/DEPLOYMENT docs | Superseded |
| **`psearch-eval-dashboard-00008-w7p`** | `df87336` | `00008-w7p` rollout | Storefront live URL linkage, camelCase API aliases, 10/10 Wave 1–4 requirement verification | Superseded |

---

## 4. Empirical Release Verification Ledger (5-Layer Pyramid & 14 Requirements)

- **Evaluated Run ID:** `probe-1791941093-29012b` (`data_source: "spanner"`, `is_sample: false`)
- **Target Backend Revision:** `psearch-serving-00025-5mx` (`text_model: "gemini-3.8-flash"`, `pro_model: "gemini-4-argon"`, `embedding_model: "gemini-embedding-2"`, `active_embedding_version: "v2"`, `genai_location: "global"`)
- **Quantitative Search Relevance & Latency:**
  - `overall_ndcg_at_10`: **`0.980`** (Target SLA $\ge 0.850$)
  - `overall_mrr_at_10`: **`1.000`** (Target SLA $\ge 0.900$)
  - `optimal_alpha`: **`0.65`** (NDCG@10 = `0.987` across 6-point $\alpha \in [0.00, 1.00]$ sweep)
  - `p50_latency_ms`: **`35.4 ms`** · `p95_latency_ms`: **`54.1 ms`**
- **14 Biggy Search Requirements (`4.1.1`–`4.1.14` across Waves 1–5):** **`14 / 14 PASS` (`100%`)**
- **5-Layer Product Evaluation Methodology Pyramid (`L1–L5`):** **`76 / 76 Scenarios PASS` (`100%` Threshold Met)**
  - **L1 (`l1_trajectory_sequence`):** `12 / 12 PASS` (`100%`) — NL intent, opt-out, spell check, synonyms, RRF fusion, Stage-2 LTR, session boost, metadata envelope.
  - **L2 (`l2_policy_guardrails`):** `15 / 15 PASS` (`100%`) — Tenant isolation (`acme` vs `default`), `pt-BR`/`en-US`/`es-MX` partitioning, stock gating & OOS tail demotion, B2B contract pricing, CEP `01310-100` SLA, boost cap ($\le 2.0\times$), negative price & admin write rejection.
  - **L3 (`l3_adversarial_redteam`):** `16 / 16 PASS` (`100%`) — SQLi (`UNION SELECT`, tautology), prompt injection, XML/role spoofing, XSS, Cyrillic homoglyphs, oversized payload & parameter clamping, null byte, burst concurrency, malformed JSON, idempotency.
  - **L4 (`l4_routing_and_parity`):** `28 / 28 PASS` (`100%`) — 10 retrieval mode routing classifications + 18 structural parity checks between `psearch-serving` and `/api/workbench`.
  - **L5 (`l5_live_vertex_and_telemetry`):** `5 / 5 PASS` (`100%`) — Live `/health`, Vertex AI `global` Argon/Flash/Embedding models, Spanner `eval_runs`, Firestore collections, and GCS snapshots.

---

## 5. Operational Deployment Commands

```bash
# 1. Build and deploy evaluation dashboard to Cloud Run via Cloud Build
export GCP_PROJECT="<GCP_PROJECT_ID>"
gcloud builds submit --config=eval-dashboard/cloudbuild.yaml --project="${GCP_PROJECT}"

# 2. Or deploy via standalone deployment script
GCP_PROJECT="<GCP_PROJECT_ID>" bash eval-dashboard/scripts/deploy.sh

# 3. Verify live Cloud Run revision and /api/eval payload
curl -s https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/eval | \
  jq '{run_id, pass_count, overall_ndcg_at_10, overall_mrr_at_10, p50_latency_ms, p95_latency_ms, pyramid_summary}'
```
