# Product

<!-- impeccable:product-v2 -->

## Summary
`psearch-eval` is the Evaluation, Observability & Software Factory Intelligence Workbench for the Scion `psearch` initiative (`duboc/scion-experiment`).

## Platform
web

## Positioning
An instrument-grade engineering control tower that pairs real-time Scion multi-agent telemetry with live Cloud Spanner hybrid search relevance (`NDCG@10`, `MRR@10`) and latency (`p50/p95/p99`) evaluation across the 10-item Biggy Search / IS 2.0 backlog.

## Operating Context
Used continuously by the Product Owner (`duboc`) and Autonomous Agent Orchestrators (`@eval-lead` and `@lead`) to monitor parallel agent execution (`psearch` on `main` and `psearch-eval` on `eval-dashboard`), run interactive RRF alpha probes against `psearch-serving`, and diagnose workflow or query bottlenecks.

## Evidence on Hand
- Live HTTP probes against `psearch-serving` backed by Cloud Spanner Enterprise (`psearch-instance/psearch-db`, 100 PU) and BigQuery (`psearch_dataset`) in `riojucu-sandbox`.
- Live agent state synchronized from Scion Hub (`hub.db`) across all 10 agents (`psearch` + `psearch-eval`).
- Live GitHub Issues (`#1–#10`) and branch commit history from `duboc/scion-experiment`.

## Product Principles
1. Truth over theater: every metric, latency reading, issue status, and agent state reflects live system state.
2. Earned familiarity: standard web controls, dense tabular data (`tabular-nums`), and hairline structural alignment outrank decorative chrome.
3. Zero AI-slop: no eyebrow kickers above headings, no 4-equal-box KPI cards, no nested cards, no >1px colored side borders, no purple-on-dark gradients, no emoji icons.

## Stack
Go 1.24 HTTP server (`cmd/eval-server`) with embedded HTML5/CSS (`oklch()` token system) and authored SVG visualizations deployed on Cloud Run (`riojucu-sandbox`, `us-central1`).
