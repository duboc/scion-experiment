# Design System — `psearch-eval` Workbench

<!-- impeccable:design-v2 -->

## Visual Direction & World
**World:** *Precision Telemetry Ledger (Kinpaku & Warm Stone / Carbon Ink)*
**Mode:** `Operate`
`psearch-eval` is built as an instrument-grade engineering workbench inspired by classic Braun / Dieter Rams industrial telemetry and financial order-book terminals. Rather than wrapping everything in padded SaaS cards with glowing badges, the interface uses a structural **hairline ledger grid**, warm paper-and-ink OKLCH surfaces (with an instant Carbon-Ink dark-mode switch for low-light operations), high-density tabular numerals, and authored SVG vector graphics.

## Typography
- **Primary UI & Headings (`--font-sans`):** `'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif`
  - Chosen for its clean geometric proportions and high x-height at compact operational sizes.
  - Fixed `rem` scale (1.15 ratio):
    - `--text-xs`: `0.75rem` (`12px`, line-height `1.4`) — metadata, table headers, axis ticks
    - `--text-sm`: `0.8125rem` (`13px`, line-height `1.45`) — dense table cells, filter controls, evidence notes
    - `--text-base`: `0.9375rem` (`15px`, line-height `1.55`) — primary body and analytical findings (`65–72ch` measure)
    - `--text-lg`: `1.125rem` (`18px`, weight `600`, tracking `-0.015em`) — section headings
    - `--text-xl`: `1.375rem` (`22px`, weight `650`, tracking `-0.025em`) — workspace title
    - `--text-display`: `2.25rem` (`36px`, weight `700`, tracking `-0.035em`, `tabular-nums`) — primary focal readout
- **Data, IDs, Queries & Metrics (`--font-mono`):** `'JetBrains Mono', ui-monospace, SFMono-Regular, monospace`
  - Reserved strictly for measurements, latencies, NDCG/MRR scores, commit SHAs, agent handles (`@lead`), and SQL/API paths.
  - Always paired with `font-variant-numeric: tabular-nums`.

## Color System (OKLCH Tinted Neutrals & Semantic States)
All neutrals are tinted toward warm stone (`hue 75–85`) in Daylight mode and cool graphite (`hue 250`) in Carbon mode — never pure `#000000`, `#ffffff`, or dead gray.

### Daylight Ledger (Default — High-Ambient Product Review)
- `--bg-canvas`: `oklch(97.8% 0.007 80)` (Warm alabaster paper)
- `--bg-surface`: `oklch(99.3% 0.004 80)` (Crisp ledger sheet)
- `--bg-subtle`: `oklch(95.2% 0.010 80)` (Recessed toolbar & table header surface)
- `--bg-elevated`: `oklch(93.0% 0.013 80)` (Active / selected row tint)
- `--border-hairline`: `oklch(87.5% 0.014 80)` (1px structural grid divider)
- `--border-strong`: `oklch(76.0% 0.018 80)` (Interactive control border)
- `--text-primary`: `oklch(19.5% 0.016 255)` (Deep carbon ink, contrast > 14:1)
- `--text-secondary`: `oklch(42.0% 0.018 255)` (Tinted slate-ink, contrast > 7:1)
- `--text-muted`: `oklch(54.0% 0.016 255)` (Metadata ink, contrast >= 4.6:1)

### Carbon Telemetry (Dark Mode — Continuous Monitoring)
- `--bg-canvas`: `oklch(14.5% 0.012 250)`
- `--bg-surface`: `oklch(17.5% 0.014 250)`
- `--bg-subtle`: `oklch(20.5% 0.016 250)`
- `--bg-elevated`: `oklch(24.0% 0.020 250)`
- `--border-hairline`: `oklch(27.5% 0.016 250)`
- `--border-strong`: `oklch(38.0% 0.020 250)`
- `--text-primary`: `oklch(95.5% 0.008 80)`
- `--text-secondary`: `oklch(76.0% 0.012 80)`
- `--text-muted`: `oklch(62.0% 0.014 80)`

### Semantic & Action Accents
- `--accent-primary`: `oklch(52.0% 0.165 245)` (Cobalt instrument blue — primary actions & active tab indicator)
- `--status-pass`: `oklch(52.0% 0.145 155)` / surface `oklch(94.5% 0.035 155)` (Verified / Running / PASS)
- `--status-warn`: `oklch(58.0% 0.155 68)` / surface `oklch(95.0% 0.040 75)` (Partial / Executing / Attention)
- `--status-gap`: `oklch(53.0% 0.185 28)` / surface `oklch(95.0% 0.035 28)` (Regression / Error / Blocked)

## Layout, Rhythm & Anti-Slop Rules
1. **Asymmetric Focal Strip, Not 4 Equal KPI Boxes:** The top summary strip uses an asymmetric split: a primary Search Quality readout (`NDCG@10` + `MRR@10` + RRF $\alpha$ response curve) on the left, paired with a compact inline operational ledger on the right (Latency `p50/p95/p99`, Backlog Readiness bar, and Active Agent ratio).
2. **Hairline Ledger Architecture, Not Nested Cards:** Major functional zones sit inside a unified 1px structural grid (`border: 1px solid var(--border-hairline)`) rather than floating cards inside cards.
3. **Zero Eyebrow Kickers:** Headings stand on their own typographic weight without small uppercase labels above them.
4. **Browser Surfaces Themed:** Custom `::selection`, `caret-color`, scrollbar thumb/track, `:focus-visible` outline (`2px solid var(--accent-primary)` with `2px` offset), and `font-variant-numeric: tabular-nums` on every data element.
