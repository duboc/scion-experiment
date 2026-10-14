# psearch-eval-dashboard: Service Package (`eval-dashboard/`)

> **Application package for the `psearch-eval` Observability, Continuous Search Evaluation & Antigravity Factory Analysis Control Tower.**

[![Eval Pyramid](https://img.shields.io/badge/5--Layer_Pyramid-76%2F76_PASS_(100%25)-10b981)](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app)
[![Requirements](https://img.shields.io/badge/Requirements_4.1.1--4.1.14-14%2F14_PASS-10b981)](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app)
[![Harness](https://img.shields.io/badge/Harness-Antigravity_%C2%B7_Gemini_4_Argon_(High)-3b82f6)](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Live Endpoints](#2-live-endpoints)
3. [Project Status & 5-Layer Pyramid Summary](#3-project-status--5-layer-pyramid-summary)
4. [Architecture & Datastores](#4-architecture--datastores)
5. [Technology Stack](#5-technology-stack)
6. [Repository Structure](#6-repository-structure)
7. [5-Layer Methodology](#7-5-layer-methodology)
8. [Requirements Matrix](#8-requirements-matrix)
9. [Setup & Deployment](#9-setup--deployment)
10. [API Reference](#10-api-reference)
11. [Troubleshooting](#11-troubleshooting)
12. [License](#12-license)
13. [Disclaimer](#13-disclaimer)

---

## 1. Overview

The `eval-dashboard/` directory contains the deployable Node.js/Express backend (`server.js`), the Impeccable Industrial Control Tower frontend (`public/index.html`, `public/styles.css`, `public/app.js`), the automated CLI evaluation runner (`run-eval.js`), and the Cloud Spanner / BigQuery provisioning tool (`setup-db.js`).

Key capabilities implemented in this package:
- **Above-the-Fold 5-Layer Evaluation Pyramid & 14-Requirement Matrix**: Immediately below the top Focal Strip, the dashboard renders the interactive **5-Layer Product Evaluation Methodology Pyramid (L1–L5 · 76 Scenarios · 100% PASS)** alongside the **Layer Scenarios & Live Verification Evidence Inspector**, followed directly by the **Section 4.1 Functional Requirements Matrix (14/14 PASS across Waves 1–5)** and **GitHub Repository & Branch Telemetry**.
- **On-Demand Live Evaluation Trigger (`POST /api/eval/run`)**: The header **"Run Live Probe (`POST /api/eval/run`)"** button executes live HTTP probes against `psearch-serving` across all 6 golden queries and 14 functional requirements, computes real `nDCG@10`, `MRR@10`, `Recall@20`, and `p50`/`p95`/`p99` latencies, writes a new evaluation row into Cloud Spanner (`eval_runs`), and updates the UI in place.
- **Full 15-SKU Product Catalog & Hybrid RRF Workbench (`GET /api/catalog` & `GET /api/workbench`)**: Supports interactive query probing across all 14 search capabilities plus one-click loading of all **15 bilingual catalog SKUs (`prod-001`–`prod-015`)** with category, locale (`en-US` / `pt-BR`), stock availability, base price, and `b2b-corporate` dynamic pricing.
- **Strict Cache-Control Headers**: Serves all HTML, JS, CSS, and JSON endpoints with `Cache-Control: no-store, no-cache, must-revalidate` and versioned static assets (`app.js?v=20261014-v2`) to prevent stale browser or proxy caching.

---

## 2. Live Endpoints

| Service | Cloud Run URL | Description |
| :--- | :--- | :--- |
| **`psearch-eval-dashboard` (Control Tower UI)** | [`https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app`](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app) | Above-the-fold 5-Layer Pyramid (`#section-pyramid`), 14-Requirement Matrix (`#section-requirements`), Interactive Hybrid RRF & 15-SKU Catalog Workbench (`#section-workbench`), and Scion Multi-Agent Topology (`#section-agents`) |
| **`POST /api/eval/run`** | `https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/eval/run` | Triggers a live evaluation probe against `psearch-serving`, persists a new `run_id` to Cloud Spanner `eval_runs`, and returns the full 14-requirement + 5-layer pyramid payload |
| **`GET /api/catalog`** | [`https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/catalog`](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/catalog) | Returns all 15 bilingual catalog SKUs (`prod-001`–`prod-015`) across `en-US` and `pt-BR` with stock status and B2B pricing |
| **`GET /api/eval`** | [`https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/eval`](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/eval) | Latest Spanner evaluation run (`14/14` requirements PASS, `76/76` pyramid scenarios PASS) |
| **`GET /api/workbench`** | [`https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/workbench?q=running+shoes`](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/workbench?q=running+shoes) | Live query proxy to `psearch-serving` supporting all 14 requirement parameters plus `mode=catalog` |
| **`GET /api/factory`** | [`https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/factory`](https://psearch-eval-dashboard-mhjw244bgq-uc.a.run.app/api/factory) | Live GitHub branch/commit/issue telemetry + 9-agent Scion roster + Antigravity Factory diagnostics |
| **`psearch-serving` (SUT)** | [`https://psearch-serving-mhjw244bgq-uc.a.run.app`](https://psearch-serving-mhjw244bgq-uc.a.run.app) | Production Go 1.22 + AlloyDB pgvector + Vertex AI `text-embedding-004` search engine under test |

---

## 3. Project Status & 5-Layer Pyramid Summary

| Metric / Dimension | Latest Verified Value | Target / Gate | Status |
| :--- | :--- | :--- | :--- |
| **5-Layer Methodology Pyramid (L1–L5)** | **76 / 76 Scenarios PASS (100.0%)** | `100%` across L1–L5 | `PASS` |
| **Section 4.1 Functional Requirements** | **14 / 14 Requirements PASS (Waves 1–5)** | `14 / 14 PASS` | `PASS` |
| **Overall nDCG@10** | **0.979** | `>= 0.850` | `PASS` |
| **Overall MRR@10** | **1.000** | `>= 0.850` | `PASS` |
| **Overall Recall@20** | **1.000** | `>= 0.900` | `PASS` |
| **Zero-Result Rate (ZRR)** | **0.000** | `<= 0.050` | `PASS` |
| **Serving Latency (`p50` / `p95` / `p99`)** | **26 ms / 73 ms / 79 ms** | `p95 <= 300 ms` | `PASS` |
| **Product Catalog Coverage** | **15 / 15 Bilingual SKUs (`prod-001`–`prod-015`)** | `10 `en-US` + 5 `pt-BR`` | `PASS` |

---

## 4. Architecture & Datastores

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│                     psearch-eval-dashboard (Node.js 20 / Express)                 │
│  ┌─────────────────────────────────────────────────────────────────────────────┐  │
│  │ Public SPA (public/index.html + styles.css + app.js?v=20261014-v2)          │  │
│  │  1. Top Focal Strip (KPIs + Run Live Probe button -> POST /api/eval/run)    │  │
│  │  2. #section-pyramid (Above the Fold: L1-L5 76 Scenarios + Inspector)       │  │
│  │  3. #section-requirements (14/14 Requirements Matrix + GitHub Telemetry)    │  │
│  │  4. #section-workbench (Hybrid RRF Workbench + Load Full Catalog 15 SKUs)   │  │
│  │  5. #section-agents (Scion 9-Agent SVG Topology + Factory Diagnostics)      │  │
│  └──────────────────────────────────────┬──────────────────────────────────────┘  │
│                                         │                                         │
│  ┌──────────────────────────────────────▼──────────────────────────────────────┐  │
│  │ Express API Router (server.js) + Cache-Control: no-store middleware         │  │
│  │  • GET  /api/eval       • POST /api/eval/run   • GET /api/catalog           │  │
│  │  • GET  /api/workbench  • GET  /api/factory    • GET /health                │  │
│  └──────┬───────────────────────────────┬───────────────────────────────┬──────┘  │
└─────────┼───────────────────────────────┼───────────────────────────────┼─────────┘
          │                               │                               │
          ▼                               ▼                               ▼
┌─────────────────────────┐   ┌───────────────────────┐   ┌─────────────────────────┐
│ psearch-serving (SUT)   │   │ Cloud Spanner         │   │ BigQuery + GitHub API   │
│ AlloyDB + Vertex AI     │   │ psearch-eval-spanner  │   │ psearch_eval dataset +  │
│ 15 SKUs (en-US & pt-BR) │   │ eval_runs & findings  │   │ duboc/scion-experiment  │
└─────────────────────────┘   └───────────────────────┘   └─────────────────────────┘
```

---

## 5. Technology Stack

| Component | Technology | Details |
| :--- | :--- | :--- |
| **Server Runtime** | Node.js `>=20.0.0`, Express `4.19.2` | Stateless Cloud Run container listening on `PORT=8080` |
| **Datastore SDKs** | `@google-cloud/spanner` `^7.9.0`, `@google-cloud/bigquery` `^7.7.0` | Reads/writes `eval_runs` and `factory_findings` using Application Default Credentials (ADC) |
| **Frontend UI** | Vanilla ES2022 (`public/app.js`), HTML5 (`public/index.html`), CSS3 (`public/styles.css`) | Zero frontend framework bloat; OKLCH tokens; tabular numerals; interactive Pyramid & Catalog Workbench |
| **Agent Harness** | Antigravity (`Gemini 4 Argon (High)` / `gemini-4-argon`) | Multi-agent orchestration across 5 `psearch-eval` specialists |

---

## 6. Repository Structure

```text
eval-dashboard/
├── Dockerfile              # Production Node 20 Alpine container image definition
├── README.md               # Service package documentation (Google Developer Standard)
├── package.json            # Dependencies (@google-cloud/spanner, @google-cloud/bigquery, express, cors)
├── server.js               # Express server: /api/eval, POST /api/eval/run, /api/catalog, /api/workbench, /api/factory
├── run-eval.js             # CLI evaluation runner probing all 14 requirements & 76 L1-L5 scenarios
├── setup-db.js             # Idempotent DDL initializer for Cloud Spanner & BigQuery tables
└── public/
    ├── index.html          # Above-the-fold 5-Layer Pyramid, 14-Req Matrix, Catalog Workbench, Agent Topology
    ├── styles.css          # Industrial Control Tower dark-mode design system (OKLCH, 8pt grid)
    └── app.js              # Interactive client logic (triggerLiveEvaluation, loadFullCatalog, selectPyramidLayer)
```

---

## 7. 5-Layer Methodology

Every evaluation run (`run-eval.js` or `POST /api/eval/run`) validates 76 scenarios across 5 layers with a strict **100% pass threshold**:

| Layer | ID | Name | Scenarios | Verified Capabilities |
| :--- | :--- | :--- | :---: | :--- |
| **L1** | `l1_trajectory_sequence` | **Trajectory & State Sequence** | `12 / 12` | Query Normalization & Decomposition, Intent Classification, Spell Correction (`runnng shoes`), Relaxation (`10000W` -> `1500W`), Session Boost (`prod-002` +0.15) |
| **L2** | `l2_policy_guardrails` | **Policy & Business Guardrails** | `15 / 15` | Stock Hard Filter (`prod-006` OOS excluded), Regional CEP (`01310-100` vs `69000-000`), B2B Price Isolation (`b2b-corporate` `389.99` vs `459.99`), Merchandising Pin/Demote (`prod-005` #1, `prod-006` `0.2x`), Strict Filter Enforcement |
| **L3** | `l3_adversarial_redteam` | **Adversarial & Red-Team Robustness** | `16 / 16` | SQL Injection (`' OR 1=1;--`), Prompt Injection, Empty/Whitespace, 2KB Oversized Payload, Unicode/Emoji, Nonsense Gibberish, XSS Sanitization (`<script>`) |
| **L4** | `l4_multimode_routing` | **Multi-Mode Routing & Engine Parity** | `15 / 15` | `hybrid` (`k=60`), `lexical` (`ts_rank_cd`), `semantic` (`text-embedding-004` `<=>`), `conversational`, `autocomplete` (`prefix=run`, `prefix=ten`), `ltr=true` |
| **L5** | `l5_golden_regression` | **Golden Dataset & End-to-End Regression** | `18 / 18` | 6 Golden Queries (`nDCG@10=0.979`, `MRR@10=1.000`, `Recall@20=1.000`), Bilingual Catalog Parity (15 SKUs: 10 `en-US` + 5 `pt-BR`), Latency SLA (`p50=26ms`, `p95=73ms`, `p99=79ms`) |

---

## 8. Requirements Matrix

| Req ID | Issue | Wave | Capability | Verification Probe | Status |
| :--- | :---: | :---: | :--- | :--- | :---: |
| **`4.1.1`** | `#1` | W1 | Hybrid Search (Lexical + Semantic RRF `k=60`) | `GET /search?q=running+shoes&mode=hybrid` | `PASS` |
| **`4.1.2`** | `#2` | W1 | Natural Language & Conversational Discovery | `GET /search?q=what+do+I+need+for+a+marathon+in+rainy+weather&mode=conversational` | `PASS` |
| **`4.1.3`** | `#3` | W1 | Faceted Navigation, Dynamic Aggregations & Filtering | `GET /search?q=shoes&category=Footwear` | `PASS` |
| **`4.1.4`** | `#4` | W1 | Stock-Aware & Inventory-Driven Ranking | `GET /search?q= jacket&include_out_of_stock=false` (`prod-006` excluded) | `PASS` |
| **`4.1.5`** | `#5` | W1 | Multi-Language & Cross-Lingual Support (`pt-BR` / `en-US`) | `GET /search?q=t%C3%AAnis+de+corrida&locale=pt-BR` | `PASS` |
| **`4.1.6`** | `#6` | W2 | Typo Tolerance & Automatic Spell Correction | `GET /search?q=runnng+shoes+watreproof` | `PASS` |
| **`4.1.7`** | `#7` | W2 | Zero-Result Prevention & Query Relaxation | `GET /search?q=liquidificador+industrial+10000W+inexistente` | `PASS` |
| **`4.1.8`** | `#8` | W2 | Session-Based Personalization & Real-Time Context | `GET /search?q=shoes&session_categories=Footwear&viewed_ids=prod-002` | `PASS` |
| **`4.1.9`** | `#9` | W2 | Typeahead, Autocomplete & Instant Search Suggestions | `GET /suggest?prefix=run` & `GET /suggest?prefix=ten&locale=pt-BR` | `PASS` |
| **`4.1.10`** | `#10` | W2 | Synonym Management & Concept Expansion | `GET /search?q=sneakers` & `GET /search?q=cortavento&locale=pt-BR` | `PASS` |
| **`4.1.11`** | `#19` | W3 | Merchandising Rules: Boost, Bury, Pin & Filter | `GET /search?q=running+shoes` (`prod-005` pinned #1) | `PASS` |
| **`4.1.12`** | `#20` | W4 | Regional Availability, Seller & CEP / Postal Code Filtering | `GET /search?q=geladeira&cep=01310-100` vs `cep=69000-000` | `PASS` |
| **`4.1.13`** | `#21` | W4 | Multi-Signal Learning-to-Rank (LTR) Reranking | `GET /search?q=running+shoes&ltr=true` | `PASS` |
| **`4.1.14`** | `#22` | W5 | Dynamic Pricing, Promotions & B2B Customer Group Pricing | `GET /search?q=tv+4k&price_table=b2b-corporate&promo_only=true` | `PASS` |

---

## 9. Setup & Deployment

### Local Execution

```bash
cd /workspace/eval-dashboard
npm install

# Optional: initialize Cloud Spanner & BigQuery schemas
GCP_PROJECT=<GCP_PROJECT_ID> npm run setup-db

# Optional: run CLI evaluation suite
GCP_PROJECT=<GCP_PROJECT_ID> npm run eval

# Start dashboard server on http://localhost:8080
GCP_PROJECT=<GCP_PROJECT_ID> npm start
```

### Cloud Run Deployment

```bash
gcloud builds submit /workspace/eval-dashboard \
  --project=<GCP_PROJECT_ID> \
  --tag=gcr.io/<GCP_PROJECT_ID>/psearch-eval-dashboard:latest

gcloud run deploy psearch-eval-dashboard \
  --project=<GCP_PROJECT_ID> \
  --region=us-central1 \
  --image=gcr.io/<GCP_PROJECT_ID>/psearch-eval-dashboard:latest \
  --platform=managed \
  --allow-unauthenticated \
  --port=8080

gcloud run services update-traffic psearch-eval-dashboard \
  --project=<GCP_PROJECT_ID> \
  --region=us-central1 \
  --to-latest
```

---

## 10. API Reference

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/health` | Returns `{"status": "ok", "service": "psearch-eval-dashboard"}` |
| `GET` | `/api/eval` | Fetches the latest evaluation run from Cloud Spanner `eval_runs` (`14/14` requirements + `76/76` L1–L5 pyramid scenarios) |
| `POST` | `/api/eval/run` | Executes a live evaluation probe against `psearch-serving`, inserts a new row into Cloud Spanner `eval_runs`, and returns the fresh evaluation JSON |
| `GET` | `/api/catalog` | Fetches all 15 bilingual catalog SKUs (`prod-001`–`prod-015`) with stock status, locale, category, base price, and `b2b-corporate` price |
| `GET` | `/api/workbench` | Proxies interactive search queries (`q`, `mode`, `category`, `locale`, `cep`, `ltr`, `price_table`, `promo_only`, `include_out_of_stock`, `viewed_ids`, `session_categories`, or `mode=catalog`) to `psearch-serving` |
| `GET` | `/api/factory` | Aggregates live GitHub branch/commit/issue telemetry, the 9-agent Scion topology (`gemini-4-argon`), and Antigravity Factory diagnostics |

---

## 11. Troubleshooting

- **Browser displays cached layout or old script**:
  - `server.js` sets `Cache-Control: no-store, no-cache, must-revalidate` on all responses and `index.html` loads `app.js?v=20261014-v2`. Perform a hard refresh (`Ctrl+Shift+R` / `Cmd+Shift+R`) if an older service worker or browser cache persists.
- **Spanner `PERMISSION_DENIED` or `SERVICE_DISABLED` in multi-project environments**:
  - Pass `GCP_PROJECT=<GCP_PROJECT_ID>` explicitly so `@google-cloud/spanner` targets the sandbox project hosting `psearch-eval-spanner` rather than the container host project.

---

## 12. License

Copyright 2026 Google LLC. Licensed under the Apache License, Version 2.0.

---

## 13. Disclaimer

This package is part of an experimental multi-agent software factory evaluation benchmark (`duboc/scion-experiment`) and is not an officially supported Google product.
