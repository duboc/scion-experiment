# `eval-dashboard` — Package Guide, Probe Workers & Telemetry Consolidation

Package-level operational guide for the **`psearch-eval-dashboard`** Cloud Run service, the live search evaluation & 5-Layer Pyramid probe worker (`scripts/probe_search.py`), and the multi-store telemetry consolidation worker (`scripts/consolidate_factory_data.py`).

---

## 1. Live Endpoints & Cloud Resources

| Resource | Active Revision / Instance | Description | URL / Resource Path |
|---|---|---|---|
| **Live Evaluation Dashboard** | `psearch-eval-dashboard-00010-z5g` / `00011-tsf` | Cloud Run Express Server & Impeccable UI Workbench | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) |
| **Target Search Backend** | `psearch-serving-00025-5mx` | Spanner Hybrid Search + `gemini-3.8-flash` / `gemini-4-argon` | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) |
| **Cloud Spanner Database** | `psearch-instance` / `psearch-db` | Authoritative `eval_runs` & `factory_agent_status` tables | `projects/<GCP_PROJECT_ID>/instances/psearch-instance/databases/psearch-db` |
| **Cloud Firestore (Native)** | `(default)` | Collections: `agents`, `tasks`, `eval_runs`, `requirements_compliance`, `pyramid_layers` | `projects/<GCP_PROJECT_ID>/databases/(default)/documents` |
| **Cloud Storage (GCS)** | Snapshot Bucket | Immutable JSON factory & evaluation state snapshots | `gs://psearch-eval-data-<GCP_PROJECT_ID>/snapshots/` |
| **Source Branch** | `eval-dashboard` (Orphan) | Isolated `psearch-eval` control tower branch | [duboc/scion-experiment@eval-dashboard](https://github.com/duboc/scion-experiment/tree/eval-dashboard) |

---

## 2. Package Layout

```text
eval-dashboard/
├── server.js                           # Express HTTP server (/api/eval, /api/agents, /api/backlog, /api/workbench, /api/info)
├── package.json                        # Node.js dependencies (express, @google-cloud/spanner)
├── Dockerfile                          # Production container image definition for Cloud Run
├── cloudbuild.yaml                     # Cloud Build pipeline for building & deploying to Cloud Run
├── public/
│   ├── index.html                      # Impeccable Design System UI (5-Layer Pyramid, 14 Reqs, Alpha SVG, Topology)
│   └── app.js                          # Reactive telemetry client, Pyramid layer filter, Daylight/Carbon theme engine
└── scripts/
    ├── probe_search.py                 # Live probe suite: 6 golden queries, alpha sweep, 14 reqs, 76 Pyramid scenarios
    ├── record_spanner_run.js           # Parameterized Spanner DML writer for eval_runs (PARSE_JSON)
    ├── consolidate_factory_data.py     # Consolidates Spanner + GitHub into Firestore & GCS snapshots
    ├── ingest_telemetry.py             # Syncs factory_agent_status telemetry
    └── deploy.sh                       # Standalone Cloud Build + Cloud Run deployment script
```

---

## 3. Running the Live Probe Worker (`scripts/probe_search.py`)

`scripts/probe_search.py` executes zero-mock live HTTP probes against `psearch-serving` and `psearch-eval-dashboard` across 5 stages:

1. **Golden Query Benchmark (`6 queries`, `en-US` & `pt-BR`)**:
   - Computes `NDCG@10`, `MRR@10`, and `p50` / `p95` / `p99` latency percentiles (`running shoes`, `tenis corrida`, `developer laptop`, `noise cancelling headphones`, `geladeira inox`, `cafeteira espresso`).
2. **Live Hybrid $\alpha$ Parameter Sweep (`run_alpha_sweep()`)**:
   - Probes $\alpha \in \{0.00, 0.25, 0.50, 0.65, 0.80, 1.00\}$ to verify the Spanner hybrid score formula $(1-\alpha)\cdot\text{text} + \alpha\cdot\text{vector}$ and identify `optimal_alpha`.
3. **14 Biggy Search Requirements Compliance (`probe_all_requirements()`)**:
   - Probes all 14 functional requirements (`4.1.1` through `4.1.14` across Waves 1–5: Issues `#1`–`#10`, `#19`–`#22`, plus `#11` 1024-d `gemini-embedding-2` `v2` and `#23` `gemini-3.8-flash` & `gemini-4-argon` on Vertex `global`).
4. **5-Layer Product Evaluation Methodology Pyramid (`run_5_layer_pyramid()`, `76 scenarios`, `100%` threshold)**:
   - **L1 (`l1_trajectory_sequence`, 12 scenarios):** NL intent extraction, opt-out, spell correction, exact-match bypass, synonym expansion, $0.3\times$ weight damping, RRF fusion, $\alpha$ weighting, Stage-2 LTR re-ranking, LTR score population, session boost, and metadata envelope integrity.
   - **L2 (`l2_policy_guardrails`, 15 scenarios):** Tenant isolation (`acme` vs `default`), strict locale partitioning (`pt-BR`, `en-US`, `es-MX`), stock precedence & OOS tail demotion, B2B corporate pricing (`b2b-corporate`), retail isolation, invalid price table fallback, CEP (`01310-100`) regional SLA, invalid CEP rejection (`HTTP 400`), merchandising boost cap ($\le 2.0\times$), negative price rejection (`HTTP 400`), and read-only admin protection (`HTTP 403`).
   - **L3 (`l3_adversarial_redteam`, 16 scenarios):** SQLi (`UNION SELECT`, tautology), prompt injection jailbreak, fake `<system>` XML tags, role spoofing, XSS script tags, Cyrillic homoglyphs, 1800-char oversized query, negative/oversized `alpha` clamping, extreme/negative `limit` clamping, null-byte injection, 5-worker concurrent burst invariance, malformed JSON rejection (`HTTP 400`), and deterministic ranking idempotency.
   - **L4 (`l4_routing_and_parity`, 28 scenarios):** 10 retrieval mode routing classifications + 18 structural parity checks comparing direct `psearch-serving` (`/api/search`) against `/api/workbench`.
   - **L5 (`l5_live_vertex_and_telemetry`, 5 scenarios):** Live `/health`, Vertex AI `global` Argon/Flash/Embedding models, Spanner `eval_runs` persistence, Firestore compliance sync, and GCS snapshot export.
5. **Cloud Spanner Persistence (`record_spanner_run.js`)**:
   - Invokes `node scripts/record_spanner_run.js <tmp.json>` to insert the verified run into Cloud Spanner `eval_runs` using parameterized `PARSE_JSON(@json)` and `PENDING_COMMIT_TIMESTAMP()`.

### Execution Commands
```bash
# Run full live probe sweep and record to Cloud Spanner eval_runs
export GCP_PROJECT="<GCP_PROJECT_ID>"
export SERVING_URL="https://psearch-serving-mhjw244bgq-uc.a.run.app"
export DASHBOARD_URL="https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app"
python3 eval-dashboard/scripts/probe_search.py

# Dry-run mode (executes all live probes without inserting into Spanner)
python3 eval-dashboard/scripts/probe_search.py --no-record
```

---

## 4. Running the Consolidation Worker (`scripts/consolidate_factory_data.py`)

`scripts/consolidate_factory_data.py` synchronizes the authoritative Cloud Spanner and GitHub telemetry into Cloud Firestore `(default)` and Google Cloud Storage (`gs://psearch-eval-data-<GCP_PROJECT_ID>/snapshots/`), followed by a read-back verification phase:

1. **Step `[1/6]` (`agents`)**: Reads all 10 agents (`psearch` + `psearch-eval`) from Spanner `factory_agent_status` and writes documents to Firestore `agents`.
2. **Step `[2/6]` (`tasks`)**: Fetches repository issues from GitHub (`duboc/scion-experiment`) and writes documents to Firestore `tasks`.
3. **Step `[3/6]` (`eval_runs`)**: Reads recent evaluation runs from Spanner `eval_runs` and syncs to Firestore `eval_runs`.
4. **Step `[4/6]` (`requirements_compliance`)**: Writes all 14 Biggy Search requirement statuses (`4.1.1`–`4.1.14`) to Firestore `requirements_compliance`.
5. **Step `[5/6]` (`pyramid_layers`)**: Writes all 5 layers of the Product Evaluation Pyramid (`L1`–`L5`, 76 scenarios) to Firestore `pyramid_layers`.
6. **Step `[6/6]` (GCS Snapshot & Read-Back Verification)**: Uploads `latest_factory_state.json` and `<timestamp>_factory_state.json` to `gs://psearch-eval-data-<GCP_PROJECT_ID>/snapshots/` and reads back all 5 Firestore collections + GCS object to verify integrity.

### Execution Command
```bash
export GCP_PROJECT="<GCP_PROJECT_ID>"
python3 eval-dashboard/scripts/consolidate_factory_data.py
```

---

## 5. Local Development & Cloud Run Deployment

```bash
# Start Express server locally on port 8080
GCP_PROJECT="<GCP_PROJECT_ID>" PORT=8080 node eval-dashboard/server.js

# Deploy to Cloud Run via deploy.sh
GCP_PROJECT="<GCP_PROJECT_ID>" bash eval-dashboard/scripts/deploy.sh
```
