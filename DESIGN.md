---
name: psearch-eval
description: Factory Intelligence, 5-Layer Product Evaluation Pyramid & Search Quality Workbench for the Scion psearch software factory
mode: Operate
world: Precision Telemetry Ledger
colors:
  # Daylight Theme Tokens (Kinpaku & Warm Stone)
  daylight-bg-canvas: "oklch(97.0% 0.008 245)"
  daylight-bg-surface: "oklch(99.4% 0.003 245)"
  daylight-bg-subtle: "oklch(94.5% 0.010 245)"
  daylight-bg-elevated: "oklch(91.5% 0.014 245)"
  daylight-border-hairline: "oklch(87.0% 0.012 245)"
  daylight-border-strong: "oklch(74.0% 0.018 245)"
  daylight-text-primary: "oklch(19.5% 0.016 255)"
  daylight-text-secondary: "oklch(40.0% 0.018 255)"
  daylight-text-muted: "oklch(50.0% 0.016 255)"
  daylight-accent-primary: "oklch(52.0% 0.165 245)"
  daylight-accent-hover: "oklch(46.0% 0.165 245)"
  daylight-accent-tint: "oklch(95.0% 0.025 245)"
  daylight-status-pass: "oklch(52.0% 0.145 155)"
  daylight-status-pass-bg: "oklch(94.5% 0.035 155)"
  daylight-status-warn: "oklch(58.0% 0.155 68)"
  daylight-status-warn-bg: "oklch(95.0% 0.040 75)"
  daylight-status-gap: "oklch(53.0% 0.185 28)"
  daylight-status-gap-bg: "oklch(95.0% 0.035 28)"

  # Carbon Theme Tokens (Carbon Ink & Deep Slate)
  carbon-bg-canvas: "oklch(14.5% 0.012 250)"
  carbon-bg-surface: "oklch(17.5% 0.014 250)"
  carbon-bg-subtle: "oklch(20.5% 0.016 250)"
  carbon-bg-elevated: "oklch(24.0% 0.020 250)"
  carbon-border-hairline: "oklch(27.5% 0.016 250)"
  carbon-border-strong: "oklch(38.0% 0.020 250)"
  carbon-text-primary: "oklch(95.5% 0.008 80)"
  carbon-text-secondary: "oklch(76.0% 0.012 80)"
  carbon-text-muted: "oklch(62.0% 0.014 80)"
  carbon-accent-primary: "oklch(68.0% 0.155 245)"
  carbon-accent-hover: "oklch(74.0% 0.155 245)"
  carbon-accent-tint: "oklch(25.0% 0.045 245)"
  carbon-status-pass: "oklch(74.0% 0.150 155)"
  carbon-status-pass-bg: "oklch(24.0% 0.045 155)"
  carbon-status-warn: "oklch(78.0% 0.155 75)"
  carbon-status-warn-bg: "oklch(25.0% 0.050 75)"
  carbon-status-gap: "oklch(72.0% 0.170 28)"
  carbon-status-gap-bg: "oklch(25.0% 0.050 28)"
typography:
  display:
    fontFamily: "'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 600
    lineHeight: "1.2"
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 600
    lineHeight: "1.3"
    letterSpacing: "-0.015em"
  title:
    fontFamily: "'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 600
    lineHeight: "1.35"
    letterSpacing: "-0.01em"
  body:
    fontFamily: "'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.55"
    letterSpacing: "normal"
  label:
    fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, monospace"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: "1.4"
    letterSpacing: "0.02em"
rounded:
  none: "0px"
  sm: "2px"
  md: "4px"
  full: "9999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "14px"
  lg: "20px"
  xl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.daylight-accent-primary}"
    textColor: "{colors.daylight-bg-surface}"
    rounded: "{rounded.none}"
    padding: "7px 14px"
  table-cell:
    padding: "8px 12px"
    borderBottom: "1px solid {colors.daylight-border-hairline}"
