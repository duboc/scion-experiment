---
name: psearch-eval
description: Factory Intelligence & Search Quality Workbench for the Scion psearch software factory
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

**System Role:** Precision Telemetry & Quality Evaluation Workbench  
**Mode:** Operate  
**World:** Precision Telemetry Ledger (*Kinpaku & Warm Stone* / *Carbon Ink*)  
**Lead Engineer:** `@ui-craftsman` (Antigravity Harness) reporting to `@eval-lead`  
**Compliance Target:** WCAG AA (Contrast $\ge 4.5:1$, Visible Focus Rings, Tabular Alignment)  

---

## 1. Overview

The `psearch-eval` user interface is engineered according to the **Impeccable Design System** under the **Operate** interaction mode. In Operate mode, the user arrives to accomplish precise technical tasks: verifying search relevance, analyzing multi-agent progress, diagnosing query rank degradation, and validating SLA percentiles.

The visual world is the **Precision Telemetry Ledger**:
- **Utilitarian Elegance**: An interface that feels like a precision high-density laboratory ledger or aerospace flight console rather than a generic SaaS template.
- **No Card Nesting**: Structural separation is achieved via hairline boundary rules (`1px solid var(--border-hairline)`) and subtle background shifts (`var(--bg-subtle)`), eliminating nested "cards inside cards" anti-patterns.
- **Dual-Mode Chromatic Harmony**: Seamless switching between high-clarity **Daylight** (warm stone canvas with deep slate ink) and zero-glare **Carbon** (deep obsidian canvas with high-legibility phosphor ink).

---

## 2. Colors

All colors are formulated in the perceptual **OKLCH** color space to guarantee linear perceptual lightness steps, consistent chroma saturation, and accessibility contrast across both themes.

### Daylight Palette (Kinpaku & Warm Stone)
- `--bg-canvas`: `oklch(97.0% 0.008 245)` — Warm mineral ground, glare-free under ambient light.
- `--bg-surface`: `oklch(99.4% 0.003 245)` — Crisp paper-like elevated surface.
- `--bg-subtle`: `oklch(94.5% 0.010 245)` — Structural dividers and table alternate banding.
- `--border-hairline`: `oklch(87.0% 0.012 245)` — Fine 1px architectural containment.
- `--border-strong`: `oklch(74.0% 0.018 245)` — Active borders, focus, and input boundaries.
- `--text-primary`: `oklch(19.5% 0.016 255)` — Deep charcoal ink ($12.5:1$ contrast against surface).
- `--text-secondary`: `oklch(40.0% 0.018 255)` — Secondary labels ($6.8:1$ contrast).
- `--text-muted`: `oklch(50.0% 0.016 255)` — Metadata and units ($4.6:1$ contrast, strictly AA-safe).

### Carbon Palette (Carbon Ink & Deep Slate)
- `--bg-canvas`: `oklch(14.5% 0.012 250)` — Deep charcoal slate.
- `--bg-surface`: `oklch(17.5% 0.014 250)` — Base module surface.
- `--bg-subtle`: `oklch(20.5% 0.016 250)` — Subtle contrast framing.
- `--border-hairline`: `oklch(27.5% 0.016 250)` — Restrained hairline boundary.
- `--text-primary`: `oklch(95.5% 0.008 80)` — Near-white illumination ($13.1:1$ contrast).
- `--text-secondary`: `oklch(76.0% 0.012 80)` — Legible telemetry readouts ($7.2:1$ contrast).

### Semantic Status Tokens
- **PASS**: `--status-pass: oklch(52% 0.145 155)` / BG: `oklch(94.5% 0.035 155)` (Verified compliance).
- **WARN / PARTIAL**: `--status-warn: oklch(58% 0.155 68)` / BG: `oklch(95% 0.040 75)` (Degraded latency / partial feature).
- **GAP / FAIL**: `--status-gap: oklch(53% 0.185 28)` / BG: `oklch(95% 0.035 28)` (Unimplemented / SLA breach).

---

## 3. Typography

The typographic hierarchy establishes clear informational density without crowding:

1. **Primary Interface Font**: **`Albert Sans`** (`wght@400;500;600;700`)
   - Geometric clarity with open apertures for high legibility at micro-scales.
   - Text balancing (`text-wrap: balance`) enabled on titles to prevent orphan words.
2. **Telemetry & Code Font**: **`JetBrains Mono`** (`wght@400;500;600`)
   - Mandatory for all numbers, latencies, percentiles, git SHAs, UUIDs, and SQL expressions.
   - Enforced `font-variant-numeric: tabular-nums` to ensure perfectly aligned columns in data tables.

