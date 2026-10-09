# `psearch` Knowledge Plane — Master Index

Welcome to the canonical **Knowledge Plane** for the `psearch` Software Factory on [`duboc/scion-experiment`](https://github.com/duboc/scion-experiment).
All agents (`lead`, `backend`, `frontend`, `reviewer`, `deployer`) MUST consult this index and the linked ADRs before implementing any GitHub Issue (`#1`–`#10`).

## 1. Architecture Decision Records (ADRs by Backlog Wave)

| ADR | Covered GitHub Issues | Functional Requirements (Section 4.1) |
| :--- | :--- | :--- |
| [`adr/ADR-001-hybrid-and-vector-1024.md`](adr/ADR-001-hybrid-and-vector-1024.md) | **Issue #1**, **Issue #2** | **4.1.1 Busca Híbrida** (Dynamic $\alpha$ RRF weight in SQL) & **4.1.2 Busca Vetorial** (1024-dim ScaNN, Blue/Green `embedding_v1`/`v2` switch, `embedding_cache` in Spanner) |
| [`adr/ADR-002-catalog-locale-avail.md`](adr/ADR-002-catalog-locale-avail.md) | **Issue #3**, **Issue #4** | **4.1.3 Disponibilidade** (`is_available`, `stock`, `channels`, unavailable sorted last) & **4.1.4 Catálogo Multilanguage** (`tenant_id`, `locale` composite indexing) |
| [`adr/ADR-003-facets-and-synonyms.md`](adr/ADR-003-facets-and-synonyms.md) | **Issue #5**, **Issue #7** | **4.1.5 Facets Server-Side** (Concurrent Go `errgroup` + Spanner `GROUP BY`) & **4.1.7 Low-Relevance Synonyms** (`search_synonyms` table + `0.3 * SCORE()` query expansion) |
| [`adr/ADR-004-merchandising-boosts.md`](adr/ADR-004-merchandising-boosts.md) | **Issue #6** | **4.1.6 Relevance & Merchandising Rules** (`merchandising_rules` in Spanner + 8 numeric boost signals in SQL score) |
| [`adr/ADR-005-delivery-promises.md`](adr/ADR-005-delivery-promises.md) | **Issue #8** | **4.1.8 Delivery Promises** (`products` → `skus` → `regional_inventory` via Spanner `INTERLEAVE IN PARENT` + CEP/ZIP SLA filter) |
| [`adr/ADR-006-autocomplete-and-ltr.md`](adr/ADR-006-autocomplete-and-ltr.md) | **Issue #9**, **Issue #10** | **4.1.9 Autocomplete** (BigQuery Analytics + Spanner `TOKENIZE_NGRAMS`) & **4.1.10 Learning to Rank** (2-Stage Spanner Top-100 + Go ~2ms in-memory re-ranker) |

## 2. API Contracts & Operational Playbooks

- [`contracts/biggy-search-openapi.yaml`](contracts/biggy-search-openapi.yaml) — Canonical REST contract for `psearch-serving` (`/api/search`, `/api/autocomplete`, `/api/rules`, `/api/synonyms`, `/api/info`, `/api/health`).
- [`playbooks/SPANNER-DDL-MIGRATION-GUIDE.md`](playbooks/SPANNER-DDL-MIGRATION-GUIDE.md) — Rules for online Cloud Spanner DDL updates in `riojucu-sandbox` (`psearch-instance` / `psearch-db`).
- [`playbooks/GITHUB-ISSUE-FACTORY-WORKFLOW.md`](playbooks/GITHUB-ISSUE-FACTORY-WORKFLOW.md) — Trunk-based execution protocol for `lead`, `backend`, `frontend`, `reviewer`, and `deployer`.