---

# Impeccable Design System Specification — `psearch-eval`

**System Role:** Precision Telemetry, 5-Layer Product Evaluation Pyramid & Quality Workbench  
**Mode:** Operate  
**World:** Precision Telemetry Ledger (*Kinpaku & Warm Stone* / *Carbon Ink*)  
**Lead Engineer:** `@ui-craftsman` (Antigravity Harness · `agy v1.3.4` · `gemini-4-argon`) reporting to `@eval-lead`  
**Compliance Target:** WCAG AA (Contrast $\ge 4.5:1$, Visible Focus Rings, Tabular Alignment)  

---

## 1. Overview & Architectural Philosophy

The `psearch-eval` user interface (`eval-dashboard/public/index.html` and `eval-dashboard/public/app.js`) is engineered according to the **Impeccable Design System** (`impeccable.style`) under the **Operate** interaction mode. Operators arrive to verify search relevance (`NDCG@10`, `MRR@10`), inspect the **5-Layer Product Evaluation Methodology Pyramid (`L1–L5`, 76 scenarios, 100% threshold)**, audit **14 Biggy Search Requirements (`4.1.1`–`4.1.14` across Waves 1–5)**, and monitor the 10-agent software factory topology.

The visual world is the **Precision Telemetry Ledger**:
- **Utilitarian Elegance**: High-density laboratory ledger geometry with zero decorative fluff or marketing banners.
- **No Card Nesting**: Structural separation is achieved strictly via `1px solid var(--border-hairline)` boundaries and subtle surface shifts (`var(--bg-subtle)`), avoiding nested cards.
- **Dual-Mode Chromatic Harmony**: Seamless toggle (`toggleTheme()`) between **Daylight** (*Kinpaku & Warm Stone*) and **Carbon** (*Carbon Ink & Deep Slate*), persisted in `localStorage` (`psearch-theme`).

---

## 2. OKLCH Color System & Semantic Badges

All colors are defined in the perceptual **OKLCH** color space to ensure linear lightness steps and WCAG AA contrast ratios across both themes:

### Daylight Palette (`[data-theme="daylight"]`)
- `--bg-canvas`: `oklch(97.0% 0.008 245)` — Warm mineral ground, glare-free under ambient light.
- `--bg-surface`: `oklch(99.4% 0.003 245)` — Crisp ledger surface.
- `--bg-subtle`: `oklch(94.5% 0.010 245)` — Structural dividers, table headers, and alternate banding.
- `--border-hairline`: `oklch(87.0% 0.012 245)` — Fine 1px architectural containment.
- `--text-primary`: `oklch(19.5% 0.016 255)` — Deep charcoal ink ($12.5:1$ contrast).
- `--text-secondary`: `oklch(40.0% 0.018 255)` — Secondary labels ($6.8:1$ contrast).
- `--text-muted`: `oklch(50.0% 0.016 255)` — Metadata and units ($4.6:1$ contrast, strictly AA-safe).

### Carbon Palette (`[data-theme="carbon"]`)
- `--bg-canvas`: `oklch(14.5% 0.012 250)` — Deep obsidian slate.
- `--bg-surface`: `oklch(17.5% 0.014 250)` — Base ledger surface.
- `--bg-subtle`: `oklch(20.5% 0.016 250)` — Subtle framing surface.
- `--border-hairline`: `oklch(27.5% 0.016 250)` — Restrained hairline boundary.
- `--text-primary`: `oklch(95.5% 0.008 80)` — Near-white illumination ($13.1:1$ contrast).
- `--text-secondary`: `oklch(76.0% 0.012 80)` — Legible telemetry readouts ($7.2:1$ contrast).