### Type Scale
| Role | Font Family | Size | Weight | Tracking |
|---|---|---|---|---|
| Page Header (`h1`) | Albert Sans | `1.25rem` (20px) | 600 | `-0.025em` |
| Zone Header (`h2`) | Albert Sans | `1.0625rem` (17px) | 600 | `-0.015em` |
| Sub-Header (`h3`) | Albert Sans | `0.9375rem` (15px) | 600 | `-0.010em` |
| Body Text (`p`) | Albert Sans | `0.875rem` (14px) | 400 | `normal` |
| Monospace / Code | JetBrains Mono | `0.8125rem` (13px) | 500 | `normal` |
| Micro Telemetry | JetBrains Mono | `0.75rem` (12px) | 500 | `+0.02em` |

---

## 4. Layout

The layout uses an unyielding, high-density structured framework:

- **Sticky Operational Bar**: Fixed top viewport header containing service identifiers, branch metadata, live sync indicators, theme toggle, and manual sweep trigger.
- **Asymmetric Focal Strip**: A primary 3-column asymmetric summary container:
  1. *Relevance Readout*: Large tabular metrics (NDCG@10 and MRR@10) with historical delta indicators.
  2. *Interactive SVG Alpha Curve*: Dynamic visualization of NDCG@10 vs. Hybrid $\alpha$ parameter sweep with the peak highlighted.
  3. *Latency Percentile Matrix*: p50, p95, and p99 millisecond benchmarks against target thresholds.
- **Two-Column Primary Split**:
  - *Left Column (60% width)*: 10 Biggy Search Requirements Ledger with wave filtering tabs.
  - *Right Column (40% width)*: Interactive Hybrid Query Workbench and Multi-Agent Topology Ledger.
- **Responsive Fluidity**: Collapses gracefully to a stacked single-column layout on viewports $< 1080\text{px}$.

---

## 5. Elevation & Depth

In keeping with the Precision Telemetry Ledger aesthetic:
- **No Heavy Drop Shadows**: Traditional diffuse drop shadows are eliminated. Depth is communicated strictly via tonal surface shifts and crisp border hairlines.
- **Elevated Interactive States**: Modals, tooltips, and flyouts use high-precision micro-shadows:
  - Daylight: `0 6px 20px -4px oklch(20% 0.02 255 / 0.08)`
  - Carbon: `0 8px 24px -4px oklch(8% 0.01 250 / 0.45)`
- **Hover Transitions**: Restrained $160\text{ms}$ cubic-bezier transitions (`cubic-bezier(0.22, 1, 0.36, 1)`) with zero bouncy cartoon physics.

---

## 6. Shapes

- **Geometric Rectilinearity**: Sharp, precise edges dominate (`border-radius: 0px` on containers, table cells, and major panels) to evoke ledger engineering sheets.
- **Subtle Micro-Radii**: Controlled `2px` or `4px` radius applied exclusively to interactive inputs, pills, buttons, and status indicator dots to maintain tactile ergonomics.
- **Status Dots**: Distinctive $6\text{px} \times 6\text{px}$ indicator pills with solid semantic fills.

---

## 7. Components

### 7.1 Interactive Hybrid RRF Query Workbench
- Input grid combining query string, dynamic $\alpha$ weight selector ($0.00$ to $1.00$), and locale selector (`pt-BR`, `en-US`, `es-MX`).
- Instantaneous asynchronous execution updating live result cards with rank numbers, stock availability badges, and decomposed scoring vectors.

### 7.2 Requirements Compliance Ledger
- Data table with categorical wave filters (All, Wave 1: Core Hybrid, Wave 2: Inventory & Facets, Wave 3: Merchandising & LTR).
- Status badges accompanied by mandatory text labels: `PASS`, `PARTIAL`, `GAP`, or `not measured`.
- Columnar latency alignment and expandable technical evidence drawers.

### 7.3 Multi-Agent Topology Graph & Inspector
- Direct SVG/DOM node graph depicting agent relationships (`orchestrator` $\to$ `lead` $\to$ `backend` / `frontend` / `reviewer` / `deployer`).
- Interactive node selection binding to a dedicated agent inspector panel displaying turn counts, model invocations, and current task focus.

---

## 8. Do's and Don'ts

### Do's
- **DO** use `font-variant-numeric: tabular-nums` on all numbers, latencies, scores, and dates.
- **DO** maintain strict WCAG AA contrast ratios ($\ge 4.5:1$) for all text and data labels in both themes.
- **DO** ensure status is never conveyed by color alone (always include unambiguous text labels and status dots).
- **DO** query real Spanner / Cloud Run probe endpoints; report real data or unmeasured states.
- **DO** provide visible, high-contrast focus rings (`outline: 2px solid var(--accent-primary); outline-offset: 2px`) for keyboard accessibility.

### Don'ts
- **DON'T** nest cards inside cards or use floating amorphous gradient bubbles.
- **DON'T** use synthetic or invented fallback metrics (e.g. "98.5% uptime" or "0.89 NDCG" without an underlying Spanner `eval_runs` record).
- **DON'T** hardcode dark mode or light mode values directly in component CSS; always reference CSS custom properties.
- **DON'T** allow text truncation without accessible tooltip inspection for long technical error messages or SQL evidence strings.
