# `psearch-eval` — Control Tower, Search Quality Eval & Multi-Agent Monitor

This branch (`eval-dashboard`) belongs to the parallel Scion project **`psearch-eval`**, which monitors and evaluates the **`psearch`** Software Factory (`main` branch) and its 10-item Biggy Search backlog (`Issues #1–#10`).

---

## Live Service & Console URLs

| Service / Component | Description | Live Endpoint / Console URL |
|---|---|---|
| **`psearch-eval-dashboard`** | Cloud Run Service (Impeccable UI) | [https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) |
| **`psearch-serving`** | Target search backend under evaluation | [https://psearch-serving-mhjw244bgq-uc.a.run.app](https://psearch-serving-mhjw244bgq-uc.a.run.app) |
| **Cloud Spanner Instance** | Instance `psearch-instance` (100 PU) / DB `psearch-db` | [Spanner Database Console](https://console.cloud.google.com/spanner/instances/psearch-instance/databases/psearch-db/details?project=riojucu-sandbox) |
| **GitHub Source Branch** | Branch `eval-dashboard` | [duboc/scion-experiment@eval-dashboard](https://github.com/duboc/scion-experiment/tree/eval-dashboard) |
| **Design System** | Impeccable high-craft UI principles | [Impeccable Style Guide](https://impeccable.style) |

---

## Project Structure

```
/workspace/
├── cmd/eval-server/         # Go-based evaluation server & Impeccable embedded UI
│   ├── main.go
│   └── ui.html
├── internal/eval/           # Search quality evaluator engine & Biggy requirement probes
│   ├── runner.go
│   └── runner_test.go
├── eval-dashboard/          # Standalone dashboard package, scripts & Dockerfile
│   ├── server.js            # Express API server for telemetry and evaluation
│   ├── public/
│   │   ├── index.html       # Impeccable UI workbench
│   │   └── app.js           # Client-side reactive telemetry bindings
│   ├── scripts/
│   │   ├── ingest_telemetry.py # Multi-agent factory telemetry ingestion
│   │   ├── probe_search.py     # Live NDCG@10, MRR@10, latency probe worker
│   │   └── deploy.sh           # Cloud Run deployment script
│   └── Dockerfile
├── DESIGN.md                # Impeccable Design System specifications
├── DEPLOYMENT.md            # Cloud Run service metadata & deployment status
└── PRODUCT.md               # Product requirements and 10 Biggy Search specs
```

---

## Quick Start & Verification

```bash
# 1. Verify Impeccable design quality
/workspace/.gemini/skills/impeccable/scripts/impeccable detect cmd/eval-server/ui.html
/workspace/.gemini/skills/impeccable/scripts/impeccable detect eval-dashboard/public/index.html

# 2. Run unit tests
go test ./...

# 3. Probe psearch-serving quality (NDCG@10, MRR@10, latency)
python3 eval-dashboard/scripts/probe_search.py
```