### Semantic Status Badges
- **`.badge--pass` / `.badge--HEALTHY`**: `--status-pass: oklch(52% 0.145 155)` (Verified `PASS` / `100%` threshold met).
- **`.badge--partial` / `.badge--WARNING`**: `--status-warn: oklch(58% 0.155 68)` (Partial compliance or latency warning).
- **`.badge--pending` / `.badge--CRITICAL`**: `--status-gap: oklch(53% 0.185 28)` (Unmeasured, failed check, or critical diagnostic).

---

## 3. Typography & Tabular Discipline

1. **Primary Interface Font**: **`Albert Sans`** (`wght@400;500;600;700`) with `text-wrap: balance` on section headers.
2. **Telemetry & Code Font**: **`JetBrains Mono`** (`wght@400;500;600`) with mandatory `font-variant-numeric: tabular-nums` (`.mono`, `.num`) on all scores, latencies, scenario counts (`76 / 76`), git SHAs, and agent turn counters.

---

## 4. Layout & Component Architecture

### 4.1 Sticky Operational Topbar & Asymmetric Focal Strip
- **Topbar**: Displays service identity (`psearch-eval`), live status dot, storefront link (`#link-storefront`), manual refresh trigger (`refreshAll()`), and Daylight/Carbon theme toggle (`#theme-toggle`).
- **Asymmetric Focal Strip**:
  - **Primary Metric Readout**: `NDCG@10` (`#val-ndcg`), `MRR@10` (`#val-mrr`), optimal hybrid weight (`#val-opt-alpha`), backend mode (`#lbl-backend-mode`), and UTC probe timestamp (`#val-timestamp`).
  - **Secondary Telemetry Strip**:
    - `p50 Latency` (`#val-p50`) & `p95 / p99 Latency` (`#val-p95`)
    - `Requirements Status` (`#val-reqs-pass` $\to$ `14 / 14 PASS · 0 PARTIAL`)
    - `5-Layer Pyramid Status` (`#val-pyramid-pass` $\to$ `76 / 76 PASS (100%)`)
    - `Active Agents` (`#val-agents-cnt` $\to$ `10 / 10 running`)
  - **Inline SVG Alpha Parameter Sweep (`#svg-alpha-curve`)**: Plots live `NDCG@10` across $\alpha \in \{0.00, 0.25, 0.50, 0.65, 0.80, 1.00\}$ with peak annotation.

### 4.2 5-Layer Product Evaluation Methodology Pyramid (`#section-pyramid`)
- **Pyramid Stack (`#pyramid-stack` & `.pyramid-layer-card`)**:
  - Renders 5 horizontal tier rows (`L1` through `L5`), each displaying:
    - Tier badge (`L1`–`L5`) and layer title (`L1. Trajectory & State Sequence`, `L2. Policy & Business Guardrails`, `L3. Adversarial & Red-Team Robustness`, `L4. Multi-Mode Routing & Engine Parity`, `L5. Live Vertex AI & Factory Telemetry`).
    - Progress bar track (`.pyramid-bar-track` / `.pyramid-bar-fill`) proportional to `scenarios_passed / scenarios_count` (`100%` fill in `--status-pass`).
    - Tabular scenario counter (`12/12`, `15/15`, `16/16`, `28/28`, `5/5`) and `PASS` status badge.
- **Interactive Scenario Drilldown Ledger (`#tbody-pyramid-scenarios`)**:
  - Filterable via layer buttons (`All Layers (76)`, `L1 (12)`, `L2 (15)`, `L3 (16)`, `L4 (28)`, `L5 (5)`) bound to `setPyramidLayerFilter(layerId, btn)`.
  - Columns: `Layer`, `Scenario ID`, `Scenario Name`, `Status Badge`, `Latency (ms)`, and `Empirical Evidence`.

### 4.3 14 Biggy Search Requirements Compliance Ledger (`#section-requirements`)
- **Wave Filter Bar**: Interactive filter buttons (`All`, `Wave 1`, `Wave 2`, `Wave 3`, `Wave 4`, `Wave 5`) bound to `setWaveFilter(wave, btn)`.
- **Ledger Table (`#tbody-reqs`)**: Renders all 14 requirements (`4.1.1`–`4.1.14`, Issues `#1`–`#10` and `#19`–`#22`) with clickable GitHub issue links, wave tags, owner agents, `PASS` badges, and live probe evidence.

