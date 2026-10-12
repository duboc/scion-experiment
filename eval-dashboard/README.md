# psearch-eval-dashboard

Evaluation & Observability console for the `psearch` multi-agent factory on Google Cloud Platform (`riojucu-sandbox`).

---

## Live Endpoints & Storage Resources

| Resource | Description | Live Endpoint / Resource URL |
|---|---|---|
| **Live Evaluation Dashboard** | Cloud Run Service (Impeccable UI) | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) |
| **Target Search Service** | Target search backend under evaluation | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) |
| **Cloud Spanner Database** | Instance `psearch-instance` / DB `psearch-db` | [Spanner Console](https://console.cloud.google.com/spanner/instances/psearch-instance/databases/psearch-db/details?project=riojucu-sandbox) |
| **Cloud Firestore (Native)** | Database `(default)`: collections `agents`, `tasks`, `eval_runs`, `requirements_compliance` | [Firestore Console](https://console.cloud.google.com/firestore/databases/-default-/data/panel?project=riojucu-sandbox) |
| **Cloud Storage (GCS)** | Private bucket: `gs://psearch-eval-data-riojucu-sandbox/snapshots/` | [GCS Console](https://console.cloud.google.com/storage/browser/psearch-eval-data-riojucu-sandbox?project=riojucu-sandbox) |
| **Source Repository** | Branch `eval-dashboard` | [duboc/scion-experiment@eval-dashboard](https://github.com/duboc/scion-experiment/tree/eval-dashboard) |

---

## Scripts & Operations

- `scripts/consolidate_factory_data.py`: Consolidates multi-agent factory data into Firestore (`agents`, `tasks`, `eval_runs`, `requirements_compliance`) and exports JSON snapshots to GCS.
- `scripts/ingest_telemetry.py`: Ingests agent telemetry from Cloud Spanner `factory_agent_status` into `public/telemetry.json`.
- `scripts/probe_search.py`: Probes `psearch-serving` for NDCG@10, MRR@10, and latency percentiles, recording results into Cloud Spanner `eval_runs`.
- `scripts/deploy.sh`: Builds and deploys the container to Cloud Run in `riojucu-sandbox` with Secret Manager `github-token`.
