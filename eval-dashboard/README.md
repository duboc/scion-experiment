# psearch-eval-dashboard

Evaluation & Observability console for the `psearch` multi-agent factory on Google Cloud Platform (`riojucu-sandbox`).

---

## Live Endpoints & Console URLs

| Resource | Description | Live Endpoint / Console URL |
|---|---|---|
| **Live Evaluation Dashboard** | Cloud Run Service (Impeccable UI) | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) |
| **Target Search Service** | Target search backend under evaluation | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) |
| **Cloud Spanner Database** | Instance `psearch-instance` / DB `psearch-db` | [Spanner Console](https://console.cloud.google.com/spanner/instances/psearch-instance/databases/psearch-db/details?project=riojucu-sandbox) |
| **Cloud Firestore** | Realtime agent state, tasks & 10 Biggy reqs | [Firestore Console](https://console.cloud.google.com/firestore/databases/-default-/data/panel?project=riojucu-sandbox) |
| **Cloud Storage (GCS)** | Raw telemetry traces & evaluation dumps | [gs://psearch-eval-data-riojucu-sandbox](https://console.cloud.google.com/storage/browser/psearch-eval-data-riojucu-sandbox?project=riojucu-sandbox) |
| **Source Repository** | Branch `eval-dashboard` | [duboc/scion-experiment@eval-dashboard](https://github.com/duboc/scion-experiment/tree/eval-dashboard) |

---

## Scripts & Operations

- `scripts/ingest_telemetry.py`: Ingests agent telemetry and turn usage into Firestore and GCS.
- `scripts/probe_search.py`: Probes `psearch-serving` for NDCG@10, MRR@10, and latency percentiles.
- `scripts/deploy.sh`: Builds and deploys the container to Cloud Run in `riojucu-sandbox`.
