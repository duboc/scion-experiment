# Product

<!-- impeccable:product-v2 -->

## Summary
`psearch-eval` is the **Evaluation, Observability & Software Factory Intelligence Workbench** for the Scion `psearch` initiative (`duboc/scion-experiment`). It continuously observes what the `psearch` multi-agent software factory (`lead`, `backend`, `frontend`, `reviewer`, `deployer`) is doing in Scion, benchmarks the live `psearch-serving` engine against the 10-item Biggy Search / IS 2.0 backlog (Section 4.1, GitHub Issues `#1–#10`), and diagnoses agent and search-quality bottlenecks so the Product Owner and Tech Lead can steer the factory with empirical evidence.

## Platform
`web`

## Stack
Go 1.24 HTTP server (`cmd/eval-server`) + zero-dependency Vanilla HTML5, CSS Custom Properties (`oklch()` token system), and authored SVG data visualizations deployed on Cloud Run (`riojucu-sandbox`, `us-central1`).

## Users & Operating Context
- **Primary Users:** Product Owner / Product Manager (`duboc`) and Autonomous Agent Orchestrators (`@eval-lead` and `@lead`).
- **Situation & Job:** Inspecting live multi-agent execution across Scion projects (`psearch` on `main` and `psearch-eval` on `eval-dashboard`), verifying whether newly shipped commits actually improve search relevance (`NDCG@10`, `MRR@10`) and tail latency (`p95`/`p99`) on Cloud Spanner (`psearch-instance/psearch-db`), and identifying which agent or backlog item needs intervention next.
- **Visitor Mode:** `Operate` (with `Read` depth on architectural diagnostics and ADRs).

## Capabilities & Evidence
1. **Scion Agent Factory Telemetry & Diagnostics:** Live inspection of all agents across `psearch` (`lead`, `backend`, `frontend`, `reviewer`, `deployer`) and `psearch-eval` (`eval-lead`, `factory-analyst`, `search-evaluator`, `ui-craftsman`, `eval-deployer`), tracking harness, phase, activity, turns, model calls, delegation topology, and automated bottleneck recommendations.
2. **Golden Query Relevance & Latency Lab:** Live execution of golden e-commerce queries (`running shoes`, `tenis corrida`, `developer laptop`, `noise cancelling headphones`, `geladeira inox`, `cafeteira espresso`) across varying RRF $\alpha$ weights, computing `NDCG@10`, `MRR@10`, and `p50/p95/p99` latency against `psearch-serving`.
3. **Interactive Hybrid RRF Workbench:** Real-time query probe allowing the Product Owner to test any query + $\alpha \in [0, 1]$ against `psearch-serving` and inspect per-product lexical vs. vector rank fusion.
4. **Section 4.1 Backlog & Wave Readiness Matrix:** Live HTTP verification of GitHub Issues `#1–#10` across Waves 1–4 paired with commit activity from `duboc/scion-experiment`.

## Constraints & Principles
- **Truth over theater:** Every metric, latency reading, issue state, and agent status reflects real state from `psearch-serving`, `hub.db`, Cloud Spanner, and GitHub (`duboc/scion-experiment`).
- **Impeccable Craft Floor:** Zero AI-slop patterns (no eyebrow kickers above headings, no 4-equal-box KPI ribbons, no nested cards, no >1px side-tab borders, no purple-on-dark gradients, no untinted grays, no emoji icons).