### 4.4 Interactive Hybrid RRF Query Workbench & Multi-Agent Topology
- **Query Workbench (`runWorkbenchQuery()`)**: Live query input (`#wb-query`), hybrid $\alpha$ selector (`#wb-alpha`), and locale selector (`#wb-locale`) executing real-time proxy probes via `/api/workbench` and displaying stock badges and hybrid scores in `#tbody-workbench`.
- **Multi-Agent Factory Topology & Diagnostics**:
  - Interactive topology node pills for both `psearch` (`lead`, `backend`, `frontend`, `reviewer`, `deployer`) and `psearch-eval` (`eval-lead`, `search-evaluator`, `factory-analyst`, `ui-craftsman`, `eval-deployer`), bound to `#tbody-agents`.
  - Factory Diagnostics Ledger (`#diag-container`) and Git Commit Ledger (`#tbody-commits`).

---

## 5. Express Server Routes (`eval-dashboard/server.js`)

| HTTP Route | Method | Data Source | Response Contract |
|---|---|---|---|
| **`GET /api/eval`** | `GET` | Cloud Spanner `eval_runs` (`ORDER BY created_at DESC LIMIT 1`) | Returns `run_id`, `overall_ndcg_at_10`, `overall_mrr_at_10`, `p50_latency_ms`, `p95_latency_ms`, `p99_latency_ms`, `optimal_alpha`, `pass_count` (`14`), `requirements` (`14` items), `pyramid_layers` (`5` layers), `pyramid_summary` (`76/76 PASS`), `alpha_curve`, `golden_queries`, and `diagnostics`. |
| **`GET /api/agents`** | `GET` | Cloud Spanner `factory_agent_status` | Returns `source: "spanner"` and `agents` array (`10` agents across `psearch` and `psearch-eval`) with `phase`, `activity`, `current_turns`, `model_calls`, and `last_summary`. |
| **`GET /api/backlog`** | `GET` | GitHub REST API (`duboc/scion-experiment`) | Returns `issues`, `main_commits`, and `eval_commits` authenticated via Secret Manager `github-token`. |
| **`GET /api/workbench`** | `GET` | Live HTTP proxy to `psearch-serving` (`/api/search`) | Proxies `q`, `alpha`, and `locale` to `psearch-serving`, returning `results`, `facets`, `effective_alpha`, and `probe_latency_ms`. |
| **`GET /api/info`** | `GET` | Service runtime configuration | Returns `service: "psearch-eval-dashboard"`, `spanner_instance: "psearch-instance"`, `spanner_database: "psearch-db"`, and `serving_url`. |
| **`GET /health`** | `GET` | Container health probe | Returns `200 OK` with `{"status": "ok"}`. |

---

## 6. Do's and Don'ts

### Do's
- **DO** use `font-variant-numeric: tabular-nums` on all numbers, latencies, scores, scenario tallies, and timestamps.
- **DO** maintain strict WCAG AA contrast ratios ($\ge 4.5:1$) for all text and data labels in both Daylight and Carbon themes.
- **DO** ensure status is never conveyed by color alone (always pair semantic colors with explicit text badges: `PASS`, `PARTIAL`, `FAIL`, `HEALTHY`, `WARNING`, `CRITICAL`).
- **DO** query real Cloud Spanner `eval_runs` and live `psearch-serving` endpoints; never invent synthetic fallback metrics.

### Don'ts
- **DON'T** nest cards inside cards or use decorative gradient blobs.
- **DON'T** hardcode theme colors outside CSS custom properties.
- **DON'T** commit real GCP project IDs, personal emails, or secret tokens into documentation or client assets.
