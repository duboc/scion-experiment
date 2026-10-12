# psearch-eval-dashboard

Evaluation & Observability console for the `psearch` multi-agent factory on Google Cloud Platform (`riojucu-sandbox`).

---

## Live Endpoints & Console URLs

| Resource | Description | Live Endpoint / Console URL |
|---|---|---|
| **Live Evaluation Dashboard** | Cloud Run Service (Impeccable UI) | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) |
| **Target Search Service** | Target search backend under evaluation | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) |
| **Cloud Spanner Database** | Instance `psearch-instance` / DB `psearch-db` | [Spanner Console](https://console.cloud.google.com/spanner/instances/psearch-instance/databases/psearch-db/details?project=riojucu-sandbox) |
| **Source Repository** | Branch `eval-dashboard` | [duboc/scion-experiment@eval-dashboard](https://github.com/duboc/scion-experiment/tree/eval-dashboard) |

---

## Scripts & Operations

- `scripts/ingest_telemetry.py`: Ingests agent telemetry from Cloud Spanner `factory_agent_status` into `public/telemetry.json`.
- `scripts/probe_search.py`: Probes `psearch-serving` for NDCG@10, MRR@10, and latency percentiles, recording results into Cloud Spanner `eval_runs`.
- `scripts/deploy.sh`: Builds and deploys the container to Cloud Run in `riojucu-sandbox` with Secret Manager `github-token`.
